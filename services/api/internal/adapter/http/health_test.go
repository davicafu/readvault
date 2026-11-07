package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "readvault/api/internal/adapter/http"
)

type pingerFunc func(context.Context) error

func (f pingerFunc) Ping(ctx context.Context) error { return f(ctx) }

func TestHealthz(t *testing.T) {
	t.Parallel()

	failing := pingerFunc(func(context.Context) error { return errors.New("db down") })
	srv := apihttp.NewHandler(apihttp.Deps{DB: failing})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadyz(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ping           error
		expectedStatus int
		expectedBody   string
	}{
		{name: "database reachable", expectedStatus: http.StatusOK, expectedBody: "ok"},
		{name: "database unreachable", ping: errors.New("dial tcp: secret-host:5432"), expectedStatus: http.StatusServiceUnavailable, expectedBody: "unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := pingerFunc(func(context.Context) error { return tt.ping })
			srv := apihttp.NewHandler(apihttp.Deps{DB: db})

			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

			if rec.Code != tt.expectedStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.expectedStatus)
			}
			var body struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decoding body %q: %v", rec.Body.String(), err)
			}
			if body.Status != tt.expectedBody {
				t.Errorf("status field = %q, want %q", body.Status, tt.expectedBody)
			}
		})
	}
}

func TestUnknownRouteReturnsJSONError(t *testing.T) {
	t.Parallel()

	srv := apihttp.NewHandler(apihttp.Deps{DB: pingerFunc(func(context.Context) error { return nil })})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding body %q: %v", rec.Body.String(), err)
	}
	if body.Error.Code != "not_found" {
		t.Errorf("error code = %q, want %q", body.Error.Code, "not_found")
	}
}
