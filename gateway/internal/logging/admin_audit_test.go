package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestAudit(t *testing.T) (*AuditWriter, string) {
	t.Helper()
	dir := t.TempDir()
	a, err := NewAuditWriter(AuditOptions{Dir: dir, MaxSizeMB: 1, MaxBackups: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	// Pin the clock so the test can match the timestamp prefix exactly.
	a.now = func() time.Time { return time.Date(2026, 5, 6, 12, 34, 56, 0, time.UTC) }
	return a, filepath.Join(dir, "admin-audit.log")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAuditWriter_LogChange_LineFormat(t *testing.T) {
	a, path := newTestAudit(t)

	a.LogChange("192.168.1.42", map[string]ChangeValue{
		"upstream.host": {Old: `"192.168.1.20"`, New: `"192.168.1.21"`},
		"upstream.port": {Old: "5402", New: "5403"},
	})
	a.Close() // flush

	got := readFile(t, path)
	want := `2026-05-06T12:34:56Z 192.168.1.42 changed 2 field(s): upstream.host="192.168.1.20"->"192.168.1.21", upstream.port=5402->5403` + "\n"
	if got != want {
		t.Errorf("line\n got: %q\nwant: %q", got, want)
	}
}

func TestAuditWriter_LogAuth_LoginOk(t *testing.T) {
	a, path := newTestAudit(t)

	a.LogAuth("10.0.0.1", "login ok")
	a.Close()

	got := readFile(t, path)
	want := "2026-05-06T12:34:56Z 10.0.0.1 login ok\n"
	if got != want {
		t.Errorf("line\n got: %q\nwant: %q", got, want)
	}
}

func TestAuditWriter_LogAuth_BlankIPBecomesDash(t *testing.T) {
	a, path := newTestAudit(t)
	a.LogAuth("", "login failed")
	a.Close()

	got := readFile(t, path)
	if !strings.HasSuffix(got, " - login failed\n") {
		t.Errorf("expected blank IP rendered as '-'; got %q", got)
	}
}

func TestAuditWriter_NilReceiver_NoPanic(t *testing.T) {
	var a *AuditWriter
	a.LogChange("x", map[string]ChangeValue{"a": {Old: "1", New: "2"}})
	a.LogAuth("x", "login ok")
	if err := a.Close(); err != nil {
		t.Errorf("Close on nil: %v", err)
	}
}

func TestAuditWriter_EmptyDir_DiscardsButReturnsWriter(t *testing.T) {
	a, err := NewAuditWriter(AuditOptions{Dir: ""})
	if err != nil {
		t.Fatal(err)
	}
	a.LogAuth("1.2.3.4", "login ok") // must not panic / crash
	if err := a.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestAuditWriter_CloseIdempotent(t *testing.T) {
	a, _ := newTestAudit(t)
	if err := a.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
