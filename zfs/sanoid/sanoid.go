// Package sanoid provides a ZFS provider that checks pool health via the sanoid CLI.
package sanoid

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
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
// Returns (ok, state, output, err): state is non-empty when exit code 1-4 (pool problem, sanoid healthy).
func getPoolState(ctx context.Context, run func(context.Context, string, ...string) commandExecutor, p models.Property) (bool, string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := run(ctx, "sanoid", "--monitor-"+models.PropStr[p])
	raw, err := cmd.Output()
	output := strings.TrimSpace(string(raw))
	if err == nil {
		return true, "", output, nil
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return false, "", output, err
	}
	exitCode := exitError.ExitCode()
	if exitCode < 0 || exitCode > 4 {
		return false, "", output, err
	}
	return false, sanoidState(exitCode), output, nil
}

// GetPoolConfigs gathers autodiscovery configs for the health, capacity and snapshot binary sensors.
func GetPoolConfigs(device models.Device, interval time.Duration) map[models.Property]models.MqttConfig {
	configs := make(map[models.Property]models.MqttConfig, len(models.PropStr))
	unique_id_pre := mqttclient.NormalizeStr(device.Name) + "_pool_"
	for prop, propStr := range models.PropStr {
		unique_id := unique_id_pre + propStr
		topic := "homeassistant/binary_sensor/" + unique_id + "/state"
		attrTopic := "homeassistant/binary_sensor/" + unique_id + "/attributes"
		configs[prop] = models.MqttConfig{Name: "Pool " + propStr, StateTopic: topic, JsonAttributesTopic: attrTopic, DeviceClass: "problem", UniqueID: unique_id, Device: device, ValueTemplate: "{{ value_json.sensor }}", ExpireAfter: int((interval * 2).Seconds()), ForceUpdate: true}
	}
	return configs
}

// NewSanoidProvider returns a provider that runs sanoid to check pool state.
func NewSanoidProvider(device models.Device, interval time.Duration) *SanoidProvider {
	return &SanoidProvider{
		configs:   GetPoolConfigs(device, interval),
		shellExec: func(ctx context.Context, name string, arg ...string) commandExecutor { return exec.CommandContext(ctx, name, arg...) },
	}
}

// Entries runs sanoid for each monitored property and returns the current sensor states.
func (p *SanoidProvider) Entries(ctx context.Context) ([]models.Entry, error) {
	entries := make([]models.Entry, 0, len(p.configs))
	for property, config := range p.configs {
		ok, state, output, err := getPoolState(ctx, p.shellExec, property)
		if err != nil {
			return nil, err
		}
		if !ok && state != "" {
			Logger.Warn().Str("mod", "sanoid").Str("state", state).Msg("")
		}
		attrs, err := json.Marshal(map[string]string{"output": output})
		if err != nil {
			return nil, err
		}
		entries = append(entries, models.Entry{
			Config:     config,
			Domain:     "binary_sensor",
			Payload:    mqttclient.ProblemPayload(ok),
			Attributes: attrs,
		})
	}
	return entries, nil
}
