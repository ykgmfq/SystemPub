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

// runZpool runs `zpool status` and parses its output.
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

// collectLeafVdevs recursively returns vdevs with no children, such as disks and files.
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

// zpoolSensorUID returns the unique ID of a pool sensor.
func zpoolSensorUID(poolGUID uint64, suffix string) string {
	return fmt.Sprintf("zpool_%d_%s", poolGUID, suffix)
}

// zpoolDevice returns the Home Assistant device that represents the pool.
func zpoolDevice(pool *zpoolPool) models.Device {
	return models.Device{
		Name:         pool.Name,
		Model:        "ZFS Pool",
		Manufacturer: "OpenZFS",
		SWversion:    fmt.Sprintf("%d", pool.SpaVersion),
		Identifiers:  [1]string{fmt.Sprintf("%d", pool.PoolGUID)},
	}
}

// config returns the config fields shared by every sensor of the pool.
func (s poolSensors) config(name, suffix string, domain models.Domain) models.MqttConfig {
	uid := zpoolSensorUID(s.guid, suffix)
	return models.MqttConfig{
		Name:        name,
		StateTopic:  mqttclient.StateTopic(domain, uid),
		UniqueID:    uid,
		Device:      s.device,
		ExpireAfter: int((s.interval * 2).Seconds()),
		ForceUpdate: true,
	}
}

// health returns a health binary sensor, which reports a problem unless the state is ONLINE.
func (s poolSensors) health(name, suffix, state string, attrs map[string]any) (models.Entry, error) {
	raw, err := json.Marshal(attrs)
	if err != nil {
		return models.Entry{}, err
	}
	cfg := s.config(name, suffix, models.BinarySensor)
	cfg.DeviceClass = "problem"
	cfg.JsonAttributesTopic = mqttclient.AttributesTopic(models.BinarySensor, cfg.UniqueID)
	ok := state == "ONLINE"
	return models.Entry{Config: cfg, Domain: models.BinarySensor, Payload: mqttclient.ProblemPayload(ok), Attributes: raw, Problem: !ok}, nil
}

// size returns a sensor that reports a size in GiB.
func (s poolSensors) size(name, suffix string, bytes int64) models.Entry {
	cfg := s.config(name, suffix, models.Sensor)
	cfg.DeviceClass = "data_size"
	cfg.StateClass = "measurement"
	cfg.UnitOfMeasurement = "GiB"
	return models.Entry{Config: cfg, Domain: models.Sensor, Payload: fmt.Appendf(nil, "%.2f", float64(bytes)/gib)}
}

// counter returns a sensor that reports an ever-growing count, such as errors.
func (s poolSensors) counter(name, suffix string, n int64) models.Entry {
	cfg := s.config(name, suffix, models.Sensor)
	cfg.StateClass = "total_increasing"
	// Counters have no attributes of their own, but carry the fire drill flag.
	cfg.JsonAttributesTopic = mqttclient.AttributesTopic(models.Sensor, cfg.UniqueID)
	return models.Entry{Config: cfg, Domain: models.Sensor, Payload: []byte(strconv.FormatInt(n, 10))}
}

// scrubAttributes returns the scrub details shown as attributes of the pool health sensor.
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

// diskAttributes returns the disk details shown as attributes of the disk health sensor.
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

// diskEntries returns the health sensor and error counters of one disk.
func diskEntries(s poolSensors, leaf *vdev) ([]models.Entry, error) {
	key := mqttclient.NormalizeStr(leaf.Name)
	health, err := s.health(leaf.Name+" health", key+"_health", leaf.State, diskAttributes(leaf))
	if err != nil {
		return nil, err
	}
	return []models.Entry{
		health,
		s.counter(leaf.Name+" read errors", key+"_read_errors", leaf.ReadErrors),
		s.counter(leaf.Name+" write errors", key+"_write_errors", leaf.WriteErrors),
		s.counter(leaf.Name+" checksum errors", key+"_checksum_errors", leaf.ChecksumErrors),
	}, nil
}

// buildPoolEntries returns the sensors of one pool, followed by the sensors of each of its disks.
func buildPoolEntries(pool *zpoolPool, interval time.Duration) ([]models.Entry, error) {
	s := poolSensors{guid: pool.PoolGUID, device: zpoolDevice(pool), interval: interval}
	health, err := s.health("Pool health", "health", pool.State, scrubAttributes(pool.ScanStats))
	if err != nil {
		return nil, err
	}
	entries := []models.Entry{
		health,
		s.counter("Pool errors", "errors", int64(pool.ErrorCount)),
		s.counter("Scrub errors", "scrub_errors", int64(pool.ScanStats.Errors)),
	}

	// Capacity and disks are both read from the root vdev.
	root := pool.Vdevs[pool.Name]
	if root == nil {
		return entries, nil
	}
	entries = append(entries,
		s.size("Allocated space", "alloc", root.AllocSpace),
		s.size("Total space", "total", root.TotalSpace),
		s.size("Free space", "free", root.TotalSpace-root.AllocSpace),
	)
	for _, leaf := range collectLeafVdevs(root) {
		disk, err := diskEntries(s, leaf)
		if err != nil {
			return nil, err
		}
		entries = append(entries, disk...)
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

// newZpoolProvider returns a provider that runs the real zpool command.
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
