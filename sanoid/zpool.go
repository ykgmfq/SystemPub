package sanoid

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
)

const gib = float64(1 << 30)

func runZpool(exec func(string, ...string) zpoolExecutor) (*zpoolStatus, error) {
	out, err := exec("zpool", "status", "-j", "--json-int").Output()
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

// buildPoolEntries constructs all binary_sensor and sensor entries for one pool.
func buildPoolEntries(pool *zpoolPool, interval time.Duration) []zpoolSensorEntry {
	device := zpoolDevice(pool)
	guid := pool.PoolGUID
	var entries []zpoolSensorEntry

	// Pool health binary_sensor with scrub attributes
	healthUID := zpoolSensorUID(guid, "health")
	healthCfg := makeSensorConfig("Pool health", healthUID, "binary_sensor", "problem", "", "", device, interval)
	healthCfg.JsonAttributesTopic = zpoolAttrTopic("binary_sensor", healthUID)
	entries = append(entries, zpoolSensorEntry{
		config:  healthCfg,
		domain:  "binary_sensor",
		payload: func() []byte { return mqttclient.ProblemPayload(pool.State == "ONLINE") },
		attrs: func() ([]byte, error) {
			m := map[string]any{
				"scrub_state":    pool.ScanStats.State,
				"scrub_function": pool.ScanStats.Function,
			}
			if pool.ScanStats.StartTime != 0 {
				m["scrub_start"] = time.Unix(pool.ScanStats.StartTime, 0).UTC().Format(time.RFC3339)
				m["scrub_end"] = time.Unix(pool.ScanStats.EndTime, 0).UTC().Format(time.RFC3339)
			}
			return json.Marshal(m)
		},
	})

	// Capacity sensors (from root vdev)
	rootVdev := pool.Vdevs[pool.Name]
	if rootVdev != nil {
		allocVal := float64(rootVdev.AllocSpace) / gib
		allocUID := zpoolSensorUID(guid, "alloc")
		entries = append(entries, zpoolSensorEntry{
			config:  makeSensorConfig("Allocated space", allocUID, "sensor", "data_size", "measurement", "GiB", device, interval),
			domain:  "sensor",
			payload: func() []byte { return []byte(fmt.Sprintf("%.2f", allocVal)) },
		})

		totalVal := float64(rootVdev.TotalSpace) / gib
		totalUID := zpoolSensorUID(guid, "total")
		entries = append(entries, zpoolSensorEntry{
			config:  makeSensorConfig("Total space", totalUID, "sensor", "data_size", "measurement", "GiB", device, interval),
			domain:  "sensor",
			payload: func() []byte { return []byte(fmt.Sprintf("%.2f", totalVal)) },
		})
	}

	// Error sensors
	errVal := int64(pool.ErrorCount)
	errUID := zpoolSensorUID(guid, "errors")
	entries = append(entries, zpoolSensorEntry{
		config:  makeSensorConfig("Pool errors", errUID, "sensor", "", "total_increasing", "", device, interval),
		domain:  "sensor",
		payload: func() []byte { return []byte(strconv.FormatInt(errVal, 10)) },
	})

	scrubVal := int64(pool.ScanStats.Errors)
	scrubUID := zpoolSensorUID(guid, "scrub_errors")
	entries = append(entries, zpoolSensorEntry{
		config:  makeSensorConfig("Scrub errors", scrubUID, "sensor", "", "total_increasing", "", device, interval),
		domain:  "sensor",
		payload: func() []byte { return []byte(strconv.FormatInt(scrubVal, 10)) },
	})

	// Per-disk entries
	if rootVdev != nil {
		for _, leaf := range collectLeafVdevs(rootVdev) {
			leaf := leaf
			diskKey := mqttclient.NormalizeStr(leaf.Name)

			diskHealthUID := zpoolSensorUID(guid, diskKey+"_health")
			diskHealthCfg := makeSensorConfig(leaf.Name+" health", diskHealthUID, "binary_sensor", "problem", "", "", device, interval)
			diskHealthCfg.JsonAttributesTopic = zpoolAttrTopic("binary_sensor", diskHealthUID)
			entries = append(entries, zpoolSensorEntry{
				config:  diskHealthCfg,
				domain:  "binary_sensor",
				payload: func() []byte { return mqttclient.ProblemPayload(leaf.State == "ONLINE") },
				attrs: func() ([]byte, error) {
					m := map[string]any{"slow_ios": leaf.SlowIOs}
					if leaf.Path != "" {
						m["path"] = leaf.Path
					}
					if leaf.DevID != "" {
						m["devid"] = leaf.DevID
					}
					return json.Marshal(m)
				},
			})

			for _, s := range []struct {
				suffix string
				name   string
				val    int64
			}{
				{diskKey + "_read_errors", leaf.Name + " read errors", leaf.ReadErrors},
				{diskKey + "_write_errors", leaf.Name + " write errors", leaf.WriteErrors},
				{diskKey + "_checksum_errors", leaf.Name + " checksum errors", leaf.ChecksumErrors},
			} {
				s := s
				uid := zpoolSensorUID(guid, s.suffix)
				entries = append(entries, zpoolSensorEntry{
					config:  makeSensorConfig(s.name, uid, "sensor", "", "total_increasing", "", device, interval),
					domain:  "sensor",
					payload: func() []byte { return []byte(strconv.FormatInt(s.val, 10)) },
				})
			}
		}
	}

	return entries
}

func publishZpoolDiscovery(pubs chan *paho.Publish, entry zpoolSensorEntry) error {
	var (
		msg *paho.Publish
		err error
	)
	if entry.domain == "sensor" {
		msg, err = mqttclient.GetSensorDiscovery(entry.config)
	} else {
		msg, err = mqttclient.GetDiscovery(entry.config)
	}
	if err != nil {
		return err
	}
	pubs <- msg
	return nil
}

func publishZpoolState(pubs chan *paho.Publish, entry zpoolSensorEntry) error {
	pubs <- &paho.Publish{Topic: entry.config.StateTopic, Payload: entry.payload(), Retain: true}
	if entry.attrs != nil {
		b, err := entry.attrs()
		if err != nil {
			return err
		}
		pubs <- &paho.Publish{Topic: entry.config.JsonAttributesTopic, Payload: b, Retain: true}
	}
	return nil
}

// updateZpool runs zpool status, then publishes discovery (if requested) and state for all pools.
func (client SanoidClient) updateZpool(withDiscovery bool) error {
	status, err := runZpool(client.zpoolExec)
	if err != nil {
		return err
	}
	for _, pool := range status.Pools {
		for _, e := range buildPoolEntries(pool, client.Interval) {
			if withDiscovery {
				if err := publishZpoolDiscovery(client.Pubs, e); err != nil {
					return err
				}
			}
			if err := publishZpoolState(client.Pubs, e); err != nil {
				return err
			}
		}
	}
	return nil
}
