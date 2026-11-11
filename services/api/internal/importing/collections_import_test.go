//go:build integration

package importing_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/importing"
)

// shelvedFile has two suggested collections below the browser's bookmarks
// bar, and one bookmark directly on the bar, which gets none.
const shelvedFile = `<DL><p>
	<DT><H3 PERSONAL_TOOLBAR_FOLDER="true">Bookmarks bar</H3>
	<DL><p>
		<DT><H3>Streaming</H3>
		<DL><p>
			<DT><A HREF="https://kafka.apache.org/" TAGS="kafka">Kafka</A>
			<DT><H3>Flink</H3>
			<DL><p>
				<DT><A HREF="https://flink.apache.org/">Flink</A>
			</DL><p>
		</DL><p>
		<DT><H3>go</H3>
		<DL><p>
			<DT><A HREF="https://go.dev/">Go</A>
		</DL><p>
		<DT><A HREF="https://example.com/loose">Loose</A>
	</DL><p>
</DL>`

func TestPreview_SuggestsACollectionPerBookmark(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	addCollection(t, pool, "Go")

	got, err := svc.Preview(t.Context(), owner, strings.NewReader(shelvedFile))
	if err != nil {
		t.Fatalf("Preview() error: %v", err)
	}

	var suggested []string
	for _, c := range got.Candidates {
		suggested = append(suggested, deref(c.Collection))
	}
	if expected := []string{"Streaming", "Streaming", "go", ""}; !slices.Equal(suggested, expected) {
		t.Errorf("suggested collections = %q, want %q", suggested, expected)
	}

	expected := []importing.SuggestedCollection{
		{Name: "go", Slug: "go", Bookmarks: 1, Existing: true},
		{Name: "Streaming", Slug: "streaming", Bookmarks: 2, Existing: false},
	}
	if !slices.Equal(got.Collections, expected) {
		t.Errorf("collections = %+v, want %+v", got.Collections, expected)
	}
}

// Folders whose names share a slug suggest one collection, under the first
// spelling, so every bookmark of the group names it the same way.
func TestPreview_GroupsFoldersThatShareASlug(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t)
	file := `<DL><p>
		<DT><H3>Go</H3><DL><p><DT><A HREF="https://example.com/a">A</A></DL><p>
		<DT><H3>go</H3><DL><p><DT><A HREF="https://example.com/b">B</A></DL><p>
	</DL>`

	got, err := svc.Preview(t.Context(), owner, strings.NewReader(file))
	if err != nil {
		t.Fatalf("Preview() error: %v", err)
	}
	for _, c := range got.Candidates {
		if deref(c.Collection) != "Go" {
			t.Errorf("candidate %d collection = %q, want the group's name Go", c.Position, deref(c.Collection))
		}
	}
	expected := []importing.SuggestedCollection{{Name: "Go", Slug: "go", Bookmarks: 2}}
	if !slices.Equal(got.Collections, expected) {
		t.Errorf("collections = %+v, want %+v", got.Collections, expected)
	}
}

func TestPreview_WarnsAboutACollectionNameTooLong(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t)
	long := strings.Repeat("x", 65)
	file := `<DL><p><DT><H3>` + long + `</H3><DL><p><DT><A HREF="https://example.com/a">A</A></DL><p></DL>`

	got, err := svc.Preview(t.Context(), owner, strings.NewReader(file))
	if err != nil {
		t.Fatalf("Preview() error: %v", err)
	}
	c := got.Candidates[0]
	if c.Collection != nil {
		t.Errorf("collection = %q, want none", *c.Collection)
	}
	if len(c.Warnings) != 1 || c.Warnings[0].Code != "collection_too_long" {
		t.Errorf("warnings = %+v, want collection_too_long", c.Warnings)
	}
	if len(got.Collections) != 0 {
		t.Errorf("collections = %+v, want none", got.Collections)
	}
}

