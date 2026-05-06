// Command tcp-emitter is a Mock AMB-side TCP server for field-test scenarios.
//
// It accepts TCP connections (one per gateway under test) and pushes
// wire-encoded P3 PASSING records at a configurable cadence, optionally
// interleaved with STATUS frames. Frames are valid enough that the gateway's
// upstream reader, the WS fan-out path and the SPA parser all behave as if a
// real decoder were on the other end (docs/test-strategy.md §6.4).
//
// This is NOT a substitute for the gateway's built-in --mock source; that one
// runs in-process. This binary is designed to live on a separate LAN host so
// the gateway exercises real socket reconnect behaviour over WiFi.
//
// Example:
//
//	go run . --port 5403 --interval-ms 1500 --ponders 1,2,3
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hama-jp/AMB_RC_Lap_Timer/tools/fieldtest/internal/frame"
)

type config struct {
	Port        int     `json:"port"`
	IntervalMs  int     `json:"interval_ms"`
	JitterRatio float64 `json:"jitter_ratio"`
	Ponders     string  `json:"ponders"`
	DecoderID   uint32  `json:"decoder_id"`
	StatusEvery int     `json:"status_every"`
	Seed        int64   `json:"seed"`
}

func defaultConfig() config {
	return config{
		Port:        5403,
		IntervalMs:  1500,
		JitterRatio: 0.5,
		Ponders:     "1,2,3",
		DecoderID:   0x00041D17,
		StatusEvery: 30,
		Seed:        0,
	}
}

func main() {
	cfg := defaultConfig()

	var configPath string
	flag.IntVar(&cfg.Port, "port", cfg.Port, "TCP listen port (AMB default 5403)")
	flag.IntVar(&cfg.IntervalMs, "interval-ms", cfg.IntervalMs, "mean inter-frame interval in ms")
	flag.Float64Var(&cfg.JitterRatio, "jitter-ratio", cfg.JitterRatio, "uniform jitter as ratio of interval (e.g. 0.5 = ±50%)")
	flag.StringVar(&cfg.Ponders, "ponders", cfg.Ponders, "comma-separated transponder IDs (decimal or 0x..)")
	flag.Func("decoder-id", "DECODER_ID value (decimal or 0x..); default 0x00041D17", func(s string) error {
		v, err := parseUint32(s)
		if err != nil {
			return err
		}
		cfg.DecoderID = v
		return nil
	})
	flag.IntVar(&cfg.StatusEvery, "status-every", cfg.StatusEvery, "emit one STATUS frame every N PASSING frames (0 disables)")
	flag.Int64Var(&cfg.Seed, "seed", cfg.Seed, "RNG seed; 0 means time-based")
	flag.StringVar(&configPath, "config", "", "optional JSON config (overlaid on top of flags)")
	flag.Parse()

	if configPath != "" {
		if err := loadConfig(configPath, &cfg); err != nil {
			log.Fatalf("config: %v", err)
		}
	}

	ponders, err := parsePonders(cfg.Ponders)
	if err != nil {
		log.Fatalf("ponders: %v", err)
	}
	if len(ponders) == 0 {
		log.Fatalf("ponders: at least one ID required")
	}
	if cfg.IntervalMs <= 0 {
		log.Fatalf("interval-ms must be > 0")
	}
	if cfg.JitterRatio < 0 || cfg.JitterRatio >= 1 {
		log.Fatalf("jitter-ratio must be in [0, 1)")
	}

	addr := fmt.Sprintf(":%d", cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}
	defer ln.Close()
	log.Printf("listening on %s — ponders=%v interval=%dms ±%.0f%% status-every=%d",
		addr, ponders, cfg.IntervalMs, cfg.JitterRatio*100, cfg.StatusEvery)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	var clientID uint64
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				log.Printf("shutdown")
				return
			}
			log.Printf("accept: %v", err)
			continue
		}
		id := atomic.AddUint64(&clientID, 1)
		go serve(ctx, conn, id, ponders, cfg)
	}
}

func serve(ctx context.Context, conn net.Conn, id uint64, ponders []uint32, cfg config) {
	remote := conn.RemoteAddr().String()
	log.Printf("client %d connected from %s", id, remote)
	defer func() {
		conn.Close()
		log.Printf("client %d disconnected (%s)", id, remote)
	}()

	seed := cfg.Seed
	if seed == 0 {
		seed = time.Now().UnixNano() ^ int64(id)
	}
	r := rand.New(rand.NewSource(seed))

	start := time.Now()
	ponderIdx := 0
	var passingCounter uint32 = 0x001185E3 // mirror captured PASSING_NUMBER baseline
	frameCount := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		dur := jitteredInterval(r, time.Duration(cfg.IntervalMs)*time.Millisecond, cfg.JitterRatio)
		select {
		case <-ctx.Done():
			return
		case <-time.After(dur):
		}

		var payload []byte
		if cfg.StatusEvery > 0 && frameCount > 0 && frameCount%cfg.StatusEvery == 0 {
			payload = frame.BuildStatus(frame.StatusArgs{
				Noise:        0x0006,
				Temperature:  0x001B,
				InputVoltage: 0x79,
				GPS:          0x00,
				DecoderID:    cfg.DecoderID,
			})
		} else {
			elapsed := time.Since(start)
			payload = frame.BuildPassing(frame.PassingArgs{
				PassingNumber: passingCounter,
				Transponder:   ponders[ponderIdx],
				RTCTimeUs:     uint64(elapsed.Microseconds()),
				Strength:      uint16(160 + r.Intn(15)),
				Hits:          uint16(120 + r.Intn(170)),
				Flags:         0,
				DecoderID:     cfg.DecoderID,
			})
			passingCounter++
			ponderIdx = (ponderIdx + 1) % len(ponders)
		}
		frameCount++

		if err := writeAll(conn, payload); err != nil {
			log.Printf("client %d write: %v", id, err)
			return
		}
	}
}

func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}

func jitteredInterval(r *rand.Rand, base time.Duration, jitter float64) time.Duration {
	if jitter <= 0 {
		return base
	}
	delta := (r.Float64()*2 - 1) * jitter // [-jitter, +jitter)
	d := time.Duration(float64(base) * (1 + delta))
	if d < 0 {
		return 0
	}
	return d
}

func parsePonders(s string) ([]uint32, error) {
	parts := strings.Split(s, ",")
	out := make([]uint32, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := parseUint32(p)
		if err != nil {
			return nil, fmt.Errorf("bad ponder %q: %w", p, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func parseUint32(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	base := 10
	if strings.HasPrefix(strings.ToLower(s), "0x") {
		s = s[2:]
		base = 16
	}
	v, err := strconv.ParseUint(s, base, 32)
	if err != nil {
		return 0, err
	}
	return uint32(v), nil
}

func loadConfig(path string, cfg *config) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	return dec.Decode(cfg)
}
