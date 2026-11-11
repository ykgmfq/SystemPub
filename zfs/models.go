package zfs

import (
	"context"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
)

// Provider delivers sensor entries for discovery and state updates.
type Provider interface {
	Entries(context.Context) ([]models.Entry, error)
}

// ZfsServer runs all ZFS providers on a ticker and publishes to MQTT.
type ZfsServer struct {
	Discover  chan bool
	providers []Provider
	interval  time.Duration
	pubs      chan *paho.Publish
	logger    zerolog.Logger
}
