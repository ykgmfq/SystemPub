// Provides checks for the state of pool health, capacity and snapshots on the system.
package sanoid

import (
	"os/exec"
	"strings"

	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
)

var Logger zerolog.Logger

// Interface for injecting mock output in tests
type commandExecutor interface {
	Output() ([]byte, error)
}

// Gets overwritten in tests
var shellCommandFunc = func(name string, arg ...string) commandExecutor {
	return exec.Command(name, arg...)
}

// Runs Sanoid to check one of pool health, capacity and snapshots, and returns true if the output is "OK"
func getPoolState(p models.Property) bool {
	cmd := shellCommandFunc("sanoid", "--monitor-"+models.PropStr[p])
	out, err := cmd.Output()
	output := string(out)
	if err != nil {
		Logger.Error().Err(err).Msg("Failed to execute sanoid")
		return false
	}
	ok := strings.HasPrefix(output, "OK")
	if !ok {
		Logger.Warn().Str("sanoid output", output).Msg("")
	}
	return ok
}

// Gathers the current state of health, capacity and snapshots of the pools
func GetPoolStates() map[models.Property]bool {
	result := make(map[models.Property]bool, len(models.PropStr))
	for p := range models.PropStr {
		result[p] = getPoolState(p)
	}
	return result
}

// Gathers autodiscovery struct for the binary health, capacity and snapshot sensors
func GetPoolConfigs(device models.Device) map[models.Property]models.MqttConfig {
	configs := make(map[models.Property]models.MqttConfig, len(models.PropStr))
	//iterate over all properties
	for prop, propStr := range models.PropStr {
		unique_id := device.Name + "_pool_" + propStr
		topic := "homeassistant/binary_sensor/" + unique_id + "/state"
		configs[prop] = models.MqttConfig{Name: "Pool " + propStr, StateTopic: topic, DeviceClass: "problem", UniqueID: unique_id, Device: device, ValueTemplate: "{{ value_json.sensor }}", ExpireAfter: 600, ForceUpdate: true}
	}
	return configs
}

// Reads the current Sanoid states and returns update messages with the sensor states.
func GetUpdates(poolConfigs map[models.Property]models.MqttConfig) []*paho.Publish {
	states := GetPoolStates()
	updates := make([]*paho.Publish, len(states))
	// get the current state of the pools and the unit via go routines
	for p, state := range states {
		update := paho.Publish{
			QoS:     1,
			Topic:   poolConfigs[p].StateTopic,
			Payload: mqttclient.ProblemPayload(state),
		}
		updates[p] = &update
	}
	return updates
}

// Returns the discovery messages for the Sanoid sensors.
func GetDiscoveries(poolConfigs map[models.Property]models.MqttConfig) []*paho.Publish {
	discoveries := make([]*paho.Publish, len(poolConfigs))
	for i, config := range poolConfigs {
		discoveries[i] = mqttclient.GetDiscovery(config)
	}
	return discoveries
}
