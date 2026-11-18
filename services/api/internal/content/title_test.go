//go:build integration

package content_test

import (
	"strings"
	"testing"
	"time"

	"readvault/api/internal/content"
)

const titledPage = `<html><head><title>Queues | Blog</title><meta property="og:title" content="Queues, explained"></head>
<body><article><p>Queues decouple producers and consumers.</p></article></body></html>`

func TestFetch_GivesAnArticleSavedUnderItsAddressThePageTitle(t *testing.T) {
	t.Parallel()
	edited := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		title    string
		editedAt *time.Time
		want     string
	}{
		{"titled with its address", articleURL, nil, "Queues, explained"},
		{"titled with its canonical address", "https://blog.example/queues", nil, "Queues, explained"},
		{"without a title", "", nil, "Queues, explained"},
		{"with a bookmark title", "My queues note", nil, "My queues note"},
		{"with its address as an edited title", articleURL, &edited, articleURL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, f, pool, id := setup(t)
			if _, err := pool.Exec(t.Context(), `UPDATE articles SET title = $2, title_edited_at = $3,
				canonical_url = 'https://blog.example/queues' WHERE id = $1`, id, tc.title, tc.editedAt); err != nil {
				t.Fatal(err)
			}
			var updatedBefore time.Time
			if err := pool.QueryRow(t.Context(), `SELECT updated_at FROM articles WHERE id = $1`, id).Scan(&updatedBefore); err != nil {
				t.Fatal(err)
			}
			f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(titledPage)}

			if _, err := svc.Fetch(t.Context(), owner, id); err != nil {
				t.Fatalf("Fetch() error: %v", err)
			}
			var title string
			var editedAt *time.Time
			var updated time.Time
			if err := pool.QueryRow(t.Context(), `SELECT title, title_edited_at, updated_at FROM articles WHERE id = $1`, id).
				Scan(&title, &editedAt, &updated); err != nil {
				t.Fatal(err)
			}
			if title != tc.want {
				t.Errorf("title %q, want %q", title, tc.want)
			}
			// Filling in the title is not an edit by the reader.
			if (editedAt == nil) != (tc.editedAt == nil) || !updated.Equal(updatedBefore) {
				t.Errorf("titleEditedAt %v, updatedAt %v (was %v): the download must not count as an edit", editedAt, updated, updatedBefore)
			}
		})
	}
}

func TestFetch_PageWithoutATitleKeepsTheAddressAsTitle(t *testing.T) {
	t.Parallel()
	svc, f, pool, id := setup(t)
	if _, err := pool.Exec(t.Context(), `UPDATE articles SET title = original_url WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(articlePage)}
	if _, err := svc.Fetch(t.Context(), owner, id); err != nil {
		t.Fatal(err)
	}
	var title string
	if err := pool.QueryRow(t.Context(), `SELECT title FROM articles WHERE id = $1`, id).Scan(&title); err != nil || title != articleURL {
		t.Errorf("title %q (err %v), want the address kept", title, err)
	}
}

func TestUpload_GivesAnArticleSavedUnderItsAddressThePageTitle(t *testing.T) {
	t.Parallel()
	svc, _, pool, id := setup(t)
	if _, err := pool.Exec(t.Context(), `UPDATE articles SET title = original_url WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Upload(t.Context(), owner, id, "queues.html", strings.NewReader(titledPage)); err != nil {
		t.Fatal(err)
	}
	var title string
	if err := pool.QueryRow(t.Context(), `SELECT title FROM articles WHERE id = $1`, id).Scan(&title); err != nil || title != "Queues, explained" {
		t.Errorf("title %q (err %v), want the page title", title, err)
	}
}
