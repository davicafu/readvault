package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"readvault/api/internal/content"
)

func (s *stubContent) DownloadSummary(_ context.Context, userID string) (content.DownloadSummary, error) {
	s.gotUser = userID
	return content.DownloadSummary{Queued: 3, Missing: 2, Failed: 1, Incomplete: 4}, s.err
}

func (s *stubContent) QueueMissing(_ context.Context, userID string) (int, error) {
	s.gotUser = userID
	return 2, s.err
}

func TestDownloads_SummaryAndQueueForTheServerOwner(t *testing.T) {
	t.Parallel()
	svc := &stubContent{}

	rec := serveContent(t, svc, httptest.NewRequest(http.MethodGet, "/api/content-downloads", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != `{"queued":3,"missing":2,"failed":1,"incomplete":4}`+"\n" || svc.gotUser != testOwner {
		t.Errorf("GET: %d %s (user %q)", rec.Code, rec.Body, svc.gotUser)
	}

	svc.gotUser = ""
	rec = serveContent(t, svc, httptest.NewRequest(http.MethodPost, "/api/content-downloads", nil))
	if rec.Code != http.StatusAccepted || rec.Body.String() != `{"queued":2}`+"\n" || svc.gotUser != testOwner {
		t.Errorf("POST: %d %s (user %q)", rec.Code, rec.Body, svc.gotUser)
	}
}

func TestDownloads_FailureIsAServerError(t *testing.T) {
	t.Parallel()
	svc := &stubContent{err: errors.New("database down")}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		rec := serveContent(t, svc, httptest.NewRequest(method, "/api/content-downloads", nil))
		if rec.Code != http.StatusInternalServerError || errorCode(t, rec) != "internal" {
			t.Errorf("%s: %d %s, want 500 internal", method, rec.Code, rec.Body)
		}
	}
}
