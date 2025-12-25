// Data models for SystemPub
package models

import (
	"net/url"

	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

// Custom URL type to handle YAML decoding
type YAMLURL struct {
	*url.URL
}

func (u *YAMLURL) UnmarshalYAML(value *yaml.Node) error {
	parsed, err := url.Parse(value.Value)
	if err != nil {
		return err
	}
	u.URL = parsed
	return nil
}

// Device information for Home Assistant autodiscovery
type Device struct {
	Name         string    `json:"name"`
	Model        string    `json:"model"`
	Manufacturer string    `json:"manufacturer"`
	SWversion    string    `json:"sw_version"`
	Identifiers  [1]string `json:"identifiers"`
}

// Sensor configuration for Home Assistant autodiscovery
type MqttConfig struct {
	Name          string `json:"name"`
	DeviceClass   string `json:"device_class"`
	StateTopic    string `json:"state_topic"`
	UniqueID      string `json:"unique_id"`
	ValueTemplate string `json:"value_template"`
	Device        Device `json:"device"`
	ExpireAfter   int    `json:"expire_after"`
	ForceUpdate   bool   `json:"force_update"`
}

// ZFS pool properties
type Property int

const (
	Health Property = iota
	Snaphots
	Capacity
)

// map property to string
var PropStr = map[Property]string{
	Health:   "health",
	Snaphots: "snapshots",
	Capacity: "capacity",
}

// Ouput of `hostnamectl` command
type Hostnamectl struct {
	Hostname                  string `json:"Hostname"`
	OperatingSystemPrettyName string `json:"OperatingSystemPrettyName"`
	MachineID                 string `json:"MachineID"`
	HardwareVendor            string `json:"HardwareVendor"`
	HardwareModel             string `json:"HardwareModel"`
}

// MQTT server location and credentials
type MQTT struct {
	Host     YAMLURL `yaml:"host"`
	User     string  `yaml:"user"`
	Password string  `yaml:"password"`
}

// Application configuration, as read from the configuration file
type SystemPubConfig struct {
	MQTTServer MQTT          `yaml:"mqttserver"`
	Loglevel   zerolog.Level `yaml:"loglevel"`
}

// Default MQTT server configuration
func MQTTdefault() MQTT {
	return MQTT{Host: YAMLURL{&url.URL{Scheme: "mqtt", Host: "localhost:1883"}}}
}

// Default application configuration
func SystemPubConfigDefault() SystemPubConfig {
	return SystemPubConfig{MQTTServer: MQTTdefault(), Loglevel: zerolog.InfoLevel}
}
