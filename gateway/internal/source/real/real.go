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
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/upstream"
)

// DialFunc abstracts net.Dialer.DialContext for tests.
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// SleepFunc abstracts a context-aware sleep for tests.
type SleepFunc func(ctx context.Context, d time.Duration) error

// Source connects to a TCP upstream and exposes a stable byte stream that
// hides reconnects from the caller.
//
// Addr and Backoff are *initial* values supplied at construction. Once Read
// has been called they must only be mutated through ApplyUpstream and
// ApplyBackoff respectively, which take the same mutex the Read loop uses
// to read them; admin /api/config relies on this contract
// (docs/architecture.md §3.5.5).
type Source struct {
	Logger *zap.Logger
	Dial   DialFunc
	Sleep  SleepFunc

	mu      sync.Mutex
	addr    string
	backoff *upstream.Backoff
	conn    net.Conn
	attempt int // attempts since last successful connect (0 = next try is first)
	buf     []byte
}

// recvBufSize is the per-Read scratch buffer. Aligned with the
// docs/protocol-p3.md §1 "受信バッファ目安: 10240 bytes" and the reference
// implementation `AmbP3/decoder.py`'s recv(10240). TCP itself does the
// chunking; this size is just how much we let conn.Read return at once.
const recvBufSize = 10240

// New returns a Source ready to call Read. Logger and Backoff must be non-nil.
func New(addr string, bo *upstream.Backoff, logger *zap.Logger) *Source {
	return &Source{
		Logger:  logger,
		Dial:    (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		Sleep:   ctxSleep,
		addr:    addr,
		backoff: bo,
		buf:     make([]byte, recvBufSize),
	}
}

// Addr returns the current upstream "host:port". Live-updated by
// ApplyUpstream — never read the addr field directly outside of methods
// that already hold the mutex.
func (s *Source) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

// Backoff returns the current backoff strategy. Live-updated by
// ApplyBackoff. Returned pointer is valid for reads (its own internal
// state is concurrency-safe enough for the read-mostly access pattern of
// connect()), but callers must not mutate it.
func (s *Source) Backoff() *upstream.Backoff {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.backoff
}

// ApplyUpstream swaps the dial target and forces the Read loop to drop the
// current connection. The next Read attempt will dial the new host:port.
// Safe to call concurrently with Read; the watcher goroutine in Read holds
// a local copy of the old conn so it stays valid for its own Close call
// (docs/architecture.md §3.5.5; PR #80 race fix).
func (s *Source) ApplyUpstream(host string, port int) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	s.mu.Lock()
	s.addr = addr
	conn := s.conn
	s.conn = nil
	s.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

// ApplyBackoff replaces the reconnect strategy. New parameters take effect
// from the next reconnect attempt; an in-flight backoff sleep is unaffected
// (it ends on its own, then connect() calls Backoff() to read fresh values).
func (s *Source) ApplyBackoff(initial, max time.Duration, jitter float64) {
	nb := upstream.NewBackoff(initial, max, jitter)
	s.mu.Lock()
	s.backoff = nb
	s.mu.Unlock()
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
//
// The watcher must hold a local copy of the conn — once Read returns, the
// parent may set s.conn = nil and re-dial. If the watcher then woke on a
// late ctx cancellation it would otherwise dereference nil (Issue #40,
// originally surfaced by go test -race in CI on Linux).
func (s *Source) Read(ctx context.Context) ([]byte, error) {
	for {
		s.mu.Lock()
		conn := s.conn
		s.mu.Unlock()
		if conn == nil {
			if err := s.connect(ctx); err != nil {
				return nil, err
			}
			s.mu.Lock()
			conn = s.conn
			s.mu.Unlock()
			if conn == nil {
				// connect() returned nil error but ApplyUpstream raced and
				// nilled the conn between the two locks; loop and re-dial.
				continue
			}
		}
		readDone := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				_ = conn.Close()
			case <-readDone:
			}
		}()
		n, err := conn.Read(s.buf)
		close(readDone)
		if err != nil {
			if ctx.Err() != nil {
				_ = conn.Close()
				s.mu.Lock()
				if s.conn == conn {
					s.conn = nil
				}
				s.mu.Unlock()
				return nil, ctx.Err()
			}
			s.Logger.Warn("upstream read error, will reconnect",
				zap.String("addr", s.Addr()), zap.Error(err))
			_ = conn.Close()
			s.mu.Lock()
			if s.conn == conn {
				s.conn = nil
			}
			s.mu.Unlock()
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
	s.mu.Lock()
	conn := s.conn
	s.conn = nil
	s.mu.Unlock()
	if conn == nil {
		return nil
	}
	return conn.Close()
}

// connect dials with backoff until it succeeds or ctx is cancelled.
func (s *Source) connect(ctx context.Context) error {
	for {
		s.mu.Lock()
		attempt := s.attempt
		addr := s.addr
		bo := s.backoff
		s.mu.Unlock()

		if attempt > 0 {
			d := bo.Next(attempt - 1)
			s.Logger.Info("upstream reconnect backoff",
				zap.Int("attempt", attempt), zap.Duration("wait", d))
			if err := s.Sleep(ctx, d); err != nil {
				return err
			}
		}

		s.mu.Lock()
		s.attempt++
		// Re-read addr after the sleep — ApplyUpstream may have rotated it.
		addr = s.addr
		s.mu.Unlock()

		s.Logger.Info("dialing upstream", zap.String("addr", addr))
		conn, err := s.Dial(ctx, "tcp", addr)
		if err != nil {
			s.Logger.Warn("upstream dial failed",
				zap.String("addr", addr), zap.Error(err))
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}

		s.mu.Lock()
		s.Logger.Info("upstream connected", zap.String("addr", addr))
		s.conn = conn
		s.attempt = 0
		s.mu.Unlock()
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
