// SystemPub is a service that publishes the state of ZFS pools and systemd units to an MQTT server.
// It is intended to be used with Home Assistant, and publishes the state of the pools and units as binary sensors.
// Autodiscovery is supported, so there is no need for any further configuration in Home Assistant.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/autopaho/queue/memory"
	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/sanoid"
	"github.com/ykgmfq/SystemPub/systemd"

	"gopkg.in/yaml.v3"
)

var logger zerolog.Logger

func getDefaultConfig() models.SystemPubConfig {
	return models.SystemPubConfig{MQTTServer: models.MQTT{Host: "localhost", Port: 1883}, Loglevel: zerolog.InfoLevel}
}

// Handles server disconnects
func serverDis(d *paho.Disconnect) {
	if d.Properties != nil {
		logger.Info().Msg("server requested disconnect: " + d.Properties.ReasonString)
	} else {
		logger.Info().Msg("server requested disconnect; reason code: " + string(d.ReasonCode))
	}
}

// Returns a discovery message for a given sensor
func getDiscovery(config models.MqttConfig) *autopaho.QueuePublish {
	payload, err := json.Marshal(config)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to marshal discovery message")
		panic(err)
	}
	disovery := paho.Publish{
		QoS:     1,
		Topic:   "homeassistant/binary_sensor/" + config.UniqueID + "/config",
		Payload: payload,
	}
	return &autopaho.QueuePublish{Publish: &disovery}
}

func clientError(err error) { logger.Error().Err(err).Msg("client error") }

// Reads the current Sanoid states, and the Systemd unit state, and updates the state of the sensors
func update(context context.Context, cm *autopaho.ConnectionManager, poolConfigs map[models.Property]models.MqttConfig, unitConfig models.MqttConfig) {
	payload := map[bool][]byte{
		true:  []byte("OFF"),
		false: []byte("ON"),
	}
	// get the current state of the pools and the unit via go routines
	for p, state := range sanoid.GetPoolStates() {
		update := paho.Publish{
			QoS:     1,
			Topic:   poolConfigs[p].StateTopic,
			Payload: payload[state],
		}
		publish := autopaho.QueuePublish{Publish: &update}
		if err := cm.PublishViaQueue(context, &publish); err != nil {
			logger.Error().Err(err).Msg("Failed to publish pool state")
		}
	}
	failed := systemd.GetUnitState()
	update := paho.Publish{
		QoS:     1,
		Payload: payload[failed],
		Topic:   unitConfig.StateTopic,
	}
	publish := autopaho.QueuePublish{Publish: &update}
	if err := cm.PublishViaQueue(context, &publish); err != nil {
		logger.Error().Err(err).Msg("Failed to publish unit state")
	}
}

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

func main() {
	// Logging
	logger = zerolog.New(os.Stdout).With().Logger()
	sanoid.Logger = logger
	systemd.Logger = logger

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

	// Get sensor configuration
	poolConfigs := sanoid.GetPoolConfigs(device)
	unitConfig := systemd.GetUnitConfig(device)
	for _, config := range poolConfigs {
		logger.Debug().Interface("poolConfig", config).Msg("")
	}
	logger.Debug().Interface("unitConfig", unitConfig).Msg("")

	// Set connection context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverUrl := url.URL{
		Scheme: "mqtt",
		Host:   fmt.Sprintf("%s:%d", config.MQTTServer.Host, config.MQTTServer.Port),
	}

	// Instantiated late, after client is defined and connected
	var connManage *autopaho.ConnectionManager

	publishDiscovery := func() {
		for _, config := range poolConfigs {
			if err := connManage.PublishViaQueue(ctx, getDiscovery(config)); err != nil {
				if ctx.Err() == nil {
					panic(err)
				}
			}
		}
		if err := connManage.PublishViaQueue(ctx, getDiscovery(unitConfig)); err != nil {
			if ctx.Err() == nil {
				panic(err)
			}
		}
		logger.Debug().Msg("Published discovery messages")
		update(ctx, connManage, poolConfigs, unitConfig)
	}

	connectError := func(err error) { logger.Error().Err(err).Msg("") }

	onconn := func(cm *autopaho.ConnectionManager, connAck *paho.Connack) {
		logger.Info().Msg("Connected to MQTT server")
		sub := &paho.Subscribe{
			Subscriptions: []paho.SubscribeOptions{
				{Topic: "homeassistant/status", QoS: 1},
			},
		}
		if _, err := cm.Subscribe(context.Background(), sub); err != nil {
			logger.Error().Err(err).Msg("Failed to subscribe to homeassistant status")
		}
		logger.Info().Msg("Subscribed to homeassistant status")
		publishDiscovery()
	}

	onpub := []func(paho.PublishReceived) (bool, error){
		func(pr paho.PublishReceived) (bool, error) {
			if pr.Packet.Topic == "homeassistant/status" && string(pr.Packet.Payload) == "online" {
				logger.Info().Msg("Homeassistant is online")
				publishDiscovery()
			}
			return true, nil
		}}

	// Client configuration
	queue := memory.New()
	cliCfg := autopaho.ClientConfig{
		Queue:                         queue,
		ServerUrls:                    []*url.URL{&serverUrl},
		KeepAlive:                     20,
		CleanStartOnInitialConnection: false,
		SessionExpiryInterval:         60,
		OnConnectionUp:                onconn,
		OnConnectError:                connectError,
		ClientConfig: paho.ClientConfig{
			ClientID:           fmt.Sprintf("systemPub@%s_%s", device.Name, device.Identifiers[0][:4]),
			OnPublishReceived:  onpub,
			OnClientError:      clientError,
			OnServerDisconnect: serverDis,
		},
	}
	logger.Debug().Str("clientID", cliCfg.ClientID).Msg("")

	// starts process; will reconnect until context cancelled
	connManage, err := autopaho.NewConnection(ctx, cliCfg)
	if err != nil {
		panic(err)
	}
	if err = connManage.AwaitConnection(ctx); err != nil {
		panic(err)
	}

	// Main routine
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			update(ctx, connManage, poolConfigs, unitConfig)
			logger.Debug().Msg("Updated sensors")
		case <-ctx.Done():
			return
		}
	}
}