func TestImport_CreatesTheSuggestedCollectionsAndReusesExistingOnes(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	addCollection(t, pool, "Go")

	report, err := svc.Import(t.Context(), owner, importing.Choices{}, strings.NewReader(shelvedFile))
	if err != nil {
		t.Fatalf("Import() error: %v", err)
	}
	if expected := (importing.ReportTotals{Inserted: 4}); report.Totals != expected {
		t.Errorf("totals = %+v, want %+v", report.Totals, expected)
	}
	assertCollections(t, pool, map[string]string{
		"https://kafka.apache.org/": "streaming",
		"https://flink.apache.org/": "streaming",
		"https://go.dev/":           "go",
		"https://example.com/loose": "",
	})
	if n := count(t, pool, "SELECT count(*) FROM collections"); n != 2 {
		t.Errorf("collections = %d, want 2 (Streaming created, Go reused)", n)
	}
	var name string
	if err := pool.QueryRow(t.Context(), `SELECT name FROM collections WHERE slug = 'go'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Go" {
		t.Errorf("existing collection name = %q, want it kept", name)
	}

	again, err := svc.Import(t.Context(), owner, importing.Choices{}, strings.NewReader(shelvedFile))
	if err != nil {
		t.Fatalf("second Import() error: %v", err)
	}
	if expected := (importing.ReportTotals{Skipped: 4}); again.Totals != expected {
		t.Errorf("second import totals = %+v, want %+v", again.Totals, expected)
	}
	if n := count(t, pool, "SELECT count(*) FROM collections"); n != 2 {
		t.Errorf("collections after a second import = %d, want 2", n)
	}
}

func TestImport_AppliesThePersonsChoices(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)

	choices := importing.Choices{
		// Rename a suggestion for all its bookmarks, and drop another.
		Collections: map[string]*string{"streaming": new("Data streaming"), "go": nil},
		Bookmarks: map[int]importing.BookmarkChoice{
			// The first Streaming bookmark goes nowhere and loses its tag.
			1: {SetCollection: true, SetTags: true, Tags: []string{}},
			// A bookmark with no suggestion gets one, and tags.
			4: {SetCollection: true, Collection: new("Backend"), SetTags: true, Tags: []string{"Web", "web", "http"}},
		},
	}
	report, err := svc.Import(t.Context(), owner, choices, strings.NewReader(shelvedFile))
	if err != nil {
		t.Fatalf("Import() error: %v", err)
	}
	if expected := (importing.ReportTotals{Inserted: 4}); report.Totals != expected {
		t.Errorf("totals = %+v, want %+v", report.Totals, expected)
	}
	assertCollections(t, pool, map[string]string{
		"https://kafka.apache.org/": "",
		"https://flink.apache.org/": "data-streaming",
		"https://go.dev/":           "",
		"https://example.com/loose": "backend",
	})
	if n := count(t, pool, "SELECT count(*) FROM collections"); n != 2 {
		t.Errorf("collections = %d, want only the ones in use", n)
	}
	if got := articleTags(t, pool, "https://kafka.apache.org/"); len(got) != 0 {
		t.Errorf("tags of the first bookmark = %v, want none", got)
	}
	if got := articleTags(t, pool, "https://example.com/loose"); !slices.Equal(got, []string{"http", "web"}) {
		t.Errorf("tags of the last bookmark = %v, want [http web]", got)
	}
}

func TestImport_GivesAnExistingArticleACollectionOnlyWhenItHasNone(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ctx := t.Context()

	// Everything exists already; the Streaming bookmarks have no collection,
	// and then one of them is put in another.
	dropStreaming := importing.Choices{Collections: map[string]*string{"Streaming": nil}}
	if _, err := svc.Import(ctx, owner, dropStreaming, strings.NewReader(shelvedFile)); err != nil {
		t.Fatal(err)
	}
	id := addCollection(t, pool, "Mine")
	if _, err := pool.Exec(ctx, `UPDATE articles SET collection_id = $1 WHERE canonical_url = 'https://flink.apache.org/'`, id); err != nil {
		t.Fatal(err)
	}

	report, err := svc.Import(ctx, owner, importing.Choices{}, strings.NewReader(shelvedFile))
	if err != nil {
		t.Fatalf("Import() error: %v", err)
	}
	if expected := (importing.ReportTotals{Updated: 1, Skipped: 3}); report.Totals != expected {
		t.Errorf("totals = %+v, want %+v", report.Totals, expected)
	}
	assertCollections(t, pool, map[string]string{
		"https://kafka.apache.org/": "streaming",
		"https://flink.apache.org/": "mine",
		"https://go.dev/":           "go",
		"https://example.com/loose": "",
	})
}

func TestImport_RejectsInvalidChoicesWithoutWritingAnything(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 65)
	cases := map[string]struct {
		choices importing.Choices
		field   string
	}{
		"collection name too long": {importing.Choices{Collections: map[string]*string{"streaming": &long}}, "collections.streaming"},
		"blank collection name":    {importing.Choices{Bookmarks: map[int]importing.BookmarkChoice{2: {SetCollection: true, Collection: new("  ")}}}, "bookmarks.2.collection"},
		"tag too long":             {importing.Choices{Bookmarks: map[int]importing.BookmarkChoice{2: {SetTags: true, Tags: []string{long}}}}, "bookmarks.2.tags"},
		"bookmark not in the file": {importing.Choices{Bookmarks: map[int]importing.BookmarkChoice{9: {SetTags: true, Tags: []string{"x"}}}}, "bookmarks.9"},
		"unknown suggestion":       {importing.Choices{Collections: map[string]*string{"nope": nil}}, "collections.nope"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			svc, pool := newService(t)

			_, err := svc.Import(t.Context(), owner, tc.choices, strings.NewReader(shelvedFile))
			var choiceErr *importing.ChoiceError
			if !errors.As(err, &choiceErr) {
				t.Fatalf("Import() error = %v, want a ChoiceError", err)
			}
			if choiceErr.Field != tc.field {
				t.Errorf("field = %q, want %q", choiceErr.Field, tc.field)
			}
			if n := count(t, pool, "SELECT count(*) FROM articles"); n != 0 {
				t.Errorf("articles = %d, want nothing written", n)
			}
			if n := count(t, pool, "SELECT count(*) FROM collections"); n != 0 {
				t.Errorf("collections = %d, want nothing written", n)
			}
		})
	}
}

func addCollection(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(), `INSERT INTO collections (user_id, name, slug) VALUES ($1, $2, lower($2)) RETURNING id`,
		owner, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func assertCollections(t *testing.T, pool *pgxpool.Pool, expected map[string]string) {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT a.canonical_url, coalesce(c.slug, '')
		FROM articles a LEFT JOIN collections c ON c.id = a.collection_id`)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	var url, slug string
	for rows.Next() {
		if err := rows.Scan(&url, &slug); err != nil {
			t.Fatal(err)
		}
		got[url] = slug
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(expected) {
		t.Errorf("articles = %v, want %v", got, expected)
	}
	for u, s := range expected {
		if got[u] != s {
			t.Errorf("collection of %s = %q, want %q", u, got[u], s)
		}
	}
}

func articleTags(t *testing.T, pool *pgxpool.Pool, canonicalURL string) []string {
	t.Helper()
	var tags []string
	if err := pool.QueryRow(t.Context(), `SELECT coalesce(array_agg(t.slug ORDER BY t.slug), '{}') FROM article_tags at
		JOIN tags t ON t.id = at.tag_id JOIN articles a ON a.id = at.article_id
		WHERE a.canonical_url = $1`, canonicalURL).Scan(&tags); err != nil {
		t.Fatal(err)
	}
	return tags
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
