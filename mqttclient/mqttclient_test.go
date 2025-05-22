package mqttclient

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/ykgmfq/SystemPub/models"
)

func TestNewMqttclient(t *testing.T) {
	server := models.MQTT{Host: "localhost", Port: 1883}
	device := models.Device{Name: "TestDevice", Identifiers: [1]string{"1234"}}

	client := NewMqttclient(server, device)

	assert.Equal(t, server, client.Server)
	assert.Equal(t, device, client.Device)
	assert.NotNil(t, client.Pubs)
}

func TestGetDiscovery(t *testing.T) {
	config := models.MqttConfig{
		UniqueID: "test_sensor",
		Name:     "Test Sensor",
	}

	discoveryMsg := GetDiscovery(config)

	assert.Equal(t, "homeassistant/binary_sensor/test_sensor/config", discoveryMsg.Topic)
	assert.Equal(t, byte(1), discoveryMsg.QoS)

	var payload map[string]any
	err := json.Unmarshal(discoveryMsg.Payload, &payload)
	assert.NoError(t, err)
	assert.Equal(t, "Test Sensor", payload["name"])
}

func TestProblemPayload(t *testing.T) {
	assert.Equal(t, []byte("OFF"), ProblemPayload(true))
	assert.Equal(t, []byte("ON"), ProblemPayload(false))
}
