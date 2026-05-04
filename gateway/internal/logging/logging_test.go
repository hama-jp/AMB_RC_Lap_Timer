package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestNew_StdoutOnly_NoFileSink(t *testing.T) {
	l, err := New(Options{Dir: ""})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer l.Close()
	l.Info("hello", zap.String("k", "v"))
	if len(l.closers) != 0 {
		t.Errorf("expected no file closers when Dir is empty, got %d", len(l.closers))
	}
}

func TestNew_FileSink_CreatesGatewayLog(t *testing.T) {
	dir := t.TempDir()
	l, err := New(Options{Dir: dir, MaxSizeMB: 1, MaxBackups: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Info("hello-from-test", zap.Int("answer", 42))
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	logPath := filepath.Join(dir, "gateway.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "hello-from-test") {
		t.Errorf("log file missing message; contents=%q", s)
	}
	if !strings.Contains(s, `"answer":42`) {
		t.Errorf("log file missing structured field; contents=%q", s)
	}
	// Sanity: it should be JSON-formatted (the file sink uses JSON encoder).
	if !strings.HasPrefix(strings.TrimSpace(s), "{") {
		t.Errorf("file sink should be JSON-encoded, got prefix %q", s[:min(40, len(s))])
	}
}

func TestNew_MkdirFails_DowngradesToStdoutOnly(t *testing.T) {
	// Create a regular file at the path we will pass as Dir; MkdirAll will fail.
	parent := t.TempDir()
	conflict := filepath.Join(parent, "not-a-dir")
	if err := os.WriteFile(conflict, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	l, err := New(Options{Dir: conflict})
	if err != nil {
		t.Fatalf("New should not error on mkdir failure (fail-soft): %v", err)
	}
	defer l.Close()
	if len(l.closers) != 0 {
		t.Errorf("expected no file closer when mkdir fails, got %d", len(l.closers))
	}
	// Logging must still work (to stdout).
	l.Info("still-alive")
}

func TestNonzero(t *testing.T) {
	cases := []struct {
		v, dflt, want int
	}{
		{0, 5, 5},
		{-1, 5, 5},
		{3, 5, 3},
	}
	for _, tc := range cases {
		if got := nonzero(tc.v, tc.dflt); got != tc.want {
			t.Errorf("nonzero(%d,%d) = %d, want %d", tc.v, tc.dflt, got, tc.want)
		}
	}
}

// min for Go 1.20 (built-in `min` was added in 1.21).
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
