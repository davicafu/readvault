//go:build integration

package importing_test

import (
	"slices"
	"strings"
	"testing"

	"readvault/api/internal/importing"
)

// The export brings back what was lost: a deleted collection is created
// again with its articles in it, and a deleted article is added again,
// while articles already in the library are not duplicated.
func TestImport_FromAnExportRestoresALostCollectionAndArticle(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ctx := t.Context()

	if _, err := svc.Import(ctx, owner, importing.Choices{}, strings.NewReader(readVaultExport)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM collections WHERE user_id = $1 AND slug = 'backend'`, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM articles WHERE user_id = $1 AND canonical_url = 'https://example.com/loose'`, owner); err != nil {
		t.Fatal(err)
	}

	preview, err := svc.Preview(ctx, owner, strings.NewReader(readVaultExport))
	if err != nil {
		t.Fatal(err)
	}
	if preview.Totals.Valid != 1 || preview.Totals.Existing != 1 || len(preview.Collections) != 1 || preview.Collections[0].Name != "Backend" || preview.Collections[0].Existing {
		t.Errorf("preview = %+v %+v, want 1 new, 1 existing and Backend to be created", preview.Totals, preview.Collections)
	}

	report, err := svc.Import(ctx, owner, importing.Choices{}, strings.NewReader(readVaultExport))
	if err != nil {
		t.Fatal(err)
	}
	if report.Totals.Inserted != 1 || report.Totals.Updated != 1 {
		t.Errorf("report = %+v, want the lost article inserted and the other back in its collection", report.Totals)
	}

	var collection, title string
	var tags []string
	if err := pool.QueryRow(ctx, `
		SELECT c.name, a.title, array(SELECT t.name FROM article_tags at JOIN tags t ON t.id = at.tag_id WHERE at.article_id = a.id ORDER BY t.slug)
		FROM articles a JOIN collections c ON c.id = a.collection_id
		WHERE a.user_id = $1 AND a.canonical_url = 'https://go.dev/blog/'`, owner).Scan(&collection, &title, &tags); err != nil {
		t.Fatalf("article back in its collection: %v", err)
	}
	if collection != "Backend" || title != "The Go Blog" || !slices.Equal(tags, []string{"Concurrency", "go"}) {
		t.Errorf("restored article: collection %q title %q tags %v", collection, title, tags)
	}
	var articles int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM articles WHERE user_id = $1`, owner).Scan(&articles); err != nil || articles != 2 {
		t.Errorf("articles = %d, %v; want 2, nothing duplicated", articles, err)
	}
}
