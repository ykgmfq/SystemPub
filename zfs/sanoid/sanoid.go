// Package sanoid provides a ZFS provider that checks snapshot freshness via the sanoid CLI.
package sanoid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
)

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
		4: "Dependent",
	}
	return sanoidExitLevels[exit]
}

// Runs sanoid to check snapshot freshness.
// Returns (ok, state, output, err): state is non-empty when exit code 1-4 (snapshot problem, sanoid healthy).
func getSnapshotState(ctx context.Context, run func(context.Context, string, ...string) commandExecutor) (bool, string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := run(ctx, "sanoid", "--monitor-snapshots")
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
		if len(exitError.Stderr) > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitError.Stderr)))
		}
		return false, "", output, err
	}
	return false, sanoidState(exitCode), output, nil
}

// GetSnapshotConfig returns the autodiscovery config for the snapshot binary sensor.
func GetSnapshotConfig(device models.Device, interval time.Duration) models.MqttConfig {
	uniqueID := mqttclient.NormalizeStr(device.Name) + "_pool_snapshots"
	topic := mqttclient.StateTopic(models.BinarySensor, uniqueID)
	attrTopic := mqttclient.AttributesTopic(models.BinarySensor, uniqueID)
	return models.MqttConfig{Name: "Pool snapshots", StateTopic: topic, JsonAttributesTopic: attrTopic, DeviceClass: "problem", UniqueID: uniqueID, Device: device, ExpireAfter: int((interval * 2).Seconds()), ForceUpdate: true}
}

// NewSanoidProvider returns a provider that runs sanoid to check snapshot freshness.
// It runs sanoid once and fails when sanoid itself does not work, so that the caller can carry on without it.
// Snapshot problems found by that run do not count as a failure.
func NewSanoidProvider(ctx context.Context, device models.Device, interval time.Duration, logger zerolog.Logger) (*SanoidProvider, error) {
	p := newSanoidProvider(device, interval, logger)
	if _, _, _, err := getSnapshotState(ctx, p.shellExec); err != nil {
		return nil, err
	}
	return p, nil
}

func newSanoidProvider(device models.Device, interval time.Duration, logger zerolog.Logger) *SanoidProvider {
	return &SanoidProvider{
		config: GetSnapshotConfig(device, interval),
		shellExec: func(ctx context.Context, name string, arg ...string) commandExecutor {
			return exec.CommandContext(ctx, name, arg...)
		},
		logger: logger.With().Str("mod", "sanoid").Logger(),
	}
}

// Entries runs sanoid and returns the current state of the snapshot sensor.
func (p *SanoidProvider) Entries(ctx context.Context) ([]models.Entry, error) {
	ok, state, output, err := getSnapshotState(ctx, p.shellExec)
	if err != nil {
		return nil, err
	}
	if !ok && state != "" {
		p.logger.Warn().Str("state", state).Msg("Snapshot problem")
	}
	attrs, err := json.Marshal(map[string]string{"output": output})
	if err != nil {
		return nil, err
	}
	return []models.Entry{{
		Config:     p.config,
		Domain:     models.BinarySensor,
		Payload:    mqttclient.ProblemPayload(ok),
		Attributes: attrs,
		Problem:    !ok,
	}}, nil
}
