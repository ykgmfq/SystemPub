package models

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// ParseServerURL parses an MQTT server address.
// It rejects addresses without a scheme or host, which url.Parse would otherwise accept as a bare path.
// It also rejects schemes the client cannot connect with, so that a typo fails at startup rather than on every connection attempt.
func ParseServerURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("MQTT server address %q needs a scheme and host, such as mqtt://localhost:1883", raw)
	}
	if !slices.Contains(serverSchemes, parsed.Scheme) {
		return nil, fmt.Errorf("MQTT server address %q has unsupported scheme %q, use one of %s", raw, parsed.Scheme, strings.Join(serverSchemes, ", "))
	}
	return parsed, nil
}
