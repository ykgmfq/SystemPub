package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ykgmfq/SystemPub/models"
)

type fakeProvider struct {
	entries []models.Entry
	err     error
}

func (f fakeProvider) Entries(context.Context) ([]models.Entry, error) {
	return f.entries, f.err
}

func entry(uid, domain string, problem bool, attrs []byte) models.Entry {
	return models.Entry{
		Config: models.MqttConfig{
			UniqueID:            uid,
			StateTopic:          "state/" + uid,
			JsonAttributesTopic: "attrs/" + uid,
		},
		Domain:     domain,
		Payload:    []byte("payload-" + uid),
		Attributes: attrs,
		Problem:    problem,
	}
}

func topics(msgs []*paho.Publish) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Topic)
	}
	return out
}

func TestDiscoveryMessagesUseDomain(t *testing.T) {
	msgs, err := discoveryMessages([]models.Entry{entry("a", "binary_sensor", false, nil), entry("b", "sensor", false, nil)})
	require.NoError(t, err)
	assert.Equal(t, []string{"homeassistant/binary_sensor/a/config", "homeassistant/sensor/b/config"}, topics(msgs))
}

func TestStateMessagesIncludeAttributesOnlyWhenPresent(t *testing.T) {
	msgs := stateMessages([]models.Entry{entry("a", "binary_sensor", false, []byte("{}")), entry("b", "sensor", false, nil)})
	assert.Equal(t, []string{"state/a", "attrs/a", "state/b"}, topics(msgs))
	for _, m := range msgs {
		assert.True(t, m.Retain)
	}
}

func TestHasProblem(t *testing.T) {
	assert.False(t, hasProblem(nil))
	assert.False(t, hasProblem([]models.Entry{entry("a", "sensor", false, nil)}))
	assert.True(t, hasProblem([]models.Entry{entry("a", "sensor", false, nil), entry("b", "binary_sensor", true, nil)}))
}

func TestNextInterval(t *testing.T) {
	assert.Equal(t, 10*time.Minute, nextInterval(false, 10*time.Minute))
	assert.Equal(t, problemInterval, nextInterval(true, 10*time.Minute))
	// A regular interval shorter than the problem interval is never slowed down.
	assert.Equal(t, 10*time.Second, nextInterval(true, 10*time.Second))
}

// A discovery request publishes discovery and state for all working providers, and skips a failing one.
func TestServeDiscovery(t *testing.T) {
	pubs := make(chan *paho.Publish, 16)
	server := NewServer("test", pubs, time.Hour, zerolog.Nop(),
		fakeProvider{entries: []models.Entry{entry("a", "binary_sensor", false, []byte("{}"))}},
		fakeProvider{err: errors.New("broken")},
		fakeProvider{entries: []models.Entry{entry("b", "sensor", false, nil)}},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	server.Discover <- true

	var got []string
	for range 5 {
		select {
		case msg := <-pubs:
			got = append(got, msg.Topic)
		case <-time.After(time.Second):
			t.Fatalf("expected 5 messages, got %v", got)
		}
	}
	assert.Equal(t, []string{
		"homeassistant/binary_sensor/a/config",
		"homeassistant/sensor/b/config",
		"state/a",
		"attrs/a",
		"state/b",
	}, got)
}
