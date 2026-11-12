package provider

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ykgmfq/SystemPub/models"
)

// drillNow is the fixed time at which the drill tests evaluate a drill.
var drillNow = time.Date(2026, 11, 14, 11, 0, 0, 0, time.UTC)

// problemSensor returns a problem binary sensor whose payload matches its problem state.
func problemSensor(uid string, problem bool, attrs []byte) models.Entry {
	e := entry(uid, models.BinarySensor, problem, attrs)
	e.Config.DeviceClass = "problem"
	e.Payload = []byte("OFF")
	if problem {
		e.Payload = []byte("ON")
	}
	return e
}

// counterSensor returns an error counter with the given count.
func counterSensor(uid string, count string) models.Entry {
	e := entry(uid, models.Sensor, false, nil)
	e.Config.StateClass = "total_increasing"
	e.Payload = []byte(count)
	return e
}

// sizeSensor returns a measurement sensor that a drill must not target.
func sizeSensor(uid string) models.Entry {
	e := entry(uid, models.Sensor, false, nil)
	e.Config.StateClass = "measurement"
	e.Payload = []byte("1.00")
	return e
}

// drillOn returns a drill on the given sensor that is still running at drillNow.
func drillOn(uid string) *models.Drill {
	return &models.Drill{UniqueID: uid, Until: drillNow.Add(5 * time.Minute), ErrorCount: 42}
}

// byUID indexes the entries by their unique ID.
func byUID(entries []models.Entry) map[string]models.Entry {
	m := map[string]models.Entry{}
	for _, e := range entries {
		m[e.Config.UniqueID] = e
	}
	return m
}

// attrMap decodes the attributes of the entry, or returns nil if it has none.
func attrMap(t *testing.T, e models.Entry) map[string]any {
	t.Helper()
	if e.Attributes == nil {
		return nil
	}
	var m map[string]any
	require.NoError(t, json.Unmarshal(e.Attributes, &m))
	return m
}

// TestIsDrillTarget checks that only problem sensors and error counters can be targeted.
func TestIsDrillTarget(t *testing.T) {
	assert.True(t, IsDrillTarget(problemSensor("a", false, nil)))
	assert.True(t, IsDrillTarget(counterSensor("b", "0")))
	assert.False(t, IsDrillTarget(sizeSensor("c")))
	assert.False(t, IsDrillTarget(entry("d", models.BinarySensor, false, nil)))
}

// TestDegraded checks that a problem or any counted error rules out drills.
func TestDegraded(t *testing.T) {
	assert.False(t, Degraded([]models.Entry{problemSensor("units", false, nil), counterSensor("errors", "0"), sizeSensor("size")}))
	assert.True(t, Degraded([]models.Entry{problemSensor("units", true, nil)}))
	assert.True(t, Degraded([]models.Entry{counterSensor("errors", "1")}))
	assert.True(t, Degraded([]models.Entry{counterSensor("errors", "x")}))
}

// TestApplyDrill checks that only the running drill's target reports a problem, while every target carries the drill flag.
func TestApplyDrill(t *testing.T) {
	real := []models.Entry{
		problemSensor("units", false, []byte(`{"failed_units":[]}`)),
		counterSensor("errors", "0"),
		sizeSensor("size"),
	}
	cases := []struct {
		name    string
		drill   *models.Drill
		target  string
		payload string
	}{
		{name: "problem sensor", drill: drillOn("units"), target: "units", payload: "ON"},
		{name: "error counter", drill: drillOn("errors"), target: "errors", payload: "42"},
		{name: "unknown target", drill: drillOn("other")},
		{name: "expired drill", drill: &models.Drill{UniqueID: "units", Until: drillNow, ErrorCount: 42}},
		{name: "no drill", drill: nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := applyDrill(real, runningDrill(c.drill, real, drillNow))
			require.NoError(t, err)
			got := byUID(out)
			for _, orig := range real {
				e := got[orig.Config.UniqueID]
				if !IsDrillTarget(orig) {
					assert.Equal(t, orig, e)
					continue
				}
				active := orig.Config.UniqueID == c.target
				assert.Equal(t, active, attrMap(t, e)[drillAttribute])
				assert.Equal(t, active, e.Problem)
				if active {
					assert.Equal(t, c.payload, string(e.Payload))
				} else {
					assert.Equal(t, orig.Payload, e.Payload)
				}
			}
			assert.Contains(t, attrMap(t, got["units"]), "failed_units")
		})
	}
}

