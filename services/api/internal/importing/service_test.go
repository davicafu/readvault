//go:build integration

package importing_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/importing"
	importingpg "readvault/api/internal/importing/postgres"
	"readvault/api/internal/platform/postgres"
	"readvault/api/internal/testdb"
)

const owner = postgres.DevOwnerID

func newService(t *testing.T) (*importing.Service, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	return importing.NewService(importingpg.NewRepository(pool), 100), pool
}

func fixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/bookmarks.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPreview_ClassifiesEveryRecord(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t)

	got, err := svc.Preview(t.Context(), owner, strings.NewReader(fixture(t)))
	if err != nil {
		t.Fatalf("Preview() error: %v", err)
	}

	expectedTotals := importing.PreviewTotals{Detected: 6, Valid: 3, Invalid: 2, DuplicateInFile: 1, Existing: 0}
	if got.Totals != expectedTotals {
		t.Errorf("totals = %+v, want %+v", got.Totals, expectedTotals)
	}

	type row struct {
		status    importing.Status
		canonical string
		title     string
		folder    string
		codes     string
	}
	expected := []row{
		{importing.StatusValid, "https://go.dev/blog/", "The Go Blog", "Bookmarks bar/Go", ""},
		{importing.StatusValid, "https://pkg.go.dev/net/http", "http package", "Bookmarks bar/Go", ""},
		{importing.StatusDuplicateInFile, "https://go.dev/blog/", "Go blog again", "Bookmarks bar", "duplicate_in_file"},
		{importing.StatusInvalid, "", "Bookmarklet", "Bookmarks bar", "unsupported_scheme"},
		{importing.StatusValid, "https://example.com/untitled", "", "Bookmarks bar", "missing_title"},
		{importing.StatusInvalid, "", "No href", "Bookmarks bar", "missing_url"},
	}
	if len(got.Candidates) != len(expected) {
		t.Fatalf("got %d candidates, want %d", len(got.Candidates), len(expected))
	}
	for i, e := range expected {
		c := got.Candidates[i]
		var codes []string
		for _, p := range append(slices.Clone(c.Errors), c.Warnings...) {
			codes = append(codes, p.Code)
		}
		gotRow := row{c.Status, c.CanonicalURL, c.Title, strings.Join(c.Folder, "/"), strings.Join(codes, ",")}
		if gotRow != e {
			t.Errorf("candidate %d = %+v, want %+v", i+1, gotRow, e)
		}
		if c.Position != i+1 {
			t.Errorf("candidate %d position = %d", i+1, c.Position)
		}
	}
	if got.Candidates[0].OriginalURL != "https://go.dev/blog/?utm_source=x#top" {
		t.Errorf("original url = %q, want the url as imported", got.Candidates[0].OriginalURL)
	}
	if !slices.Equal(got.Candidates[0].Tags, []string{"go", "blog"}) {
		t.Errorf("tags = %v, want [go blog]", got.Candidates[0].Tags)
	}
}

func TestImport_IsIdempotent(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ctx := t.Context()

	first, err := svc.Import(ctx, owner, importing.Choices{}, strings.NewReader(fixture(t)))
	if err != nil {
		t.Fatalf("first Import() error: %v", err)
	}
	if expected := (importing.ReportTotals{Inserted: 3, Skipped: 1, Failed: 2}); first.Totals != expected {
		t.Errorf("first import totals = %+v, want %+v", first.Totals, expected)
	}
	if first.ImportID == "" {
		t.Error("import id is empty")
	}

	second, err := svc.Import(ctx, owner, importing.Choices{}, strings.NewReader(fixture(t)))
	if err != nil {
		t.Fatalf("second Import() error: %v", err)
	}
	if expected := (importing.ReportTotals{Skipped: 4, Failed: 2}); second.Totals != expected {
		t.Errorf("second import totals = %+v, want %+v", second.Totals, expected)
	}
	if n := count(t, pool, "SELECT count(*) FROM articles"); n != 3 {
		t.Errorf("articles = %d, want 3", n)
	}
	if n := count(t, pool, "SELECT count(*) FROM tags"); n != 2 {
		t.Errorf("tags = %d, want 2", n)
	}

	failed := map[int]string{}
	for _, it := range second.Items {
		if it.Outcome == importing.OutcomeFailed {
			failed[it.Position] = it.Problems[0].Code
		}
	}
	if expected := map[int]string{4: "unsupported_scheme", 6: "missing_url"}; !mapsEqual(failed, expected) {
		t.Errorf("failed items = %v, want %v", failed, expected)
	}

	preview, err := svc.Preview(ctx, owner, strings.NewReader(fixture(t)))
	if err != nil {
		t.Fatalf("Preview() error: %v", err)
	}
	if expected := (importing.PreviewTotals{Detected: 6, Valid: 0, Invalid: 2, DuplicateInFile: 1, Existing: 3}); preview.Totals != expected {
		t.Errorf("preview after import = %+v, want %+v", preview.Totals, expected)
	}
}

