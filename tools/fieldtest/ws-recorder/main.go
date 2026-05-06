// Command ws-recorder is a headless WebSocket client for field-test
// observability. It connects to the gateway's /ws endpoint, records receive
// statistics (bytes, inter-frame latency, disconnect count) to CSV or JSONL,
// and reconnects with exponential backoff on drops
// (docs/test-strategy.md §6.4).
//
// Multiple instances can be launched concurrently (Multi-client / Soak
// scenarios) — each writes to its own --out file.
//
// Example:
//
//	go run . --url ws://localhost:8080/ws --out recorder.csv --duration-sec 600
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"nhooyr.io/websocket"
)

type recordWriter interface {
	WriteEvent(e event) error
	Close() error
}

type event struct {
	Time        time.Time
	FrameIndex  int64
	Bytes       int
	MsSincePrev int64
	// Event values: "frame", "connect", "disconnect", "shutdown".
	// "shutdown" is written exactly once when ctx is cancelled (signal or
	// --duration-sec). Harness disconnect counters should subtract or
	// filter on Event="shutdown" so a clean exit is not booked as a drop
	// (Issue #70 review notes).
	Event string
	Note  string
}

func main() {
	var (
		url           string
		outPath       string
		format        string
		rawOutPath    string
		durationSec   int
		reconnectInit int
		reconnectMax  int
		quiet         bool
	)
	flag.StringVar(&url, "url", "ws://localhost:8080/ws", "WebSocket URL to connect to")
	flag.StringVar(&outPath, "out", "recorder.csv", "stats output file path")
	flag.StringVar(&format, "format", "csv", "stats output format: csv | jsonl")
	flag.StringVar(&rawOutPath, "raw-out", "", "if set, append every received WS payload byte-for-byte to this file (for replay round-trip checks)")
	flag.IntVar(&durationSec, "duration-sec", 0, "exit after N seconds (0 = run until SIGINT)")
	flag.IntVar(&reconnectInit, "reconnect-initial-ms", 500, "initial reconnect delay in ms")
	flag.IntVar(&reconnectMax, "reconnect-max-ms", 30000, "max reconnect delay in ms")
	flag.BoolVar(&quiet, "quiet", false, "suppress per-frame stderr logging")
	flag.Parse()

	w, err := openWriter(outPath, format)
	if err != nil {
		log.Fatalf("open writer: %v", err)
	}
	defer w.Close()

	var rawOut *os.File
	if rawOutPath != "" {
		rawOut, err = os.Create(rawOutPath)
		if err != nil {
			log.Fatalf("open raw-out: %v", err)
		}
		defer rawOut.Close()
	}

	signalCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx := signalCtx
	if durationSec > 0 {
		dctx, dcancel := context.WithTimeout(ctx, time.Duration(durationSec)*time.Second)
		defer dcancel()
		ctx = dctx
	}

	log.Printf("recorder url=%s out=%s format=%s raw-out=%q duration=%ds",
		url, outPath, format, rawOutPath, durationSec)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	var (
		totalBytes  int64
		totalFrames int64
		disconnects int64
		attempt     int
	)

	start := time.Now()
	for {
		if ctx.Err() != nil {
			break
		}
		framesBefore := atomic.LoadInt64(&totalFrames)
		err := runOnce(ctx, url, w, rawOut, &totalBytes, &totalFrames, quiet)
		// Only book a disconnect when ctx is still alive: nhooyr.io/websocket
		// races between ctx cancellation and conn.Close internally, so on
		// shutdown the Read can return either ctx.Err() OR
		// "use of closed network connection". Both are clean exits when ctx
		// has been cancelled, and the harness has no way to tell them apart
		// from a genuine drop without this gate.
		if err != nil && ctx.Err() == nil {
			atomic.AddInt64(&disconnects, 1)
			_ = w.WriteEvent(event{Time: time.Now(), Event: "disconnect", Note: err.Error()})
			log.Printf("disconnect: %v", err)
		}
		if ctx.Err() != nil {
			break
		}
		// Reset backoff if the previous connection received any data.
		if atomic.LoadInt64(&totalFrames) > framesBefore {
			attempt = 0
		}
		// Exponential backoff with ±20% jitter, capped at reconnect-max-ms.
		base := time.Duration(reconnectInit) * time.Millisecond
		for i := 0; i < attempt; i++ {
			next := base * 2
			if next <= 0 || next > time.Duration(reconnectMax)*time.Millisecond {
				base = time.Duration(reconnectMax) * time.Millisecond
				break
			}
			base = next
		}
		jitter := time.Duration(float64(base) * 0.2 * (r.Float64()*2 - 1))
		delay := base + jitter
		if delay < 0 {
			delay = 0
		}
		log.Printf("reconnecting in %s (attempt %d)", delay.Round(time.Millisecond), attempt+1)
		select {
		case <-ctx.Done():
		case <-time.After(delay):
		}
		attempt++
	}

	// Distinguish shutdown reason so a long Soak that hits its timeout looks
	// the same to the harness as a Ctrl-C: both are clean exits, neither
	// should be booked as a disconnect.
	reason := "ctx done"
	switch {
	case signalCtx.Err() != nil:
		reason = "signal"
	case ctx.Err() == context.DeadlineExceeded:
		reason = "duration elapsed"
	}
	_ = w.WriteEvent(event{Time: time.Now(), Event: "shutdown", Note: reason})

	elapsed := time.Since(start)
	log.Printf("summary: runtime=%s frames=%d bytes=%d disconnects=%d shutdown=%s",
		elapsed.Round(time.Millisecond),
		atomic.LoadInt64(&totalFrames),
		atomic.LoadInt64(&totalBytes),
		atomic.LoadInt64(&disconnects),
		reason)
}

