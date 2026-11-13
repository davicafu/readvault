package fetch_test

import (
	"context"
	"testing"

	"readvault/api/internal/adapter/fetch"
)

func TestMirrorReads_MediumAndTheNewsSitesItReads(t *testing.T) {
	t.Parallel()
	reads := fetch.MirrorReads(func(_ context.Context, rawURL string) bool {
		return rawURL == "https://blog.stackademic.com/a-post"
	})

	cases := map[string]bool{
		"https://blog.stackademic.com/a-post":                           true,
		"https://www.nytimes.com/2026/11/13/technology/a-story.html":    true,
		"https://www.nytimes.com/athletic/7779741/2026/11/13/a-story/":  true,
		"https://theathletic.com/7779741/2026/11/13/a-story/":           true,
		"https://www.washingtonpost.com/technology/2026/11/13/a-story/": true,
		"https://www.economist.com/leaders/2026/11/13/a-story":          true,
		"https://www.bloomberg.com/news/articles/2026-11-13/a-story":    true,
		"https://www.reuters.com/technology/a-story-2026-11-13/":        true,
		"https://www.quora.com/What-is-Kafka":                           true,
		"https://es.quora.com/Qué-es-Kafka":                             true,
		"https://NYTIMES.com/2026/11/13/a-story.html":                   true,
		"https://notnytimes.com/a-story":                                false,
		"https://nytimes.com.evil.example/a-story":                      false,
		"https://go.dev/blog/pipelines":                                 false,
		"not a url":                                                     false,
	}
	for rawURL, expected := range cases {
		if got := reads(t.Context(), rawURL); got != expected {
			t.Errorf("MirrorReads(%q) = %v, want %v", rawURL, got, expected)
		}
	}
}
