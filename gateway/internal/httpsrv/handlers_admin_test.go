package httpsrv

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/config"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/hub"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/logging"
)

// goodCfg is the canonical valid config used across handler tests.
func goodCfg() config.Config {
	c := config.Defaults()
	c.Logging.Dir = "./logs"
	c.Records.Dir = "./records"
	return c
}

// adminTestServer wires everything the /admin/api/config handler depends
// on: a real Server, a real audit writer (to a temp dir), the test
// passphrase, and an initial admin config state pointing at <tempdir>/cfg.json.
type adminTestServer struct {
	srv        *httptest.Server
	cfgPath    string
	auditPath  string
	upstreamCh chan [2]any
	hubLimits  chan [2]int
	reconnects chan [3]any
}

func newAdminTestServer(t *testing.T, initial config.Config) *adminTestServer {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	audit, err := logging.NewAuditWriter(logging.AuditOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = audit.Close() })

	h := hub.New(zap.NewNop(), 10, 4)
	t.Cleanup(h.Close)

	cfg := Config{
		Version:         "test",
		AdminPassphrase: testPassphrase,
		AdminAudit:      audit,
	}
	s := New(cfg, h, zap.NewNop())

	upstreamCh := make(chan [2]any, 4)
	hubLimits := make(chan [2]int, 4)
	reconnects := make(chan [3]any, 4)
	hooks := ApplyHooks{
		Upstream: func(host string, port int) {
			upstreamCh <- [2]any{host, port}
		},
		HubLimits: func(max, buf int) {
			hubLimits <- [2]int{max, buf}
		},
		Reconnect: func(initialMs, maxMs int, jr float64) {
			reconnects <- [3]any{initialMs, maxMs, jr}
		},
	}
	// Tests don't need a distinct Resolved view — they exercise GET / POST /
	// hooks which read non-path fields. Pass initial as both raw and
	// resolved; the regression tests below validate the Raw path explicitly.
	s.SetAdminConfigState(initial, initial, cfgPath, hooks)

	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return &adminTestServer{
		srv:        ts,
		cfgPath:    cfgPath,
		auditPath:  filepath.Join(dir, "admin-audit.log"),
		upstreamCh: upstreamCh,
		hubLimits:  hubLimits,
		reconnects: reconnects,
	}
}

func TestAdminConfigGet_AuthRequired(t *testing.T) {
	at := newAdminTestServer(t, goodCfg())
	resp, err := http.Get(at.srv.URL + "/admin/api/config")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("status: got %d want 401", resp.StatusCode)
	}
}

func TestAdminConfigGet_Authenticated_ReturnsCurrent(t *testing.T) {
	at := newAdminTestServer(t, goodCfg())
	c := adminLogin(t, at.srv)

	resp, err := c.Get(at.srv.URL + "/admin/api/config")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	var got config.Config
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Upstream.Host != goodCfg().Upstream.Host {
		t.Errorf("upstream.host: got %q want %q", got.Upstream.Host, goodCfg().Upstream.Host)
	}
}

func TestAdminConfigPost_Validation_Returns400AndKeepsFile(t *testing.T) {
	at := newAdminTestServer(t, goodCfg())
	c := adminLogin(t, at.srv)

	bad := goodCfg()
	bad.Upstream.Port = 99999 // out of range

	body, _ := json.Marshal(bad)
	req, _ := http.NewRequest(http.MethodPost, at.srv.URL+"/admin/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d want 400", resp.StatusCode)
	}
	var payload map[string][]config.ValidationError
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	if len(payload["errors"]) == 0 {
		t.Errorf("expected errors[], got %v", payload)
	}
	// Atomic-write must NOT have happened (file should still be missing).
	if _, err := os.Stat(at.cfgPath); !os.IsNotExist(err) {
		t.Errorf("config file unexpectedly exists after validation failure: err=%v", err)
	}
}

