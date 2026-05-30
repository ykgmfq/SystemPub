// Package sanoid provides a ZFS provider that checks pool health via the sanoid CLI.
package sanoid

import (
	"errors"
	"os/exec"
	"time"

	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
)

var Logger zerolog.Logger

// Maps Sanoid exit codes to human-readable states.
func sanoidState(exit int) string {
	if exit > 4 || exit < 0 {
		return "UNKNOWN"
	}
	var sanoidExitLevels = map[int]string{
		0: "Ok",
		1: "Warning",
		2: "Critical",
		3: "Error",
	}
	return sanoidExitLevels[exit]
}

// Runs Sanoid to check one of pool health, capacity and snapshots.
// Returns (ok, state, err): state is non-empty when exit code 1-4 (pool problem, sanoid healthy).
func getPoolState(run func(string, ...string) commandExecutor, p models.Property) (bool, string, error) {
	cmd := run("sanoid", "--monitor-"+models.PropStr[p])
	err := cmd.Run()
	if err == nil {
		return true, "", nil
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return false, "", err
	}
	exitCode := exitError.ExitCode()
	if exitCode > 4 {
		return false, "", err
	}
	return false, sanoidState(exitCode), nil
}

// GetPoolConfigs gathers autodiscovery configs for the health, capacity and snapshot binary sensors.
func GetPoolConfigs(device models.Device, interval time.Duration) map[models.Property]models.MqttConfig {
	configs := make(map[models.Property]models.MqttConfig, len(models.PropStr))
	unique_id_pre := mqttclient.NormalizeStr(device.Name) + "_pool_"
	for prop, propStr := range models.PropStr {
		unique_id := unique_id_pre + propStr
		topic := "homeassistant/binary_sensor/" + unique_id + "/state"
		configs[prop] = models.MqttConfig{Name: "Pool " + propStr, StateTopic: topic, DeviceClass: "problem", UniqueID: unique_id, Device: device, ValueTemplate: "{{ value_json.sensor }}", ExpireAfter: int((interval * 2).Seconds()), ForceUpdate: true}
	}
	return configs
}

// NewSanoidProvider returns a provider that runs sanoid to check pool state.
func NewSanoidProvider(device models.Device, interval time.Duration) *SanoidProvider {
	return &SanoidProvider{
		configs:   GetPoolConfigs(device, interval),
		shellExec: func(name string, arg ...string) commandExecutor { return exec.Command(name, arg...) },
	}
}

// Entries runs sanoid for each monitored property and returns the current sensor states.
func (p *SanoidProvider) Entries() ([]models.Entry, error) {
	entries := make([]models.Entry, 0, len(p.configs))
	for property, config := range p.configs {
		ok, state, err := getPoolState(p.shellExec, property)
		if err != nil {
			return nil, err
		}
		if !ok && state != "" {
			Logger.Warn().Str("mod", "sanoid").Str("state", state).Msg("")
		}
		entries = append(entries, models.Entry{
			Config:  config,
			Domain:  "binary_sensor",
			Payload: mqttclient.ProblemPayload(ok),
		})
	}
	return entries, nil
}
