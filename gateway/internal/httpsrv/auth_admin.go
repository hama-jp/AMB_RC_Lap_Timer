// /admin authentication: a one-time passphrase generated at gateway start
// (logged once to stdout + gateway.log), exchanged for an in-memory session
// cookie at POST /admin/api/login. See docs/architecture.md §3.5.4 / §3.5.6.
//
// The model is intentionally small for "LAN + 1 operator" deployments:
//   - No persistence: passphrase and active sessions are wiped on restart.
//   - No HTTPS: cookies are not Secure, but they are HttpOnly + SameSite=Strict
//     and scoped to /admin so a phish via the lap-timing UI cannot read them.
//   - Per-IP rate limit on /admin/api/login: a failed attempt blocks that
//     same IP from another login for loginFailureCooldown. Brute-forcing 32
//     hex chars is infeasible regardless, but the cooldown keeps audit logs
//     readable.

package httpsrv

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/logging"
)

const (
	// AdminSessionCookie is set by /admin/api/login on success.
	AdminSessionCookie = "amb-rc-admin-session"
	// adminLoginPath is where the auth middleware sends unauthenticated
	// browsers (HTML requests). The SPA owns the login UI under HashRouter
	// (Issue #84), so we redirect to the in-page anchor rather than the
	// raw /admin/login path which Go would 404 (only / is served by the
	// embedded static handler).
	adminLoginPath = "/#/admin/login"

	// passphraseBytes is the raw entropy size; the hex-encoded form is 2×.
	// 16 raw bytes = 128 bits, hex = 32 chars. Plenty for an interactive
	// passphrase the operator types from a console.
	passphraseBytes = 16
	// sessionTokenBytes mirrors the passphrase: 32-hex session token.
	sessionTokenBytes = 16

	// loginFailureCooldown rate-limits repeated wrong-passphrase attempts
	// from the same IP. Spec §3.5.4 calls for ~5s; this matches.
	loginFailureCooldown = 5 * time.Second
)

// GeneratePassphrase returns a hex-encoded random passphrase. Callers should
// log this exactly once at startup (once to console, once to file via the
// shared zap logger does both at the same call site).
func GeneratePassphrase() (string, error) {
	b := make([]byte, passphraseBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// adminAuth owns the one passphrase, the active session set, and the
// per-IP cooldown table. Constructed once per Server.
type adminAuth struct {
	passphrase string

	mu        sync.Mutex
	sessions  map[string]struct{}
	lastFails map[string]time.Time

	now   func() time.Time
	audit *logging.AuditWriter
}

func newAdminAuth(passphrase string, audit *logging.AuditWriter) *adminAuth {
	return &adminAuth{
		passphrase: passphrase,
		sessions:   make(map[string]struct{}),
		lastFails:  make(map[string]time.Time),
		now:        time.Now,
		audit:      audit,
	}
}

// validSession returns true if the cookie value is a known session token.
func (a *adminAuth) validSession(token string) bool {
	if token == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.sessions[token]
	return ok
}

// issueSession adds a new session token and returns it. Callers must hold
// no locks and are expected to set the cookie on the response.
func (a *adminAuth) issueSession() (string, error) {
	b := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(b)
	a.mu.Lock()
	a.sessions[tok] = struct{}{}
	a.mu.Unlock()
	return tok, nil
}

// revokeSession removes a token from the active set. Idempotent.
func (a *adminAuth) revokeSession(token string) {
	if token == "" {
		return
	}
	a.mu.Lock()
	delete(a.sessions, token)
	a.mu.Unlock()
}

// cooldownRemaining returns >0 when ip is within the post-failure window.
func (a *adminAuth) cooldownRemaining(ip string) time.Duration {
	if ip == "" {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	last, ok := a.lastFails[ip]
	if !ok {
		return 0
	}
	if d := loginFailureCooldown - a.now().Sub(last); d > 0 {
		return d
	}
	return 0
}

func (a *adminAuth) recordFailure(ip string) {
	if ip == "" {
		return
	}
	a.mu.Lock()
	a.lastFails[ip] = a.now()
	a.mu.Unlock()
}

func (a *adminAuth) clearFailure(ip string) {
	if ip == "" {
		return
	}
	a.mu.Lock()
	delete(a.lastFails, ip)
	a.mu.Unlock()
}

// handleLogin exchanges a passphrase for a session cookie.
//
// Body:    {"passphrase": "<hex>"}
// 200:     Set-Cookie + {"ok":true}
// 400:     malformed body
// 401:     wrong passphrase ({"error":"invalid passphrase"})
// 429:     too soon after a recent failure ({"error":"rate limited", "retry_after_ms": ...})
func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ip := clientIP(r)
	if d := s.auth.cooldownRemaining(ip); d > 0 {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":          "rate limited",
			"retry_after_ms": d.Milliseconds(),
		})
		return
	}
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	// Constant-time compare so a side-channel can't infer a prefix-match
	// of the passphrase from response timing. ConstantTimeCompare returns
	// 0 (not 1) when lengths differ, so the empty-string case is covered
	// implicitly — no separate guard needed.
	if subtle.ConstantTimeCompare([]byte(body.Passphrase), []byte(s.auth.passphrase)) != 1 {
		s.auth.recordFailure(ip)
		s.auth.audit.LogAuth(ip, "login failed")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid passphrase"})
		return
	}
	tok, err := s.auth.issueSession()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session create failed"})
		return
	}
	s.auth.clearFailure(ip)
	s.auth.audit.LogAuth(ip, "login ok")
	http.SetCookie(w, &http.Cookie{
		Name:     AdminSessionCookie,
		Value:    tok,
		Path:     "/admin",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		// Secure: false — LAN HTTP only. Don't set true here or the cookie
		// won't be sent on plain http (the canonical deployment).
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleLogout invalidates the caller's session token and clears the cookie.
// Idempotent: returns 200 even if the cookie was missing or already
// invalid, which avoids the typical "logout button silently fails on a
// stale tab" UX issue.
func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tok := readSessionCookie(r)
	if tok != "" {
		s.auth.revokeSession(tok)
		s.auth.audit.LogAuth(clientIP(r), "logout")
	}
	// Clear the cookie regardless.
	http.SetCookie(w, &http.Cookie{
		Name:     AdminSessionCookie,
		Value:    "",
		Path:     "/admin",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// requireAdminAuth gates a handler behind a valid session cookie.
//
// Browsers (Accept: text/html, no JSON content-type) that fail are 302'd to
// /admin/login so the SPA can render the login form. API clients (everything
// else) get 401 + JSON. The split lets curl callers script things while the
// UX redirect still works in the browser.
func (s *Server) requireAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.auth.validSession(readSessionCookie(r)) {
			next(w, r)
			return
		}
		if wantsHTML(r) {
			http.Redirect(w, r, adminLoginPath, http.StatusSeeOther)
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
	}
}

func readSessionCookie(r *http.Request) string {
	c, err := r.Cookie(AdminSessionCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// wantsHTML decides whether to redirect or return JSON on auth failure.
// Heuristic: an Accept header containing "text/html" wins; otherwise we
// assume the caller is an API client and prefer JSON.
func wantsHTML(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html")
}

// clientIP returns the best effort remote IP for audit logging. RemoteAddr
// is host:port; we strip the port. We deliberately do NOT honour
// X-Forwarded-For: the gateway is on the LAN with no reverse proxy in
// front, and trusting that header would let any LAN client spoof the
// audit log entries.
func clientIP(r *http.Request) string {
	if r == nil || r.RemoteAddr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
