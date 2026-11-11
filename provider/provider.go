// Package provider runs sensor providers and publishes their results to MQTT.
package provider

import (
	"context"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
)

// NewServer returns a server that checks the given providers at the given interval.
// The name labels the server's log messages.
func NewServer(name string, pubs chan *paho.Publish, interval time.Duration, logger zerolog.Logger, providers ...Provider) Server {
	return Server{
		Discover:  make(chan bool),
		providers: providers,
		interval:  interval,
		pubs:      pubs,
		logger:    logger.With().Str("mod", name).Logger(),
	}
}

// Returns the discovery messages for the given entries.
func discoveryMessages(entries []models.Entry) ([]*paho.Publish, error) {
	msgs := make([]*paho.Publish, 0, len(entries))
	for _, e := range entries {
		msg, err := mqttclient.GetDiscovery(e.Domain, e.Config)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// Returns the state and attribute messages for the given entries.
func stateMessages(entries []models.Entry) []*paho.Publish {
	msgs := make([]*paho.Publish, 0, 2*len(entries))
	for _, e := range entries {
		msgs = append(msgs, &paho.Publish{Topic: e.Config.StateTopic, Payload: e.Payload, Retain: true})
		if e.Attributes != nil {
			msgs = append(msgs, &paho.Publish{Topic: e.Config.JsonAttributesTopic, Payload: e.Attributes, Retain: true})
		}
	}
	return msgs
}

// Reports whether any of the entries signals a problem.
func hasProblem(entries []models.Entry) bool {
	for _, e := range entries {
		if e.Problem {
			return true
		}
	}
	return false
}

// Returns how long to wait before the next check.
func nextInterval(problem bool, interval time.Duration) time.Duration {
	if problem {
		return min(problemInterval, interval)
	}
	return interval
}

// Returns the entries of one provider, bounded by its own deadline.
func providerEntries(ctx context.Context, p Provider) ([]models.Entry, error) {
	ctx, cancel := context.WithTimeout(ctx, entriesTimeout)
	defer cancel()
	return p.Entries(ctx)
}

// Collects the entries of all providers.
// A failing provider is logged and skipped, so that it does not hold back the others.
func (s Server) collect(ctx context.Context) []models.Entry {
	var entries []models.Entry
	for _, p := range s.providers {
		pe, err := providerEntries(ctx, p)
		if err != nil {
			s.logger.Error().Err(err).Msg("Provider failed")
			continue
		}
		entries = append(entries, pe...)
	}
	return entries
}

// Hands the messages to the MQTT client.
// It gives up once the context ends, because the client stops receiving at shutdown.
func (s Server) publish(ctx context.Context, msgs []*paho.Publish) {
	for _, msg := range msgs {
		select {
		case s.pubs <- msg:
		case <-ctx.Done():
			return
		}
	}
}

// Blocks until the next check is due.
// It reports whether the check was triggered by a new MQTT connection, which calls for discovery, and returns an error once the context ends.
func (s Server) wait(ctx context.Context, ticks <-chan time.Time) (bool, error) {
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case connected := <-s.Discover:
			if connected {
				return true, nil
			}
		case <-ticks:
			return false, nil
		}
	}
}

// Collects all entries and publishes their state, preceded by discovery messages if requested.
func (s Server) update(ctx context.Context, discover bool) []models.Entry {
	entries := s.collect(ctx)
	if discover {
		s.logger.Debug().Msg("Discovery")
		msgs, err := discoveryMessages(entries)
		if err != nil {
			s.logger.Error().Err(err).Msg("Could not build discovery messages")
		} else {
			s.publish(ctx, msgs)
		}
	}
	s.publish(ctx, stateMessages(entries))
	s.logger.Debug().Int("entries", len(entries)).Msg("Updated sensors")
	return entries
}

// Switches the ticker between the regular and the problem interval when the problem state changes.
func (s Server) adjustInterval(ticker *time.Ticker, wasProblem, isProblem bool) {
	if wasProblem == isProblem {
		return
	}
	interval := nextInterval(isProblem, s.interval)
	ticker.Reset(interval)
	if isProblem {
		s.logger.Info().Dur("interval", interval).Msg("Problem detected, checking more often")
	} else {
		s.logger.Info().Dur("interval", interval).Msg("Problem resolved, back to the regular interval")
	}
}

// Serve publishes discovery and state whenever the MQTT connection comes up, and state again on every tick.
func (s Server) Serve(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	problem := false
	for {
		discover, err := s.wait(ctx, ticker.C)
		if err != nil {
			return
		}
		entries := s.update(ctx, discover)
		isProblem := hasProblem(entries)
		s.adjustInterval(ticker, problem, isProblem)
		problem = isProblem
	}
}
