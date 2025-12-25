// Provides a MQTT client for Home Assistant sensors.
package mqttclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/eclipse/paho.golang/autopaho"
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
	Server        models.MQTT
	Device        models.Device
	Pubs          chan *paho.Publish
	ConnListeners []chan bool
}

// Returns a MQTT client instance with initialized channels
func NewMqttclient(server models.MQTT, device models.Device) Mqttclient {
	return Mqttclient{
		Server:        server,
		Device:        device,
		Pubs:          make(chan *paho.Publish, 4),
		ConnListeners: make([]chan bool, 0),
	}
}

// Returns a discovery message for a given sensor
func GetDiscovery(config models.MqttConfig) *paho.Publish {
	payload, err := json.Marshal(config)
	if err != nil {
		Logger.Fatal().Str("mod", "mqtt").Err(err).Msg("Failed to marshal discovery message")
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

// Handles server disconnects.
func serverDis(d *paho.Disconnect) {
	if d.Properties != nil {
		Logger.Info().Str("mod", "mqtt").Msg("server requested disconnect: " + d.Properties.ReasonString)
	} else {
		Logger.Info().Str("mod", "mqtt").Msg("server requested disconnect; reason code: " + string(d.ReasonCode))
	}
}

// Notifies all registered listeners about the connection status to Home Assitant.
func (client Mqttclient) notifyListeners(connected bool) {
	for _, listener := range client.ConnListeners {
		listener <- connected
	}
}

// Creates a new MQTT connection with the given configuration.
func (client Mqttclient) createConnection(ctx context.Context) (*autopaho.ConnectionManager, error) {
	onconn := func(cm *autopaho.ConnectionManager, connAck *paho.Connack) {
		Logger.Info().Str("mod", "mqtt").Msg("Connected to MQTT server")
		sub := &paho.Subscribe{
			Subscriptions: []paho.SubscribeOptions{
				{Topic: "homeassistant/status", QoS: 1},
			},
		}
		if _, err := cm.Subscribe(context.Background(), sub); err != nil {
			Logger.Error().Str("mod", "mqtt").Err(err).Msg("Failed to subscribe to homeassistant status")
		}
		Logger.Info().Str("mod", "mqtt").Msg("Subscribed to homeassistant status")
		client.notifyListeners(true)
	}

	onpub := func(pr paho.PublishReceived) (bool, error) {
		Logger.Debug().Str("mod", "mqtt").Interface("msg", &pr.Packet).Msg("Received message")
		if pr.Packet.Topic == "homeassistant/status" && string(pr.Packet.Payload) == "online" {
			Logger.Info().Str("mod", "mqtt").Msg("Homeassistant is online")
			client.notifyListeners(true)
		}
		return true, nil
	}

	onerr := func(err error) {
		client.notifyListeners(false)
		Logger.Error().Err(err).Msg("")
	}

	onpubl := func(p *paho.Publish) {
		Logger.Debug().Str("mod", "mqtt").Str("topic", p.Topic).Msg("Published message")
	}

	// Client configuration
	user := client.Server.User
	if client.Server.User == "" {
		user = fmt.Sprintf("systemPub@%s", client.Device.Name)
	}
	cliCfg := autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{client.Server.Host.URL},
		KeepAlive:                     20,
		CleanStartOnInitialConnection: false,
		SessionExpiryInterval:         60,
		OnConnectionUp:                onconn,
		OnConnectError:                onerr,
		ClientConfig: paho.ClientConfig{
			ClientID:           fmt.Sprintf("systemPub@%s_%s", client.Device.Name, client.Device.Identifiers[0][:4]),
			OnPublishReceived:  []func(paho.PublishReceived) (bool, error){onpub},
			OnClientError:      clientError,
			OnServerDisconnect: serverDis,
			PublishHook:        onpubl,
		},
		ConnectUsername: user,
		ConnectPassword: []byte(client.Server.Password),
	}
	Logger.Debug().Str("mod", "mqtt").Str("username", cliCfg.ConnectUsername).Str("clientID", cliCfg.ClientID).Msg("")

	Logger.Debug().Str("mod", "mqtt").Msg("Starting connection")
	connManage, err := autopaho.NewConnection(ctx, cliCfg)
	if err != nil {
		return nil, err
	}
	return connManage, nil
}

// Long-running routine that handles the MQTT connection and publishes messages.
func (client Mqttclient) Serve(ctx context.Context) {
	mqttctx, cancel := context.WithCancel(ctx)
	defer cancel()
	connManage, err := client.createConnection(mqttctx)
	if err != nil {
		return
	}
	for {
		select {
		case <-mqttctx.Done():
			// Cleanup and exit
			fmt.Println("Routine stopped")
			return
		case pub, ok := <-client.Pubs:
			// Check if the channel is closed
			if !ok {
				fmt.Println("Work channel closed, exiting routine")
				return
			}
			if _, err := connManage.Publish(mqttctx, pub); err != nil {
				Logger.Error().Str("mod", "mqtt").Err(err).Msg("Failed to publish")
			}
		}
	}
}
