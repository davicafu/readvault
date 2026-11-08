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
	"readvault/api/internal/tag"
)

// stubLibrary records the request the handler passed to the library service.
type stubLibrary struct {
	err       error
	gotUser   string
	gotParams library.ListParams
	gotID     string
	gotPatch  library.Patch
}

func (s *stubLibrary) Export(_ context.Context, userID string) (library.Export, error) {
	s.gotUser = userID
	if s.err != nil {
		return library.Export{}, s.err
	}
	return library.Export{SchemaVersion: 1, Articles: []library.Article{{ID: "a1"}}}, nil
}

func (s *stubLibrary) Update(_ context.Context, userID, id string, p library.Patch) (library.Article, error) {
	s.gotUser, s.gotID, s.gotPatch = userID, id, p
	if s.err != nil {
		return library.Article{}, s.err
	}
	return library.Article{ID: id, Title: *p.Title}, nil
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

func (s *stubLibrary) Tags(_ context.Context, userID string) ([]library.TagCount, error) {
	s.gotUser = userID
	if s.err != nil {
		return nil, s.err
	}
	return []library.TagCount{{Tag: tag.Tag{Name: "Go", Slug: "go"}, Articles: 2}}, nil
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
	expected := library.ListParams{Search: "go blog", Tags: []string{"go", "news"}, Sort: "title_asc", Cursor: "abc"}
	if p := svc.gotParams; p.Search != expected.Search || !slices.Equal(p.Tags, expected.Tags) ||
		p.Sort != expected.Sort || p.Limit == nil || *p.Limit != 10 || p.Cursor != expected.Cursor {
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

func TestListArticles_PassesAnExplicitZeroLimitAndOmitsAnAbsentOne(t *testing.T) {
	t.Parallel()
	tests := []struct {
		target   string
		expected *int
	}{
		{"/api/articles?limit=0", ptrInt(0)},
		{"/api/articles", nil},
		{"/api/articles?limit=", nil},
	}
	for _, tt := range tests {
		svc := &stubLibrary{}
		get(t, svc, tt.target)
		if got := svc.gotParams.Limit; (got == nil) != (tt.expected == nil) || (got != nil && *got != *tt.expected) {
			t.Errorf("%s: limit = %v, want %v", tt.target, got, tt.expected)
		}
	}
}

func ptrInt(n int) *int { return &n }

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
		{"semicolon in the query", "/api/articles?q=go;tag=news", nil, http.StatusBadRequest, "invalid_query"},
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

func patch(t *testing.T, svc *stubLibrary, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := apihttp.NewHandler(apihttp.Deps{Library: svc, OwnerID: testOwner})
	req := httptest.NewRequest(http.MethodPatch, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPatchArticle_PassesTheEditForTheServerOwner(t *testing.T) {
	t.Parallel()
	svc := &stubLibrary{}

	rec := patch(t, svc, "/api/articles/a1", `{"title":"New title","tags":["go","news"]}`)

	if rec.Code != http.StatusOK || decode(t, rec)["title"] != "New title" {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body.String())
	}
	if svc.gotUser != testOwner || svc.gotID != "a1" {
		t.Errorf("user %q id %q", svc.gotUser, svc.gotID)
	}
	if svc.gotPatch.Title == nil || *svc.gotPatch.Title != "New title" || svc.gotPatch.Tags == nil || !slices.Equal(*svc.gotPatch.Tags, []string{"go", "news"}) {
		t.Errorf("patch = %+v", svc.gotPatch)
	}
}

func TestPatchArticle_MapsErrors(t *testing.T) {
	t.Parallel()

	validation := &library.ValidationError{Problems: []library.FieldProblem{{Field: "title", Code: "required", Message: "empty"}}}
	tests := []struct {
		name           string
		body           string
		err            error
		expectedStatus int
		expectedCode   string
	}{
		{"malformed json", `{"title":`, nil, http.StatusBadRequest, "invalid_json"},
		{"unknown field", `{"title":"x","originalUrl":"https://evil.example"}`, nil, http.StatusBadRequest, "invalid_json"},
		{"field name in another case", `{"Title":"x"}`, nil, http.StatusBadRequest, "invalid_json"},
		{"two json values", `{"title":"x"}{"title":"y"}`, nil, http.StatusBadRequest, "invalid_json"},
		{"body too large", `{"title":"` + strings.Repeat("a", 70<<10) + `"}`, nil, http.StatusRequestEntityTooLarge, "body_too_large"},
		{"invalid edit", `{"title":" "}`, validation, http.StatusUnprocessableEntity, "validation_failed"},
		{"article not found", `{"title":"x"}`, library.ErrNotFound, http.StatusNotFound, "not_found"},
		{"update fails", `{"title":"x"}`, errors.New(`ERROR: deadlock detected (SQLSTATE 40P01)`), http.StatusInternalServerError, "internal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := &stubLibrary{err: tt.err}
			rec := patch(t, svc, "/api/articles/a1", tt.body)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.expectedStatus, rec.Body.String())
			}
			errBody, _ := decode(t, rec)["error"].(map[string]any)
			if errBody["code"] != tt.expectedCode {
				t.Errorf("code = %v, want %q", errBody["code"], tt.expectedCode)
			}
			if tt.expectedCode == "validation_failed" {
				if details, _ := errBody["details"].([]any); len(details) != 1 {
					t.Errorf("details = %v, want one field problem", errBody["details"])
				}
			}
			if tt.err == nil && svc.gotID != "" {
				t.Error("the service was called for a rejected body")
			}
			if strings.Contains(rec.Body.String(), "SQLSTATE") {
				t.Errorf("body leaks internal details: %s", rec.Body.String())
			}
		})
	}
}

func TestPatchArticle_RequiresAJSONContentType(t *testing.T) {
	t.Parallel()

	for _, contentType := range []string{"", "text/plain", "application/x-www-form-urlencoded"} {
		svc := &stubLibrary{}
		h := apihttp.NewHandler(apihttp.Deps{Library: svc, OwnerID: testOwner})
		req := httptest.NewRequest(http.MethodPatch, "/api/articles/a1", strings.NewReader(`{"title":"x"}`))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("%q: status = %d, want 415; body %s", contentType, rec.Code, rec.Body.String())
			continue
		}
		if errBody, _ := decode(t, rec)["error"].(map[string]any); errBody["code"] != "unsupported_media_type" {
			t.Errorf("%q: code = %v, want unsupported_media_type", contentType, errBody["code"])
		}
		if svc.gotID != "" {
			t.Errorf("%q: the service was called", contentType)
		}
	}

	rec := patch(t, &stubLibrary{}, "/api/articles/a1", `{"title":"x"}`)
	if rec.Code != http.StatusOK {
		t.Errorf("application/json: status = %d, want 200", rec.Code)
	}
}

