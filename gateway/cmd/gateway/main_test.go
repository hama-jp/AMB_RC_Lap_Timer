package main

import (
	"bytes"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestFormatListenURLs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		listen    string
		lanIPs    []string
		wantURLs  []string
		wantWarn  string // substring match; empty = expect no warning
		wantEmpty bool   // wantURLs == nil
	}{
		{
			name:     "bind-all empty host enumerates LAN IPs",
			listen:   ":8080",
			lanIPs:   []string{"192.168.1.42", "10.0.0.5"},
			wantURLs: []string{"http://192.168.1.42:8080/", "http://10.0.0.5:8080/"},
		},
		{
			name:     "bind-all 0.0.0.0 enumerates LAN IPs",
			listen:   "0.0.0.0:8080",
			lanIPs:   []string{"192.168.1.42"},
			wantURLs: []string{"http://192.168.1.42:8080/"},
		},
		{
			name:     "bind-all IPv6 wildcard enumerates LAN IPs",
			listen:   "[::]:8080",
			lanIPs:   []string{"192.168.1.42"},
			wantURLs: []string{"http://192.168.1.42:8080/"},
		},
		{
			name:      "bind-all with no LAN IPs warns",
			listen:    ":8080",
			lanIPs:    nil,
			wantWarn:  "no LAN IPv4 address",
			wantEmpty: true,
		},
		{
			name:     "specific LAN IP returns single URL",
			listen:   "192.168.1.42:8080",
			lanIPs:   []string{"192.168.1.42", "10.0.0.5"},
			wantURLs: []string{"http://192.168.1.42:8080/"},
		},
		{
			name:     "loopback IP returns URL with warn",
			listen:   "127.0.0.1:8080",
			lanIPs:   []string{"192.168.1.42"},
			wantURLs: []string{"http://127.0.0.1:8080/"},
			wantWarn: "loopback-only",
		},
		{
			name:      "missing port is malformed",
			listen:    "192.168.1.42",
			lanIPs:    []string{"192.168.1.42"},
			wantWarn:  "malformed",
			wantEmpty: true,
		},
		{
			name:      "non-numeric port is invalid",
			listen:    ":nope",
			lanIPs:    []string{"192.168.1.42"},
			wantWarn:  "invalid port",
			wantEmpty: true,
		},
	}

	for _, tc := range cases {
		tc := tc // capture per-iteration; go.mod is still go1.20
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			urls, warn := formatListenURLs(tc.listen, tc.lanIPs)

			if tc.wantEmpty {
				if len(urls) != 0 {
					t.Fatalf("urls = %v, want empty", urls)
				}
			} else {
				if len(urls) != len(tc.wantURLs) {
					t.Fatalf("urls = %v, want %v", urls, tc.wantURLs)
				}
				for i, u := range urls {
					if u != tc.wantURLs[i] {
						t.Errorf("urls[%d] = %q, want %q", i, u, tc.wantURLs[i])
					}
				}
			}

			if tc.wantWarn == "" && warn != "" {
				t.Errorf("warn = %q, want empty", warn)
			}
			if tc.wantWarn != "" && !strings.Contains(warn, tc.wantWarn) {
				t.Errorf("warn = %q, want substring %q", warn, tc.wantWarn)
			}
		})
	}
}

func TestAnnounceListenURLs_BannerIncludesURL(t *testing.T) {
	// announceListenURLs calls the real lanIPv4s() so we can't pin exact
	// output, but for a host-pinned listen the URL is deterministic and the
	// banner must contain it. This is the smoke test for the stdout banner.
	var buf bytes.Buffer
	announceListenURLs("192.168.1.42:8080", zap.NewNop(), &buf)
	got := buf.String()
	if !strings.Contains(got, "http://192.168.1.42:8080/") {
		t.Fatalf("banner missing URL; got:\n%s", got)
	}
	if !strings.Contains(got, "Open this URL on a phone") {
		t.Fatalf("banner missing the operator hint; got:\n%s", got)
	}
}

func TestAnnounceListenURLs_LoopbackSilentBanner(t *testing.T) {
	// Loopback listen still emits the URL (operator may have set it on
	// purpose for a local test), but the warning must be visible. We verify
	// the banner still prints — the warn goes through zap, not stdout.
	var buf bytes.Buffer
	announceListenURLs("127.0.0.1:8080", zap.NewNop(), &buf)
	if !strings.Contains(buf.String(), "http://127.0.0.1:8080/") {
		t.Fatalf("loopback URL missing from banner; got:\n%s", buf.String())
	}
}

func TestAnnounceListenURLs_MalformedSkipsBanner(t *testing.T) {
	var buf bytes.Buffer
	announceListenURLs("not-a-listen-addr", zap.NewNop(), &buf)
	if buf.Len() != 0 {
		t.Fatalf("expected no banner output for malformed listen, got:\n%s", buf.String())
	}
}
