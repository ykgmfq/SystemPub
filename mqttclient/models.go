package mqttclient

import (
	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
)

// levelLogger lets the MQTT library log through zerolog at a fixed level.
type levelLogger struct {
	logger zerolog.Logger
	level  zerolog.Level
}

type Mqttclient struct {
	Server        models.MQTT
	Device        models.Device
	Pubs          chan *paho.Publish
	ConnListeners []chan bool
	logger        zerolog.Logger
}
