// Package urlnorm computes the canonical identity of an article URL.
//
// Normalization is deliberately conservative: it only removes what is known
// not to change which document a URL points to (host case, default ports,
// fragments and recognized tracking parameters). Path case, trailing slashes
// and every unrecognized query parameter are preserved, because they may
// identify different content.
package urlnorm

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	// ErrInvalidURL means the input is not an absolute URL with a host.
	ErrInvalidURL = errors.New("urlnorm: invalid url")
	// ErrUnsupportedScheme means the scheme is not http or https.
	ErrUnsupportedScheme = errors.New("urlnorm: unsupported scheme")
)

// trackingParams lists query parameters that never affect content.
// Parameters with the "utm_" prefix are always removed as well.
var trackingParams = map[string]bool{
	"fbclid":  true,
	"gclid":   true,
	"dclid":   true,
	"msclkid": true,
	"mc_cid":  true,
	"mc_eid":  true,
	"igshid":  true,
	"_hsenc":  true,
	"_hsmi":   true,
}

var defaultPorts = map[string]string{
	"http":  "80",
	"https": "443",
}

// Canonicalize returns the canonical form of raw, used as the identity of an
// article. The original URL must be kept separately for display and navigation.
func Canonicalize(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidURL)
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		return "", fmt.Errorf("%w: not absolute", ErrInvalidURL)
	}
	if _, ok := defaultPorts[scheme]; !ok {
		return "", fmt.Errorf("%w: %q", ErrUnsupportedScheme, scheme)
	}

	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("%w: missing host", ErrInvalidURL)
	}
	if strings.Contains(host, ":") { // IPv6 literal
		host = "[" + host + "]"
	}
	if port := u.Port(); port != "" && port != defaultPorts[scheme] {
		host += ":" + port
	}

	u.Scheme = scheme
	u.Host = host
	u.Fragment = ""
	u.RawFragment = ""
	u.RawQuery = stripTracking(u.RawQuery)
	u.ForceQuery = false
	if u.Path == "" && u.Opaque == "" {
		u.Path = "/"
	}

	return u.String(), nil
}

// stripTracking removes tracking parameters while preserving the order and
// the original encoding of every other parameter.
func stripTracking(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}

	kept := []string{}
	for pair := range strings.SplitSeq(rawQuery, "&") {
		if pair == "" {
			continue
		}
		key, _, _ := strings.Cut(pair, "=")
		if decoded, err := url.QueryUnescape(key); err == nil {
			key = decoded
		}
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") || trackingParams[lower] {
			continue
		}
		kept = append(kept, pair)
	}

	return strings.Join(kept, "&")
}