// TestApplyDrillWhileDegraded checks that a drill does nothing and ends once the system is degraded.
func TestApplyDrillWhileDegraded(t *testing.T) {
	cases := map[string][]models.Entry{
		"problem":       {problemSensor("units", true, nil), counterSensor("errors", "0")},
		"counted error": {problemSensor("units", false, nil), counterSensor("errors", "3")},
	}
	for name, real := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := applyDrill(real, runningDrill(drillOn("units"), real, drillNow))
			require.NoError(t, err)
			for i, e := range out {
				assert.Equal(t, real[i].Payload, e.Payload)
				assert.Equal(t, false, attrMap(t, e)[drillAttribute])
			}
			assert.Nil(t, runningDrill(drillOn("units"), real, drillNow))
		})
	}
}

// TestServeDrill checks that a drill sent to a running server is published right away.
func TestServeDrill(t *testing.T) {
	pubs := make(chan *paho.Publish, 16)
	server := NewServer("test", pubs, time.Hour, zerolog.Nop(),
		fakeProvider{entries: []models.Entry{problemSensor("units", false, nil)}},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	server.Drills <- models.Drill{UniqueID: "units", Until: time.Now().Add(time.Minute), ErrorCount: 42}

	got := map[string]string{}
	for range 2 {
		select {
		case msg := <-pubs:
			got[msg.Topic] = string(msg.Payload)
		case <-time.After(time.Second):
			t.Fatalf("expected 2 messages, got %v", got)
		}
	}
	assert.Equal(t, "ON", got["state/units"])
	assert.JSONEq(t, `{"fire_drill":true}`, got["attrs/units"])
}

// readMessages returns the topics and payloads of up to n messages, stopping early once none arrives for a short while.
func readMessages(pubs chan *paho.Publish, n int) map[string]string {
	got := map[string]string{}
	for range n {
		select {
		case msg := <-pubs:
			got[msg.Topic] = string(msg.Payload)
		case <-time.After(200 * time.Millisecond):
			return got
		}
	}
	return got
}

// TestServeEndsDrillOnShutdown checks that shutdown republishes the real state of a drilled sensor, because the MQTT server retains the fake one.
func TestServeEndsDrillOnShutdown(t *testing.T) {
	pubs := make(chan *paho.Publish, 16)
	server := NewServer("test", pubs, time.Hour, zerolog.Nop(),
		fakeProvider{entries: []models.Entry{problemSensor("units", false, nil)}},
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		server.Serve(ctx)
		close(done)
	}()
	server.Drills <- models.Drill{UniqueID: "units", Until: time.Now().Add(time.Minute), ErrorCount: 42}
	assert.Equal(t, "ON", readMessages(pubs, 2)["state/units"])

	cancel()
	<-done
	got := readMessages(pubs, 2)
	assert.Equal(t, "OFF", got["state/units"])
	assert.JSONEq(t, `{"fire_drill":false}`, got["attrs/units"])
}

// TestServeShutdownWithoutDrill checks that shutdown publishes nothing when the server runs no drill, including when the drill targets another server's sensor.
func TestServeShutdownWithoutDrill(t *testing.T) {
	cases := map[string]func(Server){
		"no drill":              func(s Server) { s.Discover <- true },
		"other server's sensor": func(s Server) { s.Drills <- *drillOn("other") },
	}
	for name, start := range cases {
		t.Run(name, func(t *testing.T) {
			pubs := make(chan *paho.Publish, 16)
			server := NewServer("test", pubs, time.Hour, zerolog.Nop(),
				fakeProvider{entries: []models.Entry{problemSensor("units", false, nil)}},
			)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				server.Serve(ctx)
				close(done)
			}()
			start(server)
			assert.NotEmpty(t, readMessages(pubs, 3))

			cancel()
			<-done
			assert.Empty(t, readMessages(pubs, 1))
		})
	}
}

// TestRunningDrillIgnoresOtherServersSensor checks that a server drops a drill whose target it does not own.
func TestRunningDrillIgnoresOtherServersSensor(t *testing.T) {
	real := []models.Entry{problemSensor("units", false, nil)}
	assert.Nil(t, runningDrill(drillOn("other"), real, drillNow))
	assert.NotNil(t, runningDrill(drillOn("units"), real, drillNow))
}
