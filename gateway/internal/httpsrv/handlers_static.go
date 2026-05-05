package httpsrv

import (
	"net/http"
	"strings"
)

// staticHandler returns the SPA file server. If WebFS is nil we fall back
// to a small inline message so the route is still navigable in dev.
func (s *Server) staticHandler() http.Handler {
	if s.cfg.WebFS == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<!doctype html><meta charset=utf-8><title>AMB RC Lap Timer</title><h1>AMB RC Lap Timer</h1><p>SPA bundle not embedded in this build.</p>`))
		})
	}

	fileSrv := http.FileServer(http.FS(s.cfg.WebFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Defence-in-depth against path traversal — http.ServeMux already
		// canonicalises the URL, but reject obviously-wrong paths early.
		if strings.Contains(r.URL.Path, "..") {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		// LAN-only deployment: avoid aggressive caching so a fresh build
		// shows up without manual reload.
		w.Header().Set("Cache-Control", "no-cache")
		fileSrv.ServeHTTP(w, r)
	})
}
