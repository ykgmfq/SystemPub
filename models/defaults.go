package models

import (
	"net/url"

	"github.com/rs/zerolog"
)

func MQTTdefault() MQTT {
	return MQTT{Host: YAMLURL{&url.URL{Scheme: "mqtt", Host: "localhost:1883"}}}
}

func SystemPubConfigDefault() SystemPubConfig {
	return SystemPubConfig{MQTTServer: MQTTdefault(), Loglevel: zerolog.InfoLevel}
}
