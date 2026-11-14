package http_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

const testOwner = "00000000-0000-0000-0000-000000000001"

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, rec.Body.String())
	}
	return got
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	errBody, _ := decode(t, rec)["error"].(map[string]any)
	code, _ := errBody["code"].(string)
	return code
}
