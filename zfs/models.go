package zfs

import (
	"context"

	"github.com/ykgmfq/SystemPub/models"
)

// Provider delivers sensor entries for discovery and state updates.
type Provider interface {
	Entries(context.Context) ([]models.Entry, error)
}
