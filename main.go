// SystemPub is a service that publishes the state of ZFS pools and systemd units to an MQTT server.
// It is intended to be used with Home Assistant, and publishes the state of the pools and units as binary sensors.
// Autodiscovery is supported, so there is no need for any further configuration in Home Assistant.
package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
	"github.com/ykgmfq/SystemPub/systemd"
	"github.com/ykgmfq/SystemPub/zfs"

	"gopkg.in/yaml.v3"
)

var version = "1.3.6"

// Reads the MQTT password from systemd credentials directory
// Falls back to the password in the config if credential is not available
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

// Reads the configuration file and returns the application configuration.
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

// Reports readiness and connection status to systemd and pets the watchdog when one is configured.
// It keeps consuming connection updates even without a watchdog, because the MQTT client blocks until every listener has received them.
func watchdog(ctx context.Context, conn chan bool, logger zerolog.Logger) {
	watchTime, err := daemon.SdWatchdogEnabled(false)
	if err != nil {
		logger.Warn().Err(err).Msg("Could not determine systemd watchdog settings")
	}
	daemon.SdNotify(false, daemon.SdNotifyReady)
	daemon.SdNotify(false, "STATUS=Connecting...")
	var ticks <-chan time.Time
	if err == nil && watchTime > 0 {
		timer := time.NewTicker(watchTime / 2)
		defer timer.Stop()
		ticks = timer.C
	}
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
		case <-ticks:
			daemon.SdNotify(false, daemon.SdNotifyWatchdog)
		}
	}
}

func main() {
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

	// Logging starts at the level given on the command line and switches to the configured level once the configuration has been read.
	startLevel := zerolog.InfoLevel
	if *debug {
		startLevel = zerolog.DebugLevel
	}
	baseLogger := zerolog.New(os.Stdout).Level(startLevel)
	logger := baseLogger.With().Str("mod", "main").Logger()

	// Configuration
	config := models.SystemPubConfigDefault()
	if *configPath != "" {
		conf, err := readConfig(*configPath, logger)
		if err != nil {
			logger.Fatal().Err(err).Msg("Could not read configuration file")
		}
		config = conf
	}

	// Load MQTT password from credentials directory if available
	password, err := loadMQTTPassword()
	if err != nil {
		logger.Fatal().Err(err).Msg("Could not read password from credentials directory")
	}
	if password != "" {
		if config.MQTTServer.Password != "" {
			logger.Warn().Msg("Overriding MQTT password from configuration file with credentials directory")
		}
		config.MQTTServer.Password = password
	}

	if *debug {
		config.Loglevel = zerolog.DebugLevel
	}
	if *mqttServerHost != "" {
		if parsedURL, err := url.Parse(*mqttServerHost); err == nil {
			config.MQTTServer.Host = models.YAMLURL{URL: parsedURL}
		} else {
			logger.Error().Err(err).Msg("Malformed MQTT server URL")
		}
	}
	baseLogger = baseLogger.Level(config.Loglevel)
	logger = logger.Level(config.Loglevel)
	logger.Debug().Str("SystemPub version", version).Msg("")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dev, err := systemd.GetDevice(ctx)
	if err != nil {
		logger.Fatal().Err(err).Msg("Could not get device info")
	}
	wdconn := make(chan bool)
	mqttClient := mqttclient.NewMqttclient(config.MQTTServer, dev, baseLogger)
	systemdClient := systemd.NewDbusclient(mqttClient.Pubs, dev, 10*time.Minute, baseLogger)
	zfsServer := zfs.NewZfsServer(mqttClient.Pubs, dev, 20*time.Minute, baseLogger)
	mqttClient.ConnListeners = append(mqttClient.ConnListeners, systemdClient.Discover, zfsServer.Discover, wdconn)

	// A failed publisher must end the process, so that systemd stops getting watchdog pings and restarts the service.
	mqttErr := make(chan error, 1)
	go func() { mqttErr <- mqttClient.Serve(ctx) }()
	go systemdClient.Serve(ctx)
	go zfsServer.Serve(ctx)
	go watchdog(ctx, wdconn, logger)

	select {
	case <-ctx.Done():
	case err := <-mqttErr:
		if err != nil {
			logger.Fatal().Err(err).Msg("MQTT publisher stopped")
		}
	}
}
