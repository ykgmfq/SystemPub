package provider

import (
	"encoding/json"
	"slices"
	"strconv"
	"time"

	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
)

// isProblemSensor reports whether the sensor is a binary sensor that signals a problem.
func isProblemSensor(e models.Entry) bool {
	return e.Domain == models.BinarySensor && e.Config.DeviceClass == "problem"
}

// isErrorCounter reports whether the sensor counts errors.
func isErrorCounter(e models.Entry) bool {
	return e.Domain == models.Sensor && e.Config.StateClass == "total_increasing"
}

// IsDrillTarget reports whether a fire drill can fake a problem on the given sensor.
// Only problem sensors and error counters qualify, because alerts are built on those.
func IsDrillTarget(e models.Entry) bool {
	return isProblemSensor(e) || isErrorCounter(e)
}

// Degraded reports whether the system has an actual problem, which rules out fire drills.
// Any error ever counted makes the system degraded, even though counters alone do not mark a problem.
// A counter that does not read exactly zero counts as an error, so that a malformed payload errs on the side of no drill.
func Degraded(entries []models.Entry) bool {
	for _, e := range entries {
		if e.Problem || (isErrorCounter(e) && string(e.Payload) != "0") {
			return true
		}
	}
	return false
}

// runningDrill returns the drill if it still fakes a problem on one of the given sensors, and nil otherwise.
// Every server receives every drill, so a server that does not own the target drops it here.
// A drill stops early once the system is degraded, so that it can neither add to nor hide a real alert.
func runningDrill(drill *models.Drill, real []models.Entry, now time.Time) *models.Drill {
	if drill == nil || !now.Before(drill.Until) || Degraded(real) || !owns(real, drill.UniqueID) {
		return nil
	}
	return drill
}

// owns reports whether the sensors include the one with the given unique ID.
func owns(real []models.Entry, uniqueID string) bool {
	return slices.ContainsFunc(real, func(e models.Entry) bool { return e.Config.UniqueID == uniqueID })
}

// fakeProblem returns the entry reporting a problem.
// Drills only run while every counter is zero, so a counter simply jumps to the drill's error count.
func fakeProblem(e models.Entry, errorCount int64) models.Entry {
	if e.Domain == models.BinarySensor {
		e.Payload = mqttclient.ProblemPayload(false)
	} else {
		e.Payload = []byte(strconv.FormatInt(errorCount, 10))
	}
	e.Problem = true
	return e
}

// flagDrill returns the entry with the fire drill flag in its attributes, if it publishes any.
// Values are kept as raw JSON, so that large numbers survive unchanged.
func flagDrill(e models.Entry, active bool) (models.Entry, error) {
	if e.Config.JsonAttributesTopic == "" {
		return e, nil
	}
	m := map[string]json.RawMessage{}
	if e.Attributes != nil {
		if err := json.Unmarshal(e.Attributes, &m); err != nil {
			return models.Entry{}, err
		}
	}
	flag, err := json.Marshal(active)
	if err != nil {
		return models.Entry{}, err
	}
	m[drillAttribute] = flag
	attrs, err := json.Marshal(m)
	if err != nil {
		return models.Entry{}, err
	}
	e.Attributes = attrs
	return e, nil
}

// applyDrill returns the entries as they are published, with the target of a running drill reporting a problem.
// A nil drill means that none is running.
// Every possible target carries the fire drill flag, so that alert automations can tell a drill from a real problem.
func applyDrill(real []models.Entry, drill *models.Drill) ([]models.Entry, error) {
	out := make([]models.Entry, 0, len(real))
	for _, e := range real {
		if !IsDrillTarget(e) {
			out = append(out, e)
			continue
		}
		active := drill != nil && e.Config.UniqueID == drill.UniqueID
		if active {
			e = fakeProblem(e, drill.ErrorCount)
		}
		flagged, err := flagDrill(e, active)
		if err != nil {
			return nil, err
		}
		out = append(out, flagged)
	}
	return out, nil
}
