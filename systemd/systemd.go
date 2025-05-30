// Provides checks for the state of systemd units.
// Also used to query the properties of the client host device
package systemd

import (
	"context"
	"encoding/json"
	"os/exec"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
)

var Logger zerolog.Logger

// Returns a MqttConfig for the systemd units binary sensor.
func getUnitConfig(device models.Device, interval time.Duration) models.MqttConfig {
	unique_id := device.Name + "_units"
	return models.MqttConfig{Name: "Systemd units", StateTopic: "homeassistant/binary_sensor/" + unique_id + "/state", DeviceClass: "problem", UniqueID: unique_id, Device: device, ValueTemplate: "{{ value_json.sensor }}", ExpireAfter: int((interval * 2).Seconds()), ForceUpdate: true}
}

// Returns the client device properties.
func GetDevice() models.Device {
	out, err := exec.Command("hostnamectl", "--json=short").Output()
	if err != nil {
		Logger.Fatal().Str("mod", "systemd").Err(err).Msg("")
		panic(err)
	}
	var status models.Hostnamectl
	if err = json.Unmarshal(out, &status); err != nil {
		Logger.Fatal().Str("mod", "systemd").Err(err).Msg("")
		panic(err)
	}
	device := models.Device{Name: status.Hostname, SWversion: status.OperatingSystemPrettyName, Identifiers: [1]string{status.MachineID}, Manufacturer: status.HardwareVendor, Model: status.HardwareModel}
	return device
}

// Client for systemd units monitoring via D-Bus.
type DbusClient struct {
	Conn     chan bool
	Discover chan bool
	Interval time.Duration
	Config   models.MqttConfig
	Pubs     chan *paho.Publish
}

// Returns a new DbusClient instance with initialized channels and configuration.
func NewDbusclient(pubs chan *paho.Publish, device models.Device, interval time.Duration) DbusClient {
	return DbusClient{
		Pubs:     pubs,
		Interval: interval,
		Config:   getUnitConfig(device, interval),
		Discover: make(chan bool),
		Conn:     make(chan bool),
	}
}

// Queries the systemd D-Bus for failed units and publishes the state to MQTT.
func (client DbusClient) update(ctx context.Context, conn *dbus.Conn) (bool, error) {
	states, err := conn.ListUnitsByPatternsContext(ctx, []string{"failed"}, []string{"*"})
	if err != nil {
		return false, err
	}
	ok := true
	if len(states) > 0 {
		ok = false
		for _, state := range states {
			Logger.Warn().Str("mod", "systemd").Str("failed unit", state.Name).Msg("")
		}
	}
	update := paho.Publish{
		Payload: mqttclient.ProblemPayload(ok),
		Topic:   client.Config.StateTopic,
	}
	client.Pubs <- &update
	Logger.Debug().Str("mod", "systemd").Msg("Updated sensors")
	return ok, nil
}

// Long-running routine that handles the D-Bus connection and publishes messages.
func (client DbusClient) Serve(ctx context.Context) {
	dbusctx, cancel := context.WithCancel(ctx)
	conn, err := dbus.NewWithContext(dbusctx)
	if err != nil {
		Logger.Fatal().Str("mod", "systemd").Err(err).Msg("")
		client.Conn <- false
		cancel()
		return
	}
	healthy := false
	updateTimer := time.NewTicker(time.Minute)
	up := func() {
		ok, err := client.update(ctx, conn)
		switch {
		case err != nil:
			Logger.Error().Str("mod", "systemd").Err(err).Msg("")
			return
		case !ok && healthy:
			healthy = false
			updateTimer.Stop()
			updateTimer = time.NewTicker(time.Minute)
			Logger.Info().Str("mod", "systemd").Msg("Transitioned to unhealthy state")
		case ok && !healthy:
			healthy = true
			updateTimer.Stop()
			updateTimer = time.NewTicker(client.Interval)
			Logger.Info().Str("mod", "systemd").Msg("Transitioned to healthy state")
		}
	}
	for {
		select {
		case <-dbusctx.Done():
			cancel()
			return
		case ok := <-client.Discover:
			if !ok {
				continue
			}
			discovery := mqttclient.GetDiscovery(client.Config)
			client.Pubs <- discovery
			Logger.Debug().Str("mod", "systemd").Msg("Discovery")
			up()
		case <-updateTimer.C:
			up()
		}
	}
}