func TestAdminConfigPost_UnknownField_Returns400(t *testing.T) {
	at := newAdminTestServer(t, goodCfg())
	c := adminLogin(t, at.srv)
	body := strings.NewReader(`{"upstream":{"host":"x","port":1},"unknown":"x"}`)
	req, _ := http.NewRequest(http.MethodPost, at.srv.URL+"/admin/api/config", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("status: got %d want 400", resp.StatusCode)
	}
}

func TestAdminConfigPost_HappyPath_WritesAppliesAuditsAndResponds(t *testing.T) {
	at := newAdminTestServer(t, goodCfg())
	c := adminLogin(t, at.srv)

	updated := goodCfg()
	updated.Upstream.Host = "10.0.0.5"
	updated.Upstream.Port = 5403 // unchanged
	updated.Server.MaxClients = 50

	body, _ := json.Marshal(updated)
	req, _ := http.NewRequest(http.MethodPost, at.srv.URL+"/admin/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d want 200, body=%s", resp.StatusCode, b)
	}
	var got adminConfigPostResp
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	wantApplied := []string{"server.max_clients", "upstream.host"}
	if !reflectEqual(got.Applied, wantApplied) {
		t.Errorf("applied: got %v want %v", got.Applied, wantApplied)
	}
	if len(got.RequiresRestart) != 0 {
		t.Errorf("requires_restart: got %v want []", got.RequiresRestart)
	}

	// Atomic write happened.
	written, err := os.ReadFile(at.cfgPath)
	if err != nil {
		t.Fatalf("config file: %v", err)
	}
	if !bytes.Contains(written, []byte("10.0.0.5")) {
		t.Errorf("config.json missing new host; got: %s", written)
	}

	// Apply hooks fired with the right values.
	select {
	case got := <-at.upstreamCh:
		if got != [2]any{"10.0.0.5", 5403} {
			t.Errorf("Upstream hook args: got %v", got)
		}
	default:
		t.Errorf("Upstream hook not fired")
	}
	select {
	case got := <-at.hubLimits:
		if got != [2]int{50, goodCfg().Server.ClientBufferLen} {
			t.Errorf("HubLimits hook args: got %v", got)
		}
	default:
		t.Errorf("HubLimits hook not fired")
	}

	// Audit log got a "changed N field(s)" line.
	auditBytes, err := os.ReadFile(at.auditPath)
	if err != nil {
		t.Fatalf("audit log: %v", err)
	}
	if !bytes.Contains(auditBytes, []byte("changed 2 field(s)")) {
		t.Errorf("audit log missing changed line; got: %s", auditBytes)
	}
}

