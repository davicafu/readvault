//go:build integration

package content_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/content"
)

// inCollection creates a collection for the user and puts the articles in it.
func inCollection(t *testing.T, pool *pgxpool.Pool, userID, slug string, articleIDs ...string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(),
		`INSERT INTO collections (user_id, name, slug) VALUES ($1, $2, $2) RETURNING id`, userID, slug).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for _, a := range articleIDs {
		if _, err := pool.Exec(t.Context(), `UPDATE articles SET collection_id = $1 WHERE id = $2`, id, a); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func count(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// withText saves the test page, with its images, as each article's text.
func withText(t *testing.T, svc *content.Service, articleIDs ...string) {
	t.Helper()
	for _, id := range articleIDs {
		if _, err := svc.Upload(t.Context(), owner, id, "page.html", strings.NewReader(articlePage)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRemoveTexts_ClearsEverySavedTextAndKeepsTheArticles(t *testing.T) {
	t.Parallel()
	svc, _, pool, done := setup(t)
	withText(t, svc, done)
	failed := newArticle(t, pool, owner, "https://blog.example/failed")
	if _, err := svc.Fetch(t.Context(), owner, failed); err == nil {
		t.Fatal("setup: want the download to fail")
	}
	queued := newArticle(t, pool, owner, "https://blog.example/queued")
	queue(t, pool, queued)
	other := newUser(t, pool)
	othersArticle := newArticle(t, pool, other, "https://blog.example/someone-else")
	if _, err := pool.Exec(t.Context(), `INSERT INTO article_contents (article_id, user_id, status, source, markdown, words)
		VALUES ($1, $2, 'complete', 'fetched', 'kept', 1)`, othersArticle, other); err != nil {
		t.Fatal(err)
	}

	n, err := svc.RemoveTexts(t.Context(), owner, "")
	if err != nil || n != 2 {
		t.Fatalf("RemoveTexts(all) = %d, %v; want the 2 saved entries (text and failure)", n, err)
	}
	if got := count(t, pool, `SELECT count(*) FROM article_contents WHERE user_id = $1`, owner); got != 0 {
		t.Errorf("%d texts left, want none", got)
	}
	if got := count(t, pool, `SELECT count(*) FROM article_images WHERE user_id = $1`, owner); got != 0 {
		t.Errorf("%d images left, want none", got)
	}
	if got := count(t, pool, `SELECT count(*) FROM content_downloads WHERE user_id = $1`, owner); got != 0 {
		t.Errorf("%d downloads still queued, want none", got)
	}
	if got := count(t, pool, `SELECT count(*) FROM articles WHERE id = ANY($1)`, []string{done, failed, queued}); got != 3 {
		t.Errorf("%d of 3 articles left, want all kept", got)
	}
	if got := count(t, pool, `SELECT count(*) FROM article_contents WHERE user_id = $1`, other); got != 1 {
		t.Errorf("other user's texts = %d, want theirs kept", got)
	}
	if s, err := svc.DownloadSummary(t.Context(), owner); err != nil || s.Missing != 3 {
		t.Errorf("summary after = %+v, %v; want the 3 articles without text", s, err)
	}
}

func TestRemoveTexts_OfOneCollection(t *testing.T) {
	t.Parallel()
	svc, _, pool, done := setup(t)
	inside := newArticle(t, pool, owner, "https://blog.example/inside")
	outside := newArticle(t, pool, owner, "https://blog.example/outside")
	withText(t, svc, done, inside, outside)
	queuedInside := newArticle(t, pool, owner, "https://blog.example/queued-inside")
	queuedOutside := newArticle(t, pool, owner, "https://blog.example/queued-outside")
	queue(t, pool, queuedInside, queuedOutside)
	col := inCollection(t, pool, owner, "kept-small", done, inside, queuedInside)

	n, err := svc.RemoveTexts(t.Context(), owner, col)
	if err != nil || n != 2 {
		t.Fatalf("RemoveTexts(collection) = %d, %v; want its 2 texts", n, err)
	}
	if got := count(t, pool, `SELECT count(*) FROM article_contents WHERE article_id = ANY($1)`, []string{done, inside}); got != 0 {
		t.Errorf("%d texts of the collection left, want none", got)
	}
	if got := count(t, pool, `SELECT count(*) FROM article_contents WHERE article_id = $1`, outside); got != 1 {
		t.Errorf("text outside the collection removed")
	}
	if got := queuedFor(t, pool, queuedInside, queuedOutside); got != 1 {
		t.Errorf("%d queued downloads left, want only the one outside the collection", got)
	}
	if got := count(t, pool, `SELECT count(*) FROM articles WHERE collection_id = $1`, col); got != 3 {
		t.Errorf("collection has %d articles, want its 3 kept", got)
	}
}

func TestRemoveTexts_UnknownCollectionIsNotFound(t *testing.T) {
	t.Parallel()
	svc, _, pool, done := setup(t)
	withText(t, svc, done)
	other := newUser(t, pool)
	othersCollection := inCollection(t, pool, other, "theirs")
	for _, id := range []string{"not-a-uuid", "00000000-0000-0000-0000-00000000dead", othersCollection} {
		if _, err := svc.RemoveTexts(t.Context(), owner, id); !errors.Is(err, content.ErrNotFound) {
			t.Errorf("RemoveTexts(%q) error = %v, want ErrNotFound", id, err)
		}
	}
	if got := count(t, pool, `SELECT count(*) FROM article_contents WHERE article_id = $1`, done); got != 1 {
		t.Errorf("text removed by a refused cleanup")
	}
}
