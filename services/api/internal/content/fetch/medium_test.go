package fetch_test

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"readvault/api/internal/content/fetch"
)

func TestMedium_RecognisesMediumAndItsCustomDomains(t *testing.T) {
	t.Parallel()
	resolved := map[string][]netip.Addr{
		"blog.stackademic.com":   {netip.MustParseAddr("162.159.153.4"), netip.MustParseAddr("162.159.152.4")},
		"towardsdatascience.com": {netip.MustParseAddr("104.26.11.19")},
	}
	lookups := 0
	m := fetch.NewMediumDetector(func(_ context.Context, host string) ([]netip.Addr, error) {
		lookups++
		if addrs, ok := resolved[host]; ok {
			return addrs, nil
		}
		return nil, errors.New("no such host")
	})

	cases := map[string]bool{
		"https://medium.com/@someone/a-post-123":                        true,
		"https://levelup.medium.com/a-post-123":                         true,
		"https://MEDIUM.com/x":                                          true,
		"https://blog.stackademic.com/how-would-you-design-a-webhook-1": true,
		"https://towardsdatascience.com/a-post":                         false,
		"https://notmedium.com/a-post":                                  false,
		"https://medium.com.evil.example/a-post":                        false,
		"not a url":                                                     false,
	}
	for rawURL, expected := range cases {
		if got := m.Is(t.Context(), rawURL); got != expected {
			t.Errorf("Is(%q) = %v, want %v", rawURL, got, expected)
		}
	}
	if lookups > 4 {
		t.Errorf("%d DNS lookups, want none for medium.com and its subdomains", lookups)
	}
}
