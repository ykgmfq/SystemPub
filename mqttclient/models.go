package mqttclient

import (
	"regexp"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
)

// Home Assistant listens for discovery and sensor messages below this prefix.
const discoveryPrefix = "homeassistant"

// Home Assistant announces itself on this topic when it starts.
const statusTopic = discoveryPrefix + "/status"

// Upper bound for a single request to the server, so that a stalled connection cannot block the client indefinitely.
const requestTimeout = 10 * time.Second

// Characters that are not allowed in MQTT topics and Home Assistant IDs.
var invalidChars = regexp.MustCompile(`[^a-z0-9_-]`)

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
