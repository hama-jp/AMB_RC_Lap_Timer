// Package hub fans out upstream byte messages to all connected WebSocket
// clients. It is the gateway's "byte pipe" between the upstream TCP source
// and any number of browser SPAs (docs/architecture.md §3.2).
//
// Backpressure (Issue #27): per-client bounded ring buffer. When a slow
// client's buffer is full, the OLDEST queued message is dropped to make
// room for the new one and a warning is logged. The hub never blocks the
// upstream reader — favour live data over historical completeness.
//
// Capacity (Issue #31): a configurable safety cap rejects new connections
// once the limit is reached (default 100, target 10).
package hub

import (
	"errors"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
)

// Defaults used when a Hub is constructed with zero/negative parameters.
const (
	DefaultMaxClients      = 100
	DefaultClientBufferLen = 64
)

// Errors returned by Hub.Add.
var (
	ErrHubClosed      = errors.New("hub: closed")
	ErrTooManyClients = errors.New("hub: too many clients")
)

// Hub is the fan-out hub. Construct via New, then call Add to register a
// client (typically inside an HTTP /ws handler), Broadcast to enqueue a
// message for every client, and Remove when the client disconnects.
type Hub struct {
	log        *zap.Logger
	maxClients int
	bufferLen  int
	mu         sync.Mutex
	clients    map[*Client]struct{}
	closed     bool
}

// New creates a Hub. log may be nil (a no-op logger is used). Non-positive
// limits fall back to the package defaults.
func New(log *zap.Logger, maxClients, bufferLen int) *Hub {
	if log == nil {
		log = zap.NewNop()
	}
	if maxClients <= 0 {
		maxClients = DefaultMaxClients
	}
	if bufferLen <= 0 {
		bufferLen = DefaultClientBufferLen
	}
	return &Hub{
		log:        log,
		maxClients: maxClients,
		bufferLen:  bufferLen,
		clients:    make(map[*Client]struct{}),
	}
}

// Client is a single subscriber. Use Recv to obtain the channel that yields
// fan-out messages and Done to learn when the hub has removed this client.
type Client struct {
	hub       *Hub
	send      chan []byte
	done      chan struct{}
	closeOnce sync.Once
	dropped   int64
}

// Recv returns the read end of the per-client send queue. The WS writer
// goroutine should select on this and Done().
func (c *Client) Recv() <-chan []byte { return c.send }

// Done is closed when the hub removes this client.
func (c *Client) Done() <-chan struct{} { return c.done }

// Dropped returns the running total of messages discarded due to slow
// consumer. Useful in tests and /healthz reporting.
func (c *Client) Dropped() int64 { return atomic.LoadInt64(&c.dropped) }

// Add registers a new Client. Returns ErrTooManyClients when the safety
// cap is reached or ErrHubClosed after Close.
func (h *Hub) Add() (*Client, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrHubClosed
	}
	if len(h.clients) >= h.maxClients {
		return nil, ErrTooManyClients
	}
	c := &Client{
		hub:  h,
		send: make(chan []byte, h.bufferLen),
		done: make(chan struct{}),
	}
	h.clients[c] = struct{}{}
	return c, nil
}

// Remove deregisters a Client. Idempotent: it is safe to call multiple
// times (e.g., from defer in the WS handler and again from Hub.Close).
func (h *Hub) Remove(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
	}
	h.mu.Unlock()
	c.closeDone()
}

// Broadcast enqueues msg for every currently registered client. The call
// is non-blocking — for any client whose buffer is full, the oldest
// queued message is dropped to make room.
//
// `msg` must remain valid for the duration of the call (we do not copy
// it ourselves; the caller may share a slice across all clients).
func (h *Hub) Broadcast(msg []byte) {
	h.mu.Lock()
	cs := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		cs = append(cs, c)
	}
	h.mu.Unlock()
	for _, c := range cs {
		c.send1(msg, h.log)
	}
}

// Count returns the number of currently registered clients.
func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// Close removes every client and prevents future Adds. Safe to call once
// per Hub; subsequent calls are no-ops.
func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	cs := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		cs = append(cs, c)
	}
	h.clients = nil
	h.mu.Unlock()
	for _, c := range cs {
		c.closeDone()
	}
}

func (c *Client) send1(msg []byte, log *zap.Logger) {
	// Skip clients that have already been removed; saves work and avoids
	// piling up messages in a channel nobody is reading.
	select {
	case <-c.done:
		return
	default:
	}
	select {
	case c.send <- msg:
		return
	default:
	}
	// Buffer full — drop the OLDEST and try once more.
	select {
	case <-c.send:
	default:
	}
	select {
	case c.send <- msg:
	default:
		// Should not happen given we just drained one slot, but log if so.
	}
	n := atomic.AddInt64(&c.dropped, 1)
	// Log on first drop and then once every 64 to avoid log floods.
	if n == 1 || n%64 == 0 {
		log.Warn("ws client slow, dropping old frames",
			zap.Int64("total_dropped", n))
	}
}

func (c *Client) closeDone() {
	c.closeOnce.Do(func() { close(c.done) })
}
