// Provides checks for the state of systemd units.
// Also used to query the properties of the client host device
package systemd

import (
	"encoding/json"
	"os/exec"

	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
)

var Logger zerolog.Logger

// Queries systemd for failed units. If there are any, it logs them and returns false.
func GetUnitState() bool {
	out, err := exec.Command("systemctl", "list-units", "--output=json", "--state=failed").Output()
	if err != nil {
		Logger.Error().Err(err).Msg("")
		return false
	}
	//unmarshal the json and check if there are any failed units
	var units []map[string]any
	if err = json.Unmarshal(out, &units); err != nil {
		Logger.Error().Err(err).Msg("")
		return false
	}
	for _, unit := range units {
		Logger.Warn().Str("failed unit", unit["unit"].(string)).Msg("")
	}
	return len(units) == 0
}

// Returns a MqttConfig for the systemd units binary sensor.
func GetUnitConfig(device models.Device) models.MqttConfig {
	unique_id := device.Name + "_units"
	return models.MqttConfig{Name: "Systemd units", StateTopic: "homeassistant/binary_sensor/" + unique_id + "/state", DeviceClass: "problem", UniqueID: unique_id, Device: device, ValueTemplate: "{{ value_json.sensor }}", ExpireAfter: 600, ForceUpdate: true}
}

// Returns the client device properties.
func GetDevice() models.Device {
	out, err := exec.Command("hostnamectl", "--json=short").Output()
	if err != nil {
		Logger.Fatal().Err(err).Msg("")
		panic(err)
	}
	var status models.Hostnamectl
	if err = json.Unmarshal(out, &status); err != nil {
		Logger.Fatal().Err(err).Msg("")
		panic(err)
	}
	device := models.Device{Name: status.Hostname, SWversion: status.OperatingSystemPrettyName, Identifiers: [1]string{status.MachineID}, Manufacturer: status.HardwareVendor, Model: status.HardwareModel}
	return device
}
