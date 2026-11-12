// Package firedrill fakes a problem on one sensor on request, so that the alerting in Home Assistant can be tested end to end.
package firedrill

import (
	"context"
	"math/rand/v2"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/provider"
)

// candidates returns the unique IDs of the sensors a drill can target.
func candidates(entries []models.Entry) []string {
	var ids []string
	for _, e := range entries {
		if provider.IsDrillTarget(e) {
			ids = append(ids, e.Config.UniqueID)
		}
	}
	return ids
}

// newDrill returns a drill on the given sensor that starts now.
func newDrill(target string, cfg models.FireDrill, now time.Time) models.Drill {
	return models.Drill{UniqueID: target, Until: now.Add(cfg.Duration), ErrorCount: cfg.ErrorCount}
}

// send hands the drill to every server.
// It gives up once the context ends, because the servers stop receiving at shutdown.
func send(ctx context.Context, drill models.Drill, servers []chan models.Drill) {
	for _, server := range servers {
		select {
		case server <- drill:
		case <-ctx.Done():
			return
		}
	}
}

// drill picks a target and starts a drill on it, unless there is an actual problem or nothing to target.
func drill(ctx context.Context, cfg models.FireDrill, providers []provider.Provider, servers []chan models.Drill, logger zerolog.Logger) {
	entries := provider.Collect(ctx, providers, logger)
	if provider.Degraded(entries) {
		logger.Info().Msg("Fire drill skipped because the system is degraded")
		return
	}
	ids := candidates(entries)
	if len(ids) == 0 {
		logger.Warn().Msg("Fire drill skipped because no sensor can be targeted")
		return
	}
	d := newDrill(ids[rand.IntN(len(ids))], cfg, time.Now())
	logger.Warn().Str("sensor", d.UniqueID).Time("until", d.Until).Msg("Fire drill: sensor reports a problem")
	send(ctx, d, servers)
}

// Run starts a drill for every trigger until the context ends.
func Run(ctx context.Context, triggers <-chan os.Signal, cfg models.FireDrill, providers []provider.Provider, servers []chan models.Drill, logger zerolog.Logger) {
	logger = logger.With().Str("mod", "firedrill").Logger()
	for {
		select {
		case <-ctx.Done():
			return
		case <-triggers:
			drill(ctx, cfg, providers, servers, logger)
		}
	}
}
