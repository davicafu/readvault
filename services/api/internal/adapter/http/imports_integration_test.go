//go:build integration

package http_test

import (
	"net/http"
	"os"
	"testing"

	apihttp "readvault/api/internal/adapter/http"
	"readvault/api/internal/adapter/postgres"
	"readvault/api/internal/importing"
	"readvault/api/internal/testdb"
)

func TestImportEndpoints_PreviewThenImportAgainstPostgres(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	h := apihttp.NewHandler(apihttp.Deps{
		DB:               pool,
		Imports:          importing.NewService(postgres.NewImportRepository(pool), 100),
		OwnerID:          postgres.DevOwnerID,
		MaxUploadBytes:   1 << 20,
		MaxImportRecords: 100,
	})
	fixture, err := os.ReadFile("../../importing/testdata/bookmarks.html")
	if err != nil {
		t.Fatal(err)
	}

	steps := []struct {
		path     string
		key      string
		expected map[string]float64
	}{
		{"/api/imports/preview", "totals", map[string]float64{"detected": 6, "valid": 3, "invalid": 2, "duplicateInFile": 1, "existing": 0}},
		{"/api/imports", "totals", map[string]float64{"inserted": 3, "updated": 0, "skipped": 1, "failed": 2}},
		{"/api/imports", "totals", map[string]float64{"inserted": 0, "updated": 0, "skipped": 4, "failed": 2}},
		{"/api/imports/preview", "totals", map[string]float64{"detected": 6, "valid": 0, "invalid": 2, "duplicateInFile": 1, "existing": 3}},
	}
	for i, s := range steps {
		body, ct := multipartBody(t, map[string]string{"file": string(fixture)})
		rec := post(t, h, s.path, body, ct)
		if rec.Code != http.StatusOK {
			t.Fatalf("step %d %s: status = %d; body %s", i+1, s.path, rec.Code, rec.Body.String())
		}
		totals, _ := decode(t, rec)[s.key].(map[string]any)
		for k, v := range s.expected {
			if totals[k] != v {
				t.Errorf("step %d %s: %s = %v, want %v", i+1, s.path, k, totals[k], v)
			}
		}
	}

	var owners int
	if err := pool.QueryRow(t.Context(), `SELECT count(DISTINCT user_id) FROM articles WHERE user_id = $1`, postgres.DevOwnerID).Scan(&owners); err != nil {
		t.Fatal(err)
	}
	if owners != 1 {
		t.Errorf("articles are not owned by the development owner")
	}
}
