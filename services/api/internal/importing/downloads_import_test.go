//go:build integration

package importing_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/importing"
)

const threeLinks = `<DL>
<DT><A HREF="https://a.example/one">One</A>
<DT><A HREF="https://a.example/two">Two</A>
<DT><A HREF="https://a.example/three">Three</A>
</DL>`

func TestImport_QueuesTheTextDownloadOfArticlesWithoutText(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ctx := t.Context()

	// "two" is already in the library with saved text, "three" already
	// failed to download once: neither is queued.
	var two, three string
	for url, id := range map[string]*string{"https://a.example/two": &two, "https://a.example/three": &three} {
		if err := pool.QueryRow(ctx, `INSERT INTO articles (user_id, original_url, canonical_url, title)
			VALUES ($1, $2, $2, 'x') RETURNING id`, owner, url).Scan(id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO article_contents (article_id, user_id, status, source, markdown, words)
		VALUES ($1, $2, 'complete', 'fetched', 'Text.', 1), ($3, $2, 'failed', 'fetched', '', 0)`, two, owner, three); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Import(ctx, owner, importing.Choices{}, strings.NewReader(threeLinks)); err != nil {
		t.Fatal(err)
	}
	expected := []string{"https://a.example/one"}
	if got := queuedURLs(t, pool); !slices.Equal(got, expected) {
		t.Errorf("queued = %v, want %v", got, expected)
	}

	// Importing again queues nothing twice.
	if _, err := svc.Import(ctx, owner, importing.Choices{}, strings.NewReader(threeLinks)); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, `SELECT count(*) FROM content_downloads`); n != 1 {
		t.Errorf("after a second import %d rows queued, want 1", n)
	}
}

func TestImport_QueuesAnExistingArticleWithoutText(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO articles (user_id, original_url, canonical_url, title)
		VALUES ($1, 'https://a.example/one', 'https://a.example/one', 'x')`, owner); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Import(t.Context(), owner, importing.Choices{}, strings.NewReader(threeLinks)); err != nil {
		t.Fatal(err)
	}
	expected := []string{"https://a.example/one", "https://a.example/three", "https://a.example/two"}
	if got := queuedURLs(t, pool); !slices.Equal(got, expected) {
		t.Errorf("queued = %v, want %v", got, expected)
	}
}

// queuedURLs lists the addresses of the queued articles, sorted, checking
// each row belongs to the article's owner.
func queuedURLs(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT a.original_url FROM content_downloads d
		JOIN articles a ON a.id = d.article_id AND a.user_id = d.user_id
		WHERE d.attempts = 0 AND d.claimed_until IS NULL ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	var urls []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			t.Fatal(err)
		}
		urls = append(urls, u)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return urls
}
