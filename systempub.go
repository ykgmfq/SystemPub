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
	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
	"github.com/ykgmfq/SystemPub/sanoid"
	"github.com/ykgmfq/SystemPub/systemd"

	"gopkg.in/yaml.v3"
)

var logger zerolog.Logger

// Reads the configuration file and returns the application configuration
func readConfig(location string) models.SystemPubConfig {
	config := models.SystemPubConfigDefault()
	file, err := os.Open(location)
	if err != nil {
		logger.Warn().Err(err).Msg("")
		return config
	}
	defer file.Close()
	if err = yaml.NewDecoder(file).Decode(&config); err != nil {
		logger.Fatal().Err(err).Msg("Malformed configuration file")
		panic(err)
	}
	logger.Debug().Str("location", location).Interface("content", config).Msg("")
	return config
}

// Systemd watchdog
func watchdog(ctx context.Context) {
	watchTime, err := daemon.SdWatchdogEnabled(false)
	if err != nil || watchTime <= 0 {
		return
	}
	timer := time.NewTicker(watchTime / 2)
	for {
		select {
		case <-ctx.Done():
			return
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
	mqttServerPort := flag.Int("port", 0, "MQTT server port")
	flag.Parse()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	config := readConfig(*configPath)
	if *debug {
		config.Loglevel = zerolog.DebugLevel
	}
	if *mqttServerHost != "" {
		config.MQTTServer.Host = *mqttServerHost
	}
	if *mqttServerPort != 0 {
		config.MQTTServer.Port = *mqttServerPort
	}
	zerolog.SetGlobalLevel(config.Loglevel)

	// Get system information
	device := systemd.GetDevice()
	logger.Debug().Interface("device", device).Msg("")

	// MQTT client
	client := mqttclient.NewMqttclient(config.MQTTServer, device)

	// Get sensor configuration
	poolConfigs := sanoid.GetPoolConfigs(device)
	unitConfig := systemd.GetUnitConfig(device)
	for _, config := range poolConfigs {
		logger.Debug().Interface("poolConfig", config).Msg("")
	}
	logger.Debug().Interface("unitConfig", unitConfig).Msg("")

	// Discovery messages
	moduleDiscoveries := [][]*paho.Publish{sanoid.GetDiscoveries(poolConfigs), systemd.GetDiscoveries(unitConfig)}

	// Set connection context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go client.Serve(ctx)

	updateSensors := func() {
		moduleUpdates := [][]*paho.Publish{sanoid.GetUpdates(poolConfigs), systemd.GetUpdates(unitConfig)}
		for _, updates := range moduleUpdates {
			for _, update := range updates {
				client.Pubs <- update
			}
		}
		logger.Debug().Msg("Updated sensors")
	}
	// Publish discovery messages
	publishDiscovery := func() {
		for _, discoveries := range moduleDiscoveries {
			for _, discovery := range discoveries {
				client.Pubs <- discovery
			}
		}
		logger.Debug().Msg("Published discovery messages")
	}

	// Sensor update timer
	updateTimer := time.NewTicker(5 * time.Minute)
	defer updateTimer.Stop()

	// Main routine
	daemon.SdNotify(false, daemon.SdNotifyReady)
	daemon.SdNotify(false, "STATUS=Connecting")
	go watchdog(ctx)
	for {
		select {
		case <-ctx.Done():
			daemon.SdNotify(false, daemon.SdNotifyStopping)
			return
		case <-updateTimer.C:
			updateSensors()
		case <-client.Conn:
			daemon.SdNotify(false, "STATUS=Connected to MQTT server")
			publishDiscovery()
			updateSensors()
		}
	}
}
