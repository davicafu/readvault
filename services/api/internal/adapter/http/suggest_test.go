package http_test

import (
	"context"
	"net/http"
	"testing"

	"readvault/api/internal/library"
	"readvault/api/internal/tag"
)

func (s *stubLibrary) Suggest(_ context.Context, userID, id string) (library.Suggestions, error) {
	s.gotUser, s.gotID = userID, id
	if s.err != nil {
		return library.Suggestions{}, s.err
	}
	return library.Suggestions{
		Collection: &library.CollectionRef{ID: "c1", Name: "Streaming", Slug: "streaming"},
		Tags:       []tag.Tag{{Name: "kafka", Slug: "kafka"}},
	}, nil
}

func TestSuggestions_ForTheServerOwner(t *testing.T) {
	t.Parallel()
	svc := &stubLibrary{}

	rec := get(t, svc, "/api/articles/a1/suggestions")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	if svc.gotUser != testOwner || svc.gotID != "a1" {
		t.Errorf("service got user %q id %q", svc.gotUser, svc.gotID)
	}
	expected := `{"collection":{"id":"c1","name":"Streaming","slug":"streaming"},"tags":[{"name":"kafka","slug":"kafka"}]}`
	if got := rec.Body.String(); got != expected+"\n" {
		t.Errorf("body = %s, want %s", got, expected)
	}
}

func TestSuggestions_UnknownArticle(t *testing.T) {
	t.Parallel()

	rec := get(t, &stubLibrary{err: library.ErrNotFound}, "/api/articles/a1/suggestions")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body %s", rec.Code, rec.Body.String())
	}
	if code, _ := decode(t, rec)["error"].(map[string]any)["code"]; code != "not_found" {
		t.Errorf("error code = %v, want not_found", code)
	}
}
