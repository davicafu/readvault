//go:build integration

package library_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"readvault/api/internal/library"
)

func TestList_FiltersByTheStateOfTheSavedText(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ids := seed(t, pool, owner,
		seedArticle{url: "https://a.example/complete", title: "complete", created: base},
		seedArticle{url: "https://a.example/incomplete", title: "incomplete", created: base.Add(time.Hour)},
		seedArticle{url: "https://a.example/failed", title: "failed", created: base.Add(2 * time.Hour)},
		seedArticle{url: "https://a.example/missing", title: "missing", created: base.Add(3 * time.Hour)},
	)
	if _, err := pool.Exec(t.Context(), `INSERT INTO article_contents (article_id, user_id, status, source, markdown, words, problem)
		VALUES ($1, $4, 'complete', 'fetched', 'Text.', 1, NULL),
		       ($2, $4, 'incomplete', 'fetched', 'Start.', 1, NULL),
		       ($3, $4, 'failed', 'fetched', '', 0, 'http_status')`, ids[0], ids[1], ids[2], owner); err != nil {
		t.Fatal(err)
	}

	for text, expected := range map[string][]string{
		"complete":   {"complete"},
		"incomplete": {"incomplete"},
		"failed":     {"failed"},
		"missing":    {"missing"},
		"":           {"complete", "incomplete", "failed", "missing"},
	} {
		page, err := svc.List(t.Context(), owner, library.ListParams{Text: text, Sort: "created_asc"})
		if err != nil {
			t.Fatalf("text=%q: %v", text, err)
		}
		var got []string
		for _, a := range page.Items {
			got = append(got, a.Title)
		}
		if !slices.Equal(got, expected) {
			t.Errorf("text=%q: %v, want %v", text, got, expected)
		}
	}

	_, err := svc.List(t.Context(), owner, library.ListParams{Text: "downloaded"})
	var ve *library.ValidationError
	if !errors.As(err, &ve) || len(ve.Problems) != 1 || ve.Problems[0].Field != "text" || ve.Problems[0].Code != "invalid" {
		t.Errorf("text=downloaded: %v, want a text/invalid problem", err)
	}
}
