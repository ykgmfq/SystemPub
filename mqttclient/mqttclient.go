package mqttclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
)

// NormalizeStr makes a string usable in MQTT topics and Home Assistant IDs.
func NormalizeStr(input string) string {
	return invalidChars.ReplaceAllString(strings.ToLower(input), "-")
}

// NewMqttclient returns a client with initialized channels.
func NewMqttclient(server models.MQTT, device models.Device, logger zerolog.Logger) Mqttclient {
	return Mqttclient{
		Server:        server,
		Device:        device,
		Pubs:          make(chan *paho.Publish, 4),
		ConnListeners: make([]chan bool, 0),
		logger:        logger.With().Str("mod", "mqtt").Logger(),
	}
}

// sensorTopic returns the topic below which Home Assistant expects all messages for the given sensor.
func sensorTopic(domain models.Domain, uniqueID string) string {
	return discoveryPrefix + "/" + string(domain) + "/" + uniqueID
}

// StateTopic returns the topic that carries the state of the given sensor.
func StateTopic(domain models.Domain, uniqueID string) string {
	return sensorTopic(domain, uniqueID) + "/state"
}

// AttributesTopic returns the topic that carries the attributes of the given sensor.
func AttributesTopic(domain models.Domain, uniqueID string) string {
	return sensorTopic(domain, uniqueID) + "/attributes"
}

// GetDiscovery returns the discovery message that announces the given sensor to Home Assistant.
func GetDiscovery(domain models.Domain, config models.MqttConfig) (*paho.Publish, error) {
	payload, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return &paho.Publish{
		QoS:     1,
		Retain:  true,
		Topic:   sensorTopic(domain, config.UniqueID) + "/config",
		Payload: payload,
	}, nil
}

// ProblemPayload returns the payload of a problem sensor.
// The logic is inverted: a sensor that is ok is off.
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

// clientError logs errors on the client side.
func (client Mqttclient) clientError(err error) { client.logger.Error().Err(err).Msg("Client error") }

// serverDis logs a disconnect requested by the server.
func (client Mqttclient) serverDis(d *paho.Disconnect) {
	event := client.logger.Info().Uint8("reason code", uint8(d.ReasonCode))
	if d.Properties != nil {
		event = event.Str("reason", d.Properties.ReasonString)
	}
	event.Msg("Server requested disconnect")
}

// notifyListeners notifies all registered listeners about the connection status to Home Assistant.
// It gives up once the context ends, because listeners stop receiving at shutdown.
func (client Mqttclient) notifyListeners(ctx context.Context, connected bool) {
	for _, listener := range client.ConnListeners {
		select {
		case listener <- connected:
		case <-ctx.Done():
			return
		}
	}
}

// onConnectionUp subscribes to the Home Assistant status topic whenever the connection comes up, so that discovery can be repeated after Home Assistant restarts.
func (client Mqttclient) onConnectionUp(ctx context.Context, cm *autopaho.ConnectionManager) {
	client.logger.Info().Msg("Connected to MQTT server")
	sub := &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{
			{Topic: statusTopic, QoS: 1},
		},
	}
	subCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	if _, err := cm.Subscribe(subCtx, sub); err != nil {
		client.logger.Error().Err(err).Msg("Failed to subscribe to homeassistant status")
	} else {
		client.logger.Info().Msg("Subscribed to homeassistant status")
	}
	client.notifyListeners(ctx, true)
}

// onPublishReceived treats Home Assistant coming online like a new connection, so that it receives discovery again.
func (client Mqttclient) onPublishReceived(ctx context.Context, pr paho.PublishReceived) (bool, error) {
	client.logger.Debug().Interface("msg", &pr.Packet).Msg("Received message")
	if pr.Packet.Topic == statusTopic && string(pr.Packet.Payload) == "online" {
		client.logger.Info().Msg("Homeassistant is online")
		client.notifyListeners(ctx, true)
	}
	return true, nil
}

// onConnectError reports a failed connection attempt to the listeners and the log.
func (client Mqttclient) onConnectError(ctx context.Context, err error) {
	client.notifyListeners(ctx, false)
	client.logger.Error().Err(err).Msg("Connection attempt failed")
}

// onPublished logs every outgoing message.
func (client Mqttclient) onPublished(p *paho.Publish) {
	client.logger.Debug().Str("topic", p.Topic).Msg("Published message")
}

