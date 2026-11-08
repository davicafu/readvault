//go:build integration

package content_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/adapter/postgres"
	"readvault/api/internal/content"
	"readvault/api/internal/testdb"
)

const owner = postgres.DevOwnerID

// fakeFetcher serves canned responses by URL and records the requests.
type fakeFetcher struct {
	mu        sync.Mutex
	responses map[string]content.Fetched
	errs      map[string]error
	requested []string
}

func (f *fakeFetcher) Fetch(_ context.Context, rawURL string, maxBytes int64, _ []string) (content.Fetched, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requested = append(f.requested, rawURL)
	if err, ok := f.errs[rawURL]; ok {
		return content.Fetched{}, err
	}
	got, ok := f.responses[rawURL]
	if !ok {
		return content.Fetched{}, &content.FetchError{Problem: content.ProblemHTTPStatus, Status: 404}
	}
	if int64(len(got.Body)) > maxBytes {
		return content.Fetched{}, &content.FetchError{Problem: content.ProblemTooLarge}
	}
	if got.URL == "" {
		got.URL = rawURL
	}
	return got, nil
}

func pngBytes(t *testing.T, side int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, side, side))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newArticle(t *testing.T, pool *pgxpool.Pool, userID, rawURL string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(), `INSERT INTO articles (user_id, original_url, canonical_url, title)
		VALUES ($1, $2, $2, 'x') RETURNING id`, userID, rawURL).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func newUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(), `INSERT INTO users DEFAULT VALUES RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

const articleURL = "https://blog.example/posts/queues"

const articlePage = `<html><body><nav>Menu</nav><article>
<h1>Queues</h1><p>Queues decouple producers and consumers.</p>
<img src="/img/diagram.png" alt="Diagram">
<img src="https://cdn.example/photo.png" alt="Photo">
<img src="https://cdn.example/page.png" alt="Not an image">
<img src="https://cdn.example/gone.png" alt="Gone">
</article></body></html>`

func setup(t *testing.T) (*content.Service, *fakeFetcher, *pgxpool.Pool, string) {
	t.Helper()
	pool := testdb.New(t)
	f := &fakeFetcher{
		responses: map[string]content.Fetched{
			"https://blog.example/img/diagram.png": {ContentType: "image/png", Body: pngBytes(t, 80)},
			"https://cdn.example/photo.png":        {ContentType: "application/octet-stream", Body: pngBytes(t, 90)},
			"https://cdn.example/page.png":         {ContentType: "image/png", Body: []byte("<html><script>x()</script></html>")},
		},
		errs: map[string]error{},
	}
	svc := content.NewService(postgres.NewContentRepository(pool), f, 64<<10)
	return svc, f, pool, newArticle(t, pool, owner, articleURL)
}

func TestUpload_SavesHTMLAsMarkdownWithTheArticleImages(t *testing.T) {
	t.Parallel()
	svc, _, _, id := setup(t)

	got, err := svc.Upload(t.Context(), owner, id, "saved.html", strings.NewReader(articlePage))
	if err != nil {
		t.Fatalf("Upload() error: %v", err)
	}
	if got.Status != content.StatusComplete || got.Source != content.SourceUploadedHTML || got.Problem != nil {
		t.Errorf("status %q source %q problem %v", got.Status, got.Source, got.Problem)
	}
	if !strings.HasPrefix(got.Markdown, "# Queues\n\nQueues decouple producers and consumers.") || strings.Contains(got.Markdown, "Menu") {
		t.Errorf("markdown = %q", got.Markdown)
	}
	if len(got.Images) != 2 || got.Images[0].SourceURL != "https://blog.example/img/diagram.png" || got.Images[1].SourceURL != "https://cdn.example/photo.png" {
		t.Errorf("images = %+v, want the diagram and the photo in text order", got.Images)
	}
	if got.MissingImages != 2 {
		t.Errorf("missing images = %d, want 2 (the HTML posing as an image and the 404)", got.MissingImages)
	}

	img, err := svc.Image(t.Context(), owner, id, got.Images[1].ID)
	if err != nil || img.ContentType != "image/png" || !bytes.Equal(img.Data, pngBytes(t, 90)) {
		t.Errorf("stored image: type %q, %d bytes, error %v; want the sniffed PNG", img.ContentType, len(img.Data), err)
	}
	stored, err := svc.Get(t.Context(), owner, id)
	if err != nil || stored.Markdown != got.Markdown || len(stored.Images) != 2 {
		t.Errorf("Get() = %+v, %v; want what Upload returned", stored, err)
	}
}

func TestUpload_KeepsMarkdownAsUploaded(t *testing.T) {
	t.Parallel()
	svc, f, _, id := setup(t)
	md := "# Notes\n\nPlain words with a ![diagram](/img/diagram.png).\n"

	got, err := svc.Upload(t.Context(), owner, id, "notes.md", strings.NewReader(md))
	if err != nil {
		t.Fatalf("Upload() error: %v", err)
	}
	if got.Markdown != md || got.Source != content.SourceUploadedMarkdown || len(got.Images) != 1 {
		t.Errorf("got %+v", got)
	}
	if len(f.requested) != 1 || f.requested[0] != "https://blog.example/img/diagram.png" {
		t.Errorf("requested %v, want only the image resolved against the article URL", f.requested)
	}
}

func TestUpload_ReplacesTheTextAndItsImages(t *testing.T) {
	t.Parallel()
	svc, _, pool, id := setup(t)
	if _, err := svc.Upload(t.Context(), owner, id, "a.html", strings.NewReader(articlePage)); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Upload(t.Context(), owner, id, "b.md", strings.NewReader("Second version without images.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Markdown != "Second version without images.\n" || len(got.Images) != 0 || got.MissingImages != 0 {
		t.Errorf("got %+v, want the new text only", got)
	}
	var images int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM article_images`).Scan(&images); err != nil || images != 0 {
		t.Errorf("stored images = %d (%v), want the old ones removed", images, err)
	}
}

