// SystemPub is a service that publishes the state of ZFS pools and systemd units to an MQTT server.
// It is intended to be used with Home Assistant, and publishes the state of the pools and units as binary sensors.
// Autodiscovery is supported, so there is no need for any further configuration in Home Assistant.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/firedrill"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
	"github.com/ykgmfq/SystemPub/provider"
	"github.com/ykgmfq/SystemPub/service"
	"github.com/ykgmfq/SystemPub/systemd"
	"github.com/ykgmfq/SystemPub/zfs/sanoid"
	"github.com/ykgmfq/SystemPub/zfs/zpool"
)

var version = "1.4.1"

// How often each group of sensors is checked while everything is healthy.
const (
	systemdInterval = 10 * time.Minute
	zfsInterval     = 20 * time.Minute
)

// drainTimeout bounds publishing the last queued messages at shutdown, so that an unreachable MQTT server cannot hold up the exit.
const drainTimeout = 15 * time.Second

// newProviders returns the providers of the systemd and ZFS sensor groups.
// Providers that do not work on this host are logged and left out.
// The returned function releases the resources held by the providers.
func newProviders(ctx context.Context, dev models.Device, logger, baseLogger zerolog.Logger) (systemdProviders, zfsProviders []provider.Provider, closeAll func()) {
	closeAll = func() {}
	units, err := systemd.NewUnitProvider(ctx, dev, systemdInterval, baseLogger)
	if err != nil {
		logger.Warn().Err(err).Msg("Could not connect to systemd, skipping the units sensor")
	} else {
		closeAll = units.Close
		systemdProviders = append(systemdProviders, units)
	}
	snapshots, err := sanoid.NewSanoidProvider(ctx, dev, zfsInterval, baseLogger)
	if err != nil {
		logger.Warn().Err(err).Msg("Sanoid does not work, skipping the snapshot sensor")
	} else {
		zfsProviders = append(zfsProviders, snapshots)
	}
	pools, err := zpool.NewZpoolProvider(ctx, zfsInterval)
	if err != nil {
		logger.Warn().Err(err).Msg("Zpool does not work, skipping the pool sensors")
	} else {
		zfsProviders = append(zfsProviders, pools)
	}
	return systemdProviders, zfsProviders, closeAll
}

// newServers returns one server per sensor group.
// Servers without providers are left out, because the MQTT client waits for every connection listener.
func newServers(pubs chan *paho.Publish, logger zerolog.Logger, systemdProviders, zfsProviders []provider.Provider) []provider.Server {
	var servers []provider.Server
	if len(systemdProviders) > 0 {
		servers = append(servers, provider.NewServer("systemd", pubs, systemdInterval, logger, systemdProviders...))
	}
	if len(zfsProviders) > 0 {
		servers = append(servers, provider.NewServer("zfs", pubs, zfsInterval, logger, zfsProviders...))
	}
	return servers
}

// drillChannels returns the channels on which the servers receive fire drills.
func drillChannels(servers []provider.Server) []chan models.Drill {
	drills := make([]chan models.Drill, 0, len(servers))
	for _, server := range servers {
		drills = append(drills, server.Drills)
	}
	return drills
}

// run sets up the sensors and runs them until a signal arrives or the MQTT client fails.
// It returns only after every routine has stopped and all resources have been released.
func run(configPath, host string, debug bool, baseLogger zerolog.Logger) error {
	// Without a handler, SIGUSR1 would terminate the process, so the handler is registered before anything else.
	drillTriggers := make(chan os.Signal, 1)
	signal.Notify(drillTriggers, syscall.SIGUSR1)
	defer signal.Stop(drillTriggers)

	logger := baseLogger.With().Str("mod", "main").Logger()
	cfg, err := service.LoadConfig(configPath, host, debug, logger)
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	baseLogger = baseLogger.Level(cfg.Loglevel)
	logger = logger.Level(cfg.Loglevel)
	logger.Debug().Str("SystemPub version", version).Msg("")

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// A failing MQTT client cancels this context, so that every other routine shuts down as well.
	ctx, cancel := context.WithCancel(sigCtx)
	defer cancel()

	dev, err := systemd.GetDevice(ctx)
	if err != nil {
		return fmt.Errorf("could not get device info: %w", err)
	}
	systemdProviders, zfsProviders, closeProviders := newProviders(ctx, dev, logger, baseLogger)
	defer closeProviders()

	mqttClient := mqttclient.NewMqttclient(cfg.MQTTServer, dev, baseLogger)
	servers := newServers(mqttClient.Pubs, baseLogger, systemdProviders, zfsProviders)
	if len(servers) == 0 {
		return errors.New("no sensors available")
	}
	wdconn := make(chan bool)
	for _, server := range servers {
		mqttClient.ConnListeners = append(mqttClient.ConnListeners, server.Discover)
	}
	mqttClient.ConnListeners = append(mqttClient.ConnListeners, wdconn)

	// The MQTT client outlives the other routines, so that the messages they queue on their way out are still published.
	mqttCtx, stopMQTT := context.WithCancel(context.WithoutCancel(ctx))
	defer stopMQTT()
	var mqttWG sync.WaitGroup
	mqttErr := make(chan error, 1)
	mqttWG.Go(func() {
		err := mqttClient.Serve(mqttCtx)
		mqttErr <- err
		if err != nil {
			cancel()
		}
	})
	var wg sync.WaitGroup
	for _, server := range servers {
		wg.Go(func() { server.Serve(ctx) })
	}
	wg.Go(func() { service.Watchdog(ctx, wdconn, logger) })
	allProviders := slices.Concat(systemdProviders, zfsProviders)
	wg.Go(func() {
		firedrill.Run(ctx, drillTriggers, cfg.FireDrill, allProviders, drillChannels(servers), baseLogger)
	})

	<-ctx.Done()
	logger.Info().Msg("Shutting down")
	service.Notify(daemon.SdNotifyStopping+"\nSTATUS=Shutting down", logger)
	wg.Wait()
	// The queue may only be closed once every routine that sends to it has returned.
	close(mqttClient.Pubs)
	drainTimer := time.AfterFunc(drainTimeout, stopMQTT)
	defer drainTimer.Stop()
	mqttWG.Wait()
	if err := <-mqttErr; err != nil {
		return fmt.Errorf("MQTT publisher stopped: %w", err)
	}
	return nil
}

// main parses the command line and runs SystemPub.
func main() {
	debug := flag.Bool("debug", false, "sets log level to debug")
	configPath := flag.String("config", "/etc/systempub.yaml", "Config file")
	mqttServerHost := flag.String("host", "", "MQTT server host")
	showVersion := flag.Bool("v", false, "show version and exit")
	flag.Parse()

	if *showVersion {
		println("SystemPub, version", version)
		return
	}

	// Logging starts at the level given on the command line and switches to the configured level once the configuration has been read.
	startLevel := zerolog.InfoLevel
	if *debug {
		startLevel = zerolog.DebugLevel
	}
	baseLogger := zerolog.New(os.Stdout).Level(startLevel)
	logger := baseLogger.With().Str("mod", "main").Logger()

	// The exit happens here, after run has released everything, and its non-zero status lets systemd restart the service.
	if err := run(*configPath, *mqttServerHost, *debug, baseLogger); err != nil {
		logger.Error().Err(err).Msg("SystemPub failed")
		service.Notify("STATUS=Failed: "+err.Error(), logger)
		os.Exit(1)
	}
}