func TestImport_KeepsUserEditsAndCompletesMissingData(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ctx := t.Context()

	if _, err := svc.Import(ctx, owner, importing.Choices{}, strings.NewReader(fixture(t))); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE articles SET title = 'My title', title_edited_at = now()
		WHERE canonical_url = 'https://go.dev/blog/'`); err != nil {
		t.Fatal(err)
	}

	update := `<DL><p>
		<DT><A HREF="https://go.dev/blog/#comments" TAGS="Go, News">Imported title</A>
		<DT><A HREF="https://example.com/untitled">Now titled</A>
		<DT><A HREF="https://pkg.go.dev/net/http">http package</A>
	</DL>`
	report, err := svc.Import(ctx, owner, importing.Choices{}, strings.NewReader(update))
	if err != nil {
		t.Fatalf("Import() error: %v", err)
	}
	if expected := (importing.ReportTotals{Updated: 2, Skipped: 1}); report.Totals != expected {
		t.Errorf("totals = %+v, want %+v", report.Totals, expected)
	}

	var title, originalURL string
	if err := pool.QueryRow(ctx, `SELECT title, original_url FROM articles WHERE canonical_url = 'https://go.dev/blog/'`).
		Scan(&title, &originalURL); err != nil {
		t.Fatal(err)
	}
	if title != "My title" {
		t.Errorf("edited title = %q, want it kept", title)
	}
	if originalURL != "https://go.dev/blog/?utm_source=x#top" {
		t.Errorf("original url = %q, want the first imported url kept", originalURL)
	}

	var tags []string
	if err := pool.QueryRow(ctx, `SELECT array_agg(t.slug ORDER BY t.slug) FROM article_tags at
		JOIN tags t ON t.id = at.tag_id JOIN articles a ON a.id = at.article_id
		WHERE a.canonical_url = 'https://go.dev/blog/'`).Scan(&tags); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tags, []string{"blog", "go", "news"}) {
		t.Errorf("tags = %v, want existing tags kept and news added", tags)
	}

	if err := pool.QueryRow(ctx, `SELECT title FROM articles WHERE canonical_url = 'https://example.com/untitled'`).
		Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "Now titled" {
		t.Errorf("missing title = %q, want it completed from the import", title)
	}
}

func TestImport_RejectsFilesOverTheRecordLimit(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	svc := importing.NewService(importingpg.NewRepository(pool), 2)

	_, err := svc.Import(t.Context(), owner, importing.Choices{}, strings.NewReader(fixture(t)))
	if err == nil || !strings.Contains(err.Error(), importing.ErrTooManyRecords.Error()) {
		t.Errorf("Import() error = %v, want ErrTooManyRecords", err)
	}
	if n := count(t, pool, "SELECT count(*) FROM articles"); n != 0 {
		t.Errorf("articles = %d, want nothing written", n)
	}
}

func count(t *testing.T, pool *pgxpool.Pool, sql string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), sql).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func mapsEqual(a, b map[int]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestImport_ConcurrentImportsDoNotDuplicate(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)

	errs := make(chan error, 4)
	for range 4 {
		go func() {
			_, err := svc.Import(t.Context(), owner, importing.Choices{}, strings.NewReader(fixture(t)))
			errs <- err
		}()
	}
	for range 4 {
		if err := <-errs; err != nil {
			t.Errorf("Import() error: %v", err)
		}
	}
	if n := count(t, pool, "SELECT count(*) FROM articles"); n != 3 {
		t.Errorf("articles = %d, want 3", n)
	}
	if n := count(t, pool, "SELECT count(*) FROM article_tags"); n != 2 {
		t.Errorf("article_tags = %d, want 2", n)
	}
}

func TestImport_RepairsInvalidTextInsteadOfFailingTheBatch(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	// Latin-1 bytes (\xe9) and a NUL byte, as old browser exports can hold.
	file := "<DL><p>" +
		`<DT><A HREF="https://example.org/l1">Good</A>` +
		"<DT><A HREF=\"https://example.org/l2\">Caf\xe9 latin1</A>" +
		"<DT><A HREF=\"https://example.org/l3\" TAGS=\"r\xe9sum\xe9\">t</A>" +
		"<DT><A HREF=\"https://example.org/l4\">nul\x00title</A>" +
		"</DL>"

	preview, err := svc.Preview(t.Context(), owner, strings.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	var repaired []int
	for _, c := range preview.Candidates {
		for _, w := range c.Warnings {
			if w.Code == "text_repaired" {
				repaired = append(repaired, c.Position)
			}
		}
	}
	if !slices.Equal(repaired, []int{2, 3, 4}) {
		t.Errorf("text_repaired warnings on %v, want [2 3 4]", repaired)
	}

	report, err := svc.Import(t.Context(), owner, importing.Choices{}, strings.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	if expected := (importing.ReportTotals{Inserted: 4}); report.Totals != expected {
		t.Errorf("totals = %+v, want %+v", report.Totals, expected)
	}
	var title string
	if err := pool.QueryRow(t.Context(), `SELECT title FROM articles WHERE canonical_url = 'https://example.org/l2'`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "Caf� latin1" {
		t.Errorf("title = %q, want the invalid byte replaced", title)
	}
}
