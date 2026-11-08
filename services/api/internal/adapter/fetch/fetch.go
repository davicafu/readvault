// Package fetch downloads public web resources for the content service.
//
// Every connection, including each redirect, is checked against an address
// guard after DNS resolution, so a hostname that resolves to a private or
// local address (or is rebound to one) cannot reach internal services.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"readvault/api/internal/content"
)

const (
	userAgent    = "ReadVault/0.1 (personal reading library)"
	maxRedirects = 5
)

var (
	errBlocked           = errors.New("address not allowed")
	errTooManyRedirects  = errors.New("too many redirects")
	errUnsupportedScheme = errors.New("redirect to a scheme other than http or https")
)

// Client downloads web resources.
type Client struct {
	http *http.Client
}

// New returns a Client that connects only to addresses allow accepts.
// Production passes PublicAddr.
func New(allow func(netip.AddrPort) bool) *Client {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
		// Control runs for each connection with the resolved address.
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil || !allow(netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())) {
				return errBlocked
			}
			return nil
		},
	}
	transport := &http.Transport{
		// No proxy from the environment: it would bypass the guard.
		Proxy:                  nil,
		DialContext:            dialer.DialContext,
		ForceAttemptHTTP2:      true,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  10 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
		IdleConnTimeout:        30 * time.Second,
		MaxIdleConns:           16,
	}
	return &Client{http: &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errTooManyRedirects
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errUnsupportedScheme
			}
			return nil
		},
	}}
}

var _ content.Fetcher = (*Client)(nil)

// blockedPrefixes are special-purpose ranges that IsGlobalUnicast and
// IsPrivate do not cover.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"), // NAT64 can embed a private IPv4
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"), // 6to4 can embed a private IPv4
}

// PublicAddr accepts public unicast addresses on ports 80 and 443.
func PublicAddr(ap netip.AddrPort) bool {
	if p := ap.Port(); p != 80 && p != 443 {
		return false
	}
	a := ap.Addr().Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// Fetch implements content.Fetcher. It accepts a 2xx response whose media
// type is in accept ("image/*" allows a family) and whose body fits in
// maxBytes. Failures are *content.FetchError with a stable problem code.
func (c *Client) Fetch(ctx context.Context, rawURL string, maxBytes int64, accept []string) (content.Fetched, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return content.Fetched{}, &content.FetchError{Problem: content.ProblemBlockedAddress, Err: errBlocked}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return content.Fetched{}, &content.FetchError{Problem: content.ProblemBlockedAddress, Err: err}
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", strings.Join(accept, ", "))

	resp, err := c.http.Do(req)
	if err != nil {
		return content.Fetched{}, classify(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return content.Fetched{}, &content.FetchError{Problem: content.ProblemHTTPStatus, Status: resp.StatusCode}
	}
	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if !accepted(mediaType, accept) {
		return content.Fetched{}, &content.FetchError{Problem: content.ProblemNotHTML, Err: fmt.Errorf("media type %q", mediaType)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return content.Fetched{}, classify(ctx, err)
	}
	if int64(len(body)) > maxBytes {
		return content.Fetched{}, &content.FetchError{Problem: content.ProblemTooLarge}
	}
	return content.Fetched{URL: resp.Request.URL.String(), ContentType: mediaType, Body: body}, nil
}

func accepted(mediaType string, accept []string) bool {
	for _, a := range accept {
		if a == mediaType || (strings.HasSuffix(a, "/*") && strings.HasPrefix(mediaType, strings.TrimSuffix(a, "*"))) {
			return true
		}
	}
	return false
}

func classify(ctx context.Context, err error) *content.FetchError {
	var netErr net.Error
	switch {
	case errors.Is(err, errBlocked):
		return &content.FetchError{Problem: content.ProblemBlockedAddress, Err: err}
	case errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil || (errors.As(err, &netErr) && netErr.Timeout()):
		return &content.FetchError{Problem: content.ProblemTimeout, Err: err}
	default:
		return &content.FetchError{Problem: content.ProblemUnreachable, Err: err}
	}
}
