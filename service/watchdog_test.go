package service

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// Without a systemd watchdog, connection updates must still be consumed so the MQTT client never blocks on them.
func TestWatchdogConsumesWithoutSystemd(t *testing.T) {
	t.Setenv("WATCHDOG_USEC", "")
	t.Setenv("NOTIFY_SOCKET", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := make(chan bool)
	go Watchdog(ctx, conn, zerolog.Nop())
	for _, connected := range []bool{true, false, true} {
		select {
		case conn <- connected:
		case <-time.After(time.Second):
			t.Fatal("watchdog did not consume connection update")
		}
	}
}
