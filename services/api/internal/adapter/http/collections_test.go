package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apihttp "readvault/api/internal/adapter/http"
	"readvault/api/internal/library"
)

func (s *stubLibrary) Collections(_ context.Context, userID string) ([]library.Collection, error) {
	s.gotUser = userID
	if s.err != nil {
		return nil, s.err
	}
	return []library.Collection{{ID: "c1", Name: "Reading", Slug: "reading", Articles: 3}}, nil
}

func (s *stubLibrary) CreateCollection(_ context.Context, userID, name string) (library.Collection, error) {
	s.gotUser, s.gotName = userID, name
	if s.err != nil {
		return library.Collection{}, s.err
	}
	return library.Collection{ID: "c1", Name: name, Slug: "x"}, nil
}

func (s *stubLibrary) RenameCollection(_ context.Context, userID, id, name string) (library.Collection, error) {
	s.gotUser, s.gotID, s.gotName = userID, id, name
	if s.err != nil {
		return library.Collection{}, s.err
	}
	return library.Collection{ID: id, Name: name, Slug: "x"}, nil
}

func (s *stubLibrary) DeleteCollection(_ context.Context, userID, id string) error {
	s.gotUser, s.gotID = userID, id
	return s.err
}

func send(t *testing.T, svc *stubLibrary, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := apihttp.NewHandler(apihttp.Deps{Library: svc, OwnerID: testOwner})
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCollections_ListCreateRenameDelete(t *testing.T) {
	t.Parallel()

	svc := &stubLibrary{}
	rec := send(t, svc, http.MethodGet, "/api/collections", "")
	list, _ := decode(t, rec)["collections"].([]any)
	var first map[string]any
	if len(list) > 0 {
		first, _ = list[0].(map[string]any)
	}
	if rec.Code != http.StatusOK || len(list) != 1 || first["name"] != "Reading" || first["articles"] != float64(3) || svc.gotUser != testOwner {
		t.Errorf("list: status %d user %q body %s", rec.Code, svc.gotUser, rec.Body.String())
	}

	svc = &stubLibrary{}
	rec = send(t, svc, http.MethodPost, "/api/collections", `{"name":"Reading"}`)
	if rec.Code != http.StatusCreated || decode(t, rec)["id"] != "c1" || svc.gotName != "Reading" || svc.gotUser != testOwner {
		t.Errorf("create: status %d name %q body %s", rec.Code, svc.gotName, rec.Body.String())
	}

	svc = &stubLibrary{}
	rec = send(t, svc, http.MethodPatch, "/api/collections/c1", `{"name":"To read"}`)
	if rec.Code != http.StatusOK || decode(t, rec)["name"] != "To read" || svc.gotID != "c1" || svc.gotName != "To read" {
		t.Errorf("rename: status %d id %q name %q body %s", rec.Code, svc.gotID, svc.gotName, rec.Body.String())
	}

	svc = &stubLibrary{}
	rec = send(t, svc, http.MethodDelete, "/api/collections/c1", "")
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 || svc.gotID != "c1" || svc.gotUser != testOwner {
		t.Errorf("delete: status %d id %q body %q", rec.Code, svc.gotID, rec.Body.String())
	}
}

func TestCollections_MapsErrors(t *testing.T) {
	t.Parallel()
	validation := &library.ValidationError{Problems: []library.FieldProblem{{Field: "name", Code: "required", Message: "empty"}}}
	tests := []struct {
		name, method, target, body string
		err                        error
		expectedStatus             int
		expectedCode               string
	}{
		{"name already used", http.MethodPost, "/api/collections", `{"name":"x"}`, library.ErrCollectionExists, http.StatusConflict, "collection_exists"},
		{"invalid name", http.MethodPost, "/api/collections", `{"name":""}`, validation, http.StatusUnprocessableEntity, "validation_failed"},
		{"unknown field", http.MethodPost, "/api/collections", `{"name":"x","userId":"u"}`, nil, http.StatusBadRequest, "invalid_json"},
		{"rename missing", http.MethodPatch, "/api/collections/c9", `{"name":"x"}`, library.ErrCollectionNotFound, http.StatusNotFound, "not_found"},
		{"rename onto another", http.MethodPatch, "/api/collections/c1", `{"name":"x"}`, library.ErrCollectionExists, http.StatusConflict, "collection_exists"},
		{"delete missing", http.MethodDelete, "/api/collections/c9", "", library.ErrCollectionNotFound, http.StatusNotFound, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := send(t, &stubLibrary{err: tt.err}, tt.method, tt.target, tt.body)
			if rec.Code != tt.expectedStatus || errorCode(t, rec) != tt.expectedCode {
				t.Errorf("status %d code %q, want %d %q; body %s", rec.Code, errorCode(t, rec), tt.expectedStatus, tt.expectedCode, rec.Body.String())
			}
		})
	}
}

func TestCollections_NotFoundSaysCollection(t *testing.T) {
	t.Parallel()
	rec := send(t, &stubLibrary{err: library.ErrCollectionNotFound}, http.MethodDelete, "/api/collections/c9", "")

	errBody, _ := decode(t, rec)["error"].(map[string]any)
	if msg, _ := errBody["message"].(string); !strings.Contains(msg, "collection") {
		t.Errorf("message = %q, want it to name the collection", msg)
	}
}

func TestPatchArticle_PassesTheCollection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		body     string
		set      bool
		expected string // "" means nil
	}{
		{`{"collectionId":"c1"}`, true, "c1"},
		{`{"collectionId":null}`, true, ""},
		{`{"title":"x"}`, false, ""},
	}
	for _, tt := range tests {
		svc := &stubLibrary{}
		rec := patch(t, svc, "/api/articles/a1", tt.body)
		got := svc.gotPatch.CollectionID
		if rec.Code != http.StatusOK || got.Set != tt.set || (tt.expected == "") != (got.Value == nil) || (got.Value != nil && *got.Value != tt.expected) {
			t.Errorf("%s: status %d, collection %+v", tt.body, rec.Code, got)
		}
	}
}

func TestListArticles_PassesTheCollectionFilter(t *testing.T) {
	t.Parallel()
	svc := &stubLibrary{}

	get(t, svc, "/api/articles?collection=reading")

	if svc.gotParams.Collection != "reading" {
		t.Errorf("collection = %q, want reading", svc.gotParams.Collection)
	}
}
