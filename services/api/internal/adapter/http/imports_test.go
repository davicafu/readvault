package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apihttp "readvault/api/internal/adapter/http"
	"readvault/api/internal/importing"
)

const (
	testOwner      = "00000000-0000-0000-0000-000000000001"
	testMaxBytes   = 32
	testMaxRecords = 7
)

// stubImports records what the handler passed to the importing service.
type stubImports struct {
	err      error
	gotUser  string
	gotBytes string
}

func (s *stubImports) read(userID string, file io.Reader) error {
	s.gotUser = userID
	b, err := io.ReadAll(file)
	s.gotBytes = string(b)
	if err != nil {
		return fmt.Errorf("reading html: %w", err)
	}
	return s.err
}

func (s *stubImports) Preview(_ context.Context, userID string, file io.Reader) (importing.Preview, error) {
	if err := s.read(userID, file); err != nil {
		return importing.Preview{}, err
	}
	return importing.Preview{Totals: importing.PreviewTotals{Detected: 1, Valid: 1}, Candidates: []importing.Candidate{}}, nil
}

func (s *stubImports) Import(_ context.Context, userID string, file io.Reader) (importing.Report, error) {
	if err := s.read(userID, file); err != nil {
		return importing.Report{}, err
	}
	return importing.Report{ImportID: "import-1", Totals: importing.ReportTotals{Inserted: 1}, Items: []importing.ReportItem{}}, nil
}

func newImportHandler(svc *stubImports) http.Handler {
	return apihttp.NewHandler(apihttp.Deps{
		Imports:          svc,
		OwnerID:          testOwner,
		MaxUploadBytes:   testMaxBytes,
		MaxImportRecords: testMaxRecords,
	})
}

// multipartBody builds a form with the given fields; "file" is sent as a file part.
func multipartBody(t *testing.T, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, value := range fields {
		var w io.Writer
		var err error
		if name == "file" {
			w, err = mw.CreateFormFile("file", "bookmarks.html")
		} else {
			w, err = mw.CreateFormField(name)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

func post(t *testing.T, h http.Handler, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, rec.Body.String())
	}
	return got
}

func TestImportEndpoints_PassTheUploadedFileForTheServerOwner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path        string
		expectedKey string
	}{
		{"/api/imports/preview", "candidates"},
		{"/api/imports", "importId"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			svc := &stubImports{}
			body, ct := multipartBody(t, map[string]string{
				"file":   "<DL><DT><A HREF=x>",
				"userId": "someone-else",
			})

			rec := post(t, newImportHandler(svc), tt.path, body, ct)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
			}
			if _, ok := decode(t, rec)[tt.expectedKey]; !ok {
				t.Errorf("body has no %q: %s", tt.expectedKey, rec.Body.String())
			}
			if svc.gotUser != testOwner {
				t.Errorf("user = %q, want the server owner %q", svc.gotUser, testOwner)
			}
			if svc.gotBytes != "<DL><DT><A HREF=x>" {
				t.Errorf("file = %q, want the uploaded bytes", svc.gotBytes)
			}
		})
	}
}

func TestImportEndpoints_RejectBadUploads(t *testing.T) {
	t.Parallel()

	atLimit := strings.Repeat("a", testMaxBytes)
	tests := []struct {
		name            string
		fields          map[string]string
		rawBody         string
		serviceErr      error
		expectedStatus  int
		expectedCode    string
		expectedDetails map[string]any
	}{
		{name: "file exactly at the size limit", fields: map[string]string{"file": atLimit}, expectedStatus: http.StatusOK},
		{name: "not a multipart form", rawBody: `{"file":"x"}`, expectedStatus: http.StatusBadRequest, expectedCode: "invalid_upload"},
		{name: "form without a file", fields: map[string]string{"other": "x"}, expectedStatus: http.StatusBadRequest, expectedCode: "missing_file"},
		{
			name: "file over the size limit", fields: map[string]string{"file": atLimit + "a"},
			expectedStatus: http.StatusRequestEntityTooLarge, expectedCode: "file_too_large",
			expectedDetails: map[string]any{"maxBytes": float64(testMaxBytes)},
		},
		{
			name: "too many records", fields: map[string]string{"file": "x"},
			serviceErr:     fmt.Errorf("%w: limit is %d", importing.ErrTooManyRecords, testMaxRecords),
			expectedStatus: http.StatusUnprocessableEntity, expectedCode: "too_many_records",
			expectedDetails: map[string]any{"maxRecords": float64(testMaxRecords)},
		},
		{
			name: "internal failure", fields: map[string]string{"file": "x"},
			serviceErr:     errors.New(`checking existing articles: ERROR: relation "articles" does not exist (SQLSTATE 42P01)`),
			expectedStatus: http.StatusInternalServerError, expectedCode: "internal",
		},
	}
	for _, path := range []string{"/api/imports/preview", "/api/imports"} {
		for _, tt := range tests {
			t.Run(path+"/"+tt.name, func(t *testing.T) {
				t.Parallel()
				body, ct := io.Reader(strings.NewReader(tt.rawBody)), "application/json"
				if tt.fields != nil {
					body, ct = multipartBody(t, tt.fields)
				}

				rec := post(t, newImportHandler(&stubImports{err: tt.serviceErr}), path, body, ct)

				if rec.Code != tt.expectedStatus {
					t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.expectedStatus, rec.Body.String())
				}
				if tt.expectedCode == "" {
					return
				}
				errBody, _ := decode(t, rec)["error"].(map[string]any)
				if errBody["code"] != tt.expectedCode {
					t.Errorf("error code = %v, want %q; body %s", errBody["code"], tt.expectedCode, rec.Body.String())
				}
				if tt.expectedDetails != nil {
					if details, _ := errBody["details"].(map[string]any); fmt.Sprint(details) != fmt.Sprint(tt.expectedDetails) {
						t.Errorf("details = %v, want %v", details, tt.expectedDetails)
					}
				}
				if strings.Contains(rec.Body.String(), "SQLSTATE") || strings.Contains(rec.Body.String(), "relation") {
					t.Errorf("body leaks internal details: %s", rec.Body.String())
				}
			})
		}
	}
}
