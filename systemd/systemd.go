package systemd

import (
	"context"
	"encoding/json"
	"os/exec"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
)

// getUnitConfig returns the configuration of the failed units binary sensor.
func getUnitConfig(device models.Device, interval time.Duration) models.MqttConfig {
	unique_id := mqttclient.NormalizeStr(device.Name) + "_units"
	stateTopic := mqttclient.StateTopic(models.BinarySensor, unique_id)
	attrTopic := mqttclient.AttributesTopic(models.BinarySensor, unique_id)
	return models.MqttConfig{Name: "Systemd units", StateTopic: stateTopic, JsonAttributesTopic: attrTopic, DeviceClass: "problem", UniqueID: unique_id, Device: device, ExpireAfter: int((interval * 2).Seconds()), ForceUpdate: true}
}

// GetDevice returns the properties of the host device.
func GetDevice(ctx context.Context) (models.Device, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "hostnamectl", "--json=short").Output()
	if err != nil {
		return models.Device{}, err
	}
	var status hostnamectl
	if err = json.Unmarshal(out, &status); err != nil {
		return models.Device{}, err
	}
	return models.Device{Name: status.Hostname, SWversion: status.OperatingSystemPrettyName, Identifiers: [1]string{status.MachineID}, Manufacturer: status.HardwareVendor, Model: status.HardwareModel}, nil
}

// NewUnitProvider connects to systemd over D-Bus and returns a provider for the units sensor.
func NewUnitProvider(ctx context.Context, device models.Device, interval time.Duration, logger zerolog.Logger) (*UnitProvider, error) {
	conn, err := dbus.NewWithContext(ctx)
	if err != nil {
		return nil, err
	}
	return &UnitProvider{
		conn:   conn,
		config: getUnitConfig(device, interval),
		logger: logger.With().Str("mod", "systemd").Logger(),
	}, nil
}

// Close ends the D-Bus connection.
func (p *UnitProvider) Close() {
	p.conn.Close()
}

// unitEntry returns the sensor entry for the given list of failed units.
func unitEntry(config models.MqttConfig, failedUnits []string) (models.Entry, error) {
	attrs, err := json.Marshal(map[string][]string{"failed_units": failedUnits})
	if err != nil {
		return models.Entry{}, err
	}
	ok := len(failedUnits) == 0
	return models.Entry{
		Config:     config,
		Domain:     models.BinarySensor,
		Payload:    mqttclient.ProblemPayload(ok),
		Attributes: attrs,
		Problem:    !ok,
	}, nil
}

// Entries queries systemd for failed units and returns the units sensor.
func (p *UnitProvider) Entries(ctx context.Context) ([]models.Entry, error) {
	states, err := p.conn.ListUnitsByPatternsContext(ctx, []string{"failed"}, []string{"*"})
	if err != nil {
		return nil, err
	}
	failedUnits := make([]string, 0, len(states))
	for _, state := range states {
		p.logger.Warn().Str("unit", state.Name).Msg("Failed unit")
		failedUnits = append(failedUnits, state.Name)
	}
	entry, err := unitEntry(p.config, failedUnits)
	if err != nil {
		return nil, err
	}
	return []models.Entry{entry}, nil
}
