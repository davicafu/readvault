package http_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apihttp "readvault/api/internal/adapter/http"
	"readvault/api/internal/content"
)

// stubContent records what the handler passed to the content service.
type stubContent struct {
	err      error
	gotUser  string
	gotID    string
	gotImage string
	gotName  string
	gotCopy  bool
	gotFile  string
}

func (s *stubContent) result(id string) (content.Content, error) {
	if s.err != nil {
		return content.Content{}, s.err
	}
	return content.Content{ArticleID: id, Status: content.StatusComplete, Markdown: "# Hi\n", Images: []content.ImageInfo{}}, nil
}

func (s *stubContent) Get(_ context.Context, userID, articleID string) (content.Content, error) {
	s.gotUser, s.gotID = userID, articleID
	return s.result(articleID)
}

func (s *stubContent) Upload(_ context.Context, userID, articleID, fileName string, file io.Reader) (content.Content, error) {
	s.gotUser, s.gotID, s.gotName = userID, articleID, fileName
	b, err := io.ReadAll(file)
	if err != nil {
		return content.Content{}, err
	}
	s.gotFile = string(b)
	return s.result(articleID)
}

func (s *stubContent) Fetch(_ context.Context, userID, articleID string) (content.Content, error) {
	s.gotUser, s.gotID = userID, articleID
	return s.result(articleID)
}

func (s *stubContent) FetchCopy(_ context.Context, userID, articleID string) (content.Content, error) {
	s.gotUser, s.gotID, s.gotCopy = userID, articleID, true
	return s.result(articleID)
}

func (s *stubContent) Image(_ context.Context, userID, articleID, imageID string) (content.Image, error) {
	s.gotUser, s.gotID, s.gotImage = userID, articleID, imageID
	if s.err != nil {
		return content.Image{}, s.err
	}
	return content.Image{ContentType: "image/png", Data: []byte("\x89PNG")}, nil
}

const testMaxArticleBytes = 1024

func serveContent(t *testing.T, svc *stubContent, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	h := apihttp.NewHandler(apihttp.Deps{Content: svc, OwnerID: testOwner, MaxArticleBytes: testMaxArticleBytes})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func articleUpload(t *testing.T, name, body string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte(body))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/articles/a1/content", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	errBody, _ := decode(t, rec)["error"].(map[string]any)
	code, _ := errBody["code"].(string)
	return code
}

func TestContent_UploadPassesTheFileForTheServerOwner(t *testing.T) {
	t.Parallel()
	svc := &stubContent{}

	rec := serveContent(t, svc, articleUpload(t, "page.html", "<p>hi</p>"))

	if rec.Code != http.StatusOK || decode(t, rec)["markdown"] != "# Hi\n" {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body.String())
	}
	if svc.gotUser != testOwner || svc.gotID != "a1" || svc.gotName != "page.html" || svc.gotFile != "<p>hi</p>" {
		t.Errorf("got user %q id %q name %q file %q", svc.gotUser, svc.gotID, svc.gotName, svc.gotFile)
	}
}

func TestContent_GetAndFetch(t *testing.T) {
	t.Parallel()
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/articles/a1/content", nil),
		httptest.NewRequest(http.MethodPost, "/api/articles/a1/content/fetch", nil),
	} {
		svc := &stubContent{}
		rec := serveContent(t, svc, req)
		if rec.Code != http.StatusOK || decode(t, rec)["status"] != "complete" || svc.gotUser != testOwner || svc.gotID != "a1" {
			t.Errorf("%s %s: status %d user %q id %q; body %s", req.Method, req.URL.Path, rec.Code, svc.gotUser, svc.gotID, rec.Body.String())
		}
	}
}

