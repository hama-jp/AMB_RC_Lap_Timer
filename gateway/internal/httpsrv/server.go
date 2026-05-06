// Package httpsrv hosts the gateway's HTTP/WebSocket surface
// (docs/architecture.md §3.1):
//
//	GET  /             SPA bundle (index.html / assets, served from
//	                   gateway/internal/webassets/dist via go:embed)
//	GET  /assets/*     same as above (the FileServer handles it under /)
//	GET  /healthz      JSON: { upstream, clients, uptime_sec, version }
//	GET  /admin        seat-saver stub HTML — the real WebUI is Issue #8
//	GET  /logs         tail of the gateway log file (ndjson)
//	GET  /ws           WebSocket fan-out via internal/hub
//
// The package keeps its dependencies minimal: standard library plus
// nhooyr.io/websocket for the upgrade. Routing uses http.ServeMux.
package httpsrv

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/hub"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/logging"
)

// UpstreamState is a label the /healthz handler reports. The gateway main
// loop updates it via Server.SetUpstreamState. Values are documentation
// strings, not enums, so future modes (replay-finished etc.) cost nothing.
type UpstreamState string

const (
	UpstreamConnecting UpstreamState = "connecting"
	UpstreamConnected  UpstreamState = "connected"
	UpstreamReplay     UpstreamState = "replay"
	UpstreamMock       UpstreamState = "mock"
	UpstreamFinished   UpstreamState = "finished"
)

// Config is everything Server needs to know up front.
type Config struct {
	// Addr is the bind string (e.g. ":8080") — passed straight to net.Listen.
	Addr string
	// Version is reported in /healthz and the /admin stub.
	Version string
	// WebFS is the filesystem rooted at the SPA's dist/. Tests inject a
	// fake here; production callers pass the result of webassets.FS().
	WebFS fs.FS
	// LogPath is the gateway's current log file (rotated by lumberjack).
	// /logs returns the tail of this file. Empty disables /logs.
	LogPath string
	// MaxLogTailLines bounds the /logs response. 0 = use default.
	MaxLogTailLines int
	// AdminPassphrase is the one-time passphrase that grants /admin access.
	// Generated once per process at startup (see cmd/gateway). Empty
	// disables /admin auth — only test code should leave this empty.
	AdminPassphrase string
	// AdminAudit is the writer for logs/admin-audit.log. Nil is permitted;
	// auth handlers fall back to no-op audit (LogChange / LogAuth on a nil
	// receiver are safe).
	AdminAudit *logging.AuditWriter
}

// Server bundles the http.Server, the hub, and the runtime state needed by
// /healthz. Construct via New, then call ListenAndServe.
type Server struct {
	cfg         Config
	hub         *hub.Hub
	log         *zap.Logger
	srv         *http.Server
	started     time.Time
	upstream    atomic.Value // UpstreamState
	auth        *adminAuth
	adminConfig atomic.Pointer[configState] // /admin/api/config snapshot + hooks
}

// New builds a Server but does not start listening. Pass the same hub the
// upstream reader broadcasts into.
func New(cfg Config, h *hub.Hub, log *zap.Logger) *Server {
	if log == nil {
		log = zap.NewNop()
	}
	if cfg.MaxLogTailLines <= 0 {
		cfg.MaxLogTailLines = 200
	}
	s := &Server{
		cfg:     cfg,
		hub:     h,
		log:     log,
		started: time.Now(),
		auth:    newAdminAuth(cfg.AdminPassphrase, cfg.AdminAudit),
	}
	s.upstream.Store(UpstreamConnecting)
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	s.srv = &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

// SetUpstreamState updates the label reported by /healthz.
func (s *Server) SetUpstreamState(state UpstreamState) {
	s.upstream.Store(state)
}

// Addr returns the bind address (after Listen, this reflects the real port
// when the original Addr ended in :0).
func (s *Server) Addr() string { return s.srv.Addr }

// Handler returns the underlying mux for tests.
func (s *Server) Handler() http.Handler { return s.srv.Handler }

// ListenAndServe blocks until the server stops. Returns http.ErrServerClosed
// on graceful shutdown; any other error indicates a real failure.
func (s *Server) ListenAndServe() error {
	s.log.Info("http server listening", zap.String("addr", s.cfg.Addr))
	return s.srv.ListenAndServe()
}

// Shutdown gracefully drains in-flight requests, up to ctx's deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/logs", s.handleLogs)
	mux.HandleFunc("/ws", s.handleWS)

	// /admin auth surface — login/logout are public, everything else
	// requires a valid session cookie. The middleware redirects browsers
	// to /admin/login and replies 401 JSON to API clients (auth_admin.go).
	mux.HandleFunc("/admin/api/login", s.handleAdminLogin)
	mux.HandleFunc("/admin/api/logout", s.handleAdminLogout)
	mux.HandleFunc("/admin/api/config", s.requireAdminAuth(s.handleAdminConfig))
	mux.HandleFunc("/admin", s.requireAdminAuth(s.handleAdminStub))

	// The SPA serves /admin/login and any future /admin sub-routes (#84).
	// They flow through the static handler below.
	mux.Handle("/", s.staticHandler())
}

// writeJSON serializes v as JSON, returning a 500 on encoder failure.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
