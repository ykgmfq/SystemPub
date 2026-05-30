package sanoid

import (
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/ykgmfq/SystemPub/models"
)

type commandExecutor interface {
	Run() error
}

type zpoolExecutor interface {
	Output() ([]byte, error)
}

// JSON structs for `zpool status -j --json-int`

type scanStats struct {
	Function  string `json:"function"`
	State     string `json:"state"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
	Errors    int    `json:"errors"`
}

type vdev struct {
	Name           string           `json:"name"`
	VdevType       string           `json:"vdev_type"`
	State          string           `json:"state"`
	AllocSpace     int64            `json:"alloc_space"`
	TotalSpace     int64            `json:"total_space"`
	ReadErrors     int64            `json:"read_errors"`
	WriteErrors    int64            `json:"write_errors"`
	ChecksumErrors int64            `json:"checksum_errors"`
	SlowIOs        int64            `json:"slow_ios"`
	Path           string           `json:"path"`
	DevID          string           `json:"devid"`
	Vdevs          map[string]*vdev `json:"vdevs"`
}

type zpoolPool struct {
	Name       string           `json:"name"`
	State      string           `json:"state"`
	PoolGUID   uint64           `json:"pool_guid"` // uint64: value overflows int64
	SpaVersion int              `json:"spa_version"`
	ScanStats  scanStats        `json:"scan_stats"` // zero-value safe when absent
	ErrorCount int              `json:"error_count"`
	Vdevs      map[string]*vdev `json:"vdevs"`
}

type zpoolStatus struct {
	Pools map[string]*zpoolPool `json:"pools"`
}

type zpoolSensorEntry struct {
	config  models.MqttConfig
	domain  string
	payload func() []byte
	attrs   func() ([]byte, error) // nil if no attributes
}

// SanoidClient checks pool health, capacity and snapshots and publishes results to MQTT.
type SanoidClient struct {
	Discover  chan bool
	Interval  time.Duration
	Config    map[models.Property]models.MqttConfig
	Pubs      chan *paho.Publish
	zpoolExec func(string, ...string) zpoolExecutor
	shellExec func(string, ...string) commandExecutor
}
