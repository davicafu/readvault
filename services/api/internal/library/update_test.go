//go:build integration

package library_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/importing"
	importingpg "readvault/api/internal/importing/postgres"
	"readvault/api/internal/library"
)

func ptr[T any](v T) *T { return &v }

func slugsOf(a library.Article) []string {
	out := []string{}
	for _, t := range a.Tags {
		out = append(out, t.Slug)
	}
	return out
}

// snapshot captures the stored state of the library, to prove a rejected
// edit changed nothing.
func snapshot(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var s string
	if err := pool.QueryRow(t.Context(), `SELECT
		coalesce((SELECT string_agg(id::text || title || coalesce(title_edited_at::text, '') || updated_at::text, ',' ORDER BY id) FROM articles), '') ||
		coalesce((SELECT string_agg(article_id::text || tag_id::text, ',' ORDER BY article_id, tag_id) FROM article_tags), '') ||
		coalesce((SELECT string_agg(slug, ',' ORDER BY slug) FROM tags), '')`).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestUpdate_EditsTheTitleAndMarksItEdited(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ids := seed(t, pool, owner, seedArticle{url: "https://go.dev/blog/", title: "The Go Blog", created: base, tags: []string{"go"}})

	got, err := svc.Update(t.Context(), owner, ids[0], library.Patch{Title: ptr("  Go   blog  ")})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if got.Title != "Go blog" {
		t.Errorf("title = %q, want it trimmed with collapsed spaces", got.Title)
	}
	if got.TitleEditedAt == nil || !got.UpdatedAt.After(base) || !got.CreatedAt.Equal(base) {
		t.Errorf("titleEditedAt %v, updatedAt %v, createdAt %v", got.TitleEditedAt, got.UpdatedAt, got.CreatedAt)
	}
	if got.OriginalURL != "https://go.dev/blog/" || !slices.Equal(slugsOf(got), []string{"go"}) {
		t.Errorf("url %q and tags %v must not change", got.OriginalURL, got.Tags)
	}
	if got.TagsEditedAt != nil {
		t.Errorf("tagsEditedAt = %v, want nil after a title-only edit", got.TagsEditedAt)
	}

	stored, err := svc.Get(t.Context(), owner, ids[0])
	if err != nil || stored.Title != "Go blog" {
		t.Errorf("stored title = %q, %v", stored.Title, err)
	}
}

func TestUpdate_ReplacesTheTagSet(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ids := seed(t, pool, owner, seedArticle{url: "https://a.example/1", title: "one", created: base, tags: []string{"go", "news"}})

	got, err := svc.Update(t.Context(), owner, ids[0], library.Patch{Tags: &[]string{"GO", "Rust  lang", "rust lang"}})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if !slices.Equal(slugsOf(got), []string{"go", "rust-lang"}) {
		t.Errorf("tags = %v, want [go rust-lang]", got.Tags)
	}
	if got.Tags[0].Name != "go" {
		t.Errorf("existing tag name = %q, want the stored spelling kept", got.Tags[0].Name)
	}
	if got.TitleEditedAt != nil || got.Title != "one" {
		t.Errorf("a tag edit must not touch the title: %q, %v", got.Title, got.TitleEditedAt)
	}
	if got.TagsEditedAt == nil {
		t.Error("tagsEditedAt = nil, want the tags marked as edited")
	}
	if !got.UpdatedAt.After(base) {
		t.Errorf("updatedAt = %v, want it advanced", got.UpdatedAt)
	}

	cleared, err := svc.Update(t.Context(), owner, ids[0], library.Patch{Tags: &[]string{}})
	if err != nil || len(cleared.Tags) != 0 {
		t.Errorf("empty tag list: tags %v, error %v; want no tags", cleared.Tags, err)
	}
}

func TestUpdate_RejectsInvalidInputWithoutWriting(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ids := seed(t, pool, owner, seedArticle{url: "https://a.example/1", title: "one", created: base, tags: []string{"go"}})
	before := snapshot(t, pool)

	many := make([]string, 51)
	for i := range many {
		many[i] = "tag" + strings.Repeat("x", i)
	}
	tests := []struct {
		name     string
		patch    library.Patch
		field    string
		expected string
	}{
		{"nothing to change", library.Patch{}, "body", "empty"},
		{"blank title", library.Patch{Title: ptr("   ")}, "title", "required"},
		{"title too long", library.Patch{Title: ptr(strings.Repeat("é", 501))}, "title", "too_long"},
		{"blank tag", library.Patch{Tags: &[]string{"go", " "}}, "tags", "empty"},
		{"tag too long", library.Patch{Tags: &[]string{strings.Repeat("a", 65)}}, "tags", "too_long"},
		{"too many tags", library.Patch{Tags: &many}, "tags", "too_many"},
		{"title with a NUL byte", library.Patch{Title: ptr("a\x00b")}, "title", "invalid"},
		{"tag with invalid UTF-8", library.Patch{Tags: &[]string{"caf\xe9"}}, "tags", "invalid"},
	}
	for _, tt := range tests {
		_, err := svc.Update(t.Context(), owner, ids[0], tt.patch)
		var verr *library.ValidationError
		if !errors.As(err, &verr) || len(verr.Problems) != 1 || verr.Problems[0].Field != tt.field || verr.Problems[0].Code != tt.expected {
			t.Errorf("%s: error = %#v, want one %s/%s problem", tt.name, err, tt.field, tt.expected)
		}
	}
	if after := snapshot(t, pool); after != before {
		t.Error("a rejected edit changed stored data")
	}
}

func TestUpdate_OnlyFindsTheUsersOwnArticles(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	other := newUser(t, pool)
	theirs := seed(t, pool, other, seedArticle{url: "https://a.example/theirs", title: "theirs", created: base})
	before := snapshot(t, pool)

	for _, id := range []string{theirs[0], "not-a-uuid", "00000000-0000-0000-0000-00000000dead"} {
		if _, err := svc.Update(t.Context(), owner, id, library.Patch{Title: ptr("mine now")}); !errors.Is(err, library.ErrNotFound) {
			t.Errorf("Update(%q) error = %v, want ErrNotFound", id, err)
		}
	}
	if after := snapshot(t, pool); after != before {
		t.Error("a rejected edit changed stored data")
	}
}

func TestUpdate_EditedTitleAndTagsSurviveAReimport(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	imports := importing.NewService(importingpg.NewRepository(pool), 100)
	file := `<DL><DT><A HREF="https://go.dev/blog/" TAGS="go">Imported title</A></DL>`
	if _, err := imports.Import(t.Context(), owner, importing.Choices{}, strings.NewReader(file)); err != nil {
		t.Fatal(err)
	}
	page, err := svc.List(t.Context(), owner, library.ListParams{})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("list after import: %+v, %v", page, err)
	}
	id := page.Items[0].ID
	if _, err := svc.Update(t.Context(), owner, id, library.Patch{Title: ptr("My title"), Tags: &[]string{"news"}}); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond)
	report, err := imports.Import(t.Context(), owner, importing.Choices{}, strings.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(t.Context(), owner, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "My title" {
		t.Errorf("title = %q, want the edit kept after a re-import", got.Title)
	}
	if !slices.Equal(slugsOf(got), []string{"news"}) {
		t.Errorf("tags = %v, want the edited set kept: an import must not add back a tag the user removed", got.Tags)
	}
	if report.Totals.Skipped != 1 || report.Totals.Updated != 0 {
		t.Errorf("report totals = %+v, want the article skipped", report.Totals)
	}
}

func TestUpdate_ImportStillAddsTagsWhenOnlyTheTitleWasEdited(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	imports := importing.NewService(importingpg.NewRepository(pool), 100)
	ids := seed(t, pool, owner, seedArticle{url: "https://go.dev/blog/", title: "one", created: base, tags: []string{"news"}})
	if _, err := svc.Update(t.Context(), owner, ids[0], library.Patch{Title: ptr("My title")}); err != nil {
		t.Fatal(err)
	}

	file := `<DL><DT><A HREF="https://go.dev/blog/" TAGS="go">Imported title</A></DL>`
	if _, err := imports.Import(t.Context(), owner, importing.Choices{}, strings.NewReader(file)); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(t.Context(), owner, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(slugsOf(got), []string{"go", "news"}) {
		t.Errorf("tags = %v, want the import to add go", got.Tags)
	}
}

func TestUpdate_ConcurrentEditsApplyOneAfterTheOther(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ids := seed(t, pool, owner, seedArticle{url: "https://a.example/1", title: "one", created: base, tags: []string{"start"}})

	const writers = 8
	errs := make(chan error, writers)
	for i := range writers {
		go func() {
			_, err := svc.Update(t.Context(), owner, ids[0], library.Patch{Tags: &[]string{"a" + strings.Repeat("x", i), "shared"}})
			errs <- err
		}()
	}
	for range writers {
		if err := <-errs; err != nil {
			t.Errorf("Update() error: %v", err)
		}
	}

	got, err := svc.Get(t.Context(), owner, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if slugs := slugsOf(got); len(slugs) != 2 || slugs[1] != "shared" {
		t.Errorf("tags = %v, want exactly one writer's set, not a mix", slugs)
	}
}

func TestUpdate_ConcurrentEditsOfDifferentArticlesDoNotDeadlock(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ids := seed(t, pool, owner,
		seedArticle{url: "https://a.example/1", title: "one", created: base},
		seedArticle{url: "https://a.example/2", title: "two", created: base},
	)

	for round := range 20 {
		tags := make([]string, 30)
		for i := range tags {
			tags[i] = "r" + strings.Repeat("x", round) + "-" + string(rune('a'+i%26)) + strings.Repeat("y", i/26)
		}
		reversed := slices.Clone(tags)
		slices.Reverse(reversed)

		errs := make(chan error, 2)
		go func() { _, err := svc.Update(t.Context(), owner, ids[0], library.Patch{Tags: &tags}); errs <- err }()
		go func() { _, err := svc.Update(t.Context(), owner, ids[1], library.Patch{Tags: &reversed}); errs <- err }()
		for range 2 {
			if err := <-errs; err != nil {
				t.Fatalf("round %d: Update() error: %v", round, err)
			}
		}
	}
}

func TestUpdate_ConcurrentImportAndEditDoNotDeadlock(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	imports := importing.NewService(importingpg.NewRepository(pool), 100)
	ids := seed(t, pool, owner, seedArticle{url: "https://a.example/edited", title: "edited", created: base})

	for round := range 20 {
		tags := make([]string, 30)
		for i := range tags {
			tags[i] = fmt.Sprintf("r%02d-%02d", round, i)
		}
		descending := slices.Clone(tags)
		slices.Reverse(descending)
		file := fmt.Sprintf(`<DL><DT><A HREF="https://a.example/imported-%d" TAGS="%s">imported</A></DL>`, round, strings.Join(descending, ","))

		errs := make(chan error, 2)
		go func() {
			_, err := imports.Import(t.Context(), owner, importing.Choices{}, strings.NewReader(file))
			errs <- err
		}()
		go func() { _, err := svc.Update(t.Context(), owner, ids[0], library.Patch{Tags: &tags}); errs <- err }()
		for range 2 {
			if err := <-errs; err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
		}
	}
}

func TestUpdate_ConcurrentImportAndTagEditOfTheSameArticle(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	imports := importing.NewService(importingpg.NewRepository(pool), 100)

	for round := range 20 {
		url := fmt.Sprintf("https://a.example/same-%d", round)
		ids := seed(t, pool, owner, seedArticle{url: url, title: "same", created: base})
		tags := make([]string, 30)
		for i := range tags {
			tags[i] = fmt.Sprintf("s%02d-%02d", round, i)
		}
		descending := slices.Clone(tags)
		slices.Reverse(descending)
		file := fmt.Sprintf(`<DL><DT><A HREF="%s" TAGS="%s,imported-%d">same</A></DL>`, url, strings.Join(descending, ","), round)

		errs := make(chan error, 2)
		go func() {
			_, err := imports.Import(t.Context(), owner, importing.Choices{}, strings.NewReader(file))
			errs <- err
		}()
		go func() { _, err := svc.Update(t.Context(), owner, ids[0], library.Patch{Tags: &tags}); errs <- err }()
		for range 2 {
			if err := <-errs; err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
		}

		got, err := svc.Get(t.Context(), owner, ids[0])
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(slugsOf(got), tags) {
			t.Fatalf("round %d: tags = %v, want exactly the edited set whichever finished first", round, slugsOf(got))
		}
	}
}

// waitForLockWait waits until a session of the test database waits on a lock.
func waitForLockWait(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no session waited on a lock")
}

func TestImport_WaitsForATagEditInProgressAndKeepsIt(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	imports := importing.NewService(importingpg.NewRepository(pool), 100)
	ids := seed(t, pool, owner, seedArticle{url: "https://go.dev/blog/", title: "one", created: base})

	// A tag edit that has locked the article, as Update does first, and has
	// not marked its tags yet.
	edit, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = edit.Rollback(t.Context()) }()
	if _, err := edit.Exec(t.Context(), `SELECT 1 FROM articles WHERE id = $1 FOR UPDATE`, ids[0]); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := imports.Import(t.Context(), owner, importing.Choices{}, strings.NewReader(`<DL><DT><A HREF="https://go.dev/blog/" TAGS="go">x</A></DL>`))
		done <- err
	}()
	waitForLockWait(t, pool)
	if _, err := edit.Exec(t.Context(), `UPDATE articles SET tags_edited_at = now() WHERE id = $1`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if err := edit.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Import() error: %v", err)
	}

	got, err := svc.Get(t.Context(), owner, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tags) != 0 {
		t.Errorf("tags = %v, want none: the import ran after the edit committed", got.Tags)
	}
}