// username returns the configured user name, or one derived from the device name.
func username(server models.MQTT, device models.Device) string {
	if server.User != "" {
		return server.User
	}
	return fmt.Sprintf("systemPub@%s", device.Name)
}

// clientID returns a client ID that is unique per host.
// The device identifier must have at least models.MinMachineIDLength characters, which GetDevice guarantees.
func clientID(device models.Device) string {
	return fmt.Sprintf("systemPub@%s_%s", device.Name, device.Identifiers[0][:models.MinMachineIDLength])
}

// buildClientConfig returns the autopaho configuration with all callbacks.
// The callbacks run under the given context, so that they stop together with the connection.
func (client Mqttclient) buildClientConfig(ctx context.Context) autopaho.ClientConfig {
	// The library reports errors and debug output on separate streams, which are mapped to the matching log levels.
	pahoLogger := client.logger.With().Str("lib", "paho").Logger()
	pahoErrors := levelLogger{logger: pahoLogger, level: zerolog.ErrorLevel}
	pahoDebug := levelLogger{logger: pahoLogger, level: zerolog.DebugLevel}
	return autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{client.Server.Host.URL},
		KeepAlive:                     20,
		CleanStartOnInitialConnection: false,
		SessionExpiryInterval:         60,
		OnConnectionUp: func(cm *autopaho.ConnectionManager, _ *paho.Connack) {
			client.onConnectionUp(ctx, cm)
		},
		OnConnectError: func(err error) { client.onConnectError(ctx, err) },
		Errors:         pahoErrors,
		Debug:          pahoDebug,
		PahoErrors:     pahoErrors,
		TlsCfg:         &tls.Config{},
		ClientConfig: paho.ClientConfig{
			ClientID: clientID(client.Device),
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){
				func(pr paho.PublishReceived) (bool, error) { return client.onPublishReceived(ctx, pr) },
			},
			OnClientError:      client.clientError,
			OnServerDisconnect: client.serverDis,
			PublishHook:        client.onPublished,
		},
		ConnectUsername: username(client.Server, client.Device),
		ConnectPassword: []byte(client.Server.Password),
	}
}

// createConnection opens the connection to the MQTT server.
func (client Mqttclient) createConnection(ctx context.Context) (*autopaho.ConnectionManager, error) {
	cfg := client.buildClientConfig(ctx)
	client.logger.Debug().Str("username", cfg.ConnectUsername).Str("clientID", cfg.ClientConfig.ClientID).Msg("")
	client.logger.Debug().Msg("Starting connection")
	return autopaho.NewConnection(ctx, cfg)
}

// Serve handles the MQTT connection and publishes queued messages until the context ends.
// It returns nil when the context is cancelled and an error when publishing can no longer continue.
// Either way, it returns only after the connection has been closed cleanly or the attempt has timed out.
func (client Mqttclient) Serve(ctx context.Context) error {
	mqttctx, cancel := context.WithCancel(ctx)
	defer cancel()
	connManage, err := client.createConnection(mqttctx)
	if err != nil {
		return fmt.Errorf("failed to create MQTT connection: %w", err)
	}
	err = client.publishLoop(mqttctx, connManage)
	// Cancelling the connection's context makes autopaho send a disconnect to the server.
	cancel()
	client.logger.Info().Msg("Disconnecting from MQTT server")
	return errors.Join(err, awaitShutdown(connManage))
}

// publishLoop publishes queued messages until the context ends.
func (client Mqttclient) publishLoop(ctx context.Context, cm *autopaho.ConnectionManager) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case pub, ok := <-client.Pubs:
			if !ok {
				return errors.New("MQTT work channel closed")
			}
			if err := publish(ctx, cm, pub); err != nil {
				client.logger.Error().Err(err).Msg("Failed to publish")
			}
		}
	}
}

// awaitShutdown waits for the connection manager to finish closing the connection.
func awaitShutdown(cm *autopaho.ConnectionManager) error {
	select {
	case <-cm.Done():
		return nil
	case <-time.After(requestTimeout):
		return errors.New("timed out while closing the MQTT connection")
	}
}

// publish publishes one message, bounded by its own deadline so that a stalled acknowledgement does not hold up the queue.
func publish(ctx context.Context, cm *autopaho.ConnectionManager, pub *paho.Publish) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	_, err := cm.Publish(ctx, pub)
	return err
}
