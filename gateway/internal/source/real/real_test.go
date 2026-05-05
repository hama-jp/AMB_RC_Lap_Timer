package real

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/upstream"
)

// fakeConn is a net.Conn whose Read returns scripted chunks then EOF or
// the supplied error. Writes are silently consumed.
type fakeConn struct {
	chunks    [][]byte
	idx       int
	readAfter error // returned after all chunks are exhausted (defaults to io.EOF)
	closed    atomic.Bool
}

func (c *fakeConn) Read(p []byte) (int, error) {
	if c.closed.Load() {
		return 0, io.EOF
	}
	if c.idx >= len(c.chunks) {
		if c.readAfter != nil {
			return 0, c.readAfter
		}
		return 0, io.EOF
	}
	n := copy(p, c.chunks[c.idx])
	c.idx++
	return n, nil
}

func (c *fakeConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *fakeConn) Close() error                     { c.closed.Store(true); return nil }
func (c *fakeConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *fakeConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *fakeConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error { return nil }

func newSourceWithFakes(t *testing.T, dial DialFunc) *Source {
	t.Helper()
	return &Source{
		Addr:    "fake:5403",
		Backoff: &upstream.Backoff{Initial: time.Microsecond, Max: time.Microsecond, Jitter: 0, Rand: rand.New(rand.NewSource(1))},
		Logger:  zap.NewNop(),
		Dial:    dial,
		Sleep:   func(ctx context.Context, d time.Duration) error { return nil }, // no real sleep
		buf:     make([]byte, 4096),
	}
}

func TestRead_HappyPath_ReturnsBytes(t *testing.T) {
	conn := &fakeConn{chunks: [][]byte{[]byte("hello"), []byte("world")}}
	s := newSourceWithFakes(t, func(_ context.Context, _, _ string) (net.Conn, error) { return conn, nil })

	got, err := s.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("hello")) {
		t.Errorf("first read: got %q want %q", got, "hello")
	}
	got, err = s.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("world")) {
		t.Errorf("second read: got %q want %q", got, "world")
	}
}

func TestRead_DialFailureThenSuccess_Reconnects(t *testing.T) {
	var dialCount atomic.Int32
	conn := &fakeConn{chunks: [][]byte{[]byte("ok")}}
	dial := func(_ context.Context, _, _ string) (net.Conn, error) {
		n := dialCount.Add(1)
		if n < 3 {
			return nil, errors.New("connection refused")
		}
		return conn, nil
	}
	s := newSourceWithFakes(t, dial)
	got, err := s.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("ok")) {
		t.Errorf("got %q want ok", got)
	}
	if dialCount.Load() != 3 {
		t.Errorf("expected 3 dial attempts, got %d", dialCount.Load())
	}
}

func TestRead_ReadError_ReconnectsTransparently(t *testing.T) {
	// First conn returns 1 chunk then a transient read error. Second conn
	// returns the next chunk cleanly.
	conn1 := &fakeConn{chunks: [][]byte{[]byte("first")}, readAfter: errors.New("connection reset by peer")}
	conn2 := &fakeConn{chunks: [][]byte{[]byte("second")}}
	conns := []*fakeConn{conn1, conn2}
	var idx atomic.Int32
	dial := func(_ context.Context, _, _ string) (net.Conn, error) {
		i := idx.Add(1) - 1
		return conns[i], nil
	}
	s := newSourceWithFakes(t, dial)

	got, _ := s.Read(context.Background())
	if !bytes.Equal(got, []byte("first")) {
		t.Errorf("first read: got %q", got)
	}
	got, _ = s.Read(context.Background())
	if !bytes.Equal(got, []byte("second")) {
		t.Errorf("after-reconnect read: got %q", got)
	}
}

// blockingConn simulates "TCP connected but no bytes flowing" — Read blocks
// until Close is called. Used to exercise ctx cancellation during a stuck
// conn.Read call (field incident 2026-05-05).
type blockingConn struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockingConn() *blockingConn {
	return &blockingConn{closed: make(chan struct{})}
}

func (c *blockingConn) Read(p []byte) (int, error) {
	<-c.closed
	return 0, io.EOF
}
func (c *blockingConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *blockingConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}
func (c *blockingConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *blockingConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *blockingConn) SetDeadline(time.Time) error      { return nil }
func (c *blockingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *blockingConn) SetWriteDeadline(time.Time) error { return nil }

// Regression for the field-2026-05-05 incident: with the upstream connected
// but silent (no transponders on track), conn.Read blocked forever and ctx
// cancellation (Ctrl+C) had no effect. Read must close the conn when ctx
// fires so the syscall unblocks and Read returns ctx.Err().
func TestRead_ContextCanceled_DuringConnRead_ReturnsCtxErr(t *testing.T) {
	conn := newBlockingConn()
	s := newSourceWithFakes(t, func(_ context.Context, _, _ string) (net.Conn, error) {
		return conn, nil
	})

	ctx, cancel := context.WithCancel(context.Background())

	type result struct {
		err error
	}
	ch := make(chan result, 1)
	go func() {
		_, err := s.Read(ctx)
		ch <- result{err}
	}()

	// Let the goroutine enter conn.Read before cancelling.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case r := <-ch:
		if !errors.Is(r.err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read did not return after ctx cancellation — conn.Read still blocked")
	}
}

func TestRead_ContextCanceled_DuringDial_ReturnsCtxErr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dial := func(c context.Context, _, _ string) (net.Conn, error) {
		return nil, c.Err()
	}
	s := newSourceWithFakes(t, dial)
	_, err := s.Read(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestClose_BeforeRead_NoOp(t *testing.T) {
	s := newSourceWithFakes(t, func(_ context.Context, _, _ string) (net.Conn, error) { return nil, errors.New("nope") })
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClose_AfterDial_ClosesUnderlying(t *testing.T) {
	conn := &fakeConn{chunks: [][]byte{[]byte("x")}}
	s := newSourceWithFakes(t, func(_ context.Context, _, _ string) (net.Conn, error) { return conn, nil })
	_, _ = s.Read(context.Background())
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if !conn.closed.Load() {
		t.Errorf("underlying conn was not closed")
	}
}

func TestRead_RealLoopback_Smoke(t *testing.T) {
	// Sanity: spin up a TCP server on :0 so we exercise the production
	// Dial/Sleep paths rather than the test fakes.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no localhost TCP available: %v", err)
	}
	defer ln.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = c.Write([]byte("ping"))
	}()

	src := New(ln.Addr().String(), upstream.NewBackoff(time.Millisecond, time.Millisecond, 0), zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := src.Read(ctx)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !bytes.Equal(got, []byte("ping")) {
		t.Errorf("got %q want ping", got)
	}
	_ = src.Close()
	wg.Wait()
}
