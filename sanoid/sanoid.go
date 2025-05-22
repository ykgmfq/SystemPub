// Provides checks for the state of pool health, capacity and snapshots on the system.
package sanoid

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
)

var Logger zerolog.Logger

// Interface for injecting mock output in tests
type commandExecutor interface {
	Output() ([]byte, error)
}

// Gets overwritten in tests
var shellCommandFunc = func(name string, arg ...string) commandExecutor {
	return exec.Command(name, arg...)
}

// Runs Sanoid to check one of pool health, capacity and snapshots, and returns true if the output is "OK"
func getPoolState(p models.Property) (bool, error) {
	cmd := shellCommandFunc("sanoid", "--monitor-"+models.PropStr[p])
	out, err := cmd.Output()
	output := string(out)
	if err != nil {
		return false, err
	}
	ok := strings.HasPrefix(output, "OK")
	if !ok {
		Logger.Warn().Str("mod", "sanoid").Str("sanoid output", output).Msg("")
	}
	return ok, nil
}

// Gathers autodiscovery struct for the binary health, capacity and snapshot sensors
func GetPoolConfigs(device models.Device, interval time.Duration) map[models.Property]models.MqttConfig {
	configs := make(map[models.Property]models.MqttConfig, len(models.PropStr))
	//iterate over all properties
	for prop, propStr := range models.PropStr {
		unique_id := device.Name + "_pool_" + propStr
		topic := "homeassistant/binary_sensor/" + unique_id + "/state"
		configs[prop] = models.MqttConfig{Name: "Pool " + propStr, StateTopic: topic, DeviceClass: "problem", UniqueID: unique_id, Device: device, ValueTemplate: "{{ value_json.sensor }}", ExpireAfter: int((interval * 2).Seconds()), ForceUpdate: true}
	}
	return configs
}

type SanoidClient struct {
	Discover chan bool
	Interval time.Duration
	Config   map[models.Property]models.MqttConfig
	Pubs     chan *paho.Publish
}

func NewSanoidClient(pubs chan *paho.Publish, device models.Device, interval time.Duration) SanoidClient {
	return SanoidClient{
		Pubs:     pubs,
		Interval: interval,
		Config:   GetPoolConfigs(device, interval),
		Discover: make(chan bool),
	}
}

func (client SanoidClient) update() error {
	for property, config := range client.Config {
		state, err := getPoolState(property)
		if err != nil {
			return err
		}
		update := paho.Publish{
			Topic:   config.StateTopic,
			Payload: mqttclient.ProblemPayload(state),
		}
		client.Pubs <- &update
	}
	return nil
}

func (client SanoidClient) Serve(ctx context.Context) {
	updateTimer := time.NewTicker(client.Interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-client.Discover:
			for _, c := range client.Config {
				discovery := mqttclient.GetDiscovery(c)
				client.Pubs <- discovery
			}
			Logger.Debug().Str("mod", "sanoid").Msg("Discovery")
			if err := client.update(); err != nil {
				Logger.Error().Str("mod", "sanoid").Err(err).Msg("")
			} else {
				Logger.Debug().Str("mod", "sanoid").Msg("Updated sensors")
			}
		case <-updateTimer.C:
			if err := client.update(); err != nil {
				Logger.Error().Str("mod", "sanoid").Err(err).Msg("")
			} else {
				Logger.Debug().Str("mod", "sanoid").Msg("Updated sensors")
			}
		}
	}
}
