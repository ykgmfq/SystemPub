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
