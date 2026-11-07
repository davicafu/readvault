//go:build integration

package library_test

import (
	"slices"
	"testing"
	"time"

	"readvault/api/internal/library"
)

func TestExport_HoldsEveryArticleOfTheUserOnly(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	other := newUser(t, pool)
	seed(t, pool, other, seedArticle{url: "https://a.example/theirs", title: "theirs", created: base})
	ids := seed(t, pool, owner,
		seedArticle{url: "https://a.example/old", title: "old", created: base, folder: []string{"Bookmarks bar"}, tags: []string{"go"}},
		seedArticle{url: "https://a.example/new", title: "new", created: base.Add(time.Hour)},
	)
	start := time.Now()

	got, err := svc.Export(t.Context(), owner)
	if err != nil {
		t.Fatalf("Export() error: %v", err)
	}
	if got.SchemaVersion != library.ExportSchemaVersion || got.ExportedAt.Before(start.Add(-time.Second)) {
		t.Errorf("header = version %d, exported %v", got.SchemaVersion, got.ExportedAt)
	}
	if len(got.Articles) != 2 || got.Articles[0].ID != ids[0] || got.Articles[1].ID != ids[1] {
		t.Fatalf("articles = %+v, want the owner's two articles, oldest first", got.Articles)
	}
	a := got.Articles[0]
	if a.OriginalURL != "https://a.example/old" || a.CanonicalURL != "https://a.example/old" || a.Title != "old" ||
		!slices.Equal(a.Folder, []string{"Bookmarks bar"}) || a.BookmarkedAt == nil || !a.CreatedAt.Equal(base) ||
		len(a.Tags) != 1 || a.Tags[0].Slug != "go" {
		t.Errorf("article = %+v, want URLs, title, tags and metadata kept", a)
	}
}

func TestExport_EmptyLibraryHasNoArticles(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t)

	got, err := svc.Export(t.Context(), owner)
	if err != nil || got.Articles == nil || len(got.Articles) != 0 {
		t.Errorf("Export() = %+v, %v; want an empty, non-nil list", got.Articles, err)
	}
}
