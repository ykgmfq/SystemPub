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
		Drills:    make(chan models.Drill),
		providers: providers,
		interval:  interval,
		pubs:      pubs,
		logger:    logger.With().Str("mod", name).Logger(),
	}
}

// discoveryMessages returns the discovery messages for the given entries.
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

// stateMessages returns the state and attribute messages for the given entries.
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

// hasProblem reports whether any of the entries signals a problem.
func hasProblem(entries []models.Entry) bool {
	for _, e := range entries {
		if e.Problem {
			return true
		}
	}
	return false
}

// nextInterval returns how long to wait before the next check.
func nextInterval(problem bool, interval time.Duration) time.Duration {
	if problem {
		return min(problemInterval, interval)
	}
	return interval
}

// providerEntries returns the entries of one provider, bounded by its own deadline.
func providerEntries(ctx context.Context, p Provider) ([]models.Entry, error) {
	ctx, cancel := context.WithTimeout(ctx, entriesTimeout)
	defer cancel()
	return p.Entries(ctx)
}

// Collect gathers the entries of all given providers.
// A failing provider is logged and skipped, so that it does not hold back the others.
func Collect(ctx context.Context, providers []Provider, logger zerolog.Logger) []models.Entry {
	var entries []models.Entry
	for _, p := range providers {
		pe, err := providerEntries(ctx, p)
		if err != nil {
			logger.Error().Err(err).Msg("Provider failed")
			continue
		}
		entries = append(entries, pe...)
	}
	return entries
}

// publish hands the messages to the MQTT client.
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

// wait blocks until the next check is due.
// It reports what triggered the check: a new MQTT connection calls for discovery, and a new drill replaces the current one.
// It returns an error once the context ends.
func (s Server) wait(ctx context.Context, ticks <-chan time.Time) (trigger, error) {
	for {
		select {
		case <-ctx.Done():
			return trigger{}, ctx.Err()
		case connected := <-s.Discover:
			if connected {
				return trigger{discover: true}, nil
			}
		case drill := <-s.Drills:
			return trigger{drill: &drill}, nil
		case <-ticks:
			return trigger{}, nil
		}
	}
}

// update collects all entries and publishes their state, preceded by discovery messages if requested.
// The returned check holds the drill only if it is still running.
func (s Server) update(ctx context.Context, discover bool, drill *models.Drill) check {
	real := Collect(ctx, s.providers, s.logger)
	drill = runningDrill(drill, real, time.Now())
	entries, err := applyDrill(real, drill)
	if err != nil {
		s.logger.Error().Err(err).Msg("Could not apply the fire drill, publishing the real state")
		entries = real
	}
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
	return check{real: real, published: entries, drill: drill}
}

// endDrill republishes the real state of the last check if a drill was running, because the faked state is retained by the MQTT server and would otherwise outlive the process.
// It runs at shutdown, so it publishes under a context of its own that the shutdown does not cancel.
func (s Server) endDrill(ctx context.Context, last check) {
	if last.drill == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), drillCleanupTimeout)
	defer cancel()
	entries, err := applyDrill(last.real, nil)
	if err != nil {
		s.logger.Error().Err(err).Msg("Could not clear the fire drill flag, publishing the real state without it")
		entries = last.real
	}
	s.logger.Info().Str("sensor", last.drill.UniqueID).Msg("Fire drill ended by shutdown")
	s.publish(ctx, stateMessages(entries))
}

// adjustInterval switches the ticker between the regular and the problem interval when the problem state changes.
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
// When the context ends, it ends a running fire drill before it returns.
func (s Server) Serve(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	problem := false
	var last check
	for {
		t, err := s.wait(ctx, ticker.C)
		if err != nil {
			s.endDrill(ctx, last)
			return
		}
		// A drill that has ended, even early, comes back as nil, so it cannot resume.
		drill := last.drill
		if t.drill != nil {
			drill = t.drill
		}
		last = s.update(ctx, t.discover, drill)
		// A drilled sensor counts as a problem, so the real state returns within the problem interval after the drill ends.
		isProblem := hasProblem(last.published)
		s.adjustInterval(ticker, problem, isProblem)
		problem = isProblem
	}
}
