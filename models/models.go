package models

import "github.com/rs/zerolog"

type Device struct {
	Name         string    `json:"name"`
	Model        string    `json:"model"`
	Manufacturer string    `json:"manufacturer"`
	SWversion    string    `json:"sw_version"`
	Identifiers  [1]string `json:"identifiers"`
}

type SanoidState struct {
	Health    bool
	Snapshots bool
	Capacity  bool
}

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

type Hostnamectl struct {
	Hostname                  string `json:"Hostname"`
	OperatingSystemPrettyName string `json:"OperatingSystemPrettyName"`
	MachineID                 string `json:"MachineID"`
	HardwareVendor            string `json:"HardwareVendor"`
	HardwareModel             string `json:"HardwareModel"`
}

type MQTT struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type SystemPubConfig struct {
	MQTTServer MQTT          `json:"mqttserver"`
	Loglevel   zerolog.Level `json:"loglevel"`
}
