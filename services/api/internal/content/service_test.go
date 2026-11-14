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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/content"
	contentpg "readvault/api/internal/content/postgres"
	"readvault/api/internal/platform/postgres"
	"readvault/api/internal/testdb"
)

const owner = postgres.DevOwnerID

// fakeFetcher serves canned responses by URL and records the requests.
type fakeFetcher struct {
	mu        sync.Mutex
	responses map[string]content.Fetched
	errs      map[string]error
	requested []string
	// delay makes every request take this long, or until ctx ends.
	delay time.Duration
}

func (f *fakeFetcher) Fetch(ctx context.Context, rawURL string, maxBytes int64, _ []string) (content.Fetched, error) {
	f.mu.Lock()
	f.requested = append(f.requested, rawURL)
	delay := f.delay
	f.mu.Unlock()
	if delay > 0 {
		select {
		case <-ctx.Done():
			return content.Fetched{}, ctx.Err()
		case <-time.After(delay):
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
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
	svc := content.NewService(contentpg.NewRepository(pool), f, 64<<10)
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
	svc := content.NewService(contentpg.NewRepository(pool), &fakeFetcher{}, 8<<20)
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

const mirror = "https://mirror.example/"

// everyPage lets the mirror serve every article.
func everyPage(context.Context, string) bool { return true }

func setupWithMirror(t *testing.T) (*content.Service, *fakeFetcher, string) {
	t.Helper()
	_, f, pool, id := setup(t)
	svc := content.NewService(contentpg.NewRepository(pool), f, 64<<10, content.WithMirror(mirror, everyPage))
	return svc, f, id
}

const mirrorPage = `<html><body><nav>Mirror menu</nav><article><h1>Queues</h1><p>The whole article, free to read on the mirror.</p></article></body></html>`

func TestFetch_UsesTheMirrorWhenTheOriginalFails(t *testing.T) {
	t.Parallel()
	svc, f, id := setupWithMirror(t)
	f.errs[articleURL] = &content.FetchError{Problem: content.ProblemHTTPStatus, Status: 403}
	f.responses[mirror+articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(mirrorPage)}

	got, err := svc.Fetch(t.Context(), owner, id)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if got.Status != content.StatusComplete || got.Source != content.SourceFetched || got.Problem != nil ||
		!strings.Contains(got.Markdown, "The whole article") {
		t.Errorf("got %+v, want the mirror's text saved as complete", got)
	}
}

func TestFetch_UsesTheMirrorForAPaidPreview(t *testing.T) {
	t.Parallel()
	svc, f, id := setupWithMirror(t)
	f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(`<html><body><article><p>Member-only story</p><p>The first lines.</p></article></body></html>`)}
	f.responses[mirror+articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(mirrorPage)}

	got, err := svc.Fetch(t.Context(), owner, id)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if got.Status != content.StatusComplete || !strings.Contains(got.Markdown, "The whole article") {
		t.Errorf("got %+v, want the mirror's whole text", got)
	}
}

func TestFetch_KeepsTheOriginalOutcomeWhenTheMirrorDoesNotHelp(t *testing.T) {
	t.Parallel()

	t.Run("paid preview, mirror fails", func(t *testing.T) {
		t.Parallel()
		svc, f, id := setupWithMirror(t)
		f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(`<html><body><article><p>Member-only story</p><p>The first lines.</p></article></body></html>`)}
		f.errs[mirror+articleURL] = &content.FetchError{Problem: content.ProblemTimeout}

		got, err := svc.Fetch(t.Context(), owner, id)
		if err != nil || got.Status != content.StatusIncomplete || !strings.Contains(got.Markdown, "The first lines") {
			t.Errorf("got %+v, %v; want the preview saved as incomplete", got, err)
		}
	})

	t.Run("original fails, mirror is a paid preview too", func(t *testing.T) {
		t.Parallel()
		svc, f, id := setupWithMirror(t)
		f.errs[articleURL] = &content.FetchError{Problem: content.ProblemHTTPStatus, Status: 403}
		f.responses[mirror+articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(`<html><body><article><p>Member-only story</p><p>Lines.</p></article></body></html>`)}

		_, err := svc.Fetch(t.Context(), owner, id)
		var fe *content.FetchError
		if !errors.As(err, &fe) || fe.Problem != content.ProblemHTTPStatus || fe.Status != 403 {
			t.Fatalf("Fetch() error = %v, want the original 403", err)
		}
		got, err := svc.Get(t.Context(), owner, id)
		if err != nil || got.Status != content.StatusFailed || got.Problem == nil || *got.Problem != "http_status" {
			t.Errorf("got %+v, %v; want the original failure recorded", got, err)
		}
	})
}

func TestFetch_DoesNotAskTheMirrorWhenTheOriginalWorks(t *testing.T) {
	t.Parallel()
	svc, f, id := setupWithMirror(t)
	f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(`<html><body><article><p>Free text.</p></article></body></html>`)}

	if _, err := svc.Fetch(t.Context(), owner, id); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.requested {
		if strings.HasPrefix(u, mirror) {
			t.Errorf("requested %s; the mirror must not be asked for a page that works", u)
		}
	}
}

