package provider

import (
	"context"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
)

// While any sensor reports a problem, the server checks this often, so that a recovery shows up quickly.
const problemInterval = time.Minute

// Provider delivers sensor entries for discovery and state updates.
type Provider interface {
	Entries(context.Context) ([]models.Entry, error)
}

// Server runs a group of providers on a ticker and publishes their sensors to MQTT.
type Server struct {
	Discover  chan bool
	providers []Provider
	interval  time.Duration
	pubs      chan *paho.Publish
	logger    zerolog.Logger
}
