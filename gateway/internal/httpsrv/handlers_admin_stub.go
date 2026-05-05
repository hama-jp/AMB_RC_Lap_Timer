package httpsrv

import (
	"fmt"
	"net/http"
)

// handleAdminStub is the seat-saver for Issue #8. It returns 200 with a
// minimal HTML page so any client navigation against /admin succeeds; the
// real settings WebUI is intentionally out of scope for #3.
func (s *Server) handleAdminStub(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	fmt.Fprintf(w, `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>AMB RC Lap Timer — Admin</title></head>
<body>
<h1>Admin (placeholder)</h1>
<p>The settings WebUI is tracked by Issue #8. This route is reserved
so SPA navigation does not 404.</p>
<p>Gateway version: <code>%s</code></p>
</body></html>`, htmlEscape(s.cfg.Version))
}

// htmlEscape is the bare minimum to keep `%s`'d version strings safe.
// We don't take user input here, but version may carry odd characters in
// dev builds, and this stub may grow.
func htmlEscape(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			out = append(out, "&lt;"...)
		case '>':
			out = append(out, "&gt;"...)
		case '&':
			out = append(out, "&amp;"...)
		case '"':
			out = append(out, "&quot;"...)
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
