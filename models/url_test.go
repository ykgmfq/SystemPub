package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseServerURLAccepted(t *testing.T) {
	for _, raw := range []string{"mqtt://localhost:1883", "mqtts://broker.example.com:8883", "ws://192.168.0.3", "wss://broker.example.com/mqtt"} {
		parsed, err := ParseServerURL(raw)
		assert.NoError(t, err, raw)
		assert.Equal(t, raw, parsed.String())
	}
}

// Typical typos must be rejected, even though url.Parse accepts most of them.
func TestParseServerURLRejected(t *testing.T) {
	for _, raw := range []string{"localhost", "192.168.0.3", "192.168.0.3:1883", "mqtt://", "://localhost", "mqtt://bad host", "ftp://localhost", "http://localhost:1883", "tcp://localhost:1883"} {
		_, err := ParseServerURL(raw)
		assert.Error(t, err, raw)
	}
}
