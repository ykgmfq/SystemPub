package mqttclient

import (
	"github.com/eclipse/paho.golang/paho"
	"github.com/ykgmfq/SystemPub/models"
)

type Mqttclient struct {
	Server        models.MQTT
	Device        models.Device
	Pubs          chan *paho.Publish
	ConnListeners []chan bool
}
