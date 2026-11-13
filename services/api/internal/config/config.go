// Package config loads the API configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"time"
)

// Config is the runtime configuration of the API.
type Config struct {
	DatabaseURL      string
	HTTPAddr         string
	LogLevel         slog.Level
	MaxUploadBytes   int64
	MaxImportRecords int
	// MaxArticleBytes bounds an uploaded article file and a downloaded page.
	MaxArticleBytes int64
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
	// ArticleMirrorURL is the prefix of the reading mirror tried when a
	// Medium article is refused or is a paid preview; empty means none.
	ArticleMirrorURL string
	// ArticleArchiveURL is the prefix of the archive whose copy is tried, for
	// any site, when the page and the mirror fail; "" turns it off. The
	// default asks the Wayback Machine for its latest copy of the page as
	// it was published (id_).
	ArticleArchiveURL string
	// DownloadWorkers is how many article texts are downloaded at once in
	// the background; 0 turns background downloads off.
	DownloadWorkers int
}

// Load reads the configuration from the environment. DATABASE_URL is required;
// everything else has a safe default.
func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		HTTPAddr:          envOr("HTTP_ADDR", ":8080"),
		MaxUploadBytes:    10 << 20,
		MaxImportRecords:  20_000,
		MaxArticleBytes:   5 << 20,
		RequestTimeout:    30 * time.Second,
		ShutdownTimeout:   15 * time.Second,
		DownloadWorkers:   2,
		ArticleMirrorURL:  "https://freedium-mirror.cfd/",
		ArticleArchiveURL: "https://web.archive.org/web/2id_/",
	}

	var errs []error
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(envOr("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}
	if v, ok := os.LookupEnv("MAX_UPLOAD_BYTES"); ok {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("MAX_UPLOAD_BYTES must be a positive integer, got %q", v))
		}
		cfg.MaxUploadBytes = n
	}
	if v, ok := os.LookupEnv("MAX_IMPORT_RECORDS"); ok {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("MAX_IMPORT_RECORDS must be a positive integer, got %q", v))
		}
		cfg.MaxImportRecords = n
	}

	if v, ok := os.LookupEnv("MAX_ARTICLE_BYTES"); ok {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("MAX_ARTICLE_BYTES must be a positive integer, got %q", v))
		}
		cfg.MaxArticleBytes = n
	}

	cfg.ArticleMirrorURL = copyPrefix("ARTICLE_MIRROR_URL", cfg.ArticleMirrorURL, &errs)
	cfg.ArticleArchiveURL = copyPrefix("ARTICLE_ARCHIVE_URL", cfg.ArticleArchiveURL, &errs)

	if v := os.Getenv("DOWNLOAD_WORKERS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 16 {
			errs = append(errs, fmt.Errorf("DOWNLOAD_WORKERS must be an integer from 0 to 16, got %q", v))
		}
		cfg.DownloadWorkers = n
	}

	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("loading config: %w", err)
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// copyPrefix reads the address prefix of a reading copy (the mirror or the
// archive) from the environment variable name: unset keeps def, "off" turns
// it off. The article address is appended as a path, so a prefix without
// one gets a final slash.
func copyPrefix(name, def string, errs *[]error) string {
	v := os.Getenv(name)
	switch v {
	case "":
		return def
	case "off":
		return ""
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		*errs = append(*errs, fmt.Errorf("%s must be an http or https address, got %q", name, v))
		return ""
	}
	if u.Path == "" && u.RawQuery == "" {
		v += "/"
	}
	return v
}
