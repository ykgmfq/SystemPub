package mqttclient

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ykgmfq/SystemPub/models"
)

func TestNewMqttclient(t *testing.T) {
	server := models.SystemPubConfigDefault().MQTTServer
	device := models.Device{Name: "TestDevice", Identifiers: [1]string{"1234"}}

	client := NewMqttclient(server, device, zerolog.Nop())

	assert.Equal(t, server, client.Server)
	assert.Equal(t, device, client.Device)
	assert.NotNil(t, client.Pubs)
}

// Messages from the library's error stream must be logged at error level, so they are visible at the default log level.
func TestLevelLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := levelLogger{logger: zerolog.New(&buf).Level(zerolog.InfoLevel), level: zerolog.ErrorLevel}
	logger.Println("connect failed:", 42)
	assert.JSONEq(t, `{"level":"error","message":"connect failed: 42"}`, buf.String())

	buf.Reset()
	hidden := levelLogger{logger: zerolog.New(&buf).Level(zerolog.InfoLevel), level: zerolog.DebugLevel}
	hidden.Printf("queued %d packets", 3)
	assert.Empty(t, buf.String())
}

func TestGetDiscovery(t *testing.T) {
	config := models.MqttConfig{
		UniqueID: "test_sensor",
		Name:     "Test Sensor",
	}

	discoveryMsg, err := GetDiscovery(models.BinarySensor, config)
	require.NoError(t, err)

	assert.Equal(t, "homeassistant/binary_sensor/test_sensor/config", discoveryMsg.Topic)
	assert.Equal(t, byte(1), discoveryMsg.QoS)

	var payload map[string]any
	err = json.Unmarshal(discoveryMsg.Payload, &payload)
	assert.NoError(t, err)
	assert.Equal(t, "Test Sensor", payload["name"])

	sensorMsg, err := GetDiscovery(models.Sensor, config)
	require.NoError(t, err)
	assert.Equal(t, "homeassistant/sensor/test_sensor/config", sensorMsg.Topic)
}

func TestSensorTopics(t *testing.T) {
	assert.Equal(t, "homeassistant/binary_sensor/x/state", StateTopic(models.BinarySensor, "x"))
	assert.Equal(t, "homeassistant/sensor/x/attributes", AttributesTopic(models.Sensor, "x"))
}

func TestProblemPayload(t *testing.T) {
	assert.Equal(t, []byte("OFF"), ProblemPayload(true))
	assert.Equal(t, []byte("ON"), ProblemPayload(false))
}

func TestNormalizeStr(t *testing.T) {
	strPairs := []struct {
		in, want string
	}{
		{"SimpleName", "simplename"},
		{"Mixed_CASE-Name", "mixed_case-name"},
		{"Name with spaces", "name-with-spaces"},
		{"Special!@#Chars", "special---chars"},
		{"Domain.Name", "domain-name"},
		{"Ünicode", "-nicode"},
		{"123Numbers", "123numbers"},
		{"mixed spaces &/slashes", "mixed-spaces---slashes"},
		{"", ""},
		{"___underscores___", "___underscores___"},
		{"CAPS_AND spaces!", "caps_and-spaces-"},
	}
	for _, strPair := range strPairs {
		test := func(t *testing.T) {
			got := NormalizeStr(strPair.in)
			assert.Equal(t, strPair.want, got)
		}
		t.Run(strPair.in, test)
	}
}
