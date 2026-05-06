package hub

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestAdd_BelowLimit_Succeeds(t *testing.T) {
	h := New(zap.NewNop(), 10, 4)
	defer h.Close()
	for i := 0; i < 10; i++ {
		if _, err := h.Add(); err != nil {
			t.Fatalf("Add #%d: %v", i, err)
		}
	}
	if got := h.Count(); got != 10 {
		t.Errorf("Count: got %d want 10", got)
	}
}

func TestAdd_PastLimit_ReturnsErrTooManyClients(t *testing.T) {
	h := New(zap.NewNop(), 100, 4)
	defer h.Close()
	for i := 0; i < 100; i++ {
		if _, err := h.Add(); err != nil {
			t.Fatalf("Add #%d: %v", i, err)
		}
	}
	_, err := h.Add()
	if !errors.Is(err, ErrTooManyClients) {
		t.Errorf("expected ErrTooManyClients, got %v", err)
	}
}

func TestAdd_AfterClose_ReturnsErrHubClosed(t *testing.T) {
	h := New(zap.NewNop(), 10, 4)
	h.Close()
	_, err := h.Add()
	if !errors.Is(err, ErrHubClosed) {
		t.Errorf("expected ErrHubClosed, got %v", err)
	}
}

func TestBroadcast_DeliversToAllClients(t *testing.T) {
	h := New(zap.NewNop(), 10, 4)
	defer h.Close()
	c1, _ := h.Add()
	c2, _ := h.Add()
	c3, _ := h.Add()

	msg := []byte("hello")
	h.Broadcast(msg)

	for i, c := range []*Client{c1, c2, c3} {
		select {
		case got := <-c.Recv():
			if !bytes.Equal(got, msg) {
				t.Errorf("client %d got %q want %q", i, got, msg)
			}
		case <-time.After(time.Second):
			t.Errorf("client %d did not receive", i)
		}
	}
}

func TestBroadcast_SlowClient_DropsOldestNotNew(t *testing.T) {
	logger := zap.NewNop()
	h := New(logger, 10, 2) // very small buffer
	defer h.Close()

	c, _ := h.Add()

	// Fill the buffer with 5 messages without reading any. Buffer holds 2;
	// the slow-client policy must drop oldest, so the final state should be
	// the LAST 2 messages.
	h.Broadcast([]byte("a"))
	h.Broadcast([]byte("b"))
	h.Broadcast([]byte("c"))
	h.Broadcast([]byte("d"))
	h.Broadcast([]byte("e"))

	got := make([][]byte, 0, 2)
	for i := 0; i < 2; i++ {
		select {
		case m := <-c.Recv():
			got = append(got, m)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("expected 2 buffered messages, got %d", len(got))
		}
	}

	want := [][]byte{[]byte("d"), []byte("e")}
	for i, w := range want {
		if !bytes.Equal(got[i], w) {
			t.Errorf("buffered[%d] = %q want %q", i, got[i], w)
		}
	}

	if dropped := c.Dropped(); dropped < 3 {
		t.Errorf("dropped counter: got %d want >= 3", dropped)
	}
}

func TestBroadcast_OneSlowClient_DoesNotStallOthers(t *testing.T) {
	// Big enough so the fast client doesn't drop messages of its own.
	h := New(zap.NewNop(), 10, 128)
	defer h.Close()

	slow, _ := h.Add()
	fast, _ := h.Add()

	// Run a fast consumer that reads everything.
	got := make(chan []byte, 200)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case msg := <-fast.Recv():
				got <- msg
			case <-fast.Done():
				return
			}
		}
	}()

	// Slow client never reads. Time 50 broadcasts — the slow client must
	// not stall them. With 128-deep buffers, broadcast is non-blocking
	// even when one client never drains.
	t0 := time.Now()
	for i := 0; i < 50; i++ {
		h.Broadcast([]byte{byte(i)})
	}
	if d := time.Since(t0); d > 500*time.Millisecond {
		t.Errorf("50 broadcasts took %v — slow client appears to be blocking", d)
	}

	deadline := time.After(2 * time.Second)
	received := 0
	for received < 50 {
		select {
		case <-got:
			received++
		case <-deadline:
			t.Fatalf("fast client only got %d of 50 messages", received)
		}
	}
	_ = slow
	h.Remove(fast)
	wg.Wait()
}

func TestRemove_ClosesDone_AndIsIdempotent(t *testing.T) {
	h := New(zap.NewNop(), 10, 4)
	defer h.Close()
	c, _ := h.Add()
	h.Remove(c)
	select {
	case <-c.Done():
	case <-time.After(time.Second):
		t.Fatal("Done not closed after Remove")
	}
	// Second Remove must not panic.
	h.Remove(c)
}

func TestClose_RemovesAllClients_AndDoneFires(t *testing.T) {
	h := New(zap.NewNop(), 10, 4)
	c1, _ := h.Add()
	c2, _ := h.Add()
	h.Close()

	for i, c := range []*Client{c1, c2} {
		select {
		case <-c.Done():
		case <-time.After(time.Second):
			t.Errorf("client %d Done not fired after Close", i)
		}
	}
	if got := h.Count(); got != 0 {
		t.Errorf("Count after Close: got %d want 0", got)
	}
}

func TestBroadcast_ToRemovedClient_DoesNotPanic(t *testing.T) {
	h := New(zap.NewNop(), 10, 1)
	c, _ := h.Add()
	h.Remove(c)
	// Should be a no-op.
	h.Broadcast([]byte("ignored"))
	select {
	case msg := <-c.Recv():
		t.Errorf("removed client received %q (expected no message)", msg)
	default:
	}
}

func TestSetLimits_AppliesToNextAdd(t *testing.T) {
	// Start with a 2-client cap; the third Add must fail.
	h := New(zap.NewNop(), 2, 4)
	defer h.Close()
	if _, err := h.Add(); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Add(); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Add(); !errors.Is(err, ErrTooManyClients) {
		t.Fatalf("3rd Add: got %v want ErrTooManyClients", err)
	}

	// Raise the cap; 3rd Add now succeeds.
	h.SetLimits(5, 8)
	max, buf := h.Limits()
	if max != 5 || buf != 8 {
		t.Errorf("Limits after SetLimits: got (%d,%d) want (5,8)", max, buf)
	}
	if _, err := h.Add(); err != nil {
		t.Errorf("3rd Add post-raise: %v", err)
	}
}

func TestSetLimits_NonPositive_FallsBackToDefaults(t *testing.T) {
	h := New(zap.NewNop(), 10, 4)
	defer h.Close()
	h.SetLimits(0, -1)
	max, buf := h.Limits()
	if max != DefaultMaxClients || buf != DefaultClientBufferLen {
		t.Errorf("Limits: got (%d,%d) want (%d,%d)",
			max, buf, DefaultMaxClients, DefaultClientBufferLen)
	}
}
