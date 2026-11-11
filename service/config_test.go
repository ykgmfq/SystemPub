package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestReadConfig(t *testing.T) {
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

	config, err := readConfig(tempFile.Name(), zerolog.Nop())
	assert.NoError(t, err, "Failed to read config")

	assert.Equal(t, "mqtt://192.168.0.3:8080", config.MQTTServer.Host.String(), "Host value mismatch")
	assert.Equal(t, zerolog.WarnLevel, config.Loglevel, "Log level mismatch")
}

func TestLoadMQTTPassword_NoEnv(t *testing.T) {
	orig, had := os.LookupEnv("CREDENTIALS_DIRECTORY")
	if had {
		defer os.Setenv("CREDENTIALS_DIRECTORY", orig)
	} else {
		defer os.Unsetenv("CREDENTIALS_DIRECTORY")
	}
	os.Unsetenv("CREDENTIALS_DIRECTORY")

	pw, err := loadMQTTPassword()
	assert.NoError(t, err)
	assert.Equal(t, "", pw)
}

func TestLoadMQTTPassword_Success(t *testing.T) {
	orig, had := os.LookupEnv("CREDENTIALS_DIRECTORY")
	if had {
		defer os.Setenv("CREDENTIALS_DIRECTORY", orig)
	} else {
		defer os.Unsetenv("CREDENTIALS_DIRECTORY")
	}

	dir, err := os.MkdirTemp("", "creddir")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.WriteFile(filepath.Join(dir, "mqtt"), []byte("secret\n"), 0600)
	assert.NoError(t, err)

	err = os.Setenv("CREDENTIALS_DIRECTORY", dir)
	assert.NoError(t, err)

	pw, err := loadMQTTPassword()
	assert.NoError(t, err)
	assert.Equal(t, "secret", pw)
}

func TestLoadMQTTPassword_FileMissing(t *testing.T) {
	orig, had := os.LookupEnv("CREDENTIALS_DIRECTORY")
	if had {
		defer os.Setenv("CREDENTIALS_DIRECTORY", orig)
	} else {
		defer os.Unsetenv("CREDENTIALS_DIRECTORY")
	}

	dir, err := os.MkdirTemp("", "creddir2")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Setenv("CREDENTIALS_DIRECTORY", dir)
	assert.NoError(t, err)

	pw, err := loadMQTTPassword()
	assert.Error(t, err)
	assert.Equal(t, "", pw)
}

// Credentials with unexpected names are reported, so a misnamed password does not go unnoticed.
func TestOtherCredentials(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CREDENTIALS_DIRECTORY", dir)
	for _, name := range []string{"mqtt", "mqtt-password"} {
		assert.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600))
	}
	names, err := otherCredentials()
	assert.NoError(t, err)
	assert.Equal(t, []string{"mqtt-password"}, names)
}
