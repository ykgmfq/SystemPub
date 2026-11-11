package systemd

import (
	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
)

// UnitProvider reports failed systemd units as a binary sensor.
type UnitProvider struct {
	conn   *dbus.Conn
	config models.MqttConfig
	logger zerolog.Logger
}

// hostnamectl holds the part of the `hostnamectl --json=short` output that describes the device.
type hostnamectl struct {
	Hostname                  string `json:"Hostname"`
	OperatingSystemPrettyName string `json:"OperatingSystemPrettyName"`
	MachineID                 string `json:"MachineID"`
	HardwareVendor            string `json:"HardwareVendor"`
	HardwareModel             string `json:"HardwareModel"`
}
