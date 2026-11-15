package http_test

import (
	"net/http"
	"testing"
)

func TestListArticles_PassesTheReadFilter(t *testing.T) {
	t.Parallel()
	svc := &stubLibrary{}

	rec := get(t, svc, "/api/articles?read=false")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body.String())
	}
	if svc.gotParams.Read != "false" {
		t.Errorf("read = %q, want %q", svc.gotParams.Read, "false")
	}
}

func TestPatchArticle_PassesTheReadState(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		body string
		want bool
	}{{`{"read":true}`, true}, {`{"read":false}`, false}} {
		svc := &stubLibrary{}
		rec := patch(t, svc, "/api/articles/a1", tc.body)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d; body %s", tc.body, rec.Code, rec.Body.String())
		}
		if svc.gotPatch.Read == nil || *svc.gotPatch.Read != tc.want || svc.gotPatch.Title != nil || svc.gotPatch.Tags != nil {
			t.Errorf("%s: patch = %+v", tc.body, svc.gotPatch)
		}
	}

	// A read state that is not true or false is refused before the service.
	svc := &stubLibrary{}
	if rec := patch(t, svc, "/api/articles/a1", `{"read":"yes"}`); rec.Code != http.StatusBadRequest || svc.gotID != "" {
		t.Errorf(`{"read":"yes"}: status %d, service called with %q`, rec.Code, svc.gotID)
	}
}
