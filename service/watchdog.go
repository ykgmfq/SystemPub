package service

import (
	"context"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/rs/zerolog"
)

// connectionStatus returns the systemd status text for the given MQTT connection state.
func connectionStatus(connected bool) string {
	if connected {
		return "STATUS=Connected to MQTT server"
	}
	return "STATUS=Disconnected from MQTT server"
}

// Notify sends the given state to systemd.
// A failed notification is logged rather than escalated, because the service keeps working without it.
func Notify(state string, logger zerolog.Logger) {
	if _, err := daemon.SdNotify(false, state); err != nil {
		logger.Warn().Err(err).Str("state", state).Msg("Could not Notify systemd")
	}
}

// Watchdog reports readiness and connection status to systemd and pets the watchdog when one is configured.
// It keeps consuming connection updates even without a watchdog, because the MQTT client blocks until every listener has received them.
func Watchdog(ctx context.Context, conn chan bool, logger zerolog.Logger) {
	watchTime, err := daemon.SdWatchdogEnabled(false)
	if err != nil {
		logger.Warn().Err(err).Msg("Could not determine systemd watchdog settings")
	}
	Notify(daemon.SdNotifyReady+"\nSTATUS=Connecting...", logger)
	var ticks <-chan time.Time
	if err == nil && watchTime > 0 {
		timer := time.NewTicker(watchTime / 2)
		defer timer.Stop()
		ticks = timer.C
	}
	for {
		select {
		case <-ctx.Done():
			return
		case connected := <-conn:
			Notify(connectionStatus(connected), logger)
		case <-ticks:
			Notify(daemon.SdNotifyWatchdog, logger)
		}
	}
}
