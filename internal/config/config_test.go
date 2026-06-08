package config

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	data := []byte(`# comment
ssid: mynet
passphrase: secret
width: 64
height: 32
host: 10.0.0.5
port: 8080
stops:   A42N:A,C   R30N   233N:1,2,3
poll_interval: 30s
phase_interval: 3s
group_interval: 9s
`)

	var cfg Config
	cfg.Parse(data)

	if cfg.SSID != "mynet" || cfg.Passphrase != "secret" {
		t.Errorf("creds: got %q/%q", cfg.SSID, cfg.Passphrase)
	}
	if cfg.Width != 64 || cfg.Height != 32 {
		t.Errorf("dims: got %dx%d", cfg.Width, cfg.Height)
	}
	if cfg.Host != "10.0.0.5" || cfg.Port != 8080 {
		t.Errorf("host: got %s:%d", cfg.Host, cfg.Port)
	}
	// Stops split on whitespace; commas stay inside a stop's route filter.
	want := []string{"A42N:A,C", "R30N", "233N:1,2,3"}
	if len(cfg.Stops) != len(want) {
		t.Fatalf("stops: got %v", cfg.Stops)
	}
	for i := range want {
		if cfg.Stops[i] != want[i] {
			t.Errorf("stops[%d]: got %q want %q", i, cfg.Stops[i], want[i])
		}
	}
	if cfg.PollInterval != 30*time.Second || cfg.PhaseInterval != 3*time.Second || cfg.GroupInterval != 9*time.Second {
		t.Errorf("intervals: got %v/%v/%v", cfg.PollInterval, cfg.PhaseInterval, cfg.GroupInterval)
	}
}
