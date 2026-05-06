package config

import (
	"fmt"
)

// ValidationError describes a single bad field in a Config payload received
// over /admin/api/config. The handler aggregates these and returns them all
// at once (HTTP 400 with a JSON list) so the UI can highlight every bad
// input in one round trip.
type ValidationError struct {
	// Path is the dotted JSON path of the bad field, e.g. "upstream.port".
	Path string `json:"path"`
	// Message is a human-readable, UI-friendly reason. No stack traces.
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// Validate enforces the field-level rules from docs/architecture.md §3.5.5
// (range / enum constraints). Structural rules (unknown JSON fields, type
// mismatches) are caught earlier by the handler's strict decoder; Validate
// assumes the input was decodable into a Config.
//
// Empty / zero defaults are intentionally rejected so the operator cannot
// accidentally ship a half-filled config — if a field is missing, the
// caller should send the previously-loaded value back.
func Validate(c Config) []ValidationError {
	var errs []ValidationError
	add := func(path, msg string) {
		errs = append(errs, ValidationError{Path: path, Message: msg})
	}

	// listen — the bind expression is small but easy to typo. We allow
	// either ":<port>" or "<host>:<port>"; an empty string falls back to
	// the default at startup, so we accept that too.
	if c.Listen != "" {
		if !looksLikeBindAddr(c.Listen) {
			add("listen", `must be ":<port>" or "<host>:<port>" (e.g. ":8080")`)
		}
	}

	// upstream
	if c.Upstream.Host == "" {
		add("upstream.host", "required")
	}
	if c.Upstream.Port < 1 || c.Upstream.Port > 65535 {
		add("upstream.port", "must be between 1 and 65535")
	}
	if c.Upstream.Reconnect.InitialMs < 1 {
		add("upstream.reconnect.initial_ms", "must be >= 1")
	}
	if c.Upstream.Reconnect.MaxMs < c.Upstream.Reconnect.InitialMs {
		add("upstream.reconnect.max_ms", "must be >= upstream.reconnect.initial_ms")
	}
	if c.Upstream.Reconnect.JitterRatio < 0 || c.Upstream.Reconnect.JitterRatio > 1 {
		add("upstream.reconnect.jitter_ratio", "must be between 0 and 1")
	}

	// logging
	if c.Logging.Dir == "" {
		add("logging.dir", "required")
	}
	if c.Logging.MaxSizeMB < 1 || c.Logging.MaxSizeMB > 100 {
		add("logging.max_size_mb", "must be between 1 and 100")
	}
	if c.Logging.MaxBackups < 0 || c.Logging.MaxBackups > 50 {
		add("logging.max_backups", "must be between 0 and 50")
	}

	// records
	if c.Records.Dir == "" {
		add("records.dir", "required")
	}

	// replay
	switch c.Replay.Speed {
	case "", "realtime", "fast", "instant":
	default:
		add("replay.speed", `must be one of "realtime" / "fast" / "instant"`)
	}

	// server
	if c.Server.MaxClients < 1 || c.Server.MaxClients > 1000 {
		add("server.max_clients", "must be between 1 and 1000")
	}
	if c.Server.ClientBufferLen < 1 || c.Server.ClientBufferLen > 1024 {
		add("server.client_buffer_len", "must be between 1 and 1024")
	}

	return errs
}

// looksLikeBindAddr is a permissive check — net.Listen will reject anything
// truly malformed at runtime. We just want to catch the "operator pasted a
// URL or hostname without port" mistake here.
func looksLikeBindAddr(s string) bool {
	colon := -1
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			colon = i
			break
		}
	}
	if colon == -1 || colon == len(s)-1 {
		return false
	}
	for _, r := range s[colon+1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
