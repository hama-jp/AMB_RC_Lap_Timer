package httpsrv

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/hub"
)

func TestGeneratePassphrase_HexAndUnique(t *testing.T) {
	p1, err := GeneratePassphrase()
	if err != nil {
		t.Fatal(err)
	}
	if len(p1) != passphraseBytes*2 {
		t.Errorf("len: got %d want %d", len(p1), passphraseBytes*2)
	}
	if _, err := hex.DecodeString(p1); err != nil {
		t.Errorf("not hex: %v", err)
	}
	p2, _ := GeneratePassphrase()
	if p1 == p2 {
		t.Errorf("two consecutive passphrases identical (%q); RNG not wired", p1)
	}
}

func TestAdminLogin_GoodPassphrase_SetsSessionCookie(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	ts := newTestServer(t, h)

	body := strings.NewReader(`{"passphrase":"` + testPassphrase + `"}`)
	resp, err := http.Post(ts.URL+"/admin/api/login", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d want 200, body=%s", resp.StatusCode, b)
	}
	var got *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == AdminSessionCookie {
			got = c
		}
	}
	if got == nil {
		t.Fatalf("no %s cookie set", AdminSessionCookie)
	}
	if !got.HttpOnly {
		t.Errorf("cookie not HttpOnly")
	}
	if got.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie SameSite: got %v want Strict", got.SameSite)
	}
	if got.Path != "/admin" {
		t.Errorf("cookie path: got %q want /admin", got.Path)
	}
	if len(got.Value) != sessionTokenBytes*2 {
		t.Errorf("cookie value len: got %d want %d", len(got.Value), sessionTokenBytes*2)
	}
	if _, err := hex.DecodeString(got.Value); err != nil {
		t.Errorf("cookie value not hex: %v", err)
	}
}

func TestAdminLogin_BadPassphrase_401(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	ts := newTestServer(t, h)

	body := strings.NewReader(`{"passphrase":"wrong"}`)
	resp, err := http.Post(ts.URL+"/admin/api/login", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status: got %d want 401", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == AdminSessionCookie {
			t.Errorf("session cookie issued on bad login")
		}
	}
}

func TestAdminLogin_RateLimit_429AfterFailure(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	ts := newTestServer(t, h)

	// First failure → 401.
	body := strings.NewReader(`{"passphrase":"wrong"}`)
	resp, err := http.Post(ts.URL+"/admin/api/login", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("first attempt status: got %d want 401", resp.StatusCode)
	}

	// Second attempt within cooldown — even with the GOOD passphrase — must
	// 429. This protects the audit log from "first wrong then guess" noise.
	body = strings.NewReader(`{"passphrase":"` + testPassphrase + `"}`)
	resp, err = http.Post(ts.URL+"/admin/api/login", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 429 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("second attempt status: got %d want 429, body=%s", resp.StatusCode, b)
	}
	var payload map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	if _, ok := payload["retry_after_ms"]; !ok {
		t.Errorf("response missing retry_after_ms; got %v", payload)
	}
}

func TestAdminLogin_MalformedBody_400(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	ts := newTestServer(t, h)

	resp, err := http.Post(ts.URL+"/admin/api/login", "application/json",
		strings.NewReader(`{not json`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("status: got %d want 400", resp.StatusCode)
	}
}

func TestAdminLogout_AfterLogin_ClearsCookieAndRevokes(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	ts := newTestServer(t, h)

	client := adminLogin(t, ts)

	// Sanity: logged-in client can hit /admin.
	resp, err := client.Get(ts.URL + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("pre-logout /admin: got %d want 200", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/admin/api/logout", nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("logout: got %d want 200", resp.StatusCode)
	}

	// After logout, /admin/api/* must 401 again.
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/admin", nil)
	req.Header.Set("Accept", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("post-logout /admin (json): got %d want 401", resp.StatusCode)
	}
}

func TestAdmin_Unauthenticated_HtmlRedirectsToLogin(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	ts := newTestServer(t, h)

	// http.Client follows 302 by default; disable that so we can inspect
	// the redirect itself.
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/admin", nil)
	req.Header.Set("Accept", "text/html")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: got %d want %d", resp.StatusCode, http.StatusSeeOther)
	}
	if loc := resp.Header.Get("Location"); loc != adminLoginPath {
		t.Errorf("location: got %q want %q", loc, adminLoginPath)
	}
}

func TestAdmin_Unauthenticated_ApiClientGets401Json(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	ts := newTestServer(t, h)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/admin", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status: got %d want 401", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: got %q", ct)
	}
}

func TestAdminAuth_Cooldown_ExpiresAfterTimeout(t *testing.T) {
	h := hub.New(zap.NewNop(), 10, 4)
	defer h.Close()
	cfg := Config{
		Version:         "test",
		AdminPassphrase: testPassphrase,
	}
	s := New(cfg, h, zap.NewNop())
	// Inject a clock so we can advance past loginFailureCooldown without
	// sleeping in the test.
	fake := time.Unix(1_700_000_000, 0)
	s.auth.now = func() time.Time { return fake }

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Burn one failure.
	resp, _ := http.Post(ts.URL+"/admin/api/login", "application/json",
		strings.NewReader(`{"passphrase":"wrong"}`))
	resp.Body.Close()

	// Advance past cooldown. Subsequent good login must succeed.
	fake = fake.Add(loginFailureCooldown + time.Second)
	resp, err := http.Post(ts.URL+"/admin/api/login", "application/json",
		strings.NewReader(`{"passphrase":"`+testPassphrase+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("post-cooldown login: got %d want 200, body=%s", resp.StatusCode, b)
	}
}
