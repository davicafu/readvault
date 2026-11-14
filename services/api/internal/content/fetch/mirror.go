package fetch

import (
	"context"
	"net/url"
	"strings"
)

// mirrorNewsSites are the news sites the reading mirror (Freedium) reads
// besides Medium; their subdomains count too. Checked by hand on 2026-11-13:
// New York Times, The Athletic, Washington Post and Bloomberg articles came
// back whole. The others were asked for by the user and are tried anyway;
// when the mirror fails, the archive is tried next.
var mirrorNewsSites = []string{
	"nytimes.com", // The Athletic lives under nytimes.com/athletic
	"theathletic.com",
	"washingtonpost.com",
	"economist.com",
	"bloomberg.com",
	"reuters.com",
	"quora.com",
}

// MirrorReads returns which articles the reading mirror can read: those
// isMedium accepts and those on mirrorNewsSites.
func MirrorReads(isMedium func(ctx context.Context, rawURL string) bool) func(ctx context.Context, rawURL string) bool {
	return func(ctx context.Context, rawURL string) bool {
		if u, err := url.Parse(rawURL); err == nil {
			host := strings.ToLower(u.Hostname())
			for _, site := range mirrorNewsSites {
				if host == site || strings.HasSuffix(host, "."+site) {
					return true
				}
			}
		}
		return isMedium(ctx, rawURL)
	}
}
