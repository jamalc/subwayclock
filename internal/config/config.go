// Package config parses the flat key: value config files.
//
// Used by the TinyGo apps. Lines starting with # are comments; blank lines are
// skipped. This is separate from the serve backend, which has its own YAML
// config.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the settings for the TinyGo apps, parsed
// from a flat key: value config file by Parse.
type Config struct {
	Width         int
	Height        int
	SSID          string
	Passphrase    string
	Host          string
	Port          int
	Stops         []string
	PollInterval  time.Duration
	PhaseInterval time.Duration
	GroupInterval time.Duration
}

// Load reads and parses a config file from disk. Native builds (the simulate
// binary) use this. Device builds (TinyGo, baremetal) have no filesystem, so
// they embed the config at build time and call Parse directly instead.
func (cfg *Config) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	cfg.Parse(data)
	return nil
}

// Parse fills cfg from raw key: value config bytes.
func (cfg *Config) Parse(data []byte) {
	m := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		i := strings.IndexByte(line, ':')
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		m[key] = val
	}
	cfg.SSID = m["ssid"]
	cfg.Passphrase = m["passphrase"]
	cfg.Host = m["host"]
	if v, ok := m["port"]; ok {
		cfg.Port, _ = strconv.Atoi(v)
	}
	// Stops are whitespace-separated. Commas are reserved for a stop's route
	// tokens (e.g. "A42N:A,C"), and stop IDs never contain spaces.
	cfg.Stops = strings.Fields(m["stops"])
	if v, ok := m["poll_interval"]; ok {
		cfg.PollInterval, _ = time.ParseDuration(v)
	}
	if v, ok := m["phase_interval"]; ok {
		cfg.PhaseInterval, _ = time.ParseDuration(v)
	}
	if v, ok := m["group_interval"]; ok {
		cfg.GroupInterval, _ = time.ParseDuration(v)
	}
	if v, ok := m["width"]; ok {
		cfg.Width, _ = strconv.Atoi(v)
	}
	if v, ok := m["height"]; ok {
		cfg.Height, _ = strconv.Atoi(v)
	}
}
