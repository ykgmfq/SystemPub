// SystemPub is a service that publishes the state of ZFS pools and systemd units to an MQTT server.
// It is intended to be used with Home Assistant, and publishes the state of the pools and units as binary sensors.
// Autodiscovery is supported, so there is no need for any further configuration in Home Assistant.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
	"github.com/ykgmfq/SystemPub/provider"
	"github.com/ykgmfq/SystemPub/systemd"
	"github.com/ykgmfq/SystemPub/zfs/sanoid"
	"github.com/ykgmfq/SystemPub/zfs/zpool"

	"gopkg.in/yaml.v3"
)

var version = "1.4.0"

// How often each group of sensors is checked while everything is healthy.
const (
	systemdInterval = 10 * time.Minute
	zfsInterval     = 20 * time.Minute
)

// loadMQTTPassword reads the MQTT password from the systemd credentials directory, if one is set.
func loadMQTTPassword() (string, error) {
	credDir := os.Getenv("CREDENTIALS_DIRECTORY")
	if credDir == "" {
		return "", nil
	}
	data, err := os.ReadFile(filepath.Join(credDir, "mqtt"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// readConfig reads the application configuration from the given file.
func readConfig(location string, logger zerolog.Logger) (models.SystemPubConfig, error) {
	config := models.SystemPubConfigDefault()
	file, err := os.Open(location)
	if errors.Is(err, fs.ErrNotExist) {
		logger.Warn().Str("location", location).Msg("Configuration file not found, using defaults")
		return config, nil
	}
	if err != nil {
		logger.Warn().Err(err).Msg("Could not open configuration file")
		return config, err
	}
	defer file.Close()
	if err = yaml.NewDecoder(file).Decode(&config); err != nil {
		logger.Error().Err(err).Msg("Malformed configuration file")
		return config, err
	}
	logger.Debug().Str("location", location).Interface("content", config).Msg("")
	return config, nil
}

// withPassword returns the configuration with the password from the credentials directory, which takes precedence over the configuration file.
func withPassword(config models.SystemPubConfig, logger zerolog.Logger) (models.SystemPubConfig, error) {
	password, err := loadMQTTPassword()
	if err != nil {
		return config, fmt.Errorf("could not read password from credentials directory: %w", err)
	}
	if password == "" {
		return config, nil
	}
	if config.MQTTServer.Password != "" {
		logger.Warn().Msg("Overriding MQTT password from configuration file with credentials directory")
	}
	config.MQTTServer.Password = password
	return config, nil
}

// withHost returns the configuration with the server host given on the command line, if any.
func withHost(config models.SystemPubConfig, host string) (models.SystemPubConfig, error) {
	if host == "" {
		return config, nil
	}
	parsedURL, err := models.ParseServerURL(host)
	if err != nil {
		return config, fmt.Errorf("malformed MQTT server URL: %w", err)
	}
	config.MQTTServer.Host = models.YAMLURL{URL: parsedURL}
	return config, nil
}

// loadConfig assembles the configuration from the file, the credentials directory and the command line.
func loadConfig(configPath, host string, debug bool, logger zerolog.Logger) (models.SystemPubConfig, error) {
	config := models.SystemPubConfigDefault()
	if configPath != "" {
		conf, err := readConfig(configPath, logger)
		if err != nil {
			return config, fmt.Errorf("could not read configuration file: %w", err)
		}
		config = conf
	}
	config, err := withPassword(config, logger)
	if err != nil {
		return config, err
	}
	if debug {
		config.Loglevel = zerolog.DebugLevel
	}
	return withHost(config, host)
}

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

// connectionStatus returns the systemd status text for the given MQTT connection state.
func connectionStatus(connected bool) string {
	if connected {
		return "STATUS=Connected to MQTT server"
	}
	return "STATUS=Disconnected from MQTT server"
}

// notify sends the given state to systemd.
// A failed notification is logged rather than escalated, because the service keeps working without it.
func notify(state string, logger zerolog.Logger) {
	if _, err := daemon.SdNotify(false, state); err != nil {
		logger.Warn().Err(err).Str("state", state).Msg("Could not notify systemd")
	}
}

// watchdog reports readiness and connection status to systemd and pets the watchdog when one is configured.
// It keeps consuming connection updates even without a watchdog, because the MQTT client blocks until every listener has received them.
func watchdog(ctx context.Context, conn chan bool, logger zerolog.Logger) {
	watchTime, err := daemon.SdWatchdogEnabled(false)
	if err != nil {
		logger.Warn().Err(err).Msg("Could not determine systemd watchdog settings")
	}
	notify(daemon.SdNotifyReady+"\nSTATUS=Connecting...", logger)
	var ticks <-chan time.Time
	if err == nil && watchTime > 0 {
		timer := time.NewTicker(watchTime / 2)
		defer timer.Stop()
		ticks = timer.C
	}
	for {
		select {
		case <-ctx.Done():
			return
		case connected := <-conn:
			notify(connectionStatus(connected), logger)
		case <-ticks:
			notify(daemon.SdNotifyWatchdog, logger)
		}
	}
}

// run sets up the sensors and runs them until a signal arrives or the MQTT client fails.
// It returns only after every routine has stopped and all resources have been released.
func run(configPath, host string, debug bool, baseLogger zerolog.Logger) error {
	logger := baseLogger.With().Str("mod", "main").Logger()
	config, err := loadConfig(configPath, host, debug, logger)
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	baseLogger = baseLogger.Level(config.Loglevel)
	logger = logger.Level(config.Loglevel)
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

	mqttClient := mqttclient.NewMqttclient(config.MQTTServer, dev, baseLogger)
	servers := newServers(mqttClient.Pubs, baseLogger, systemdProviders, zfsProviders)
	if len(servers) == 0 {
		return errors.New("no sensors available")
	}
	wdconn := make(chan bool)
	for _, server := range servers {
		mqttClient.ConnListeners = append(mqttClient.ConnListeners, server.Discover)
	}
	mqttClient.ConnListeners = append(mqttClient.ConnListeners, wdconn)

	var wg sync.WaitGroup
	mqttErr := make(chan error, 1)
	wg.Go(func() {
		err := mqttClient.Serve(ctx)
		mqttErr <- err
		if err != nil {
			cancel()
		}
	})
	for _, server := range servers {
		wg.Go(func() { server.Serve(ctx) })
	}
	wg.Go(func() { watchdog(ctx, wdconn, logger) })

	<-ctx.Done()
	logger.Info().Msg("Shutting down")
	notify(daemon.SdNotifyStopping+"\nSTATUS=Shutting down", logger)
	wg.Wait()
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
		notify("STATUS=Failed: "+err.Error(), logger)
		os.Exit(1)
	}
}
