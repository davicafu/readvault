//go:build integration

package content_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/adapter/postgres"
	"readvault/api/internal/content"
)

func queue(t *testing.T, pool *pgxpool.Pool, articleIDs ...string) {
	t.Helper()
	for _, id := range articleIDs {
		if _, err := pool.Exec(t.Context(), `INSERT INTO content_downloads (article_id, user_id) VALUES ($1, $2)`, id, owner); err != nil {
			t.Fatal(err)
		}
	}
}

// runWorker runs the worker until the queue is empty, then stops it and
// waits for it to return.
func runWorker(t *testing.T, svc *content.Service, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	w := content.NewWorker(svc, postgres.NewDownloadQueue(pool, 0), content.WorkerConfig{Workers: 2, Pause: time.Millisecond, Idle: 10 * time.Millisecond})
	stopped := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(stopped)
	}()
	deadline := time.Now().Add(20 * time.Second)
	for queuedCount(t, pool) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("the queue was not emptied in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("the worker did not stop after its context was cancelled")
	}
}

func queuedCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM content_downloads`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func requests(f *fakeFetcher, rawURL string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.requested {
		if u == rawURL {
			n++
		}
	}
	return n
}

func TestWorker_DownloadsTheQueuedArticles(t *testing.T) {
	t.Parallel()
	svc, f, pool, first := setup(t)
	second := newArticle(t, pool, owner, "https://blog.example/posts/second")
	f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(`<html><body><article><p>First text.</p></article></body></html>`)}
	f.responses["https://blog.example/posts/second"] = content.Fetched{ContentType: "text/html", Body: []byte(`<html><body><article><p>Second text.</p></article></body></html>`)}
	queue(t, pool, first, second)

	runWorker(t, svc, pool)

	for _, id := range []string{first, second} {
		got, err := svc.Get(t.Context(), owner, id)
		if err != nil || got.Status != content.StatusComplete || got.Markdown == "" {
			t.Errorf("article %s: %+v, %v; want its text saved", id, got, err)
		}
	}
}

func TestWorker_SkipsAnArticleThatAlreadyHasText(t *testing.T) {
	t.Parallel()
	svc, f, pool, id := setup(t)
	if _, err := svc.Fetch(t.Context(), owner, id); err == nil {
		t.Fatal("setup: expected the first download to fail")
	}
	before := requests(f, articleURL)
	queue(t, pool, id)

	runWorker(t, svc, pool)

	if n := requests(f, articleURL); n != before {
		t.Errorf("the article was downloaded %d more times, want none: it already has a recorded download", n-before)
	}
}

func TestWorker_RecordsAFailedDownloadAndDoesNotRetryIt(t *testing.T) {
	t.Parallel()
	svc, f, pool, id := setup(t)
	f.errs[articleURL] = &content.FetchError{Problem: content.ProblemHTTPStatus, Status: 403}
	queue(t, pool, id)

	runWorker(t, svc, pool)

	got, err := svc.Get(t.Context(), owner, id)
	if err != nil || got.Status != content.StatusFailed || got.Problem == nil || *got.Problem != "http_status" {
		t.Errorf("got %+v, %v; want the failure recorded", got, err)
	}
	if n := requests(f, articleURL); n != 1 {
		t.Errorf("downloaded %d times, want 1", n)
	}
}

func TestDownloadQueue_ClaimsEachArticleOnceUntilItsClaimRunsOut(t *testing.T) {
	t.Parallel()
	_, _, pool, id := setup(t)
	q := postgres.NewDownloadQueue(pool, 0)
	queue(t, pool, id)
	ctx := t.Context()

	job, ok, err := q.Claim(ctx, time.Hour)
	if err != nil || !ok || job.ArticleID != id || job.UserID != owner {
		t.Fatalf("Claim() = %+v, %v, %v; want the queued article", job, ok, err)
	}
	if _, ok, err := q.Claim(ctx, time.Hour); err != nil || ok {
		t.Errorf("second Claim() = %v, %v; want nothing while the first claim holds", ok, err)
	}

	// The claim runs out, as when the API stops mid-download: the article
	// is claimed again, at most 3 times in all, then dropped.
	if _, err := pool.Exec(ctx, `UPDATE content_downloads SET claimed_until = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	for attempt := 2; attempt <= 3; attempt++ {
		job, ok, err := q.Claim(ctx, time.Millisecond)
		if err != nil || !ok || job.ArticleID != id {
			t.Fatalf("attempt %d: Claim() = %+v, %v, %v; want the article again", attempt, job, ok, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, ok, err := q.Claim(ctx, time.Millisecond); err != nil || ok {
		t.Errorf("fourth Claim() = %v, %v; want nothing after 3 attempts", ok, err)
	}
	if n := queuedCount(t, pool); n != 0 {
		t.Errorf("%d rows left, want the exhausted article dropped", n)
	}
}

func TestDownloadQueue_DoneRemovesTheArticle(t *testing.T) {
	t.Parallel()
	_, _, pool, id := setup(t)
	q := postgres.NewDownloadQueue(pool, 0)
	queue(t, pool, id)

	job, _, err := q.Claim(t.Context(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Done(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	if n := queuedCount(t, pool); n != 0 {
		t.Errorf("%d rows left, want none", n)
	}
	if err := q.Done(t.Context(), job); err != nil {
		t.Errorf("Done() twice: %v, want no error", err)
	}
}

func TestDownloadQueue_WaitsBeforeHandingOutAFreshArticle(t *testing.T) {
	t.Parallel()
	_, _, pool, id := setup(t)
	q := postgres.NewDownloadQueue(pool, time.Minute)
	queue(t, pool, id)

	if _, ok, err := q.Claim(t.Context(), time.Hour); err != nil || ok {
		t.Fatalf("Claim() = %v, %v; want nothing for an article queued just now", ok, err)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE content_downloads SET queued_at = now() - interval '2 minutes'`); err != nil {
		t.Fatal(err)
	}
	if job, ok, err := q.Claim(t.Context(), time.Hour); err != nil || !ok || job.ArticleID != id {
		t.Errorf("Claim() = %+v, %v, %v; want the article once it has waited", job, ok, err)
	}
}

// A single link is imported (and queued), then downloaded at once by the
// web app while the worker runs: the page is requested once.
func TestWorker_LeavesASingleLinkToItsImmediateDownload(t *testing.T) {
	t.Parallel()
	svc, f, pool, id := setup(t)
	f.delay = 300 * time.Millisecond
	f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(`<html><body><article><p>Text.</p></article></body></html>`)}
	queue(t, pool, id)

	ctx, cancel := context.WithCancel(t.Context())
	w := content.NewWorker(svc, postgres.NewDownloadQueue(pool, time.Minute), content.WorkerConfig{Workers: 2, Pause: time.Millisecond, Idle: 10 * time.Millisecond})
	stopped := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(stopped)
	}()
	if _, err := svc.Fetch(t.Context(), owner, id); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-stopped

	if n := requests(f, articleURL); n != 1 {
		t.Errorf("the page was requested %d times, want 1", n)
	}
}

// Stopping the worker mid-download leaves the article queued and records
// nothing, so it is downloaded again after the claim runs out.
func TestWorker_ADownloadCutShortIsTriedAgain(t *testing.T) {
	t.Parallel()
	svc, f, pool, id := setup(t)
	f.delay = 2 * time.Second
	f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(`<html><body><article><p>Text.</p></article></body></html>`)}
	queue(t, pool, id)

	ctx, cancel := context.WithCancel(t.Context())
	w := content.NewWorker(svc, postgres.NewDownloadQueue(pool, 0), content.WorkerConfig{Workers: 1, Idle: 10 * time.Millisecond})
	stopped := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(stopped)
	}()
	for requests(f, articleURL) == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-stopped

	if _, err := svc.Get(t.Context(), owner, id); !errors.Is(err, content.ErrNoContent) {
		t.Errorf("Get() = %v, want nothing recorded for a download cut short", err)
	}
	if n := queuedCount(t, pool); n != 1 {
		t.Fatalf("%d rows queued, want the article still queued", n)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE content_downloads SET claimed_until = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	f.delay = 0
	runWorker(t, svc, pool)
	if got, err := svc.Get(t.Context(), owner, id); err != nil || got.Status != content.StatusComplete {
		t.Errorf("after the retry: %+v, %v; want the text saved", got, err)
	}
}

func queuedFor(t *testing.T, pool *pgxpool.Pool, articleIDs ...string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM content_downloads WHERE article_id = ANY($1::uuid[])`, articleIDs).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
