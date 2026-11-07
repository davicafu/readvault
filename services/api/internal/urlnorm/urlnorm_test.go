package urlnorm_test

import (
	"errors"
	"testing"

	"readvault/api/internal/urlnorm"
)

func TestCanonicalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercases scheme and host but keeps path case",
			input:    "HTTPS://Medium.COM/@Author/My-Post-123",
			expected: "https://medium.com/@Author/My-Post-123",
		},
		{
			name:     "removes fragment",
			input:    "https://example.com/post#section-2",
			expected: "https://example.com/post",
		},
		{
			name:     "removes recognized tracking parameters only",
			input:    "https://example.com/a?utm_source=x&id=7&fbclid=abc&utm_campaign=y&gclid=z",
			expected: "https://example.com/a?id=7",
		},
		{
			name:     "keeps unrecognized parameters in original order",
			input:    "https://example.com/search?q=go&page=2&lang=es",
			expected: "https://example.com/search?q=go&page=2&lang=es",
		},
		{
			name:     "drops empty query after removing tracking",
			input:    "https://example.com/a?utm_medium=email",
			expected: "https://example.com/a",
		},
		{
			name:     "removes default https port",
			input:    "https://example.com:443/a",
			expected: "https://example.com/a",
		},
		{
			name:     "removes default http port",
			input:    "http://example.com:80/a",
			expected: "http://example.com/a",
		},
		{
			name:     "keeps non default port",
			input:    "http://localhost:8080/a",
			expected: "http://localhost:8080/a",
		},
		{
			name:     "keeps ipv6 host with non default port",
			input:    "http://[::1]:8080/a",
			expected: "http://[::1]:8080/a",
		},
		{
			name:     "removes default port from ipv6 host",
			input:    "https://[2001:DB8::1]:443/a",
			expected: "https://[2001:db8::1]/a",
		},
		{
			name:     "empty path becomes root",
			input:    "https://example.com",
			expected: "https://example.com/",
		},
		{
			name:     "keeps trailing slash as distinct path",
			input:    "https://example.com/a/",
			expected: "https://example.com/a/",
		},
		{
			name:     "trims surrounding whitespace",
			input:    "  https://example.com/a \n",
			expected: "https://example.com/a",
		},
		{
			name:     "keeps unicode path",
			input:    "https://example.com/artículo-ñ",
			expected: "https://example.com/art%C3%ADculo-%C3%B1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := urlnorm.Canonicalize(tt.input)
			if err != nil {
				t.Fatalf("Canonicalize(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.expected {
				t.Errorf("Canonicalize(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCanonicalize_Rejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected error
	}{
		{name: "javascript scheme", input: "javascript:alert(1)", expected: urlnorm.ErrUnsupportedScheme},
		{name: "file scheme", input: "file:///etc/passwd", expected: urlnorm.ErrUnsupportedScheme},
		{name: "data scheme", input: "data:text/html,<b>x</b>", expected: urlnorm.ErrUnsupportedScheme},
		{name: "mixed case javascript", input: "JavaScript:alert(1)", expected: urlnorm.ErrUnsupportedScheme},
		{name: "empty", input: "   ", expected: urlnorm.ErrInvalidURL},
		{name: "relative", input: "/just/a/path", expected: urlnorm.ErrInvalidURL},
		{name: "missing host", input: "https:///a", expected: urlnorm.ErrInvalidURL},
		{name: "unparseable", input: "https://exa mple.com/%zz", expected: urlnorm.ErrInvalidURL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := urlnorm.Canonicalize(tt.input)
			if !errors.Is(err, tt.expected) {
				t.Errorf("Canonicalize(%q) error = %v, want %v", tt.input, err, tt.expected)
			}
		})
	}
}

func FuzzCanonicalize(f *testing.F) {
	f.Add("https://example.com/a?utm_source=x#frag")
	f.Add("javascript:alert(1)")
	f.Add("")

	f.Fuzz(func(t *testing.T, input string) {
		got, err := urlnorm.Canonicalize(input)
		if err != nil {
			return
		}
		again, err := urlnorm.Canonicalize(got)
		if err != nil {
			t.Fatalf("canonical form %q is not accepted: %v", got, err)
		}
		if again != got {
			t.Fatalf("not idempotent: %q -> %q", got, again)
		}
	})
}
