package models

import (
	"fmt"
	"net/url"
)

// ParseServerURL parses an MQTT server address.
// It rejects addresses without a scheme or host, which url.Parse would otherwise accept as a bare path.
func ParseServerURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("MQTT server address %q needs a scheme and host, such as mqtt://localhost:1883", raw)
	}
	return parsed, nil
}