func TestUpload_RejectsWithoutSaving(t *testing.T) {
	t.Parallel()
	svc, _, pool, id := setup(t)
	other := newArticle(t, pool, newUser(t, pool), "https://other.example/a")

	tests := []struct {
		name     string
		article  string
		file     string
		expected error
	}{
		{"file over the limit", id, "<p>" + strings.Repeat("a ", 40<<10) + "</p>", content.ErrTooLarge},
		{"no text", id, "<html><body><nav>only a menu</nav><script>x()</script></body></html>", content.ErrNoText},
		{"another user's article", other, "<p>text</p>", content.ErrNotFound},
		{"malformed id", "not-a-uuid", "<p>text</p>", content.ErrNotFound},
	}
	for _, tt := range tests {
		if _, err := svc.Upload(t.Context(), owner, tt.article, "page.html", strings.NewReader(tt.file)); !errors.Is(err, tt.expected) {
			t.Errorf("%s: error = %v, want %v", tt.name, err, tt.expected)
		}
	}
	var rows int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM article_contents`).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("article_contents rows = %d (%v), want none", rows, err)
	}
}

func TestUpload_RepairsTextPostgresCannotStore(t *testing.T) {
	t.Parallel()
	svc, _, _, id := setup(t)

	got, err := svc.Upload(t.Context(), owner, id, "legacy.md", strings.NewReader("caf\xe9 and a\x00b\n"))
	if err != nil {
		t.Fatalf("Upload() error: %v", err)
	}
	if got.Markdown != "caf� and ab\n" {
		t.Errorf("markdown = %q", got.Markdown)
	}
}

func TestFetch_SavesThePageAndMarksAPaidPreviewIncomplete(t *testing.T) {
	t.Parallel()
	svc, f, _, id := setup(t)
	f.responses[articleURL] = content.Fetched{
		URL:         "https://blog.example/posts/queues-final",
		ContentType: "text/html",
		Body:        []byte(`<html><head><script type="application/ld+json">{"isAccessibleForFree":false}</script></head><body><article><p>The first lines.</p><img src="../img/diagram.png" alt=""></article></body></html>`),
	}

	got, err := svc.Fetch(t.Context(), owner, id)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if got.Status != content.StatusIncomplete || got.Source != content.SourceFetched || got.Markdown == "" {
		t.Errorf("got %+v, want an incomplete fetched text", got)
	}
	if len(got.Images) != 1 || got.Images[0].SourceURL != "https://blog.example/img/diagram.png" {
		t.Errorf("images = %+v, want the image resolved against the final URL", got.Images)
	}
}

func TestFetch_FailureIsRecordedAndKeepsSavedText(t *testing.T) {
	t.Parallel()
	svc, f, _, id := setup(t)
	f.errs[articleURL] = &content.FetchError{Problem: content.ProblemHTTPStatus, Status: 403}

	_, err := svc.Fetch(t.Context(), owner, id)
	var fe *content.FetchError
	if !errors.As(err, &fe) || fe.Problem != content.ProblemHTTPStatus || fe.Status != 403 {
		t.Fatalf("Fetch() error = %v, want the 403", err)
	}
	failed, err := svc.Get(t.Context(), owner, id)
	if err != nil || failed.Status != content.StatusFailed || failed.Problem == nil || *failed.Problem != "http_status" || failed.Markdown != "" {
		t.Errorf("after a first failure: %+v, %v; want a failed entry", failed, err)
	}

	if _, err := svc.Upload(t.Context(), owner, id, "page.md", strings.NewReader("Uploaded text.\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Fetch(t.Context(), owner, id); !errors.As(err, &fe) {
		t.Fatalf("second Fetch() error = %v, want a FetchError", err)
	}
	kept, err := svc.Get(t.Context(), owner, id)
	if err != nil || kept.Status != content.StatusComplete || kept.Markdown != "Uploaded text.\n" || kept.Problem == nil {
		t.Errorf("after a failure over saved text: %+v, %v; want the text kept and the problem noted", kept, err)
	}
}

func TestFetch_PageWithoutTextIsAFailure(t *testing.T) {
	t.Parallel()
	svc, f, _, id := setup(t)
	f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(`<html><body><script>app()</script></body></html>`)}

	_, err := svc.Fetch(t.Context(), owner, id)
	var fe *content.FetchError
	if !errors.As(err, &fe) || fe.Problem != content.ProblemNoText {
		t.Errorf("Fetch() error = %v, want no_text", err)
	}
}

func TestContent_IsScopedToTheUser(t *testing.T) {
	t.Parallel()
	svc, _, pool, id := setup(t)
	saved, err := svc.Upload(t.Context(), owner, id, "a.html", strings.NewReader(articlePage))
	if err != nil {
		t.Fatal(err)
	}
	other := newUser(t, pool)

	if _, err := svc.Get(t.Context(), other, id); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("Get() by another user: %v, want ErrNotFound", err)
	}
	if _, err := svc.Image(t.Context(), other, id, saved.Images[0].ID); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("Image() by another user: %v, want ErrNotFound", err)
	}
	if _, err := svc.Fetch(t.Context(), other, id); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("Fetch() by another user: %v, want ErrNotFound", err)
	}
	fresh := newArticle(t, pool, owner, "https://blog.example/b")
	if _, err := svc.Get(t.Context(), owner, fresh); !errors.Is(err, content.ErrNoContent) {
		t.Errorf("Get() without saved text: %v, want ErrNoContent", err)
	}
}

func TestUpload_SkipsAnImageWhoseAddressIsTooLongToStore(t *testing.T) {
	t.Parallel()
	svc, f, _, id := setup(t)
	long := "https://cdn.example/" + strings.Repeat("a", 4100) + ".png"
	f.responses[long] = content.Fetched{ContentType: "image/png", Body: pngBytes(t, 80)}

	got, err := svc.Upload(t.Context(), owner, id, "a.md", strings.NewReader("Text ![x]("+long+")\n"))
	if err != nil {
		t.Fatalf("Upload() error: %v, want the text saved without that image", err)
	}
	if len(got.Images) != 0 || got.MissingImages != 1 {
		t.Errorf("images %d missing %d, want 0 and 1", len(got.Images), got.MissingImages)
	}
}

func TestUpload_TextTooLargeToStoreIsRejected(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	svc := content.NewService(postgres.NewContentRepository(pool), &fakeFetcher{}, 8<<20)
	id := newArticle(t, pool, owner, articleURL)

	// 3 MiB where every other byte is invalid become 6 MiB once each one is
	// U+FFFD: over what the text column holds.
	_, err := svc.Upload(t.Context(), owner, id, "legacy.md", strings.NewReader("word "+strings.Repeat("\xffa", 3<<19)))
	if !errors.Is(err, content.ErrTooLarge) {
		t.Errorf("Upload() error = %v, want ErrTooLarge", err)
	}
}

func TestFetch_ClassifiesErrorsThatAreNotFetchErrors(t *testing.T) {
	t.Parallel()
	svc, f, _, id := setup(t)
	f.errs[articleURL] = context.DeadlineExceeded

	_, err := svc.Fetch(t.Context(), owner, id)
	var fe *content.FetchError
	if !errors.As(err, &fe) || fe.Problem != content.ProblemTimeout {
		t.Errorf("Fetch() error = %v, want timeout", err)
	}
}
