package fetch_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"readvault/api/internal/content"
	"readvault/api/internal/content/fetch"
)

var html = []string{"text/html"}

// allowLoopback lets tests reach httptest servers; production uses PublicAddr.
func allowLoopback(netip.AddrPort) bool { return true }

func problemOf(t *testing.T, err error) *content.FetchError {
	t.Helper()
	var fe *content.FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("error = %v, want a *content.FetchError", err)
	}
	return fe
}

func TestFetch_ReturnsThePageAfterRedirects(t *testing.T) {
	t.Parallel()
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/old":
			http.Redirect(w, r, "/new", http.StatusMovedPermanently)
		case "/new":
			gotUA = r.UserAgent()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<p>hello</p>"))
		}
	}))
	defer srv.Close()

	got, err := fetch.New(allowLoopback).Fetch(t.Context(), srv.URL+"/old", 1024, html)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if string(got.Body) != "<p>hello</p>" || got.URL != srv.URL+"/new" || got.ContentType != "text/html" {
		t.Errorf("got %q from %q as %q", got.Body, got.URL, got.ContentType)
	}
	if !strings.HasPrefix(gotUA, "ReadVault/") {
		t.Errorf("User-Agent = %q, want ReadVault to identify itself", gotUA)
	}
}

func TestFetch_ReportsEachFailureWithAProblemCode(t *testing.T) {
	t.Parallel()
	hops := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/forbidden":
			http.Error(w, "no", http.StatusForbidden)
		case "/pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF"))
		case "/big":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(strings.Repeat("a", 2048)))
		case "/loop":
			hops++
			http.Redirect(w, r, "/loop?"+strconv.Itoa(hops), http.StatusFound)
		case "/to-ftp":
			http.Redirect(w, r, "ftp://files.example/a", http.StatusFound)
		case "/slow":
			time.Sleep(300 * time.Millisecond)
			_, _ = w.Write([]byte("<p>late</p>"))
		}
	}))
	defer srv.Close()

	tests := []struct {
		path     string
		problem  string
		status   int
		deadline time.Duration
	}{
		{path: "/forbidden", problem: content.ProblemHTTPStatus, status: 403},
		{path: "/pdf", problem: content.ProblemNotHTML},
		{path: "/big", problem: content.ProblemTooLarge},
		{path: "/loop", problem: content.ProblemUnreachable},
		{path: "/to-ftp", problem: content.ProblemUnreachable},
		{path: "/slow", problem: content.ProblemTimeout, deadline: 50 * time.Millisecond},
	}
	for _, tt := range tests {
		ctx := t.Context()
		if tt.deadline > 0 {
			var cancel func()
			ctx, cancel = contextWithTimeout(t, tt.deadline)
			defer cancel()
		}
		_, err := fetch.New(allowLoopback).Fetch(ctx, srv.URL+tt.path, 1024, html)
		fe := problemOf(t, err)
		if fe.Problem != tt.problem || fe.Status != tt.status {
			t.Errorf("%s: problem %q status %d, want %q %d", tt.path, fe.Problem, fe.Status, tt.problem, tt.status)
		}
	}
}

func TestFetch_RejectsAddressesThatAreNotWebURLs(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"file:///etc/passwd", "javascript:alert(1)", "gopher://x.example/", "not a url"} {
		_, err := fetch.New(allowLoopback).Fetch(t.Context(), raw, 1024, html)
		if fe := problemOf(t, err); fe.Problem != content.ProblemBlockedAddress {
			t.Errorf("%q: problem %q, want blocked_address", raw, fe.Problem)
		}
	}
}

func TestFetch_DefaultGuardBlocksLoopback(t *testing.T) {
	t.Parallel()
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()

	_, err := fetch.New(fetch.PublicAddr).Fetch(t.Context(), srv.URL, 1024, html)

	if fe := problemOf(t, err); fe.Problem != content.ProblemBlockedAddress {
		t.Errorf("problem %q, want blocked_address", fe.Problem)
	}
	if hit {
		t.Error("the request reached a loopback server")
	}
}

func TestFetch_GuardAppliesToEveryRedirect(t *testing.T) {
	t.Parallel()
	innerHit := false
	inner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { innerHit = true }))
	defer inner.Close()
	outer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, inner.URL+"/admin", http.StatusFound)
	}))
	defer outer.Close()
	_, outerPort, _ := net.SplitHostPort(strings.TrimPrefix(outer.URL, "http://"))

	// Only the first server counts as public here.
	onlyOuter := func(a netip.AddrPort) bool { return strconv.Itoa(int(a.Port())) == outerPort }
	_, err := fetch.New(onlyOuter).Fetch(t.Context(), outer.URL, 1024, html)

	if fe := problemOf(t, err); fe.Problem != content.ProblemBlockedAddress {
		t.Errorf("problem %q, want blocked_address", fe.Problem)
	}
	if innerHit {
		t.Error("the redirect reached a blocked address")
	}
}

func TestPublicAddr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		addr     string
		expected bool
	}{
		{"93.184.216.34:443", true},
		{"93.184.216.34:80", true},
		{"[2606:2800:220:1:248:1893:25c8:1946]:443", true},
		{"93.184.216.34:22", false},
		{"127.0.0.1:443", false},
		{"10.1.2.3:443", false},
		{"172.16.0.1:443", false},
		{"192.168.1.1:80", false},
		{"169.254.169.254:80", false},
		{"100.64.0.1:443", false},
		{"0.0.0.0:443", false},
		{"224.0.0.1:443", false},
		{"[::1]:443", false},
		{"[fe80::1]:443", false},
		{"[fd00::1]:443", false},
		{"[::ffff:127.0.0.1]:443", false},
		{"[64:ff9b::a00:1]:443", false},
	}
	for _, tt := range tests {
		if got := fetch.PublicAddr(netip.MustParseAddrPort(tt.addr)); got != tt.expected {
			t.Errorf("PublicAddr(%s) = %v, want %v", tt.addr, got, tt.expected)
		}
	}
}

func contextWithTimeout(t *testing.T, d time.Duration) (ctx context.Context, cancel func()) {
	return context.WithTimeout(t.Context(), d)
}
