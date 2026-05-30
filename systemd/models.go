package systemd

import (
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/ykgmfq/SystemPub/models"
)

type DbusClient struct {
	Conn     chan bool
	Discover chan bool
	Interval time.Duration
	Config   models.MqttConfig
	Pubs     chan *paho.Publish
}
