package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaults_Values(t *testing.T) {
	d := Defaults()
	if d.Upstream.Host != "192.168.10.20" {
		t.Errorf("default upstream host: got %q want 192.168.10.20", d.Upstream.Host)
	}
	if d.Upstream.Port != 5403 {
		t.Errorf("default upstream port: got %d want 5403", d.Upstream.Port)
	}
	if d.Upstream.Reconnect.InitialMs != 1000 {
		t.Errorf("default initial_ms: got %d want 1000", d.Upstream.Reconnect.InitialMs)
	}
	if d.Upstream.Reconnect.MaxMs != 30000 {
		t.Errorf("default max_ms: got %d want 30000", d.Upstream.Reconnect.MaxMs)
	}
	if d.Upstream.Reconnect.JitterRatio != 0.2 {
		t.Errorf("default jitter: got %v want 0.2", d.Upstream.Reconnect.JitterRatio)
	}
	if d.Logging.MaxSizeMB != 5 || d.Logging.MaxBackups != 5 {
		t.Errorf("default logging rotation: got %dMB/%d want 5/5",
			d.Logging.MaxSizeMB, d.Logging.MaxBackups)
	}
}

func TestLoad_Nil_ReturnsDefaults(t *testing.T) {
	got, err := Load(nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != Defaults() {
		t.Errorf("nil reader did not return defaults")
	}
}

func TestLoad_EmptyJSON_ReturnsDefaults(t *testing.T) {
	got, err := Load(strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != Defaults() {
		t.Errorf("empty JSON did not return defaults: %+v", got)
	}
}

func TestLoad_PartialOverride_KeepsOtherDefaults(t *testing.T) {
	in := `{
		"upstream": { "host": "10.0.0.1", "port": 9999,
			"reconnect": { "initial_ms": 250 }
		}
	}`
	got, err := Load(strings.NewReader(in))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Upstream.Host != "10.0.0.1" || got.Upstream.Port != 9999 {
		t.Errorf("override not applied: %+v", got.Upstream)
	}
	if got.Upstream.Reconnect.InitialMs != 250 {
		t.Errorf("initial_ms override not applied: %d", got.Upstream.Reconnect.InitialMs)
	}
	// untouched fields keep defaults
	if got.Upstream.Reconnect.MaxMs != 30000 {
		t.Errorf("max_ms should remain default 30000, got %d", got.Upstream.Reconnect.MaxMs)
	}
	if got.Upstream.Reconnect.JitterRatio != 0.2 {
		t.Errorf("jitter should remain default 0.2, got %v", got.Upstream.Reconnect.JitterRatio)
	}
	if got.Logging.MaxSizeMB != 5 {
		t.Errorf("logging.max_size_mb should remain default 5, got %d", got.Logging.MaxSizeMB)
	}
}

func TestLoad_IgnoreUnknownFields(t *testing.T) {
	// listen / replay are scheduled for #3 / #7 and must not break loading.
	in := `{ "listen": ":8080", "replay": { "speed": "fast" }, "future": { "x": 1 } }`
	got, err := Load(strings.NewReader(in))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != Defaults() {
		t.Errorf("unknown fields changed config: %+v", got)
	}
}

func TestLoad_InvalidJSON_ReturnsError(t *testing.T) {
	_, err := Load(strings.NewReader(`{not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestResolvePaths_Relative(t *testing.T) {
	base := t.TempDir() // guaranteed absolute by the os
	c := Defaults()
	c.Logging.Dir = "./logs"
	c.Records.Dir = "records"
	got := c.ResolvePaths(base)
	wantLog := filepath.Join(base, "logs")
	if got.Logging.Dir != wantLog {
		t.Errorf("logging.dir: got %q want %q", got.Logging.Dir, wantLog)
	}
	wantRec := filepath.Join(base, "records")
	if got.Records.Dir != wantRec {
		t.Errorf("records.dir: got %q want %q", got.Records.Dir, wantRec)
	}
}

func TestResolvePaths_Absolute_KeepsAsIs(t *testing.T) {
	base := t.TempDir()
	abs := filepath.Join(t.TempDir(), "elsewhere") // a different absolute path
	c := Config{Logging: LoggingConfig{Dir: abs}, Records: RecordsConfig{Dir: abs}}
	got := c.ResolvePaths(base)
	if got.Logging.Dir != filepath.Clean(abs) {
		t.Errorf("absolute path was rewritten: got %q want %q", got.Logging.Dir, abs)
	}
	if got.Records.Dir != filepath.Clean(abs) {
		t.Errorf("absolute records path was rewritten: got %q want %q", got.Records.Dir, abs)
	}
}

func TestResolvePaths_Empty_StaysEmpty(t *testing.T) {
	c := Config{Logging: LoggingConfig{Dir: ""}, Records: RecordsConfig{Dir: ""}}
	got := c.ResolvePaths(t.TempDir())
	if got.Logging.Dir != "" || got.Records.Dir != "" {
		t.Errorf("empty paths were rewritten: %+v", got)
	}
}
