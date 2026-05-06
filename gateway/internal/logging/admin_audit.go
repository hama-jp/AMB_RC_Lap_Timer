// Package logging also exposes the /admin audit log writer.
//
// The audit log lives next to gateway.log under the same logging.dir and is
// rotated by lumberjack with the same MaxSize / MaxBackups, but uses its own
// file (admin-audit.log) so an operator can read it without filtering noise
// out of the main log. Format is intentionally plain text — one line per
// event — so `tail -f` is enough; no JSON.
//
// See docs/architecture.md §3.5.6 for the line shape:
//
//	2026-05-06T12:34:56Z 192.168.1.42 changed 2 field(s): upstream.host="192.168.1.20"->"192.168.1.21", upstream.port=5402->5403
//	2026-05-06T12:34:58Z 192.168.1.42 login ok
//	2026-05-06T12:35:01Z 192.168.1.42 login failed
//	2026-05-06T12:35:14Z 192.168.1.42 logout
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// AuditOptions mirrors logging.Options but always points at admin-audit.log
// inside Dir. Empty Dir disables file output (writer becomes a no-op).
type AuditOptions struct {
	Dir        string
	MaxSizeMB  int
	MaxBackups int
}

// ChangeValue is one before/after pair for AuditWriter.LogChange. Strings
// are pre-formatted by the caller — this type does not interpret values.
type ChangeValue struct {
	Old string
	New string
}

// AuditWriter is a small sink for /admin audit events. Methods are safe for
// concurrent use; the underlying file write is serialized by mu so log lines
// don't interleave under load.
type AuditWriter struct {
	mu     sync.Mutex
	w      io.Writer
	closer io.Closer
	now    func() time.Time // injected by tests
}

// NewAuditWriter opens the audit log. If opts.Dir is empty the returned
// writer is non-nil but discards everything (callers don't need a nil check).
func NewAuditWriter(opts AuditOptions) (*AuditWriter, error) {
	if opts.Dir == "" {
		return &AuditWriter{w: io.Discard, now: time.Now}, nil
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("audit: mkdir %s: %w", opts.Dir, err)
	}
	lj := &lumberjack.Logger{
		Filename:   filepath.Join(opts.Dir, "admin-audit.log"),
		MaxSize:    nonzero(opts.MaxSizeMB, 5),
		MaxBackups: nonzero(opts.MaxBackups, 5),
		LocalTime:  true,
	}
	return &AuditWriter{w: lj, closer: lj, now: time.Now}, nil
}

// LogChange records a successful POST /admin/api/config update. fields maps
// dotted config paths (e.g. "upstream.host") to before/after values. Output
// is sorted by path for stable test assertions.
func (a *AuditWriter) LogChange(remoteIP string, fields map[string]ChangeValue) {
	if a == nil {
		return
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := fields[k]
		parts = append(parts, fmt.Sprintf("%s=%s->%s", k, v.Old, v.New))
	}
	msg := fmt.Sprintf("changed %d field(s): %s", len(keys), strings.Join(parts, ", "))
	a.write(remoteIP, msg)
}

// LogAuth records login / logout events. event is one of "login ok",
// "login failed", "logout"; pass the literal string so call sites are easy
// to audit.
func (a *AuditWriter) LogAuth(remoteIP, event string) {
	if a == nil {
		return
	}
	a.write(remoteIP, event)
}

func (a *AuditWriter) write(remoteIP, msg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.w == nil {
		return
	}
	ts := a.now().UTC().Format(time.RFC3339)
	if remoteIP == "" {
		remoteIP = "-"
	}
	// Best-effort write. Audit failures must not block the request — they
	// surface only as a missing line in the file, never as an HTTP 5xx.
	_, _ = fmt.Fprintf(a.w, "%s %s %s\n", ts, remoteIP, msg)
}

// Close flushes and closes the underlying file. Safe to call multiple times
// and on a nil receiver.
func (a *AuditWriter) Close() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closer == nil {
		return nil
	}
	err := a.closer.Close()
	a.closer = nil
	a.w = io.Discard
	return err
}
