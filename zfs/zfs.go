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

var Logger zerolog.Logger

// ZfsServer runs all ZFS providers on a ticker and publishes to MQTT.
type ZfsServer struct {
	Discover  chan bool
	providers []Provider
	interval  time.Duration
	pubs      chan *paho.Publish
}

func NewZfsServer(pubs chan *paho.Publish, device models.Device, interval time.Duration) ZfsServer {
	return ZfsServer{
		Discover:  make(chan bool),
		providers: []Provider{sanoid.NewSanoidProvider(device, interval), zpool.NewZpoolProvider(interval)},
		interval:  interval,
		pubs:      pubs,
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

func (s ZfsServer) discoverAll() {
	for _, p := range s.providers {
		entries, err := p.Entries()
		if err != nil {
			Logger.Error().Str("mod", "zfs").Err(err).Msg("")
			continue
		}
		for _, e := range entries {
			if err := s.publishDiscovery(e); err != nil {
				Logger.Error().Str("mod", "zfs").Err(err).Msg("")
			}
		}
	}
}

func (s ZfsServer) updateAll() {
	for _, p := range s.providers {
		entries, err := p.Entries()
		if err != nil {
			Logger.Error().Str("mod", "zfs").Err(err).Msg("")
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
			Logger.Debug().Str("mod", "zfs").Msg("Discovery")
			s.discoverAll()
			s.updateAll()
		case <-ticker.C:
			s.updateAll()
		}
	}
}
