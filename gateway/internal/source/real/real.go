// Package real implements source.Source over a TCP connection to the AMB
// decoder. Reconnect timings are computed by internal/upstream; this package
// owns the actual dial / read loop so the byte path stays in one place.
//
// Behaviour:
//   - On first Read, dial Addr. On dial failure, sleep Backoff.Next(attempt-1)
//     and retry until ctx is cancelled.
//   - Once connected, Read returns the next chunk of bytes. On read error,
//     the connection is closed and the loop reconnects transparently — Read
//     never surfaces transient I/O errors to the caller.
//   - Read returns ctx.Err() when ctx is cancelled.
//
// Tests inject Dial and Sleep to make the loop deterministic.
package real

import (
	"context"
	"net"
	"time"

	"go.uber.org/zap"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/upstream"
)

// DialFunc abstracts net.Dialer.DialContext for tests.
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// SleepFunc abstracts a context-aware sleep for tests.
type SleepFunc func(ctx context.Context, d time.Duration) error

// Source connects to Addr and exposes a Source interface that hides
// reconnects from the caller.
type Source struct {
	Addr    string
	Backoff *upstream.Backoff
	Logger  *zap.Logger
	Dial    DialFunc
	Sleep   SleepFunc

	conn    net.Conn
	attempt int // attempts since last successful connect (0 means "next try is the first")
	buf     []byte
}

// New returns a Source ready to call Read. Logger and Backoff must be non-nil.
func New(addr string, bo *upstream.Backoff, logger *zap.Logger) *Source {
	return &Source{
		Addr:    addr,
		Backoff: bo,
		Logger:  logger,
		Dial:    (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		Sleep:   ctxSleep,
		buf:     make([]byte, 4096),
	}
}

// Read reads the next chunk from upstream, reconnecting transparently
// on errors. Returns ctx.Err() on cancellation.
//
// conn.Read is a blocking syscall that does not natively honor ctx — when
// the upstream is connected but sending no bytes (e.g. AMB with no
// transponders on track), it blocks indefinitely. To make Ctrl+C work in
// that case, a watcher goroutine closes the conn when ctx is cancelled,
// which causes the in-flight Read to return so we can return ctx.Err().
// (Field incident 2026-05-05; see docs/incidents/2026-05-05-recorder-csv-flush.md)
func (s *Source) Read(ctx context.Context) ([]byte, error) {
	for {
		if s.conn == nil {
			if err := s.connect(ctx); err != nil {
				return nil, err
			}
		}
		readDone := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				_ = s.conn.Close()
			case <-readDone:
			}
		}()
		n, err := s.conn.Read(s.buf)
		close(readDone)
		if err != nil {
			if ctx.Err() != nil {
				_ = s.conn.Close()
				s.conn = nil
				return nil, ctx.Err()
			}
			s.Logger.Warn("upstream read error, will reconnect",
				zap.String("addr", s.Addr), zap.Error(err))
			_ = s.conn.Close()
			s.conn = nil
			// loop back to reconnect
			continue
		}
		out := make([]byte, n)
		copy(out, s.buf[:n])
		return out, nil
	}
}

// Close ends the underlying connection if any. Subsequent Read calls will
// re-dial.
func (s *Source) Close() error {
	if s.conn == nil {
		return nil
	}
	err := s.conn.Close()
	s.conn = nil
	return err
}

// connect dials with backoff until it succeeds or ctx is cancelled.
func (s *Source) connect(ctx context.Context) error {
	for {
		if s.attempt > 0 {
			d := s.Backoff.Next(s.attempt - 1)
			s.Logger.Info("upstream reconnect backoff",
				zap.Int("attempt", s.attempt), zap.Duration("wait", d))
			if err := s.Sleep(ctx, d); err != nil {
				return err
			}
		}
		s.attempt++
		s.Logger.Info("dialing upstream", zap.String("addr", s.Addr))
		conn, err := s.Dial(ctx, "tcp", s.Addr)
		if err != nil {
			s.Logger.Warn("upstream dial failed",
				zap.String("addr", s.Addr), zap.Error(err))
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		s.Logger.Info("upstream connected", zap.String("addr", s.Addr))
		s.conn = conn
		s.attempt = 0
		return nil
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
