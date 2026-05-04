// Package config loads and resolves the gateway configuration.
//
// Configuration is read from a JSON file (typically next to the executable).
// Missing fields fall back to Defaults(). Relative directory paths in the
// configuration are resolved against the executable's directory by
// ResolvePaths so the gateway behaves identically regardless of the current
// working directory (see docs/architecture.md §4.4 portable operation).
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Config is the runtime configuration for the gateway.
//
// Only fields needed by the gateway-recorder MVP (#1) are declared here.
// Fields scheduled for later phases (listen, replay) are ignored on load
// rather than rejected, so a forward-looking config.json keeps working.
type Config struct {
	Upstream UpstreamConfig `json:"upstream"`
	Logging  LoggingConfig  `json:"logging"`
	Records  RecordsConfig  `json:"records"`
}

// UpstreamConfig describes the AMB decoder TCP endpoint.
type UpstreamConfig struct {
	Host      string          `json:"host"`
	Port      int             `json:"port"`
	Reconnect ReconnectConfig `json:"reconnect"`
}

// ReconnectConfig controls the exponential backoff with jitter
// used when re-establishing the upstream TCP connection.
type ReconnectConfig struct {
	InitialMs   int     `json:"initial_ms"`
	MaxMs       int     `json:"max_ms"`
	JitterRatio float64 `json:"jitter_ratio"`
}

// LoggingConfig controls log file destination and rotation.
// Dir may be relative; ResolvePaths resolves it against the EXE directory.
type LoggingConfig struct {
	Dir        string `json:"dir"`
	MaxSizeMB  int    `json:"max_size_mb"`
	MaxBackups int    `json:"max_backups"`
}

// RecordsConfig controls where --record output is placed when the user
// passes a relative path. Dir may be relative; ResolvePaths resolves it
// against the EXE directory.
type RecordsConfig struct {
	Dir string `json:"dir"`
}

// Defaults returns the documented default configuration
// (docs/architecture.md §3.4).
func Defaults() Config {
	return Config{
		Upstream: UpstreamConfig{
			Host: "192.168.10.20",
			Port: 5403,
			Reconnect: ReconnectConfig{
				InitialMs:   1000,
				MaxMs:       30000,
				JitterRatio: 0.2,
			},
		},
		Logging: LoggingConfig{
			Dir:        "./logs",
			MaxSizeMB:  5,
			MaxBackups: 5,
		},
		Records: RecordsConfig{
			Dir: "./records",
		},
	}
}

// Load decodes JSON from r into a Config, merging onto Defaults().
// A nil reader returns Defaults() unchanged. Unknown fields are ignored
// (forward compatibility with #3 / #7 fields like listen, replay).
func Load(r io.Reader) (Config, error) {
	cfg := Defaults()
	if r == nil {
		return cfg, nil
	}
	dec := json.NewDecoder(r)
	if err := dec.Decode(&cfg); err != nil {
		if err == io.EOF {
			return cfg, nil
		}
		return Defaults(), fmt.Errorf("decode config: %w", err)
	}
	return cfg, nil
}

// LoadFile opens path and delegates to Load. If the file does not exist,
// it returns Defaults() with os.ErrNotExist so the caller can decide
// whether to bootstrap from config.example.json (see docs/architecture.md
// §4.4.6).
func LoadFile(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Defaults(), err
	}
	defer f.Close()
	return Load(f)
}

// ResolvePaths returns a copy of c with relative directory paths resolved
// against baseDir (typically filepath.Dir(os.Executable())).
// Absolute paths are kept as-is. Empty strings stay empty so the caller
// can detect "not configured" and skip that subsystem.
func (c Config) ResolvePaths(baseDir string) Config {
	c.Logging.Dir = resolveAgainst(baseDir, c.Logging.Dir)
	c.Records.Dir = resolveAgainst(baseDir, c.Records.Dir)
	return c
}

func resolveAgainst(baseDir, p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(baseDir, p))
}
