// Provides checks for the state of pool health, capacity and snapshots on the system.
package sanoid

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
)

var Logger zerolog.Logger

// Maps Sanoid exit codes to human-readable states
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

// Gathers autodiscovery struct for the binary health, capacity and snapshot sensors
func GetPoolConfigs(device models.Device, interval time.Duration) map[models.Property]models.MqttConfig {
	configs := make(map[models.Property]models.MqttConfig, len(models.PropStr))
	unique_id_pre := mqttclient.NormalizeStr(device.Name) + "_pool_"
	//iterate over all properties
	for prop, propStr := range models.PropStr {
		unique_id := unique_id_pre + propStr
		topic := "homeassistant/binary_sensor/" + unique_id + "/state"
		configs[prop] = models.MqttConfig{Name: "Pool " + propStr, StateTopic: topic, DeviceClass: "problem", UniqueID: unique_id, Device: device, ValueTemplate: "{{ value_json.sensor }}", ExpireAfter: int((interval * 2).Seconds()), ForceUpdate: true}
	}
	return configs
}

// Returns a new Sanoid client.
func NewSanoidClient(pubs chan *paho.Publish, device models.Device, interval time.Duration) SanoidClient {
	return SanoidClient{
		Pubs:      pubs,
		Interval:  interval,
		Config:    GetPoolConfigs(device, interval),
		Discover:  make(chan bool),
		zpoolExec: func(name string, arg ...string) zpoolExecutor { return exec.Command(name, arg...) },
		shellExec: func(name string, arg ...string) commandExecutor { return exec.Command(name, arg...) },
	}
}

// Updates the state of all pools by running Sanoid commands and publishing the results to MQTT.
func (client SanoidClient) update() error {
	for property, config := range client.Config {
		ok, state, err := getPoolState(client.shellExec, property)
		if err != nil {
			return err
		}
		if !ok && state != "" {
			Logger.Warn().Str("mod", "sanoid").Str("state", state).Msg("")
		}
		update := paho.Publish{
			Topic:   config.StateTopic,
			Payload: mqttclient.ProblemPayload(ok),
			Retain:  true,
		}
		client.Pubs <- &update
	}
	return nil
}

// logErrors logs non-nil errors from sanoid and zpool updates.
func logErrors(sanoidErr, zpoolErr error) {
	if sanoidErr != nil {
		Logger.Error().Str("mod", "sanoid").Err(sanoidErr).Msg("")
	}
	if zpoolErr != nil {
		Logger.Error().Str("mod", "zpool").Err(zpoolErr).Msg("")
	}
}

// Long-running routine that handles the Sanoid and ZFS pool checks and publishes messages.
func (client SanoidClient) Serve(ctx context.Context) {
	updateTimer := time.NewTicker(client.Interval)
	for {
		select {
		case <-ctx.Done():
			return
		case ok := <-client.Discover:
			if !ok {
				continue
			}
			for _, c := range client.Config {
				msg, err := mqttclient.GetDiscovery(c)
				if err != nil {
					Logger.Error().Str("mod", "sanoid").Err(err).Msg("")
					continue
				}
				client.Pubs <- msg
			}
			Logger.Debug().Str("mod", "sanoid").Msg("Discovery")
			sErr := client.update()
			zErr := client.updateZpool(true)
			logErrors(sErr, zErr)
			if sErr == nil && zErr == nil {
				Logger.Debug().Str("mod", "sanoid").Msg("Updated sensors")
			}
		case <-updateTimer.C:
			sErr := client.update()
			zErr := client.updateZpool(false)
			logErrors(sErr, zErr)
			if sErr == nil && zErr == nil {
				Logger.Debug().Str("mod", "sanoid").Msg("Updated sensors")
			}
		}
	}
}
