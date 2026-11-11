// Package zfs orchestrates ZFS-related MQTT sensor providers.
package zfs

import (
	"context"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"github.com/ykgmfq/SystemPub/mqttclient"
	"github.com/ykgmfq/SystemPub/zfs/sanoid"
	"github.com/ykgmfq/SystemPub/zfs/zpool"
)

// NewZfsServer returns a server that runs the sanoid and zpool providers.
func NewZfsServer(pubs chan *paho.Publish, device models.Device, interval time.Duration, logger zerolog.Logger) ZfsServer {
	return ZfsServer{
		Discover:  make(chan bool),
		providers: []Provider{sanoid.NewSanoidProvider(device, interval, logger), zpool.NewZpoolProvider(interval)},
		interval:  interval,
		pubs:      pubs,
		logger:    logger.With().Str("mod", "zfs").Logger(),
	}
}

func (s ZfsServer) publishDiscovery(e models.Entry) error {
	var (
		msg *paho.Publish
		err error
	)
	if e.Domain == "sensor" {
		msg, err = mqttclient.GetSensorDiscovery(e.Config)
	} else {
		msg, err = mqttclient.GetDiscovery(e.Config)
	}
	if err != nil {
		return err
	}
	s.pubs <- msg
	return nil
}

func (s ZfsServer) publishState(e models.Entry) {
	s.pubs <- &paho.Publish{Topic: e.Config.StateTopic, Payload: e.Payload, Retain: true}
	if e.Attributes != nil {
		s.pubs <- &paho.Publish{Topic: e.Config.JsonAttributesTopic, Payload: e.Attributes, Retain: true}
	}
}

func (s ZfsServer) discoverAll(ctx context.Context) {
	for _, p := range s.providers {
		entries, err := p.Entries(ctx)
		if err != nil {
			s.logger.Error().Err(err).Msg("")
			continue
		}
		for _, e := range entries {
			if err := s.publishDiscovery(e); err != nil {
				s.logger.Error().Err(err).Msg("")
			}
		}
	}
}

func (s ZfsServer) updateAll(ctx context.Context) {
	for _, p := range s.providers {
		entries, err := p.Entries(ctx)
		if err != nil {
			s.logger.Error().Err(err).Msg("")
			continue
		}
		for _, e := range entries {
			s.publishState(e)
		}
	}
}

func (s ZfsServer) Serve(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	for {
		select {
		case <-ctx.Done():
			return
		case ok := <-s.Discover:
			if !ok {
				continue
			}
			s.logger.Debug().Msg("Discovery")
			s.discoverAll(ctx)
			s.updateAll(ctx)
		case <-ticker.C:
			s.updateAll(ctx)
		}
	}
}
