package service

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
	"github.com/ykgmfq/SystemPub/models"
	"gopkg.in/yaml.v3"
)

// The name under which the MQTT password must be provided to the service as a systemd credential.
const mqttCredentialName = "mqtt"

// otherCredentials returns the names of all credentials in the credentials directory except the expected one.
func otherCredentials() ([]string, error) {
	credDir := os.Getenv("CREDENTIALS_DIRECTORY")
	if credDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(credDir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.Name() != mqttCredentialName {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// loadMQTTPassword reads the MQTT password from the systemd credentials directory, if one is set.
func loadMQTTPassword() (string, error) {
	credDir := os.Getenv("CREDENTIALS_DIRECTORY")
	if credDir == "" {
		return "", nil
	}
	data, err := os.ReadFile(filepath.Join(credDir, mqttCredentialName))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// readConfig reads the application configuration from the given file.
func readConfig(location string, logger zerolog.Logger) (models.SystemPubConfig, error) {
	config := models.SystemPubConfigDefault()
	file, err := os.Open(location)
	if errors.Is(err, fs.ErrNotExist) {
		logger.Warn().Str("location", location).Msg("Configuration file not found, using defaults")
		return config, nil
	}
	if err != nil {
		logger.Warn().Err(err).Msg("Could not open configuration file")
		return config, err
	}
	defer file.Close()
	if err = yaml.NewDecoder(file).Decode(&config); err != nil {
		logger.Error().Err(err).Msg("Malformed configuration file")
		return config, err
	}
	logger.Debug().Str("location", location).Interface("content", config).Msg("")
	return config, nil
}

// withPassword returns the configuration with the password from the credentials directory, which takes precedence over the configuration file.
func withPassword(config models.SystemPubConfig, logger zerolog.Logger) (models.SystemPubConfig, error) {
	others, err := otherCredentials()
	if err != nil {
		return config, fmt.Errorf("could not list credentials directory: %w", err)
	}
	for _, name := range others {
		logger.Warn().Str("credential", name).Msgf("Ignoring credential, the password must be named %q", mqttCredentialName)
	}
	password, err := loadMQTTPassword()
	if err != nil {
		return config, fmt.Errorf("could not read password from credentials directory: %w", err)
	}
	if password == "" {
		return config, nil
	}
	if config.MQTTServer.Password != "" {
		logger.Warn().Msg("Overriding MQTT password from configuration file with credentials directory")
	}
	config.MQTTServer.Password = password
	return config, nil
}

// withHost returns the configuration with the server host given on the command line, if any.
func withHost(config models.SystemPubConfig, host string) (models.SystemPubConfig, error) {
	if host == "" {
		return config, nil
	}
	parsedURL, err := models.ParseServerURL(host)
	if err != nil {
		return config, fmt.Errorf("malformed MQTT server URL: %w", err)
	}
	config.MQTTServer.Host = models.YAMLURL{URL: parsedURL}
	return config, nil
}

// LoadConfig assembles the configuration from the file, the credentials directory and the command line.
func LoadConfig(configPath, host string, debug bool, logger zerolog.Logger) (models.SystemPubConfig, error) {
	config := models.SystemPubConfigDefault()
	if configPath != "" {
		conf, err := readConfig(configPath, logger)
		if err != nil {
			return config, fmt.Errorf("could not read configuration file: %w", err)
		}
		config = conf
	}
	config, err := withPassword(config, logger)
	if err != nil {
		return config, err
	}
	if debug {
		config.Loglevel = zerolog.DebugLevel
	}
	return withHost(config, host)
}
