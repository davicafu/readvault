//go:build integration

package library_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/library"
)

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDelete_RemovesTheArticleWithItsTextImagesTagsAndQueuedDownload(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	user := newUser(t, pool)
	ids := seed(t, pool, user,
		seedArticle{url: "https://a.example/gone", title: "gone", created: base, tags: []string{"go", "only-here"}},
		seedArticle{url: "https://a.example/kept", title: "kept", created: base, tags: []string{"go"}},
	)
	gone, kept := ids[0], ids[1]
	for _, q := range []string{
		`INSERT INTO article_contents (article_id, user_id, status, source, markdown, words) VALUES ($1, $2, 'complete', 'fetched', 'text', 1)`,
		`INSERT INTO article_images (article_id, user_id, position, source_url, content_type, data) VALUES ($1, $2, 0, 'https://a.example/i.png', 'image/png', '\x89'::bytea)`,
		`INSERT INTO content_downloads (article_id, user_id) VALUES ($1, $2)`,
	} {
		if _, err := pool.Exec(t.Context(), q, gone, user); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.Delete(t.Context(), user, gone); err != nil {
		t.Fatalf("Delete = %v", err)
	}

	if _, err := svc.Get(t.Context(), user, gone); !errors.Is(err, library.ErrNotFound) {
		t.Errorf("Get after delete: err = %v, want ErrNotFound", err)
	}
	for table, query := range map[string]string{
		"article_contents":  `SELECT count(*) FROM article_contents WHERE article_id = $1`,
		"article_images":    `SELECT count(*) FROM article_images WHERE article_id = $1`,
		"content_downloads": `SELECT count(*) FROM content_downloads WHERE article_id = $1`,
		"article_tags":      `SELECT count(*) FROM article_tags WHERE article_id = $1`,
	} {
		if n := countRows(t, pool, query, gone); n != 0 {
			t.Errorf("%s: %d rows left, want none", table, n)
		}
	}
	if a, err := svc.Get(t.Context(), user, kept); err != nil || a.Title != "kept" {
		t.Errorf("other article: %+v, %v; want it kept", a, err)
	}
	tags, err := svc.Tags(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tc := range tags {
		names = append(names, tc.Name)
	}
	if !slices.Equal(names, []string{"go"}) {
		t.Errorf("tags after delete = %v, want only the kept article's", names)
	}
}

func TestDelete_UnknownMalformedOrAnotherUsersArticleIsNotFound(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	user, other := newUser(t, pool), newUser(t, pool)
	id := seed(t, pool, user, seedArticle{url: "https://a.example/mine", title: "mine", created: base})[0]

	if err := svc.Delete(t.Context(), other, id); !errors.Is(err, library.ErrNotFound) {
		t.Errorf("another user's delete: err = %v, want ErrNotFound", err)
	}
	if _, err := svc.Get(t.Context(), user, id); err != nil {
		t.Errorf("article after another user's delete: %v, want it kept", err)
	}
	if err := svc.Delete(t.Context(), user, "not-a-uuid"); !errors.Is(err, library.ErrNotFound) {
		t.Errorf("malformed id: err = %v, want ErrNotFound", err)
	}
	if err := svc.Delete(t.Context(), user, id); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(t.Context(), user, id); !errors.Is(err, library.ErrNotFound) {
		t.Errorf("second delete: err = %v, want ErrNotFound", err)
	}
}
