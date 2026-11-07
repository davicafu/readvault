package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	apihttp "readvault/api/internal/adapter/http"
	"readvault/api/internal/library"
)

// stubLibrary records the request the handler passed to the library service.
type stubLibrary struct {
	err       error
	gotUser   string
	gotParams library.ListParams
	gotID     string
}

func (s *stubLibrary) List(_ context.Context, userID string, p library.ListParams) (library.Page, error) {
	s.gotUser, s.gotParams = userID, p
	if s.err != nil {
		return library.Page{}, s.err
	}
	next := "next-page"
	return library.Page{Items: []library.Article{{ID: "a1", Title: "one"}}, NextCursor: &next}, nil
}

func (s *stubLibrary) Get(_ context.Context, userID, id string) (library.Article, error) {
	s.gotUser, s.gotID = userID, id
	if s.err != nil {
		return library.Article{}, s.err
	}
	return library.Article{ID: id, Title: "one"}, nil
}

func get(t *testing.T, svc *stubLibrary, target string) *httptest.ResponseRecorder {
	t.Helper()
	h := apihttp.NewHandler(apihttp.Deps{Library: svc, OwnerID: testOwner})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestListArticles_PassesTheQueryForTheServerOwner(t *testing.T) {
	t.Parallel()
	svc := &stubLibrary{}

	rec := get(t, svc, "/api/articles?q=go+blog&tag=go&tag=news&sort=title_asc&limit=10&cursor=abc&userId=someone-else")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body.String())
	}
	expected := library.ListParams{Search: "go blog", Tags: []string{"go", "news"}, Sort: "title_asc", Limit: 10, Cursor: "abc"}
	if p := svc.gotParams; p.Search != expected.Search || !slices.Equal(p.Tags, expected.Tags) ||
		p.Sort != expected.Sort || p.Limit != expected.Limit || p.Cursor != expected.Cursor {
		t.Errorf("params = %+v, want %+v", p, expected)
	}
	if svc.gotUser != testOwner {
		t.Errorf("user = %q, want the server owner", svc.gotUser)
	}
	body := decode(t, rec)
	if body["nextCursor"] != "next-page" || len(body["items"].([]any)) != 1 {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestArticleEndpoints_MapErrors(t *testing.T) {
	t.Parallel()

	validation := &library.ValidationError{Problems: []library.FieldProblem{{Field: "sort", Code: "invalid", Message: "bad sort"}}}
	tests := []struct {
		name           string
		target         string
		err            error
		expectedStatus int
		expectedCode   string
	}{
		{"limit is not a number", "/api/articles?limit=ten", nil, http.StatusBadRequest, "invalid_query"},
		{"invalid list query", "/api/articles?sort=popular", validation, http.StatusBadRequest, "invalid_query"},
		{"list fails", "/api/articles", errors.New(`ERROR: syntax error at or near "WHERE" (SQLSTATE 42601)`), http.StatusInternalServerError, "internal"},
		{"article not found", "/api/articles/missing", library.ErrNotFound, http.StatusNotFound, "not_found"},
		{"get fails", "/api/articles/a1", errors.New(`ERROR: connection refused (SQLSTATE 08006)`), http.StatusInternalServerError, "internal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := get(t, &stubLibrary{err: tt.err}, tt.target)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.expectedStatus, rec.Body.String())
			}
			errBody, _ := decode(t, rec)["error"].(map[string]any)
			if errBody["code"] != tt.expectedCode {
				t.Errorf("code = %v, want %q", errBody["code"], tt.expectedCode)
			}
			if tt.expectedCode == "invalid_query" {
				if details, _ := errBody["details"].([]any); len(details) != 1 {
					t.Errorf("details = %v, want one field problem", errBody["details"])
				}
			}
			if strings.Contains(rec.Body.String(), "SQLSTATE") {
				t.Errorf("body leaks internal details: %s", rec.Body.String())
			}
		})
	}
}

func TestGetArticle_ReturnsTheArticle(t *testing.T) {
	t.Parallel()
	svc := &stubLibrary{}

	rec := get(t, svc, "/api/articles/a1")

	if rec.Code != http.StatusOK || decode(t, rec)["id"] != "a1" {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body.String())
	}
	if svc.gotID != "a1" || svc.gotUser != testOwner {
		t.Errorf("got id %q user %q", svc.gotID, svc.gotUser)
	}
}
