package firedrill

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/provider"
)

// fakeProvider is a provider that returns fixed entries.
type fakeProvider struct {
	entries []models.Entry
}

// Entries returns the fixed entries and never fails.
func (f fakeProvider) Entries(context.Context) ([]models.Entry, error) {
	return f.entries, nil
}

// sensor returns a sensor that a drill can target: a problem sensor for the binary domain, and an error counter otherwise.
func sensor(uid string, domain models.Domain, problem bool) models.Entry {
	e := models.Entry{Config: models.MqttConfig{UniqueID: uid}, Domain: domain, Problem: problem}
	if domain == models.BinarySensor {
		e.Config.DeviceClass = "problem"
	} else {
		e.Config.StateClass = "total_increasing"
		e.Payload = []byte("0")
	}
	return e
}

// TestCandidates checks that only targetable sensors become candidates, in their original order.
func TestCandidates(t *testing.T) {
	size := models.Entry{Config: models.MqttConfig{UniqueID: "size", StateClass: "measurement"}, Domain: models.Sensor}
	entries := []models.Entry{sensor("units", models.BinarySensor, false), size, sensor("errors", models.Sensor, false)}
	assert.Equal(t, []string{"units", "errors"}, candidates(entries))
}

// TestNewDrill checks that a drill takes its duration and error count from the settings.
func TestNewDrill(t *testing.T) {
	now := time.Date(2026, 11, 14, 11, 0, 0, 0, time.UTC)
	d := newDrill("units", models.FireDrillDefault(), now)
	assert.Equal(t, models.Drill{UniqueID: "units", Until: now.Add(5 * time.Minute), ErrorCount: 42}, d)
}

// runOnce sends one trigger and returns the drills each server received within a short time.
func runOnce(t *testing.T, providers []provider.Provider) []*models.Drill {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	triggers := make(chan os.Signal, 1)
	servers := []chan models.Drill{make(chan models.Drill, 1), make(chan models.Drill, 1)}
	go Run(ctx, triggers, models.FireDrillDefault(), providers, servers, zerolog.Nop())
	triggers <- syscall.SIGUSR1

	got := make([]*models.Drill, len(servers))
	for i, server := range servers {
		select {
		case d := <-server:
			got[i] = &d
		case <-time.After(200 * time.Millisecond):
		}
	}
	return got
}

// TestRunSendsDrillToEveryServer checks that every server receives the same drill, so the one that owns the target can apply it.
func TestRunSendsDrillToEveryServer(t *testing.T) {
	providers := []provider.Provider{
		fakeProvider{entries: []models.Entry{sensor("units", models.BinarySensor, false)}},
		fakeProvider{entries: []models.Entry{sensor("errors", models.Sensor, false)}},
	}
	got := runOnce(t, providers)
	if assert.NotNil(t, got[0]) && assert.NotNil(t, got[1]) {
		assert.Equal(t, *got[0], *got[1])
		assert.Contains(t, []string{"units", "errors"}, got[0].UniqueID)
	}
}

// TestRunSkipsDuringRealProblem checks that no drill starts while a sensor reports an actual problem.
func TestRunSkipsDuringRealProblem(t *testing.T) {
	providers := []provider.Provider{
		fakeProvider{entries: []models.Entry{sensor("units", models.BinarySensor, true), sensor("errors", models.Sensor, false)}},
	}
	assert.Equal(t, []*models.Drill{nil, nil}, runOnce(t, providers))
}

// TestRunSkipsWithCountedErrors checks that no drill starts while any error counter is above zero.
func TestRunSkipsWithCountedErrors(t *testing.T) {
	errors := sensor("errors", models.Sensor, false)
	errors.Payload = []byte("1")
	providers := []provider.Provider{
		fakeProvider{entries: []models.Entry{sensor("units", models.BinarySensor, false), errors}},
	}
	assert.Equal(t, []*models.Drill{nil, nil}, runOnce(t, providers))
}
