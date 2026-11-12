//go:build integration

package content_test

import (
	"testing"

	"readvault/api/internal/content"
)

func TestDownloads_QueueEveryArticleNeverDownloadedAndSummarise(t *testing.T) {
	t.Parallel()
	svc, f, pool, done := setup(t)
	f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(`<html><body><article><p>Text.</p></article></body></html>`)}
	if _, err := svc.Fetch(t.Context(), owner, done); err != nil {
		t.Fatal(err)
	}
	failed := newArticle(t, pool, owner, "https://blog.example/failed")
	if _, err := svc.Fetch(t.Context(), owner, failed); err == nil {
		t.Fatal("setup: want the download to fail")
	}
	queuedAlready := newArticle(t, pool, owner, "https://blog.example/queued")
	queue(t, pool, queuedAlready)
	never1 := newArticle(t, pool, owner, "https://blog.example/never-1")
	never2 := newArticle(t, pool, owner, "https://blog.example/never-2")
	other := newUser(t, pool)
	newArticle(t, pool, other, "https://blog.example/someone-else")

	before, err := svc.DownloadSummary(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	expected := content.DownloadSummary{Queued: 1, Missing: 2, Failed: 1, Incomplete: 0}
	if before != expected {
		t.Errorf("summary before = %+v, want %+v", before, expected)
	}

	n, err := svc.QueueMissing(t.Context(), owner)
	if err != nil || n != 2 {
		t.Fatalf("QueueMissing() = %d, %v; want the 2 articles never downloaded", n, err)
	}
	after, err := svc.DownloadSummary(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	expected = content.DownloadSummary{Queued: 3, Missing: 0, Failed: 1, Incomplete: 0}
	if after != expected {
		t.Errorf("summary after = %+v, want %+v", after, expected)
	}
	if got := queuedFor(t, pool, never1, never2, failed, done); got != 2 {
		t.Errorf("%d of the never-downloaded, failed and done articles queued, want only the 2 never downloaded", got)
	}
	if n, err := svc.QueueMissing(t.Context(), owner); err != nil || n != 0 {
		t.Errorf("second QueueMissing() = %d, %v; want nothing new", n, err)
	}
	if s, err := svc.DownloadSummary(t.Context(), other); err != nil || s != (content.DownloadSummary{Missing: 1}) {
		t.Errorf("other user's summary = %+v, %v; want only their own article", s, err)
	}
}
