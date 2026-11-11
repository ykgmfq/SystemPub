package models

import (
	"net/url"

	"github.com/rs/zerolog"
)

// MQTTdefault returns the MQTT server settings used when none are configured.
func MQTTdefault() MQTT {
	return MQTT{Host: YAMLURL{&url.URL{Scheme: "mqtt", Host: "localhost:1883"}}}
}

// SystemPubConfigDefault returns the configuration used for options that are not set.
func SystemPubConfigDefault() SystemPubConfig {
	return SystemPubConfig{MQTTServer: MQTTdefault(), Loglevel: zerolog.InfoLevel}
}
