//go:build integration

package library_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/adapter/postgres"
	"readvault/api/internal/library"
	"readvault/api/internal/tag"
	"readvault/api/internal/testdb"
)

const owner = postgres.DevOwnerID

type seedArticle struct {
	url     string
	title   string
	created time.Time
	folder  []string
	tags    []string
}

var base = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// seed stores articles directly, so tests control creation times.
func seed(t *testing.T, pool *pgxpool.Pool, userID string, articles ...seedArticle) []string {
	t.Helper()
	ctx := t.Context()
	ids := make([]string, len(articles))
	for i, a := range articles {
		if err := pool.QueryRow(ctx, `INSERT INTO articles (user_id, original_url, canonical_url, title, original_folder, bookmarked_at, created_at, updated_at)
			VALUES ($1, $2, $2, $3, COALESCE($4, '{}'::text[]), $5, $5, $5) RETURNING id`, userID, a.url, a.title, a.folder, a.created).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
		for _, name := range a.tags {
			slug := strings.ToLower(name)
			if _, err := pool.Exec(ctx, `INSERT INTO tags (user_id, name, slug) VALUES ($1, $2, $3) ON CONFLICT (user_id, slug) DO NOTHING`, userID, name, slug); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO article_tags (user_id, article_id, tag_id)
				SELECT $1, $2, id FROM tags WHERE user_id = $1 AND slug = $3`, userID, ids[i], slug); err != nil {
				t.Fatal(err)
			}
		}
	}
	return ids
}

func newUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(), `INSERT INTO users DEFAULT VALUES RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func newService(t *testing.T) (*library.Service, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	return library.NewService(postgres.NewLibraryRepository(pool)), pool
}

func titles(items []library.Article) []string {
	out := make([]string, len(items))
	for i, a := range items {
		out[i] = a.Title
	}
	return out
}

// listAll pages through a list and returns every title in order and the page count.
func listAll(t *testing.T, svc *library.Service, p library.ListParams) ([]string, int) {
	t.Helper()
	var all []string
	pages := 0
	for {
		page, err := svc.List(t.Context(), owner, p)
		if err != nil {
			t.Fatalf("List(%+v) error: %v", p, err)
		}
		pages++
		all = append(all, titles(page.Items)...)
		if page.NextCursor == nil {
			return all, pages
		}
		if pages > 10 {
			t.Fatal("pagination does not end")
		}
		p.Cursor = *page.NextCursor
	}
}

func TestList_SortsAndPaginatesEveryOrder(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	seed(t, pool, owner,
		seedArticle{url: "https://a.example/1", title: "banana", created: base},
		seedArticle{url: "https://a.example/2", title: "Apple", created: base.Add(1 * time.Hour)},
		seedArticle{url: "https://a.example/3", title: "cherry", created: base.Add(2 * time.Hour)},
		seedArticle{url: "https://a.example/4", title: "apple", created: base.Add(3 * time.Hour)},
		seedArticle{url: "https://a.example/5", title: "Date", created: base.Add(4 * time.Hour)},
	)

	// "Apple" and "apple" share a title key; their order is by id, so titles
	// are compared without case.
	tests := []struct {
		sort     string
		expected []string
	}{
		{"", []string{"Date", "apple", "cherry", "Apple", "banana"}},
		{"created_desc", []string{"Date", "apple", "cherry", "Apple", "banana"}},
		{"created_asc", []string{"banana", "Apple", "cherry", "apple", "Date"}},
		{"title_asc", []string{"apple", "apple", "banana", "cherry", "date"}},
		{"title_desc", []string{"date", "cherry", "banana", "apple", "apple"}},
	}
	for _, tt := range tests {
		got, pages := listAll(t, svc, library.ListParams{Sort: tt.sort, Limit: ptr(2)})
		if pages != 3 || !slices.EqualFunc(got, tt.expected, strings.EqualFold) {
			t.Errorf("sort %q: %v in %d pages, want %v in 3", tt.sort, got, pages, tt.expected)
		}
	}
}

func TestList_SearchesTitleTagsAndURL(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	seed(t, pool, owner,
		seedArticle{url: "https://medium.com/@x/clean-code-123", title: "Clean code notes", created: base},
		seedArticle{url: "https://go.dev/blog/", title: "The Go Blog", created: base.Add(time.Hour), tags: []string{"Golang"}},
		seedArticle{url: "https://example.com/discount", title: "Save 50% today", created: base.Add(2 * time.Hour)},
	)

	tests := []struct {
		search   string
		expected []string
	}{
		{"go blog", []string{"The Go Blog"}},
		{"CLEAN", []string{"Clean code notes"}},
		{"medium.com", []string{"Clean code notes"}},
		{"golang", []string{"The Go Blog"}},
		{"50%", []string{"Save 50% today"}},
		{"%", []string{"Save 50% today"}},
		{"nothing-matches", []string{}},
	}
	for _, tt := range tests {
		page, err := svc.List(t.Context(), owner, library.ListParams{Search: tt.search})
		if err != nil {
			t.Fatalf("search %q: %v", tt.search, err)
		}
		if got := titles(page.Items); !slices.Equal(got, tt.expected) {
			t.Errorf("search %q = %v, want %v", tt.search, got, tt.expected)
		}
	}
}

func TestList_FiltersByEveryGivenTag(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	seed(t, pool, owner,
		seedArticle{url: "https://a.example/1", title: "go only", created: base, tags: []string{"go"}},
		seedArticle{url: "https://a.example/2", title: "go and news", created: base.Add(time.Hour), tags: []string{"go", "news"}},
		seedArticle{url: "https://a.example/3", title: "untagged", created: base.Add(2 * time.Hour)},
	)

	tests := []struct {
		tags     []string
		expected []string
	}{
		{[]string{"Go"}, []string{"go and news", "go only"}},
		{[]string{"go", "news"}, []string{"go and news"}},
		{[]string{"go", "GO"}, []string{"go and news", "go only"}},
		{[]string{"missing"}, []string{}},
	}
	for _, tt := range tests {
		page, err := svc.List(t.Context(), owner, library.ListParams{Tags: tt.tags})
		if err != nil {
			t.Fatalf("tags %v: %v", tt.tags, err)
		}
		if got := titles(page.Items); !slices.Equal(got, tt.expected) {
			t.Errorf("tags %v = %v, want %v", tt.tags, got, tt.expected)
		}
	}
}

func TestList_RejectsInvalidParameters(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	seed(t, pool, owner,
		seedArticle{url: "https://a.example/1", title: "one", created: base},
		seedArticle{url: "https://a.example/2", title: "two", created: base.Add(time.Hour)},
	)
	first, err := svc.List(t.Context(), owner, library.ListParams{Limit: ptr(1)})
	if err != nil || first.NextCursor == nil {
		t.Fatalf("first page: %+v, %v", first, err)
	}

	tests := []struct {
		name     string
		params   library.ListParams
		field    string
		expected string
	}{
		{"limit too small", library.ListParams{Limit: ptr(-1)}, "limit", "out_of_range"},
		{"limit too large", library.ListParams{Limit: ptr(101)}, "limit", "out_of_range"},
		{"limit zero", library.ListParams{Limit: ptr(0)}, "limit", "out_of_range"},
		{"search with a NUL byte", library.ListParams{Search: "go\x00"}, "q", "invalid"},
		{"search with invalid UTF-8", library.ListParams{Search: "caf\xe9"}, "q", "invalid"},
		{"tag with invalid UTF-8", library.ListParams{Tags: []string{"r\xe9sum\xe9"}}, "tag", "invalid"},
		{"unknown sort", library.ListParams{Sort: "popular"}, "sort", "invalid"},
		{"garbage cursor", library.ListParams{Cursor: "not-a-cursor"}, "cursor", "invalid"},
		{"cursor of another sort", library.ListParams{Sort: "title_asc", Cursor: *first.NextCursor}, "cursor", "invalid"},
		{"search too long", library.ListParams{Search: strings.Repeat("a", 201)}, "q", "too_long"},
	}
	for _, tt := range tests {
		_, err := svc.List(t.Context(), owner, tt.params)
		var verr *library.ValidationError
		if !errors.As(err, &verr) || len(verr.Problems) != 1 || verr.Problems[0].Field != tt.field || verr.Problems[0].Code != tt.expected {
			t.Errorf("%s: error = %#v, want one %s/%s problem", tt.name, err, tt.field, tt.expected)
		}
	}
}

func TestLibrary_OnlyShowsTheUsersOwnArticles(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	other := newUser(t, pool)
	seed(t, pool, owner, seedArticle{url: "https://a.example/mine", title: "mine", created: base})
	theirs := seed(t, pool, other, seedArticle{url: "https://a.example/theirs", title: "theirs", created: base, tags: []string{"go"}})

	page, err := svc.List(t.Context(), owner, library.ListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if got := titles(page.Items); !slices.Equal(got, []string{"mine"}) {
		t.Errorf("list = %v, want only the owner's article", got)
	}
	if _, err := svc.Get(t.Context(), owner, theirs[0]); !errors.Is(err, library.ErrNotFound) {
		t.Errorf("Get(other user's article) error = %v, want ErrNotFound", err)
	}
}

func TestGet_ReturnsTheArticleWithTagsAndProvenance(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ids := seed(t, pool, owner, seedArticle{
		url: "https://go.dev/blog/", title: "The Go Blog", created: base,
		folder: []string{"Bookmarks bar", "Go"}, tags: []string{"news", "Go"},
	})

	got, err := svc.Get(t.Context(), owner, ids[0])
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.ID != ids[0] || got.Title != "The Go Blog" || got.OriginalURL != "https://go.dev/blog/" {
		t.Errorf("article = %+v", got)
	}
	if !slices.Equal(got.Folder, []string{"Bookmarks bar", "Go"}) || got.BookmarkedAt == nil || !got.CreatedAt.Equal(base) {
		t.Errorf("provenance = folder %v, bookmarked %v, created %v", got.Folder, got.BookmarkedAt, got.CreatedAt)
	}
	var slugs []string
	for _, tg := range got.Tags {
		slugs = append(slugs, tg.Slug)
	}
	if !slices.Equal(slugs, []string{"go", "news"}) {
		t.Errorf("tags = %v, want [go news] sorted by slug", got.Tags)
	}

	for _, id := range []string{"not-a-uuid", "00000000-0000-0000-0000-00000000dead"} {
		if _, err := svc.Get(t.Context(), owner, id); !errors.Is(err, library.ErrNotFound) {
			t.Errorf("Get(%q) error = %v, want ErrNotFound", id, err)
		}
	}
}

func TestGet_ReportsTimesInUTC(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ids := seed(t, pool, owner, seedArticle{url: "https://a.example/1", title: "one", created: base})

	got, err := svc.Get(t.Context(), owner, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	for name, ts := range map[string]time.Time{"createdAt": got.CreatedAt, "updatedAt": got.UpdatedAt} {
		if ts.Location() != time.UTC {
			t.Errorf("%s = %s, want UTC whatever the process time zone", name, ts)
		}
	}
}

func TestArticles_ReportTheSavedTextStatus(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ids := seed(t, pool, owner,
		seedArticle{url: "https://a.example/1", title: "with text", created: base},
		seedArticle{url: "https://a.example/2", title: "without text", created: base.Add(time.Hour)},
	)
	if _, err := pool.Exec(t.Context(), `INSERT INTO article_contents (article_id, user_id, status, source, markdown, words)
		VALUES ($1, $2, 'incomplete', 'fetched', 'start', 1)`, ids[0], owner); err != nil {
		t.Fatal(err)
	}

	page, err := svc.List(t.Context(), owner, library.ListParams{Sort: "created_asc"})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("List() = %+v, %v", page, err)
	}
	if s := page.Items[0].ContentStatus; s == nil || *s != "incomplete" {
		t.Errorf("first article contentStatus = %v, want incomplete", s)
	}
	if s := page.Items[1].ContentStatus; s != nil {
		t.Errorf("second article contentStatus = %q, want nil", *s)
	}
	got, err := svc.Get(t.Context(), owner, ids[0])
	if err != nil || got.ContentStatus == nil || *got.ContentStatus != "incomplete" {
		t.Errorf("Get() contentStatus = %v, %v", got.ContentStatus, err)
	}
}

func TestTags_CountsTheArticlesOfEachTag(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	user, other := newUser(t, pool), newUser(t, pool)
	seed(t, pool, user,
		seedArticle{url: "https://a.example/1", title: "one", created: base, tags: []string{"Go", "news"}},
		seedArticle{url: "https://a.example/2", title: "two", created: base, tags: []string{"Go"}},
		seedArticle{url: "https://a.example/3", title: "three", created: base, tags: []string{"api"}},
	)
	seed(t, pool, other, seedArticle{url: "https://a.example/1", title: "theirs", created: base, tags: []string{"secret", "Go"}})
	// A tag left without articles after an edit is not listed.
	if _, err := pool.Exec(t.Context(), `INSERT INTO tags (user_id, name, slug) VALUES ($1, 'orphan', 'orphan')`, user); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Tags(t.Context(), user)

	if err != nil {
		t.Fatal(err)
	}
	expected := []library.TagCount{
		{Tag: tag.Tag{Name: "Go", Slug: "go"}, Articles: 2},
		{Tag: tag.Tag{Name: "api", Slug: "api"}, Articles: 1},
		{Tag: tag.Tag{Name: "news", Slug: "news"}, Articles: 1},
	}
	if !slices.Equal(got, expected) {
		t.Errorf("tags = %+v, want %+v", got, expected)
	}
}

func TestTags_EmptyLibraryHasNoTags(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)

	got, err := svc.Tags(t.Context(), newUser(t, pool))

	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("tags = %#v, err %v; want an empty, non-nil list", got, err)
	}
}
