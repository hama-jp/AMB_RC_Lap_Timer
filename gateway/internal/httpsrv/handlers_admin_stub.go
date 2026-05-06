package httpsrv

import "net/http"

// handleAdminStub redirects an authenticated direct-path visit to /admin
// over to the SPA's HashRouter equivalent (#/admin). The login form lives
// at #/admin/login; both routes are owned by the React app shipped via
// go:embed under "/" (Issue #84).
//
// The seat-saver HTML stub from PR #88 is gone: the SPA now hosts the real
// admin form, and routing it through Go would force the static handler to
// intercept extra paths.
func (s *Server) handleAdminStub(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.Redirect(w, r, "/#/admin", http.StatusSeeOther)
}
