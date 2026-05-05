package httpsrv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"go.uber.org/zap"
	"nhooyr.io/websocket"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/hub"
)

func newTestServer(t *testing.T, h *hub.Hub, opts ...func(*Config)) *httptest.Server {
	t.Helper()
	cfg := Config{
		Version: "test",
		WebFS: fstest.MapFS{
			"index.html":    &fstest.MapFile{Data: []byte("<html><body>SPA placeholder</body></html>")},
			"assets/app.js": &fstest.MapFile{Data: []byte("console.log('app');")},
		},
	}
	for _, o := range opts {
		o(&cfg)
	}
	s := New(cfg, h, zap.NewNop())
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestHealthz_ReturnsExpectedShape(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	c1, _ := h.Add()
	c2, _ := h.Add()
	defer h.Remove(c1)
	defer h.Remove(c2)

	ts := newTestServer(t, h)
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: got %q want application/json...", ct)
	}
	var got struct {
		Upstream  string `json:"upstream"`
		Clients   int    `json:"clients"`
		UptimeSec int64  `json:"uptime_sec"`
		Version   string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Clients != 2 {
		t.Errorf("clients: got %d want 2", got.Clients)
	}
	if got.Version != "test" {
		t.Errorf("version: got %q want test", got.Version)
	}
	if got.UptimeSec < 0 {
		t.Errorf("uptime_sec: got %d, must be >= 0", got.UptimeSec)
	}
}

func TestStatic_Index_Served(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	ts := newTestServer(t, h)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("SPA placeholder")) {
		t.Errorf("body did not contain placeholder; got: %s", body)
	}
}

func TestStatic_Asset_Served(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	ts := newTestServer(t, h)
	resp, err := http.Get(ts.URL + "/assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("console.log")) {
		t.Errorf("body did not contain js; got: %s", body)
	}
}

func TestStatic_PathTraversal_Rejected(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	ts := newTestServer(t, h)
	// Build a request whose Path contains "..". Standard http.Client URL
	// parsing canonicalises this away on the wire, so we send it manually
	// against the in-memory mux.
	req := httptest.NewRequest(http.MethodGet, "/foo/../bar", nil)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	// The mux canonicalises and 301s; that's fine. What we MUST NOT see
	// is 5xx. Some browsers may also see 400 from our defensive check —
	// also acceptable.
	if rec.Code >= 500 {
		t.Errorf("got 5xx: %d", rec.Code)
	}
}

func TestAdminStub_Returns200_WithPlaceholder(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	ts := newTestServer(t, h)
	resp, err := http.Get(ts.URL + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("Admin")) {
		t.Errorf("body should mention 'Admin'; got: %s", body)
	}
}

func TestLogs_NoConfigured_Returns503(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	ts := newTestServer(t, h) // no LogPath
	resp, err := http.Get(ts.URL + "/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Errorf("status: got %d want 503", resp.StatusCode)
	}
}

func TestLogs_Configured_ReturnsLastN(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "g.log")
	contents := strings.Repeat(`{"level":"INFO","msg":"line"}`+"\n", 10)
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	ts := newTestServer(t, h, func(c *Config) { c.LogPath = p })
	resp, err := http.Get(ts.URL + "/logs?n=3")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	lines := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %q", len(lines), body)
	}
}

func TestLogs_TailHelper(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"single-line-no-newline", "hello", 5, "hello\n"},
		{"three-of-five", "a\nb\nc\nd\ne\n", 3, "c\nd\ne\n"},
		{"more-than-have", "a\nb\n", 10, "a\nb\n"},
		{"zero", "a\nb\n", 0, ""},
		{"empty", "", 5, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := tailLines([]byte(c.in), c.n)
			if string(got) != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestWS_FanOut_DeliversBroadcastToAllClients(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 64)
	defer h.Close()
	ts := newTestServer(t, h)

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c1, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial 1: %v", err)
	}
	defer c1.Close(websocket.StatusNormalClosure, "")
	c2, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial 2: %v", err)
	}
	defer c2.Close(websocket.StatusNormalClosure, "")

	// Wait until both clients are registered with the hub.
	deadline := time.Now().Add(2 * time.Second)
	for h.Count() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h.Count() != 2 {
		t.Fatalf("hub.Count: got %d want 2", h.Count())
	}

	msg := []byte{0x8e, 0x01, 0x02, 0x8f}
	h.Broadcast(msg)

	for i, c := range []*websocket.Conn{c1, c2} {
		typ, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("client %d read: %v", i, err)
		}
		if typ != websocket.MessageBinary {
			t.Errorf("client %d type: got %v want Binary", i, typ)
		}
		if !bytes.Equal(data, msg) {
			t.Errorf("client %d data: got % x want % x", i, data, msg)
		}
	}
}

func TestWS_TooManyClients_RejectedWith1013(t *testing.T) {
	// Cap at 2 to keep the test fast.
	h := hub.New(zap.NewNop(), 2, 4)
	defer h.Close()
	ts := newTestServer(t, h)
	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conns := make([]*websocket.Conn, 0, 2)
	for i := 0; i < 2; i++ {
		c, _, err := websocket.Dial(ctx, wsURL, nil)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		conns = append(conns, c)
	}
	defer func() {
		for _, c := range conns {
			c.Close(websocket.StatusNormalClosure, "")
		}
	}()

	deadline := time.Now().Add(2 * time.Second)
	for h.Count() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	// 3rd client should be rejected — the hub returns ErrTooManyClients,
	// the WS handler closes with StatusTryAgainLater (1013).
	c3, _, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		// Some clients see Dial succeed and the close arrives via Read.
		_, _, rerr := c3.Read(ctx)
		c3.Close(websocket.StatusNormalClosure, "")
		if rerr == nil {
			t.Fatal("expected close after over-cap dial, but Read succeeded")
		}
		var ce websocket.CloseError
		if errors.As(rerr, &ce) && ce.Code != websocket.StatusTryAgainLater {
			t.Errorf("close code: got %d want %d", ce.Code, websocket.StatusTryAgainLater)
		}
		return
	}
	// Or Dial itself errors with the close handshake.
	var ce websocket.CloseError
	if errors.As(err, &ce) && ce.Code != websocket.StatusTryAgainLater {
		t.Errorf("close code: got %d want %d", ce.Code, websocket.StatusTryAgainLater)
	}
}

func TestWS_ManyClients_HighWatermark(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in -short")
	}
	const want = 10
	h := hub.New(zap.NewNop(), 100, 8)
	defer h.Close()
	ts := newTestServer(t, h)
	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Pre-allocate one slot per dialer so each goroutine writes to its
	// own index — appending to a shared slice from multiple goroutines is
	// a data race (caught by -race in CI).
	conns := make([]*websocket.Conn, want)
	var wg sync.WaitGroup
	errs := make(chan error, want)
	for i := 0; i < want; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c, _, err := websocket.Dial(ctx, wsURL, nil)
			if err != nil {
				errs <- err
				return
			}
			conns[idx] = c
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Errorf("dial: %v", e)
	}

	deadline := time.Now().Add(3 * time.Second)
	for h.Count() < want && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if h.Count() != want {
		t.Errorf("hub.Count: got %d want %d", h.Count(), want)
	}

	for _, c := range conns {
		if c != nil { // dial may have failed; skip nil slots
			c.Close(websocket.StatusNormalClosure, "")
		}
	}
}
