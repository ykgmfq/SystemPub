// SystemPub is a service that publishes the state of ZFS pools and systemd units to an MQTT server.
// It is intended to be used with Home Assistant, and publishes the state of the pools and units as binary sensors.
// Autodiscovery is supported, so there is no need for any further configuration in Home Assistant.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
	"github.com/ykgmfq/SystemPub/sanoid"
	"github.com/ykgmfq/SystemPub/systemd"

	"gopkg.in/yaml.v3"
)

var (
	logger  zerolog.Logger
	version = "1.1.0"
)

// Reads the configuration file and returns the application configuration
func readConfig(location string) models.SystemPubConfig {
	config := models.SystemPubConfigDefault()
	file, err := os.Open(location)
	if err != nil {
		logger.Warn().Str("mod", "main").Err(err).Msg("")
		return config
	}
	defer file.Close()
	if err = yaml.NewDecoder(file).Decode(&config); err != nil {
		logger.Fatal().Str("mod", "main").Err(err).Msg("Malformed configuration file")
		panic(err)
	}
	logger.Debug().Str("mod", "main").Str("location", location).Interface("content", config).Msg("")
	return config
}

// Systemd watchdog
func watchdog(ctx context.Context, conn chan bool) {
	watchTime, err := daemon.SdWatchdogEnabled(false)
	if err != nil || watchTime <= 0 {
		return
	}
	daemon.SdNotify(false, daemon.SdNotifyReady)
	daemon.SdNotify(false, "STATUS=Connecting...")
	timer := time.NewTicker(watchTime / 2)
	for {
		select {
		case <-ctx.Done():
			daemon.SdNotify(false, daemon.SdNotifyStopping)
			return
		case connected := <-conn:
			var status string
			if connected {
				status = "Connected to MQTT server"
			} else {
				status = "Disconnected from MQTT server"
			}
			daemon.SdNotify(false, "STATUS="+status)
		case <-timer.C:
			daemon.SdNotify(false, daemon.SdNotifyWatchdog)
		}
	}
}

func main() {
	// Logging
	logger = zerolog.New(os.Stdout).With().Logger()
	sanoid.Logger = logger
	systemd.Logger = logger
	mqttclient.Logger = logger

	// Flags and config
	debug := flag.Bool("debug", false, "sets log level to debug")
	configPath := flag.String("config", "/etc/systempub.yaml", "Config file")
	mqttServerHost := flag.String("host", "", "MQTT server host")
	showVersion := flag.Bool("v", false, "show version and exit")
	flag.Parse()

	if *showVersion {
		println("SystemPub, version", version)
		return
	}

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	config := readConfig(*configPath)
	if *debug {
		config.Loglevel = zerolog.DebugLevel
	}
	if *mqttServerHost != "" {
		config.MQTTServer.Host = *mqttServerHost
	}
	zerolog.SetGlobalLevel(config.Loglevel)
	logger.Debug().Str("mod", "main").Str("SystemPub version", version).Msg("")

	dev := systemd.GetDevice()
	wdconn := make(chan bool)
	mqttClient := mqttclient.NewMqttclient(config.MQTTServer, dev)
	systemdClient := systemd.NewDbusclient(mqttClient.Pubs, dev, 10*time.Minute)
	sanoidClient := sanoid.NewSanoidClient(mqttClient.Pubs, dev, 20*time.Minute)
	mqttClient.ConnListeners = append(mqttClient.ConnListeners, systemdClient.Discover, sanoidClient.Discover, wdconn)

	// Set connection context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go mqttClient.Serve(ctx)
	go systemdClient.Serve(ctx)
	go sanoidClient.Serve(ctx)
	go watchdog(ctx, wdconn)

	<-ctx.Done()
}