func runOnce(ctx context.Context, url string, w recordWriter, rawOut *os.File, totalBytes, totalFrames *int64, quiet bool) error {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	conn, _, err := websocket.Dial(dialCtx, url, nil)
	cancel()
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	conn.SetReadLimit(1 << 20) // 1 MiB; gateway emits small frames
	// Always close with NormalClosure. nhooyr's Close() blocks on the close
	// handshake, so on a deadline-cancelled ctx we still want to send a
	// proper close frame to keep the gateway's per-client logs clean.
	defer conn.Close(websocket.StatusNormalClosure, "")

	if err := w.WriteEvent(event{Time: time.Now(), Event: "connect", Note: url}); err != nil {
		return err
	}
	log.Printf("connected to %s", url)

	var prev time.Time
	for {
		_, data, err := conn.Read(ctx)
		now := time.Now()
		if err != nil {
			return err
		}
		var ms int64
		if !prev.IsZero() {
			ms = now.Sub(prev).Milliseconds()
		}
		prev = now
		idx := atomic.AddInt64(totalFrames, 1)
		atomic.AddInt64(totalBytes, int64(len(data)))
		if rawOut != nil {
			if _, werr := rawOut.Write(data); werr != nil {
				return fmt.Errorf("raw-out write: %w", werr)
			}
		}
		ev := event{
			Time:        now,
			FrameIndex:  idx,
			Bytes:       len(data),
			MsSincePrev: ms,
			Event:       "frame",
		}
		if err := w.WriteEvent(ev); err != nil {
			return err
		}
		if !quiet && idx%50 == 0 {
			log.Printf("frame=%d bytes=%d dt=%dms", idx, len(data), ms)
		}
	}
}

func openWriter(path, format string) (recordWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	switch format {
	case "csv":
		cw := csv.NewWriter(f)
		if err := cw.Write([]string{"timestamp", "event", "frame_index", "bytes", "ms_since_prev", "note"}); err != nil {
			f.Close()
			return nil, err
		}
		cw.Flush()
		return &csvWriter{f: f, w: cw}, nil
	case "jsonl":
		return &jsonlWriter{f: f}, nil
	default:
		f.Close()
		return nil, fmt.Errorf("unknown format %q (want csv or jsonl)", format)
	}
}

type csvWriter struct {
	f *os.File
	w *csv.Writer
}

func (c *csvWriter) WriteEvent(e event) error {
	row := []string{
		e.Time.UTC().Format(time.RFC3339Nano),
		e.Event,
		strconv.FormatInt(e.FrameIndex, 10),
		strconv.Itoa(e.Bytes),
		strconv.FormatInt(e.MsSincePrev, 10),
		e.Note,
	}
	if err := c.w.Write(row); err != nil {
		return err
	}
	c.w.Flush()
	return c.w.Error()
}

func (c *csvWriter) Close() error {
	c.w.Flush()
	return c.f.Close()
}

type jsonlWriter struct {
	f *os.File
}

func (j *jsonlWriter) WriteEvent(e event) error {
	rec := map[string]any{
		"ts":            e.Time.UTC().Format(time.RFC3339Nano),
		"event":         e.Event,
		"frame_index":   e.FrameIndex,
		"bytes":         e.Bytes,
		"ms_since_prev": e.MsSincePrev,
	}
	if e.Note != "" {
		rec["note"] = e.Note
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := j.f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func (j *jsonlWriter) Close() error {
	return j.f.Close()
}

var _ recordWriter = (*csvWriter)(nil)
var _ recordWriter = (*jsonlWriter)(nil)
