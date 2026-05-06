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
	"errors"
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
	Event       string // "frame" | "connect" | "disconnect" | "error"
	Note        string
}

func main() {
	var (
		url           string
		outPath       string
		format        string
		durationSec   int
		reconnectInit int
		reconnectMax  int
		quiet         bool
	)
	flag.StringVar(&url, "url", "ws://localhost:8080/ws", "WebSocket URL to connect to")
	flag.StringVar(&outPath, "out", "recorder.csv", "output file path")
	flag.StringVar(&format, "format", "csv", "output format: csv | jsonl")
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

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if durationSec > 0 {
		dctx, dcancel := context.WithTimeout(ctx, time.Duration(durationSec)*time.Second)
		defer dcancel()
		ctx = dctx
	}

	log.Printf("recorder url=%s out=%s format=%s duration=%ds", url, outPath, format, durationSec)

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
		err := runOnce(ctx, url, w, &totalBytes, &totalFrames, quiet)
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
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

	elapsed := time.Since(start)
	log.Printf("summary: runtime=%s frames=%d bytes=%d disconnects=%d",
		elapsed.Round(time.Millisecond),
		atomic.LoadInt64(&totalFrames),
		atomic.LoadInt64(&totalBytes),
		atomic.LoadInt64(&disconnects))
}

func runOnce(ctx context.Context, url string, w recordWriter, totalBytes, totalFrames *int64, quiet bool) error {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	conn, _, err := websocket.Dial(dialCtx, url, nil)
	cancel()
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	conn.SetReadLimit(1 << 20) // 1 MiB; gateway emits small frames
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
