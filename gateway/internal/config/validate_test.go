package config

import (
	"strings"
	"testing"
)

// goodConfig returns a Config that should pass Validate. Tests start from
// this and mutate the one field under test.
func goodConfig() Config {
	c := Defaults()
	c.Logging.Dir = "./logs"
	c.Records.Dir = "./records"
	return c
}

func TestValidate_GoodConfig_NoErrors(t *testing.T) {
	if errs := Validate(goodConfig()); len(errs) > 0 {
		t.Errorf("expected 0 errors, got %v", errs)
	}
}

func TestValidate_FieldRules(t *testing.T) {
	cases := []struct {
		name  string
		mut   func(*Config)
		path  string
		match string // substring expected in Message
	}{
		{
			name:  "upstream.host empty",
			mut:   func(c *Config) { c.Upstream.Host = "" },
			path:  "upstream.host",
			match: "required",
		},
		{
			name:  "upstream.port zero",
			mut:   func(c *Config) { c.Upstream.Port = 0 },
			path:  "upstream.port",
			match: "1 and 65535",
		},
		{
			name:  "upstream.port over",
			mut:   func(c *Config) { c.Upstream.Port = 70000 },
			path:  "upstream.port",
			match: "1 and 65535",
		},
		{
			name:  "reconnect.initial_ms zero",
			mut:   func(c *Config) { c.Upstream.Reconnect.InitialMs = 0 },
			path:  "upstream.reconnect.initial_ms",
			match: ">= 1",
		},
		{
			name:  "reconnect.max_ms below initial",
			mut:   func(c *Config) { c.Upstream.Reconnect.InitialMs = 1000; c.Upstream.Reconnect.MaxMs = 100 },
			path:  "upstream.reconnect.max_ms",
			match: ">= upstream.reconnect.initial_ms",
		},
		{
			name:  "reconnect.jitter_ratio negative",
			mut:   func(c *Config) { c.Upstream.Reconnect.JitterRatio = -0.1 },
			path:  "upstream.reconnect.jitter_ratio",
			match: "0 and 1",
		},
		{
			name:  "reconnect.jitter_ratio over 1",
			mut:   func(c *Config) { c.Upstream.Reconnect.JitterRatio = 1.5 },
			path:  "upstream.reconnect.jitter_ratio",
			match: "0 and 1",
		},
		{
			name:  "logging.dir empty",
			mut:   func(c *Config) { c.Logging.Dir = "" },
			path:  "logging.dir",
			match: "required",
		},
		{
			name:  "logging.max_size_mb zero",
			mut:   func(c *Config) { c.Logging.MaxSizeMB = 0 },
			path:  "logging.max_size_mb",
			match: "1 and 100",
		},
		{
			name:  "logging.max_size_mb over",
			mut:   func(c *Config) { c.Logging.MaxSizeMB = 200 },
			path:  "logging.max_size_mb",
			match: "1 and 100",
		},
		{
			name:  "logging.max_backups negative",
			mut:   func(c *Config) { c.Logging.MaxBackups = -1 },
			path:  "logging.max_backups",
			match: "0 and 50",
		},
		{
			name:  "records.dir empty",
			mut:   func(c *Config) { c.Records.Dir = "" },
			path:  "records.dir",
			match: "required",
		},
		{
			name:  "replay.speed bogus",
			mut:   func(c *Config) { c.Replay.Speed = "bogus" },
			path:  "replay.speed",
			match: `"realtime"`,
		},
		{
			name:  "server.max_clients zero",
			mut:   func(c *Config) { c.Server.MaxClients = 0 },
			path:  "server.max_clients",
			match: "1 and 1000",
		},
		{
			name:  "server.max_clients too big",
			mut:   func(c *Config) { c.Server.MaxClients = 5000 },
			path:  "server.max_clients",
			match: "1 and 1000",
		},
		{
			name:  "server.client_buffer_len zero",
			mut:   func(c *Config) { c.Server.ClientBufferLen = 0 },
			path:  "server.client_buffer_len",
			match: "1 and 1024",
		},
		{
			name:  "listen no port",
			mut:   func(c *Config) { c.Listen = "0.0.0.0" },
			path:  "listen",
			match: ":<port>",
		},
		{
			name:  "listen non-numeric port",
			mut:   func(c *Config) { c.Listen = ":abc" },
			path:  "listen",
			match: ":<port>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := goodConfig()
			tc.mut(&c)
			errs := Validate(c)
			found := false
			for _, e := range errs {
				if e.Path == tc.path && strings.Contains(e.Message, tc.match) {
					found = true
				}
			}
			if !found {
				t.Errorf("did not find expected error path=%s contains=%q\n  got: %v",
					tc.path, tc.match, errs)
			}
		})
	}
}

func TestValidate_AggregatesAllErrors(t *testing.T) {
	c := Config{} // entirely zero — many fields will be invalid
	errs := Validate(c)
	if len(errs) < 5 {
		t.Errorf("expected aggregate of multiple errors, got %d: %v", len(errs), errs)
	}
}

func TestValidate_ReplaySpeedEmpty_OK(t *testing.T) {
	// Empty replay.speed is the documented "use default" form. Validate
	// must allow it (Defaults() leaves it as "realtime" but a stripped
	// payload may omit it).
	c := goodConfig()
	c.Replay.Speed = ""
	if errs := Validate(c); len(errs) > 0 {
		t.Errorf("empty replay.speed should be accepted, got %v", errs)
	}
}

func TestValidate_ListenEmpty_OK(t *testing.T) {
	c := goodConfig()
	c.Listen = ""
	if errs := Validate(c); len(errs) > 0 {
		t.Errorf("empty listen should be accepted, got %v", errs)
	}
}
