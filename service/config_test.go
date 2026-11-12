package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ykgmfq/SystemPub/models"
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

// TestFireDrillConfig checks that fire drill settings fall back to the defaults field by field and that settings which cannot fake a problem are rejected.
func TestFireDrillConfig(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content string
		want    models.FireDrill
		wantErr bool
	}{
		{name: "defaults", content: "loglevel: info\n", want: models.FireDrillDefault()},
		{name: "configured", content: "firedrill:\n  duration: 10m\n  errorcount: 7\n", want: models.FireDrill{Duration: 10 * time.Minute, ErrorCount: 7}},
		{name: "partial", content: "firedrill:\n  errorcount: 7\n", want: models.FireDrill{Duration: 5 * time.Minute, ErrorCount: 7}},
		{name: "zero duration", content: "firedrill:\n  duration: 0s\n", wantErr: true},
		{name: "negative error count", content: "firedrill:\n  errorcount: -1\n", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(c.content), 0600))
			t.Setenv("CREDENTIALS_DIRECTORY", "")
			config, err := LoadConfig(path, "", false, zerolog.Nop())
			if c.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, config.FireDrill)
		})
	}
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