func TestContent_ServesAStoredImageSafely(t *testing.T) {
	t.Parallel()
	svc := &stubContent{}

	rec := serveContent(t, svc, httptest.NewRequest(http.MethodGet, "/api/articles/a1/images/i1", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "\x89PNG" {
		t.Fatalf("status = %d; body %q", rec.Code, rec.Body.String())
	}
	for header, expected := range map[string]string{
		"Content-Type":            "image/png",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'none'; sandbox",
		"Cache-Control":           "private, max-age=86400",
	} {
		if got := rec.Header().Get(header); got != expected {
			t.Errorf("%s = %q, want %q", header, got, expected)
		}
	}
	if svc.gotID != "a1" || svc.gotImage != "i1" || svc.gotUser != testOwner {
		t.Errorf("got id %q image %q user %q", svc.gotID, svc.gotImage, svc.gotUser)
	}
}

func TestContent_MapsErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		req            func(t *testing.T) *http.Request
		err            error
		expectedStatus int
		expectedCode   string
	}{
		{"article not found", get_("/api/articles/a1/content"), content.ErrNotFound, http.StatusNotFound, "not_found"},
		{"no saved text", get_("/api/articles/a1/content"), content.ErrNoContent, http.StatusNotFound, "content_not_found"},
		{"image not found", get_("/api/articles/a1/images/i1"), content.ErrNotFound, http.StatusNotFound, "not_found"},
		{"file without text", upload_("page.html", "<nav>x</nav>"), content.ErrNoText, http.StatusUnprocessableEntity, "no_text"},
		{"file over the limit", upload_("page.html", strings.Repeat("a", testMaxArticleBytes+1)), nil, http.StatusRequestEntityTooLarge, "file_too_large"},
		{"service says too large", upload_("page.html", "<p>x</p>"), content.ErrTooLarge, http.StatusRequestEntityTooLarge, "file_too_large"},
		{"download failed", post_("/api/articles/a1/content/fetch"), &content.FetchError{Problem: content.ProblemHTTPStatus, Status: 403}, http.StatusBadGateway, "fetch_failed"},
		{"internal failure", get_("/api/articles/a1/content"), errors.New("ERROR: boom (SQLSTATE XX000)"), http.StatusInternalServerError, "internal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := serveContent(t, &stubContent{err: tt.err}, tt.req(t))
			if rec.Code != tt.expectedStatus || errorCode(t, rec) != tt.expectedCode {
				t.Errorf("status %d code %q, want %d %q; body %s", rec.Code, errorCode(t, rec), tt.expectedStatus, tt.expectedCode, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "SQLSTATE") {
				t.Errorf("body leaks internal details: %s", rec.Body.String())
			}
		})
	}
}

func TestContent_FetchFailureDetailsNameTheProblem(t *testing.T) {
	t.Parallel()
	rec := serveContent(t, &stubContent{err: &content.FetchError{Problem: content.ProblemHTTPStatus, Status: 403}},
		httptest.NewRequest(http.MethodPost, "/api/articles/a1/content/fetch", nil))

	errBody, _ := decode(t, rec)["error"].(map[string]any)
	details, _ := errBody["details"].(map[string]any)
	if details["problem"] != "http_status" || details["status"] != float64(403) {
		t.Errorf("details = %v, want the problem and the HTTP status", details)
	}
}

func get_(path string) func(*testing.T) *http.Request {
	return func(*testing.T) *http.Request { return httptest.NewRequest(http.MethodGet, path, nil) }
}

func post_(path string) func(*testing.T) *http.Request {
	return func(*testing.T) *http.Request { return httptest.NewRequest(http.MethodPost, path, nil) }
}

func upload_(name, body string) func(*testing.T) *http.Request {
	return func(t *testing.T) *http.Request { return articleUpload(t, name, body) }
}

func TestContent_FetchFromAnotherCopy(t *testing.T) {
	t.Parallel()
	svc := &stubContent{}
	rec := serveContent(t, svc, httptest.NewRequest(http.MethodPost, "/api/articles/a1/content/fetch?from=copy", nil))
	if rec.Code != http.StatusOK || !svc.gotCopy || svc.gotID != "a1" || svc.gotUser != testOwner {
		t.Errorf("status %d copy %v id %q; body %s", rec.Code, svc.gotCopy, svc.gotID, rec.Body.String())
	}

	svc = &stubContent{}
	rec = serveContent(t, svc, httptest.NewRequest(http.MethodPost, "/api/articles/a1/content/fetch", nil))
	if rec.Code != http.StatusOK || svc.gotCopy {
		t.Errorf("plain fetch: status %d, used a copy: %v", rec.Code, svc.gotCopy)
	}

	rec = serveContent(t, &stubContent{}, httptest.NewRequest(http.MethodPost, "/api/articles/a1/content/fetch?from=elsewhere", nil))
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_query" {
		t.Errorf("unknown source: status %d code %q", rec.Code, errorCode(t, rec))
	}

	rec = serveContent(t, &stubContent{err: &content.FetchError{Problem: content.ProblemNoCopy}}, httptest.NewRequest(http.MethodPost, "/api/articles/a1/content/fetch?from=copy", nil))
	errBody, _ := decode(t, rec)["error"].(map[string]any)
	details, _ := errBody["details"].(map[string]any)
	if rec.Code != http.StatusBadGateway || errBody["code"] != "fetch_failed" || details["problem"] != "no_copy" {
		t.Errorf("no copy: status %d body %s", rec.Code, rec.Body.String())
	}
}
