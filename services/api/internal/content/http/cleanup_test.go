package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"readvault/api/internal/content"
)

func (s *stubContent) RemoveTexts(_ context.Context, userID, collectionID string) (int, error) {
	s.gotUser, s.gotID = userID, collectionID
	return 7, s.err
}

func TestRemoveTexts_AllOrOneCollectionForTheServerOwner(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ path, collection string }{
		{"/api/contents", ""},
		{"/api/collections/c0ffee00-0000-0000-0000-000000000001/contents", "c0ffee00-0000-0000-0000-000000000001"},
	} {
		svc := &stubContent{gotID: "unset"}
		rec := serveContent(t, svc, httptest.NewRequest(http.MethodDelete, tc.path, nil))
		if rec.Code != http.StatusOK || rec.Body.String() != `{"removed":7}`+"\n" {
			t.Errorf("DELETE %s: %d %s, want 200 {\"removed\":7}", tc.path, rec.Code, rec.Body)
		}
		if svc.gotUser != testOwner || svc.gotID != tc.collection {
			t.Errorf("DELETE %s: removed for user %q collection %q, want %q %q", tc.path, svc.gotUser, svc.gotID, testOwner, tc.collection)
		}
	}
}

func TestRemoveTexts_UnknownCollectionAndFailure(t *testing.T) {
	t.Parallel()
	rec := serveContent(t, &stubContent{err: content.ErrNotFound},
		httptest.NewRequest(http.MethodDelete, "/api/collections/nope/contents", nil))
	if rec.Code != http.StatusNotFound || errorCode(t, rec) != "not_found" {
		t.Errorf("unknown collection: %d %s, want 404 not_found", rec.Code, rec.Body)
	}
	rec = serveContent(t, &stubContent{err: errors.New("database down")},
		httptest.NewRequest(http.MethodDelete, "/api/contents", nil))
	if rec.Code != http.StatusInternalServerError || errorCode(t, rec) != "internal" {
		t.Errorf("failure: %d %s, want 500 internal", rec.Code, rec.Body)
	}
}

func (s *stubContent) RemoveText(_ context.Context, userID, articleID string) (int, error) {
	s.gotUser, s.gotID = userID, articleID
	return 1, s.err
}

func TestRemoveText_OneArticleForTheServerOwner(t *testing.T) {
	t.Parallel()
	svc := &stubContent{}
	rec := serveContent(t, svc, httptest.NewRequest(http.MethodDelete, "/api/articles/a1/content", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != `{"removed":1}`+"\n" {
		t.Errorf("DELETE: %d %s, want 200 {\"removed\":1}", rec.Code, rec.Body)
	}
	if svc.gotUser != testOwner || svc.gotID != "a1" {
		t.Errorf("removed for user %q article %q, want %q a1", svc.gotUser, svc.gotID, testOwner)
	}
	rec = serveContent(t, &stubContent{err: content.ErrNotFound},
		httptest.NewRequest(http.MethodDelete, "/api/articles/nope/content", nil))
	if rec.Code != http.StatusNotFound || errorCode(t, rec) != "not_found" {
		t.Errorf("unknown article: %d %s, want 404 not_found", rec.Code, rec.Body)
	}
}