func TestAdminConfigPost_RestartFieldsBucketed(t *testing.T) {
	at := newAdminTestServer(t, goodCfg())
	c := adminLogin(t, at.srv)

	updated := goodCfg()
	updated.Listen = ":9090"
	updated.Logging.MaxSizeMB = 10

	body, _ := json.Marshal(updated)
	req, _ := http.NewRequest(http.MethodPost, at.srv.URL+"/admin/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var got adminConfigPostResp
	_ = json.NewDecoder(resp.Body).Decode(&got)
	want := []string{"listen", "logging.max_size_mb"}
	if !reflectEqual(got.RequiresRestart, want) {
		t.Errorf("requires_restart: got %v want %v", got.RequiresRestart, want)
	}
	if len(got.Applied) != 0 {
		t.Errorf("applied: got %v want []", got.Applied)
	}
}

func TestAdminConfigPost_NoChanges_HooksNotFired(t *testing.T) {
	at := newAdminTestServer(t, goodCfg())
	c := adminLogin(t, at.srv)

	body, _ := json.Marshal(goodCfg())
	req, _ := http.NewRequest(http.MethodPost, at.srv.URL+"/admin/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var got adminConfigPostResp
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if len(got.Applied) != 0 || len(got.RequiresRestart) != 0 {
		t.Errorf("expected empty buckets, got applied=%v restart=%v",
			got.Applied, got.RequiresRestart)
	}
	if len(at.upstreamCh) != 0 || len(at.hubLimits) != 0 || len(at.reconnects) != 0 {
		t.Errorf("hooks fired without diff: u=%d h=%d r=%d",
			len(at.upstreamCh), len(at.hubLimits), len(at.reconnects))
	}
}

// Regression for Issue #101: a successful POST must round-trip
// `logging.dir` / `records.dir` as the relative strings the operator
// loaded, NOT as the baseDir-resolved absolute paths. Pre-fix, the
// /admin handler was given the resolved Config and wrote it back,
// baking C:\... into config.json and breaking USB portability.
func TestAdminConfigPost_PreservesRelativePaths(t *testing.T) {
	at := newAdminTestServer(t, goodCfg())
	c := adminLogin(t, at.srv)

	// Change a non-path field; logging.dir / records.dir stay at the
	// relative defaults (./logs / ./records).
	updated := goodCfg()
	updated.Upstream.Host = "10.0.0.42"

	body, _ := json.Marshal(updated)
	req, _ := http.NewRequest(http.MethodPost, at.srv.URL+"/admin/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d want 200, body=%s", resp.StatusCode, b)
	}

	written, err := os.ReadFile(at.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var saved config.Config
	if err := json.Unmarshal(written, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Logging.Dir != "./logs" {
		t.Errorf("logging.dir baked: got %q want %q", saved.Logging.Dir, "./logs")
	}
	if saved.Records.Dir != "./records" {
		t.Errorf("records.dir baked: got %q want %q", saved.Records.Dir, "./records")
	}
}

// Regression for Issue #101: GET must return the as-loaded view (Raw),
// not the runtime Resolved view. Otherwise the SPA reads absolute paths,
// echoes them on POST, and config.json gets baked.
func TestAdminConfigGet_ReturnsRawNotResolved(t *testing.T) {
	dir := t.TempDir()
	audit, err := logging.NewAuditWriter(logging.AuditOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = audit.Close() })
	h := hub.New(zap.NewNop(), 10, 4)
	t.Cleanup(h.Close)

	raw := goodCfg() // ./logs, ./records
	resolved := raw  // simulate ResolvePaths having moved them under a fake baseDir
	resolved.Logging.Dir = `C:\fake\base\logs`
	resolved.Records.Dir = `C:\fake\base\records`

	cfg := Config{Version: "test", AdminPassphrase: testPassphrase, AdminAudit: audit}
	s := New(cfg, h, zap.NewNop())
	s.SetAdminConfigState(raw, resolved, filepath.Join(dir, "config.json"), ApplyHooks{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	client := adminLogin(t, ts)
	resp, err := client.Get(ts.URL + "/admin/api/config")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got config.Config
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Logging.Dir != "./logs" {
		t.Errorf("GET returned resolved logging.dir: got %q want %q",
			got.Logging.Dir, "./logs")
	}
	if got.Records.Dir != "./records" {
		t.Errorf("GET returned resolved records.dir: got %q want %q",
			got.Records.Dir, "./records")
	}
}

func TestAdminConfigGet_StateNotInitialized_503(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	cfg := Config{Version: "test", AdminPassphrase: testPassphrase}
	s := New(cfg, h, zap.NewNop())
	// Skip SetAdminConfigState so adminConfig is still nil.
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	c := adminLogin(t, ts)
	resp, err := c.Get(ts.URL + "/admin/api/config")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Errorf("status: got %d want 503", resp.StatusCode)
	}
}

func TestWriteConfigAtomic_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	c := goodCfg()
	c.Upstream.Host = "10.1.1.1"
	if err := writeConfigAtomic(path, c); err != nil {
		t.Fatal(err)
	}
	// No leftover .tmp.* siblings.
	matches, _ := filepath.Glob(filepath.Join(dir, "*.tmp.*"))
	if len(matches) > 0 {
		t.Errorf("temp file not cleaned up: %v", matches)
	}
	got, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Upstream.Host != "10.1.1.1" {
		t.Errorf("round-trip host: got %q", got.Upstream.Host)
	}
}

// reflectEqual compares two []string for value equality.
func reflectEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// init-time sanity: keep configState's atomic.Pointer reachable via a no-op
// reference so a future Server-struct rename in server.go fails to compile
// here too rather than only at handler-call time.
var _ atomic.Pointer[configState]
