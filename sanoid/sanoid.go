// Provides checks for the state of pool health, capacity and snapshots on the system.
package sanoid

import (
	"os/exec"
	"strings"

	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
)

var Logger zerolog.Logger

// Runs Sanoid to check one of pool health, capacity and snapshots, and returns true if the output is "OK"
func getPoolState(p models.Property) bool {
	out, err := exec.Command("sanoid", "--monitor-"+models.PropStr[p]).Output()
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
