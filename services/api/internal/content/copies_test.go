//go:build integration

package content_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"readvault/api/internal/adapter/postgres"
	"readvault/api/internal/content"
)

const archive = "https://archive.example/web/2id_/"

const archivePage = `<html><body><article><h1>Queues</h1><p>The archived copy of the whole article.</p><img src="/img/diagram.png" alt="diagram"></article></body></html>`

// setupWithCopies gives the service the mirror, which serves only pages
// mirrorServes accepts, and the archive, which serves every page.
func setupWithCopies(t *testing.T, mirrorServes bool) (*content.Service, *fakeFetcher, string) {
	t.Helper()
	_, f, pool, id := setup(t)
	serves := func(_ context.Context, _ string) bool { return mirrorServes }
	svc := content.NewService(postgres.NewContentRepository(pool), f, 64<<10,
		content.WithMirror(mirror, serves), content.WithArchive(archive))
	return svc, f, id
}

func TestFetch_UsesTheArchivedCopyOfAnySiteThatFails(t *testing.T) {
	t.Parallel()
	tests := map[string]func(f *fakeFetcher){
		"http error": func(f *fakeFetcher) {
			f.errs[articleURL] = &content.FetchError{Problem: content.ProblemHTTPStatus, Status: 404}
		},
		"page without text": func(f *fakeFetcher) {
			f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(`<html><body></body></html>`)}
		},
		"paid preview": func(f *fakeFetcher) {
			f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(`<html><body><article><p>Member-only story</p><p>The first lines.</p></article></body></html>`)}
		},
	}
	for name, original := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			svc, f, id := setupWithCopies(t, false)
			original(f)
			f.responses[archive+articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(archivePage)}

			got, err := svc.Fetch(t.Context(), owner, id)
			if err != nil || got.Status != content.StatusComplete || !strings.Contains(got.Markdown, "archived copy of the whole article") {
				t.Errorf("got %+v, %v; want the archived copy saved as complete", got, err)
			}
			if n := requestsWithPrefix(f, mirror); n != 0 {
				t.Errorf("the mirror was asked %d times for a page it does not serve", n)
			}
		})
	}
}

// Relative images of an archived copy belong to the original site.
func TestFetch_ResolvesTheArchivedCopysImagesAgainstTheOriginalAddress(t *testing.T) {
	t.Parallel()
	svc, f, id := setupWithCopies(t, false)
	f.errs[articleURL] = &content.FetchError{Problem: content.ProblemHTTPStatus, Status: 403}
	f.responses[archive+articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(archivePage)}

	if _, err := svc.Fetch(t.Context(), owner, id); err != nil {
		t.Fatal(err)
	}
	if n := requestsWithPrefix(f, "https://blog.example/img/diagram.png"); n != 1 {
		f.mu.Lock()
		defer f.mu.Unlock()
		t.Errorf("requested %v, want the image from the original site", f.requested)
	}
}

func TestFetch_TriesTheMirrorBeforeTheArchive(t *testing.T) {
	t.Parallel()
	svc, f, id := setupWithCopies(t, true)
	f.errs[articleURL] = &content.FetchError{Problem: content.ProblemHTTPStatus, Status: 403}
	f.responses[mirror+articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(mirrorPage)}
	f.responses[archive+articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(archivePage)}

	got, err := svc.Fetch(t.Context(), owner, id)
	if err != nil || !strings.Contains(got.Markdown, "free to read on the mirror") {
		t.Errorf("got %+v, %v; want the mirror's text", got, err)
	}
	if n := requestsWithPrefix(f, archive); n != 0 {
		t.Errorf("the archive was asked %d times, want none once the mirror had the text", n)
	}
}

func TestFetch_KeepsTheOriginalFailureWhenNoCopyHelps(t *testing.T) {
	t.Parallel()
	svc, f, id := setupWithCopies(t, false)
	f.errs[articleURL] = &content.FetchError{Problem: content.ProblemHTTPStatus, Status: 403}
	// The archive has no copy: the fake answers 404.

	var fe *content.FetchError
	if _, err := svc.Fetch(t.Context(), owner, id); !errors.As(err, &fe) || fe.Status != 403 {
		t.Errorf("Fetch() error = %v, want the original 403", err)
	}
}

// Like the mirror, the archive only hears of sites that answered: an
// address that did not resolve may be an intranet host.
func TestFetch_NeverSendsUnansweredAddressesToTheArchive(t *testing.T) {
	t.Parallel()
	for _, problem := range []string{content.ProblemUnreachable, content.ProblemBlockedAddress, content.ProblemTimeout, content.ProblemNotHTML, content.ProblemTooLarge} {
		t.Run(problem, func(t *testing.T) {
			t.Parallel()
			svc, f, id := setupWithCopies(t, false)
			f.errs[articleURL] = &content.FetchError{Problem: problem}
			f.responses[archive+articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(archivePage)}

			_, _ = svc.Fetch(t.Context(), owner, id)
			if n := requestsWithPrefix(f, archive); n != 0 {
				t.Errorf("the archive was asked %d times, want never for %s", n, problem)
			}
		})
	}
}

func TestFetchCopy_ReplacesTheTextWithAnotherCopyWithoutAskingTheOriginal(t *testing.T) {
	t.Parallel()
	svc, f, id := setupWithCopies(t, false)
	f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(`<html><body><article><p>Only the start of the article.</p></article></body></html>`)}
	if _, err := svc.Fetch(t.Context(), owner, id); err != nil {
		t.Fatal(err)
	}
	f.responses[archive+articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(archivePage)}
	before := requestsWithPrefix(f, articleURL)

	got, err := svc.FetchCopy(t.Context(), owner, id)
	if err != nil || got.Status != content.StatusComplete || !strings.Contains(got.Markdown, "archived copy") {
		t.Errorf("got %+v, %v; want the archived copy saved", got, err)
	}
	if n := requestsWithPrefix(f, articleURL); n != before {
		t.Errorf("the original was asked again (%d times), want only the copies", n-before)
	}
}

func TestFetchCopy_WithoutACopyKeepsWhatWasSaved(t *testing.T) {
	t.Parallel()
	svc, f, id := setupWithCopies(t, false)
	f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(`<html><body><article><p>Only the start of the article.</p></article></body></html>`)}
	if _, err := svc.Fetch(t.Context(), owner, id); err != nil {
		t.Fatal(err)
	}

	_, err := svc.FetchCopy(t.Context(), owner, id)
	var fe *content.FetchError
	if !errors.As(err, &fe) || fe.Problem != content.ProblemNoCopy {
		t.Errorf("FetchCopy() error = %v, want no_copy", err)
	}
	got, err := svc.Get(t.Context(), owner, id)
	if err != nil || got.Status != content.StatusComplete || got.Problem != nil || !strings.Contains(got.Markdown, "Only the start") {
		t.Errorf("saved text after = %+v, %v; want it unchanged and no failure recorded", got, err)
	}
}

func TestFetchCopy_UnknownArticleIsNotFound(t *testing.T) {
	t.Parallel()
	svc, _, _ := setupWithCopies(t, false)
	if _, err := svc.FetchCopy(t.Context(), owner, "not-a-uuid"); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
