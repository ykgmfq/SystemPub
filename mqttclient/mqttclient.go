// Provides a MQTT client for Home Assistant sensors.
package mqttclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
)

// Replace invalid characters and convert to lowercase for MQTT compatibility
func NormalizeStr(input string) string {
	return invalidChars.ReplaceAllString(strings.ToLower(input), "-")
}

// Returns a MQTT client instance with initialized channels
func NewMqttclient(server models.MQTT, device models.Device, logger zerolog.Logger) Mqttclient {
	return Mqttclient{
		Server:        server,
		Device:        device,
		Pubs:          make(chan *paho.Publish, 4),
		ConnListeners: make([]chan bool, 0),
		logger:        logger.With().Str("mod", "mqtt").Logger(),
	}
}

func getTlsConfig() (*tls.Config, error) {
	rootCAPool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	var tlsConfig = tls.Config{RootCAs: rootCAPool}
	return &tlsConfig, nil
}

// Returns a discovery message for a given sensor
func GetDiscovery(config models.MqttConfig) (*paho.Publish, error) {
	payload, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return &paho.Publish{
		QoS:     1,
		Retain:  true,
		Topic:   "homeassistant/binary_sensor/" + config.UniqueID + "/config",
		Payload: payload,
	}, nil
}

// Returns a discovery message for a numeric sensor entity
func GetSensorDiscovery(config models.MqttConfig) (*paho.Publish, error) {
	payload, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return &paho.Publish{
		QoS:     1,
		Retain:  true,
		Topic:   "homeassistant/sensor/" + config.UniqueID + "/config",
		Payload: payload,
	}, nil
}

// Sensor payload for problem type. Note the inverted logic!
func ProblemPayload(ok bool) []byte {
	payload := map[bool][]byte{
		true:  []byte("OFF"),
		false: []byte("ON"),
	}
	return payload[ok]
}

// Println logs the arguments as one message, in the way fmt.Println joins them.
func (l levelLogger) Println(v ...any) {
	l.logger.WithLevel(l.level).Msg(strings.TrimSuffix(fmt.Sprintln(v...), "\n"))
}

// Printf logs a formatted message.
func (l levelLogger) Printf(format string, v ...any) {
	l.logger.WithLevel(l.level).Msg(fmt.Sprintf(format, v...))
}

// Handles client-side errors.
func (client Mqttclient) clientError(err error) { client.logger.Error().Err(err).Msg("Client error") }

// Handles server disconnects.
func (client Mqttclient) serverDis(d *paho.Disconnect) {
	event := client.logger.Info().Uint8("reason code", uint8(d.ReasonCode))
	if d.Properties != nil {
		event = event.Str("reason", d.Properties.ReasonString)
	}
	event.Msg("Server requested disconnect")
}

// Notifies all registered listeners about the connection status to Home Assitant.
func (client Mqttclient) notifyListeners(connected bool) {
	for _, listener := range client.ConnListeners {
		listener <- connected
	}
}

// buildClientConfig constructs the autopaho client configuration with all callbacks.
func (client Mqttclient) buildClientConfig(tlsConfig *tls.Config) autopaho.ClientConfig {
	onconn := func(cm *autopaho.ConnectionManager, connAck *paho.Connack) {
		client.logger.Info().Msg("Connected to MQTT server")
		sub := &paho.Subscribe{
			Subscriptions: []paho.SubscribeOptions{
				{Topic: "homeassistant/status", QoS: 1},
			},
		}
		if _, err := cm.Subscribe(context.Background(), sub); err != nil {
			client.logger.Error().Err(err).Msg("Failed to subscribe to homeassistant status")
		}
		client.logger.Info().Msg("Subscribed to homeassistant status")
		client.notifyListeners(true)
	}

	onpub := func(pr paho.PublishReceived) (bool, error) {
		client.logger.Debug().Interface("msg", &pr.Packet).Msg("Received message")
		if pr.Packet.Topic == "homeassistant/status" && string(pr.Packet.Payload) == "online" {
			client.logger.Info().Msg("Homeassistant is online")
			client.notifyListeners(true)
		}
		return true, nil
	}

	onerr := func(err error) {
		client.notifyListeners(false)
		client.logger.Error().Err(err).Msg("Connection attempt failed")
	}

	onpubl := func(p *paho.Publish) {
		client.logger.Debug().Str("topic", p.Topic).Msg("Published message")
	}

	// The library reports errors and debug output on separate streams, which are mapped to the matching log levels.
	pahoLogger := client.logger.With().Str("lib", "paho").Logger()
	pahoErrors := levelLogger{logger: pahoLogger, level: zerolog.ErrorLevel}
	pahoDebug := levelLogger{logger: pahoLogger, level: zerolog.DebugLevel}
	user := client.Server.User
	if client.Server.User == "" {
		user = fmt.Sprintf("systemPub@%s", client.Device.Name)
	}
	return autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{client.Server.Host.URL},
		KeepAlive:                     20,
		CleanStartOnInitialConnection: false,
		SessionExpiryInterval:         60,
		OnConnectionUp:                onconn,
		OnConnectError:                onerr,
		Errors:                        pahoErrors,
		Debug:                         pahoDebug,
		PahoErrors:                    pahoErrors,
		TlsCfg:                        tlsConfig,
		ClientConfig: paho.ClientConfig{
			ClientID:           fmt.Sprintf("systemPub@%s_%s", client.Device.Name, client.Device.Identifiers[0][:4]),
			OnPublishReceived:  []func(paho.PublishReceived) (bool, error){onpub},
			OnClientError:      client.clientError,
			OnServerDisconnect: client.serverDis,
			PublishHook:        onpubl,
		},
		ConnectUsername: user,
		ConnectPassword: []byte(client.Server.Password),
	}
}

// Creates a new MQTT connection with the given configuration.
func (client Mqttclient) createConnection(ctx context.Context) (*autopaho.ConnectionManager, error) {
	var tlsConfig *tls.Config
	if client.Server.Host.Scheme == "wss" || client.Server.Host.Scheme == "mqtts" {
		client.logger.Info().Msg("Using secure connection")
		conf, err := getTlsConfig()
		if err != nil {
			client.logger.Error().Err(err).Msg("Failed to get TLS config")
			return nil, err
		}
		tlsConfig = conf
	}
	cfg := client.buildClientConfig(tlsConfig)
	client.logger.Debug().Str("username", cfg.ConnectUsername).Str("clientID", cfg.ClientConfig.ClientID).Msg("")
	client.logger.Debug().Msg("Starting connection")
	return autopaho.NewConnection(ctx, cfg)
}

// Long-running routine that handles the MQTT connection and publishes messages.
// It returns nil when the context is cancelled and an error when publishing can no longer continue.
func (client Mqttclient) Serve(ctx context.Context) error {
	mqttctx, cancel := context.WithCancel(ctx)
	defer cancel()
	connManage, err := client.createConnection(mqttctx)
	if err != nil {
		return fmt.Errorf("failed to create MQTT connection: %w", err)
	}
	for {
		select {
		case <-mqttctx.Done():
			client.logger.Info().Msg("Routine stopped")
			return nil
		case pub, ok := <-client.Pubs:
			if !ok {
				return errors.New("MQTT work channel closed")
			}
			if _, err := connManage.Publish(mqttctx, pub); err != nil {
				client.logger.Error().Err(err).Msg("Failed to publish")
			}
		}
	}
}
