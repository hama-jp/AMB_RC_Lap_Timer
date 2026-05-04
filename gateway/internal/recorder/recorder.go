// Package recorder writes received bytes to disk for later replay.
//
// On --record <file>, two artifacts are produced:
//
//   - <file>           raw concatenated bytes (no framing added)
//   - <file>.timing.csv  one CSV row per chunk: offset_ms,length_bytes
//
// .timing.csv layout (frozen by docs/architecture.md §9 #5 via this PR):
//
//	offset_ms,length_bytes
//	12,234
//	1503,127
//	...
//
// `offset_ms` is milliseconds since the recorder was created (rough
// approximation of "since TCP connect"; #1's MVP starts the clock at New()).
// Replay (#7) reads .timing.csv to reproduce the original cadence.
//
// Fail-soft (docs/architecture.md §4.4.3): a write failure (e.g., USB removed)
// increments an internal counter and emits a warning log, but never returns
// an error to the caller — the caller (main loop) keeps the gateway alive
// for the WS fan-out and upstream connection.
package recorder

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Recorder owns the open .bin and .timing.csv writers.
type Recorder struct {
	mu       sync.Mutex
	bin      io.WriteCloser
	timingCl io.Closer
	timingW  *csv.Writer
	now      func() time.Time
	started  time.Time
	log      *zap.Logger
	failures int
}

// New creates a Recorder writing to binPath and binPath+".timing.csv".
// The parent directory must already exist; cmd/gateway ensures this on startup.
//
// New itself fails only when the files cannot be opened. Once created,
// subsequent write failures are downgraded to warnings (fail-soft).
func New(binPath string, log *zap.Logger) (*Recorder, error) {
	if log == nil {
		log = zap.NewNop()
	}
	bin, err := os.Create(binPath)
	if err != nil {
		return nil, fmt.Errorf("create bin: %w", err)
	}
	timing, err := os.Create(binPath + ".timing.csv")
	if err != nil {
		_ = bin.Close()
		return nil, fmt.Errorf("create timing: %w", err)
	}
	return newWithWriters(bin, timing, log, time.Now)
}

func newWithWriters(bin, timing io.WriteCloser, log *zap.Logger, now func() time.Time) (*Recorder, error) {
	r := &Recorder{
		bin:      bin,
		timingCl: timing,
		timingW:  csv.NewWriter(timing),
		now:      now,
		started:  now(),
		log:      log,
	}
	if err := r.timingW.Write([]string{"offset_ms", "length_bytes"}); err != nil {
		_ = bin.Close()
		_ = timing.Close()
		return nil, fmt.Errorf("write timing header: %w", err)
	}
	r.timingW.Flush()
	if err := r.timingW.Error(); err != nil {
		_ = bin.Close()
		_ = timing.Close()
		return nil, fmt.Errorf("flush timing header: %w", err)
	}
	return r, nil
}

// Write appends p to the .bin file and a row to the .timing.csv file.
// Errors from either underlying writer are downgraded to warnings.
func (r *Recorder) Write(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	offset := r.now().Sub(r.started).Milliseconds()
	if _, err := r.bin.Write(p); err != nil {
		r.failures++
		r.log.Warn("record bin write failed",
			zap.Error(err), zap.Int("failures", r.failures), zap.Int("len", len(p)))
		return
	}
	if err := r.timingW.Write([]string{
		strconv.FormatInt(offset, 10),
		strconv.Itoa(len(p)),
	}); err != nil {
		r.failures++
		r.log.Warn("record timing write failed",
			zap.Error(err), zap.Int("failures", r.failures))
	}
}

// Failures returns the count of write failures since New().
// Used by /healthz in #3; exposed here for unit tests.
func (r *Recorder) Failures() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failures
}

// Close flushes the CSV buffer and closes both files. Safe to call multiple times.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var firstErr error
	if r.timingW != nil {
		r.timingW.Flush()
		if err := r.timingW.Error(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if r.timingCl != nil {
		if err := r.timingCl.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		r.timingCl = nil
	}
	if r.bin != nil {
		if err := r.bin.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		r.bin = nil
	}
	return firstErr
}
