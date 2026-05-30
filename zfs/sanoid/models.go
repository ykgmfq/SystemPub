package sanoid

import "github.com/ykgmfq/SystemPub/models"

type commandExecutor interface {
	Run() error
}

// SanoidProvider checks pool health, capacity and snapshots via the sanoid CLI.
type SanoidProvider struct {
	configs   map[models.Property]models.MqttConfig
	shellExec func(string, ...string) commandExecutor
}
