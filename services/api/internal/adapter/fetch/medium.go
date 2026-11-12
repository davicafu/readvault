package fetch

import (
	"context"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// mediumAddrs are the addresses medium.com and the publications Medium
// hosts on their own domains (such as blog.stackademic.com) resolve to.
// Observed with DNS on 2026-11-12; a domain that leaves Medium resolves
// elsewhere and stops matching.
var mediumAddrs = map[netip.Addr]bool{
	netip.MustParseAddr("162.159.152.4"): true,
	netip.MustParseAddr("162.159.153.4"): true,
}

const mediumLookupTimeout = 3 * time.Second

// MediumDetector tells whether an article is served by Medium, so only those
// articles are sent to a Medium reading mirror.
type MediumDetector struct {
	lookup func(ctx context.Context, host string) ([]netip.Addr, error)
}

// NewMediumDetector returns a MediumDetector that resolves hosts with
// lookup; nil uses the system resolver.
func NewMediumDetector(lookup func(ctx context.Context, host string) ([]netip.Addr, error)) *MediumDetector {
	if lookup == nil {
		lookup = func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}
	}
	return &MediumDetector{lookup: lookup}
}

// Is reports whether rawURL is on medium.com, one of its subdomains, or a
// domain that resolves to Medium's addresses. A failed lookup means no.
func (m *MediumDetector) Is(ctx context.Context, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "medium.com" || strings.HasSuffix(host, ".medium.com") {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, mediumLookupTimeout)
	defer cancel()
	addrs, err := m.lookup(ctx, host)
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if mediumAddrs[a.Unmap()] {
			return true
		}
	}
	return false
}
