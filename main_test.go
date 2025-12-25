package main

import (
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/ykgmfq/SystemPub/models"
)

// Tests for reading configuration from a file
func TestReadConfig(t *testing.T) {
	// Create a temporary configuration file
	configData := `
mqttserver:
  host: mqtt://192.168.0.3:8080
loglevel: warn
`
	tempFile, err := os.CreateTemp("", "testconfig.yaml")
	assert.NoError(t, err, "Failed to create temporary configuration file")
	defer os.Remove(tempFile.Name())
	_, err = tempFile.Write([]byte(configData))
	assert.NoError(t, err, "Failed to write to temporary configuration file")
	tempFile.Close()

	config := readConfig(tempFile.Name())

	assert.Equal(t, "mqtt://192.168.0.3:8080", config.MQTTServer.Host.String(), "Host value mismatch")
	assert.Equal(t, zerolog.WarnLevel, config.Loglevel, "Log level mismatch")
}

// Tests for default config if no file is provided
func TestReadConfigNoFile(t *testing.T) {
	file := ""
	config := readConfig(file)
	assert.Equal(t, config, models.SystemPubConfigDefault(), "Config mismatch")
}
