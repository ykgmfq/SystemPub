// Data models for SystemPub
package models

import (
	"net/url"

	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

// Schemes accepted for the MQTT server address.
// The client enables TLS for the secure ones, so other aliases the MQTT library knows are left out.
var serverSchemes = []string{"mqtt", "mqtts", "ws", "wss"}

// YAMLURL is a URL that validates the MQTT server scheme when decoded from YAML.
type YAMLURL struct {
	*url.URL
}

// UnmarshalYAML decodes and validates a server address.
func (u *YAMLURL) UnmarshalYAML(value *yaml.Node) error {
	parsed, err := ParseServerURL(value.Value)
	if err != nil {
		return err
	}
	u.URL = parsed
	return nil
}

// MinMachineIDLength is the shortest device identifier from which the MQTT client ID can be derived.
const MinMachineIDLength = 4

// Device describes the host in the Home Assistant device registry.
type Device struct {
	Name         string    `json:"name"`
	Model        string    `json:"model"`
	Manufacturer string    `json:"manufacturer"`
	SWversion    string    `json:"sw_version"`
	Identifiers  [1]string `json:"identifiers"`
}

// MqttConfig is the Home Assistant autodiscovery configuration of one sensor.
type MqttConfig struct {
	Name                string `json:"name"`
	DeviceClass         string `json:"device_class,omitempty"`
	StateTopic          string `json:"state_topic"`
	UniqueID            string `json:"unique_id"`
	Device              Device `json:"device"`
	ExpireAfter         int    `json:"expire_after,omitempty"`
	ForceUpdate         bool   `json:"force_update"`
	StateClass          string `json:"state_class,omitempty"`
	UnitOfMeasurement   string `json:"unit_of_measurement,omitempty"`
	JsonAttributesTopic string `json:"json_attributes_topic,omitempty"`
}

// MQTT holds the location and credentials of the MQTT server.
type MQTT struct {
	Host     YAMLURL `yaml:"host"`
	User     string  `yaml:"user"`
	Password string  `yaml:"password"`
}

// SystemPubConfig is the application configuration.
type SystemPubConfig struct {
	MQTTServer MQTT          `yaml:"mqttserver"`
	Loglevel   zerolog.Level `yaml:"loglevel"`
}

// Domain is the Home Assistant entity type a sensor is published as.
type Domain string

const (
	BinarySensor Domain = "binary_sensor"
	Sensor       Domain = "sensor"
)

// Entry holds the MQTT config and current state for one sensor.
type Entry struct {
	Config     MqttConfig
	Domain     Domain
	Payload    []byte
	Attributes []byte // nil if no attributes
	Problem    bool   // true if the sensor reports a problem, which makes the server check more often
}