func TestFetch_WithoutAMirrorTheFailureStands(t *testing.T) {
	t.Parallel()
	svc, f, _, id := setup(t)
	f.errs[articleURL] = &content.FetchError{Problem: content.ProblemHTTPStatus, Status: 403}
	f.responses[mirror+articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(mirrorPage)}

	var fe *content.FetchError
	if _, err := svc.Fetch(t.Context(), owner, id); !errors.As(err, &fe) {
		t.Errorf("Fetch() error = %v, want the original failure", err)
	}
}

func TestFetch_AMirrorAddressIsNotSentToTheMirrorAgain(t *testing.T) {
	t.Parallel()
	_, f, pool, _ := setup(t)
	svc := content.NewService(contentpg.NewRepository(pool), f, 64<<10, content.WithMirror(mirror, everyPage))
	saved := mirror + articleURL
	id := newArticle(t, pool, owner, saved)
	f.errs[saved] = &content.FetchError{Problem: content.ProblemTimeout}

	_, err := svc.Fetch(t.Context(), owner, id)
	var fe *content.FetchError
	if !errors.As(err, &fe) || fe.Problem != content.ProblemTimeout {
		t.Errorf("Fetch() error = %v, want the timeout", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requested) != 1 {
		t.Errorf("requested %v, want only the saved address", f.requested)
	}
}

func TestFetch_NeverSendsABlockedAddressToTheMirror(t *testing.T) {
	t.Parallel()
	svc, f, id := setupWithMirror(t)
	f.errs[articleURL] = &content.FetchError{Problem: content.ProblemBlockedAddress}
	f.responses[mirror+articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(mirrorPage)}

	var fe *content.FetchError
	if _, err := svc.Fetch(t.Context(), owner, id); !errors.As(err, &fe) || fe.Problem != content.ProblemBlockedAddress {
		t.Errorf("Fetch() error = %v, want blocked_address", err)
	}
	if n := requestsWithPrefix(f, mirror); n != 0 {
		t.Errorf("the mirror was asked %d times, want never for a blocked address", n)
	}
}

func TestFetch_DoesNotAskTheMirrorOnceCancelled(t *testing.T) {
	t.Parallel()
	svc, f, id := setupWithMirror(t)
	f.delay = 5 * time.Second
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, _ = svc.Fetch(ctx, owner, id)
	if n := requestsWithPrefix(f, mirror); n != 0 {
		t.Errorf("the mirror was asked %d times after the download was cancelled, want 0", n)
	}
}

func requestsWithPrefix(f *fakeFetcher, prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.requested {
		if strings.HasPrefix(u, prefix) {
			n++
		}
	}
	return n
}

// Only a page whose site answered (an HTTP error or a paid preview) goes to
// the mirror: an address that did not resolve or answer may be an intranet
// host.
func TestFetch_SendsOnlyAnsweredFailuresToTheMirror(t *testing.T) {
	t.Parallel()
	for _, problem := range []string{content.ProblemUnreachable, content.ProblemTimeout, content.ProblemNotHTML, content.ProblemTooLarge} {
		t.Run(problem, func(t *testing.T) {
			t.Parallel()
			svc, f, id := setupWithMirror(t)
			f.errs[articleURL] = &content.FetchError{Problem: problem}
			f.responses[mirror+articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(mirrorPage)}

			var fe *content.FetchError
			if _, err := svc.Fetch(t.Context(), owner, id); !errors.As(err, &fe) || fe.Problem != problem {
				t.Errorf("Fetch() error = %v, want %s", err, problem)
			}
			if n := requestsWithPrefix(f, mirror); n != 0 {
				t.Errorf("the mirror was asked %d times, want never for %s", n, problem)
			}
		})
	}
}

func TestFetch_SendsOnlyPagesTheMirrorServesToIt(t *testing.T) {
	t.Parallel()
	_, f, pool, id := setup(t)
	var asked []string
	onlyMedium := func(_ context.Context, rawURL string) bool {
		asked = append(asked, rawURL)
		return false
	}
	svc := content.NewService(contentpg.NewRepository(pool), f, 64<<10, content.WithMirror(mirror, onlyMedium))
	f.errs[articleURL] = &content.FetchError{Problem: content.ProblemHTTPStatus, Status: 403}
	f.responses[mirror+articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(mirrorPage)}

	var fe *content.FetchError
	if _, err := svc.Fetch(t.Context(), owner, id); !errors.As(err, &fe) || fe.Status != 403 {
		t.Errorf("Fetch() error = %v, want the original 403", err)
	}
	if n := requestsWithPrefix(f, mirror); n != 0 {
		t.Errorf("the mirror was asked %d times, want never for a page it does not serve", n)
	}
	if len(asked) != 1 || asked[0] != articleURL {
		t.Errorf("asked about %v, want the article's address", asked)
	}
}
