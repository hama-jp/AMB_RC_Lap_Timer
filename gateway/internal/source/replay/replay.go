// Package replay implements source.Source by reading a previously recorded
// `.bin` and its `.timing.csv` companion (docs/test-strategy.md §5,
// docs/architecture.md §3.4).
//
// Speed modes:
//
//   - "realtime"  Honor the offset_ms column; sleep between chunks to
//     reproduce the original cadence. (Default.)
//   - "fast"      Honor the order and chunk sizes but emit them
//     back-to-back with no inter-chunk delay.
//   - "instant"   Same as "fast" for now; reserved for future use
//     (e.g., emit the whole file in one chunk).
//
// If `<bin>.timing.csv` does not exist, the source falls back to "instant"
// and emits one chunk = the entire file. This covers fixtures whose timing
// data was lost (e.g., the 2026-05-05 incident, see
// docs/incidents/2026-05-05-recorder-csv-flush.md).
package replay

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// SpeedMode is one of the documented replay speeds.
type SpeedMode string

const (
	SpeedRealtime SpeedMode = "realtime"
	SpeedFast     SpeedMode = "fast"
	SpeedInstant  SpeedMode = "instant"
)

// SleepFunc is a context-aware sleep, injectable for tests.
type SleepFunc func(ctx context.Context, d time.Duration) error

// Source plays back a captured .bin / .timing.csv pair as a source.Source.
type Source struct {
	bin       *os.File
	timingF   *os.File // optional, may be nil when the timing.csv is missing
	timing    *csv.Reader
	speed     SpeedMode
	log       *zap.Logger
	now       func() time.Time
	sleep     SleepFunc
	started   time.Time
	headerRow bool // have we consumed the CSV header?
	finished  bool // EOF returned once
}

// New opens binPath and (if present) binPath+".timing.csv".
// speed is parsed leniently: empty or unknown → realtime.
// log may be nil.
func New(binPath string, speed string, log *zap.Logger) (*Source, error) {
	if log == nil {
		log = zap.NewNop()
	}
	bin, err := os.Open(binPath)
	if err != nil {
		return nil, fmt.Errorf("open bin: %w", err)
	}
	mode := normalizeSpeed(speed)

	var timingF *os.File
	timingPath := binPath + ".timing.csv"
	tf, errT := os.Open(timingPath)
	switch {
	case errT == nil:
		timingF = tf
	case errors.Is(errT, os.ErrNotExist):
		log.Warn("replay: no timing.csv next to bin; falling back to instant",
			zap.String("expected", timingPath))
		mode = SpeedInstant
	default:
		bin.Close()
		return nil, fmt.Errorf("open timing: %w", errT)
	}

	s := &Source{
		bin:     bin,
		timingF: timingF,
		speed:   mode,
		log:     log,
		now:     time.Now,
		sleep:   ctxSleep,
	}
	if timingF != nil {
		s.timing = csv.NewReader(timingF)
		s.timing.FieldsPerRecord = 2
	}
	return s, nil
}

// Read returns the next chunk from the recording. Honors ctx; respects the
// speed mode. After the recording ends, returns io.EOF on every call.
func (s *Source) Read(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.finished {
		return nil, io.EOF
	}

	// Without timing.csv: emit the whole bin once.
	if s.timing == nil {
		return s.readWholeBin()
	}

	row, err := s.nextTimingRow()
	if err != nil {
		s.finished = true
		return nil, err
	}
	if s.started.IsZero() {
		s.started = s.now()
	}

	if s.speed == SpeedRealtime {
		target := s.started.Add(time.Duration(row.offsetMs) * time.Millisecond)
		if d := target.Sub(s.now()); d > 0 {
			if err := s.sleep(ctx, d); err != nil {
				return nil, err
			}
		}
	}
	// "fast" / "instant" emit immediately.

	buf := make([]byte, row.length)
	n, err := io.ReadFull(s.bin, buf)
	if err != nil {
		// Truncated bin (timing.csv promised more bytes than the file
		// holds): emit what we got and finish.
		s.finished = true
		if n == 0 {
			return nil, io.EOF
		}
		s.log.Warn("replay: bin shorter than timing.csv claimed; truncating",
			zap.Int("requested", row.length), zap.Int("got", n))
		return buf[:n], nil
	}
	return buf, nil
}

// Close closes the underlying files. Safe to call multiple times.
func (s *Source) Close() error {
	var firstErr error
	if s.bin != nil {
		if err := s.bin.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.bin = nil
	}
	if s.timingF != nil {
		if err := s.timingF.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.timingF = nil
	}
	return firstErr
}

type timingRow struct {
	offsetMs int64
	length   int
}

func (s *Source) nextTimingRow() (timingRow, error) {
	for {
		rec, err := s.timing.Read()
		if err == io.EOF {
			return timingRow{}, io.EOF
		}
		if err != nil {
			return timingRow{}, fmt.Errorf("read timing.csv: %w", err)
		}
		if !s.headerRow {
			s.headerRow = true
			// The header is "offset_ms,length_bytes".
			if len(rec) >= 1 && rec[0] == "offset_ms" {
				continue
			}
			// Header missing — treat this row as data and continue parsing.
		}
		if len(rec) != 2 {
			return timingRow{}, fmt.Errorf("timing.csv row has %d cols, want 2", len(rec))
		}
		off, err := strconv.ParseInt(rec[0], 10, 64)
		if err != nil {
			return timingRow{}, fmt.Errorf("timing.csv offset_ms: %w", err)
		}
		ln, err := strconv.Atoi(rec[1])
		if err != nil {
			return timingRow{}, fmt.Errorf("timing.csv length_bytes: %w", err)
		}
		if ln <= 0 {
			// Zero-length chunks are not meaningful — skip.
			continue
		}
		return timingRow{offsetMs: off, length: ln}, nil
	}
}

func (s *Source) readWholeBin() ([]byte, error) {
	data, err := io.ReadAll(s.bin)
	s.finished = true
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, io.EOF
	}
	return data, nil
}

func normalizeSpeed(s string) SpeedMode {
	switch SpeedMode(s) {
	case SpeedFast, SpeedInstant:
		return SpeedMode(s)
	default:
		return SpeedRealtime
	}
}

func ctxSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
