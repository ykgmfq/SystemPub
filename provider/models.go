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

// Upper bound for a single provider to deliver its entries, so that one hanging provider cannot stall the whole group.
const entriesTimeout = 30 * time.Second

// Provider delivers sensor entries for discovery and state updates.
type Provider interface {
	Entries(context.Context) ([]models.Entry, error)
}

// drillAttribute names the attribute that tells alert automations whether a sensor's problem is only a fire drill.
const drillAttribute = "fire_drill"

// drillCleanupTimeout bounds publishing the real state when a drill is cut short by shutdown, so that an unreachable MQTT server cannot hold up the exit.
const drillCleanupTimeout = 10 * time.Second

// check is the outcome of one check of the sensors.
// A nil drill means that no drill is running.
type check struct {
	real      []models.Entry
	published []models.Entry
	drill     *models.Drill
}

// trigger says why the server checks its sensors.
// A nil drill means that no new drill has started.
type trigger struct {
	discover bool
	drill    *models.Drill
}

// Server runs a group of providers on a ticker and publishes their sensors to MQTT.
type Server struct {
	Discover  chan bool
	Drills    chan models.Drill
	providers []Provider
	interval  time.Duration
	pubs      chan *paho.Publish
	logger    zerolog.Logger
}
