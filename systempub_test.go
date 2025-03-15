package main

import (
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

// Tests for reading configuration from a file
func TestReadConfig(t *testing.T) {
	// Create a temporary configuration file
	configData := `
mqttserver:
  host: 192.168.0.3
  port: 1883
loglevel: warn
`
	tempFile, err := os.CreateTemp("", "testconfig.yaml")
	assert.NoError(t, err, "Failed to create temporary configuration file")
	defer os.Remove(tempFile.Name())
	_, err = tempFile.Write([]byte(configData))
	assert.NoError(t, err, "Failed to write to temporary configuration file")
	tempFile.Close()

	config := readConfig(tempFile.Name())

	assert.Equal(t, "192.168.0.3", config.MQTTServer.Host, "Host value mismatch")
	assert.Equal(t, 1883, config.MQTTServer.Port, "Port value mismatch")
	assert.Equal(t, zerolog.WarnLevel, config.Loglevel, "Log level mismatch")
}

// Tests for default config if no file is provided
func TestReadConfigNoFile(t *testing.T) {
	file := ""
	config := readConfig(file)
	assert.Equal(t, config, getDefaultConfig(), "Config mismatch")
}
