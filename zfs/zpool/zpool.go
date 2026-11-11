// Package zpool provides a ZFS provider that reads pool status via `zpool status -j`.
package zpool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	osexec "os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
)

const gib = float64(1 << 30)

func runZpool(ctx context.Context, exec func(context.Context, string, ...string) zpoolExecutor) (*zpoolStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec(ctx, "zpool", "status", "-j", "--json-int").Output()
	if exitErr, ok := errors.AsType[*osexec.ExitError](err); ok && len(exitErr.Stderr) > 0 {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
	}
	if err != nil {
		return nil, err
	}
	var status zpoolStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// collectLeafVdevs recursively returns vdevs with no children (disks, files, etc.).
func collectLeafVdevs(v *vdev) []*vdev {
	if len(v.Vdevs) == 0 {
		return []*vdev{v}
	}
	var leaves []*vdev
	for _, child := range v.Vdevs {
		leaves = append(leaves, collectLeafVdevs(child)...)
	}
	return leaves
}

func zpoolSensorUID(poolGUID uint64, suffix string) string {
	return fmt.Sprintf("zpool_%d_%s", poolGUID, suffix)
}

func zpoolAttrTopic(domain, uid string) string {
	return "homeassistant/" + domain + "/" + uid + "/attributes"
}

func zpoolStateTopic(domain, uid string) string {
	return "homeassistant/" + domain + "/" + uid + "/state"
}

func zpoolDevice(pool *zpoolPool) models.Device {
	return models.Device{
		Name:         pool.Name,
		Model:        "ZFS Pool",
		Manufacturer: "OpenZFS",
		SWversion:    fmt.Sprintf("%d", pool.SpaVersion),
		Identifiers:  [1]string{fmt.Sprintf("%d", pool.PoolGUID)},
	}
}

func makeSensorConfig(name, uid, domain, deviceClass, stateClass, unit string, device models.Device, interval time.Duration) models.MqttConfig {
	cfg := models.MqttConfig{
		Name:        name,
		StateTopic:  zpoolStateTopic(domain, uid),
		UniqueID:    uid,
		Device:      device,
		ExpireAfter: int((interval * 2).Seconds()),
		ForceUpdate: true,
	}
	if deviceClass != "" {
		cfg.DeviceClass = deviceClass
	}
	if stateClass != "" {
		cfg.StateClass = stateClass
	}
	if unit != "" {
		cfg.UnitOfMeasurement = unit
	}
	return cfg
}

// Returns the scrub details shown as attributes of the pool health sensor.
func scrubAttributes(scan scanStats) map[string]any {
	m := map[string]any{
		"scrub_state":    scan.State,
		"scrub_function": scan.Function,
	}
	if scan.StartTime != 0 {
		m["scrub_start"] = time.Unix(scan.StartTime, 0).UTC().Format(time.RFC3339)
		m["scrub_end"] = time.Unix(scan.EndTime, 0).UTC().Format(time.RFC3339)
	}
	return m
}

// Returns the disk details shown as attributes of the disk health sensor.
func diskAttributes(leaf *vdev) map[string]any {
	m := map[string]any{"slow_ios": leaf.SlowIOs}
	if leaf.Path != "" {
		m["path"] = leaf.Path
	}
	if leaf.DevID != "" {
		m["devid"] = leaf.DevID
	}
	return m
}

// Returns a health binary sensor entry, which reports a problem unless the state is ONLINE.
func healthEntry(config models.MqttConfig, state string, attrs map[string]any) (models.Entry, error) {
	raw, err := json.Marshal(attrs)
	if err != nil {
		return models.Entry{}, err
	}
	ok := state == "ONLINE"
	return models.Entry{Config: config, Domain: "binary_sensor", Payload: mqttclient.ProblemPayload(ok), Attributes: raw, Problem: !ok}, nil
}

// Returns a sensor entry with the given payload.
func sensorEntry(config models.MqttConfig, payload []byte) models.Entry {
	return models.Entry{Config: config, Domain: "sensor", Payload: payload}
}

func gibPayload(bytes int64) []byte {
	return fmt.Appendf(nil, "%.2f", float64(bytes)/gib)
}

func countPayload(n int64) []byte {
	return []byte(strconv.FormatInt(n, 10))
}

// buildPoolEntries constructs all binary_sensor and sensor entries for one pool.
func buildPoolEntries(pool *zpoolPool, interval time.Duration) ([]models.Entry, error) {
	device := zpoolDevice(pool)
	guid := pool.PoolGUID
	var entries []models.Entry

	healthUID := zpoolSensorUID(guid, "health")
	healthCfg := makeSensorConfig("Pool health", healthUID, "binary_sensor", "problem", "", "", device, interval)
	healthCfg.JsonAttributesTopic = zpoolAttrTopic("binary_sensor", healthUID)
	health, err := healthEntry(healthCfg, pool.State, scrubAttributes(pool.ScanStats))
	if err != nil {
		return nil, err
	}
	entries = append(entries, health)

	// Capacity comes from the root vdev.
	rootVdev := pool.Vdevs[pool.Name]
	if rootVdev != nil {
		entries = append(entries,
			sensorEntry(makeSensorConfig("Allocated space", zpoolSensorUID(guid, "alloc"), "sensor", "data_size", "measurement", "GiB", device, interval), gibPayload(rootVdev.AllocSpace)),
			sensorEntry(makeSensorConfig("Total space", zpoolSensorUID(guid, "total"), "sensor", "data_size", "measurement", "GiB", device, interval), gibPayload(rootVdev.TotalSpace)),
			sensorEntry(makeSensorConfig("Free space", zpoolSensorUID(guid, "free"), "sensor", "data_size", "measurement", "GiB", device, interval), gibPayload(rootVdev.TotalSpace-rootVdev.AllocSpace)),
		)
	}

	entries = append(entries,
		sensorEntry(makeSensorConfig("Pool errors", zpoolSensorUID(guid, "errors"), "sensor", "", "total_increasing", "", device, interval), countPayload(int64(pool.ErrorCount))),
		sensorEntry(makeSensorConfig("Scrub errors", zpoolSensorUID(guid, "scrub_errors"), "sensor", "", "total_increasing", "", device, interval), countPayload(int64(pool.ScanStats.Errors))),
	)

	if rootVdev == nil {
		return entries, nil
	}
	for _, leaf := range collectLeafVdevs(rootVdev) {
		diskKey := mqttclient.NormalizeStr(leaf.Name)

		diskHealthUID := zpoolSensorUID(guid, diskKey+"_health")
		diskHealthCfg := makeSensorConfig(leaf.Name+" health", diskHealthUID, "binary_sensor", "problem", "", "", device, interval)
		diskHealthCfg.JsonAttributesTopic = zpoolAttrTopic("binary_sensor", diskHealthUID)
		diskHealth, err := healthEntry(diskHealthCfg, leaf.State, diskAttributes(leaf))
		if err != nil {
			return nil, err
		}
		entries = append(entries, diskHealth)

		for _, s := range []struct {
			suffix string
			name   string
			val    int64
		}{
			{diskKey + "_read_errors", leaf.Name + " read errors", leaf.ReadErrors},
			{diskKey + "_write_errors", leaf.Name + " write errors", leaf.WriteErrors},
			{diskKey + "_checksum_errors", leaf.Name + " checksum errors", leaf.ChecksumErrors},
		} {
			uid := zpoolSensorUID(guid, s.suffix)
			entries = append(entries, sensorEntry(makeSensorConfig(s.name, uid, "sensor", "", "total_increasing", "", device, interval), countPayload(s.val)))
		}
	}
	return entries, nil
}

// NewZpoolProvider returns a provider that reads pool status via `zpool status -j`.
// It runs zpool once and fails when that does not work, for example because ZFS is missing or too old to report JSON, so that the caller can carry on without it.
func NewZpoolProvider(ctx context.Context, interval time.Duration) (*ZpoolProvider, error) {
	p := newZpoolProvider(interval)
	if _, err := runZpool(ctx, p.execFn); err != nil {
		return nil, err
	}
	return p, nil
}

func newZpoolProvider(interval time.Duration) *ZpoolProvider {
	return &ZpoolProvider{
		interval: interval,
		execFn: func(ctx context.Context, name string, arg ...string) zpoolExecutor {
			return osexec.CommandContext(ctx, name, arg...)
		},
	}
}

// Entries runs zpool status and returns sensor entries for all pools and disks.
func (p *ZpoolProvider) Entries(ctx context.Context) ([]models.Entry, error) {
	status, err := runZpool(ctx, p.execFn)
	if err != nil {
		return nil, err
	}
	var entries []models.Entry
	for _, pool := range status.Pools {
		pe, err := buildPoolEntries(pool, p.interval)
		if err != nil {
			return nil, err
		}
		entries = append(entries, pe...)
	}
	return entries, nil
}