func TestExport_DownloadsTheOwnersLibraryAsJSON(t *testing.T) {
	t.Parallel()
	svc := &stubLibrary{}

	rec := get(t, svc, "/api/export")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="readvault-export-`) || !strings.HasSuffix(cd, `.json"`) {
		t.Errorf("Content-Disposition = %q", cd)
	}
	body := decode(t, rec)
	if body["schemaVersion"] != float64(1) || len(body["articles"].([]any)) != 1 {
		t.Errorf("body = %s", rec.Body.String())
	}
	if svc.gotUser != testOwner {
		t.Errorf("user = %q, want the server owner", svc.gotUser)
	}
}

func TestExport_FailureHidesInternalDetails(t *testing.T) {
	t.Parallel()

	rec := get(t, &stubLibrary{err: errors.New("ERROR: canceling statement (SQLSTATE 57014)")}, "/api/export")

	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "SQLSTATE") {
		t.Errorf("status = %d; body %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Disposition") != "" {
		t.Error("a failed export must not be offered as a download")
	}
}

func TestListTags_ReturnsTheOwnersTagsWithCounts(t *testing.T) {
	t.Parallel()
	svc := &stubLibrary{}

	rec := get(t, svc, "/api/tags")

	if rec.Code != http.StatusOK || svc.gotUser != testOwner {
		t.Fatalf("status = %d user %q; body %s", rec.Code, svc.gotUser, rec.Body.String())
	}
	tags, _ := decode(t, rec)["tags"].([]any)
	first, _ := tags[0].(map[string]any)
	if len(tags) != 1 || first["name"] != "Go" || first["slug"] != "go" || first["articles"] != float64(2) {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestListTags_HidesInternalFailures(t *testing.T) {
	t.Parallel()
	rec := get(t, &stubLibrary{err: errors.New("ERROR: boom (SQLSTATE XX000)")}, "/api/tags")

	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "SQLSTATE") {
		t.Errorf("status = %d; body %s", rec.Code, rec.Body.String())
	}
}
