package systemd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ykgmfq/SystemPub/models"
)

func TestUnitEntryHealthy(t *testing.T) {
	config := getUnitConfig(models.Device{Name: "Test Host"}, time.Minute)
	e, err := unitEntry(config, []string{})
	require.NoError(t, err)
	assert.Equal(t, []byte("OFF"), e.Payload)
	assert.False(t, e.Problem)
	assert.JSONEq(t, `{"failed_units":[]}`, string(e.Attributes))
	assert.Equal(t, "homeassistant/binary_sensor/test-host_units/state", e.Config.StateTopic)
}

func TestUnitEntryFailedUnits(t *testing.T) {
	config := getUnitConfig(models.Device{Name: "host"}, time.Minute)
	e, err := unitEntry(config, []string{"a.service", "b.service"})
	require.NoError(t, err)
	assert.Equal(t, []byte("ON"), e.Payload)
	assert.True(t, e.Problem)
	assert.JSONEq(t, `{"failed_units":["a.service","b.service"]}`, string(e.Attributes))
}
