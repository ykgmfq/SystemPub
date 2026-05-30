package zpool

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockZpoolCmd struct {
	data []byte
	err  error
}

func (m *mockZpoolCmd) Output() ([]byte, error) { return m.data, m.err }

func zpoolFixtureExec(t *testing.T, path string) func(string, ...string) zpoolExecutor {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return func(_ string, _ ...string) zpoolExecutor { return &mockZpoolCmd{data: data} }
}

func TestRunZpoolParsing(t *testing.T) {
	status, err := runZpool(zpoolFixtureExec(t, "zoolstatus.json"))
	require.NoError(t, err)
	pool := status.Pools["data"]
	require.NotNil(t, pool)
	assert.Equal(t, uint64(16291491892042445671), pool.PoolGUID)
	assert.Equal(t, "ONLINE", pool.State)
	assert.Equal(t, 0, pool.ErrorCount)
	assert.Equal(t, 0, pool.ScanStats.Errors)
}

func TestRunZpoolMultiPool(t *testing.T) {
	status, err := runZpool(zpoolFixtureExec(t, "zoolstatus2.json"))
	require.NoError(t, err)
	assert.Len(t, status.Pools, 2)
	assert.Equal(t, "DEGRADED", status.Pools["test2"].State)
	assert.Equal(t, "ONLINE", status.Pools["test"].State)
}

func TestCollectLeafVdevs(t *testing.T) {
	mirror := &vdev{
		Name:     "mirror-0",
		VdevType: "mirror",
		Vdevs: map[string]*vdev{
			"sda": {Name: "sda", VdevType: "disk"},
			"sdb": {Name: "sdb", VdevType: "disk"},
		},
	}
	root := &vdev{
		Name:     "data",
		VdevType: "root",
		Vdevs:    map[string]*vdev{"mirror-0": mirror},
	}
	leaves := collectLeafVdevs(root)
	assert.Len(t, leaves, 2)
	assert.ElementsMatch(t, []string{"sda", "sdb"}, []string{leaves[0].Name, leaves[1].Name})
}

func TestBuildPoolEntriesHealthy(t *testing.T) {
	status, err := runZpool(zpoolFixtureExec(t, "zoolstatus.json"))
	require.NoError(t, err)
	pool := status.Pools["data"]
	entries := buildPoolEntries(pool, 20*time.Minute)

	byUID := map[string]zpoolSensorEntry{}
	for _, e := range entries {
		byUID[e.config.UniqueID] = e
	}
	guid := pool.PoolGUID

	// Pool health binary_sensor
	health := byUID[zpoolSensorUID(guid, "health")]
	assert.Equal(t, "binary_sensor", health.domain)
	assert.Equal(t, "problem", health.config.DeviceClass)
	assert.NotEmpty(t, health.config.JsonAttributesTopic)
	assert.Equal(t, []byte("OFF"), health.payload())

	// Attributes contain scrub fields and ISO timestamps
	var attrMap map[string]any
	attrsData, err := health.attrs()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(attrsData, &attrMap))
	assert.Contains(t, attrMap, "scrub_state")
	assert.Contains(t, attrMap, "scrub_start")
	assert.Contains(t, attrMap, "scrub_end")

	// Capacity sensors
	alloc := byUID[zpoolSensorUID(guid, "alloc")]
	assert.Equal(t, "sensor", alloc.domain)
	assert.Equal(t, "data_size", alloc.config.DeviceClass)
	assert.Equal(t, "GiB", alloc.config.UnitOfMeasurement)
	assert.Equal(t, "measurement", alloc.config.StateClass)
	assert.Nil(t, alloc.attrs)

	// Pool error sensor
	errEntry := byUID[zpoolSensorUID(guid, "errors")]
	assert.Equal(t, "total_increasing", errEntry.config.StateClass)
	assert.Equal(t, []byte("0"), errEntry.payload())

	// Disk health with attributes
	diskHealth := byUID[zpoolSensorUID(guid, "sda_health")]
	assert.Equal(t, "binary_sensor", diskHealth.domain)
	var diskAttrMap map[string]any
	diskAttrsData, err := diskHealth.attrs()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(diskAttrsData, &diskAttrMap))
	assert.Contains(t, diskAttrMap, "path")
	assert.Contains(t, diskAttrMap, "devid")
	assert.Contains(t, diskAttrMap, "slow_ios")
}

func TestBuildPoolEntriesDegraded(t *testing.T) {
	status, err := runZpool(zpoolFixtureExec(t, "zoolstatus2.json"))
	require.NoError(t, err)
	pool := status.Pools["test2"]
	entries := buildPoolEntries(pool, 20*time.Minute)
	byUID := map[string]zpoolSensorEntry{}
	for _, e := range entries {
		byUID[e.config.UniqueID] = e
	}
	health := byUID[zpoolSensorUID(pool.PoolGUID, "health")]
	assert.Equal(t, []byte("ON"), health.payload())
}

func TestNoScrubTimesWhenZero(t *testing.T) {
	status, err := runZpool(zpoolFixtureExec(t, "zoolstatus2.json"))
	require.NoError(t, err)
	pool := status.Pools["test"] // no scan_stats
	entries := buildPoolEntries(pool, 20*time.Minute)
	byUID := map[string]zpoolSensorEntry{}
	for _, e := range entries {
		byUID[e.config.UniqueID] = e
	}
	health := byUID[zpoolSensorUID(pool.PoolGUID, "health")]
	var attrMap map[string]any
	attrsData, err := health.attrs()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(attrsData, &attrMap))
	assert.NotContains(t, attrMap, "scrub_start")
	assert.NotContains(t, attrMap, "scrub_end")
}

func TestZpoolDeviceIdentifierIsPureGUID(t *testing.T) {
	pool := &zpoolPool{Name: "data", PoolGUID: 16291491892042445671}
	dev := zpoolDevice(pool)
	assert.Equal(t, "16291491892042445671", dev.Identifiers[0])
}

func TestRunZpoolError(t *testing.T) {
	provider := NewZpoolProvider(20 * time.Minute)
	provider.execFn = func(_ string, _ ...string) zpoolExecutor {
		return &mockZpoolCmd{err: os.ErrNotExist}
	}
	_, err := provider.Entries()
	assert.ErrorIs(t, err, os.ErrNotExist)
}
