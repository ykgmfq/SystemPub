package models

import (
	"net/url"
	"time"

	"github.com/rs/zerolog"
)

// MQTTdefault returns the MQTT server settings used when none are configured.
func MQTTdefault() MQTT {
	return MQTT{Host: YAMLURL{&url.URL{Scheme: "mqtt", Host: "localhost:1883"}}}
}

// FireDrillDefault returns the fire drill settings used when none are configured.
// The duration leaves room for alerts that only fire after a problem has lasted a few minutes.
func FireDrillDefault() FireDrill {
	return FireDrill{Duration: 5 * time.Minute, ErrorCount: 42}
}

// SystemPubConfigDefault returns the configuration used for options that are not set.
func SystemPubConfigDefault() SystemPubConfig {
	return SystemPubConfig{MQTTServer: MQTTdefault(), Loglevel: zerolog.InfoLevel, FireDrill: FireDrillDefault()}
}
