package config_test

import (
	"strings"
	"testing"

	"readvault/api/internal/config"
)

func TestLoad_ArticleMirror(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")

	t.Setenv("ARTICLE_MIRROR_URL", "")
	cfg, err := config.Load()
	if err != nil || cfg.ArticleMirrorURL != "" {
		t.Errorf("unset: %q, %v; want no mirror", cfg.ArticleMirrorURL, err)
	}

	t.Setenv("ARTICLE_MIRROR_URL", "https://freedium-mirror.cfd/")
	cfg, err = config.Load()
	if err != nil || cfg.ArticleMirrorURL != "https://freedium-mirror.cfd/" {
		t.Errorf("set: %q, %v; want the prefix as given", cfg.ArticleMirrorURL, err)
	}

	for _, bad := range []string{"freedium-mirror.cfd", "ftp://mirror.example/", "https:///x"} {
		t.Setenv("ARTICLE_MIRROR_URL", bad)
		if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "ARTICLE_MIRROR_URL") {
			t.Errorf("%q: error = %v, want ARTICLE_MIRROR_URL rejected", bad, err)
		}
	}
}
