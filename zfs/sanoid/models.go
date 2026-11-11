package sanoid

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/ykgmfq/SystemPub/models"
)

type commandExecutor interface {
	Output() ([]byte, error)
}

// SanoidProvider checks snapshot freshness via the sanoid CLI.
type SanoidProvider struct {
	config    models.MqttConfig
	shellExec func(context.Context, string, ...string) commandExecutor
	logger    zerolog.Logger
}
