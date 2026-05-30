package zfs

import "github.com/ykgmfq/SystemPub/models"

// Provider delivers sensor entries for discovery and state updates.
type Provider interface {
	Entries() ([]models.Entry, error)
}
