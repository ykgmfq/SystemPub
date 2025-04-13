// Provides a MQTT client for Home Assistant sensors.
package mqttclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/autopaho/queue/memory"
	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
)

var Logger zerolog.Logger

// MQTT client for Home Assistant sensors.
// The device info is used for autodiscovery and associating the machine with its sensors.
// It receives to-be-published messages from the main process and sends them to the MQTT server.
// The main process can listen to the Conn channel to check if the connection is established.
type Mqttclient struct {
	Server models.MQTT
	Device models.Device
	Pubs   chan *paho.Publish
	Conn   chan bool
}

// Returns a MQTT client instance with initialized channels
func NewMqttclient(server models.MQTT, device models.Device) Mqttclient {
	return Mqttclient{
		Server: server,
		Device: device,
		Pubs:   make(chan *paho.Publish),
		Conn:   make(chan bool),
	}
}

// Returns a discovery message for a given sensor
func GetDiscovery(config models.MqttConfig) *paho.Publish {
	payload, err := json.Marshal(config)
	if err != nil {
		Logger.Fatal().Err(err).Msg("Failed to marshal discovery message")
		panic(err)
	}
	disovery := paho.Publish{
		QoS:     1,
		Topic:   "homeassistant/binary_sensor/" + config.UniqueID + "/config",
		Payload: payload,
	}
	return &disovery
}

// Sensor payload for problem type. Note the inverted logic!
func ProblemPayload(ok bool) []byte {
	payload := map[bool][]byte{
		true:  []byte("OFF"),
		false: []byte("ON"),
	}
	return payload[ok]
}

// Handles client-side errors.
func clientError(err error) { Logger.Error().Err(err).Msg("client error") }

// Handles connection errors.
func connectError(err error) { Logger.Error().Err(err).Msg("") }

// Handles server disconnects.
func serverDis(d *paho.Disconnect) {
	if d.Properties != nil {
		Logger.Info().Msg("server requested disconnect: " + d.Properties.ReasonString)
	} else {
		Logger.Info().Msg("server requested disconnect; reason code: " + string(d.ReasonCode))
	}
}

// Long-running routine that handles the MQTT connection and publishes messages.
func (client Mqttclient) Serve(ctx context.Context) {
	// Set connection context
	serverUrl := url.URL{
		Scheme: "mqtt",
		Host:   fmt.Sprintf("%s:%d", client.Server.Host, client.Server.Port),
	}

	onconn := func(cm *autopaho.ConnectionManager, connAck *paho.Connack) {
		Logger.Info().Msg("Connected to MQTT server")
		sub := &paho.Subscribe{
			Subscriptions: []paho.SubscribeOptions{
				{Topic: "homeassistant/status", QoS: 1},
			},
		}
		if _, err := cm.Subscribe(context.Background(), sub); err != nil {
			Logger.Error().Err(err).Msg("Failed to subscribe to homeassistant status")
		}
		Logger.Info().Msg("Subscribed to homeassistant status")
		client.Conn <- true
	}

	onpub := []func(paho.PublishReceived) (bool, error){
		func(pr paho.PublishReceived) (bool, error) {
			if pr.Packet.Topic == "homeassistant/status" && string(pr.Packet.Payload) == "online" {
				Logger.Info().Msg("Homeassistant is online")
				client.Conn <- true
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
			ClientID:           fmt.Sprintf("systemPub@%s_%s", client.Device.Name, client.Device.Identifiers[0][:4]),
			OnPublishReceived:  onpub,
			OnClientError:      clientError,
			OnServerDisconnect: serverDis,
		},
	}
	Logger.Debug().Str("clientID", cliCfg.ClientID).Msg("")

	// starts process; will reconnect until context cancelled
	connManage, err := autopaho.NewConnection(ctx, cliCfg)
	if err != nil {
		panic(err)
	}
	if err = connManage.AwaitConnection(ctx); err != nil {
		panic(err)
	}
	for {
		select {
		case <-ctx.Done():
			// Cleanup and exit
			fmt.Println("Routine stopped")
			return
		case pub, ok := <-client.Pubs:
			// Check if the channel is closed
			if !ok {
				fmt.Println("Work channel closed, exiting routine")
				return
			}
			publish := autopaho.QueuePublish{Publish: pub}
			if err := connManage.PublishViaQueue(ctx, &publish); err != nil {
				Logger.Error().Err(err).Msg("Failed to publish")
			}
		}
	}
}
