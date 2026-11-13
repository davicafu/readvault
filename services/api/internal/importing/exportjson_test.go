package importing_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"readvault/api/internal/importing"
)

// readVaultExport is a ReadVault JSON export (GET /api/export).
const readVaultExport = `{
  "schemaVersion": 1,
  "exportedAt": "2026-11-13T10:00:00Z",
  "articles": [
    {
      "id": "ad28cff6-c943-4457-ae8c-ea9097845a5c",
      "originalUrl": "https://go.dev/blog/?utm_source=x#top",
      "canonicalUrl": "https://go.dev/blog/",
      "title": "The Go Blog",
      "titleEditedAt": null,
      "tagsEditedAt": null,
      "contentStatus": "complete",
      "folder": ["Bookmarks bar", "Go"],
      "bookmarkedAt": "2023-11-14T22:15:00Z",
      "tags": [{ "name": "go", "slug": "go" }, { "name": "Concurrency", "slug": "concurrency" }],
      "collection": { "id": "0b6f4c1e-5b3a-4a52-9a49-3f1d2b7c8e10", "name": "Backend", "slug": "backend" },
      "createdAt": "2026-11-07T23:27:07.5Z",
      "updatedAt": "2026-11-07T23:27:07.5Z"
    },
    {
      "id": "be28cff6-c943-4457-ae8c-ea9097845a5c",
      "originalUrl": "https://example.com/loose",
      "canonicalUrl": "https://example.com/loose",
      "title": "",
      "folder": [],
      "bookmarkedAt": null,
      "tags": [],
      "collection": null
    }
  ]
}`

func TestParse_ReadsAReadVaultExport(t *testing.T) {
	t.Parallel()

	records, err := importing.Parse(strings.NewReader("\n  "+readVaultExport), 100)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	added := time.Date(2023, 11, 14, 22, 15, 0, 0, time.UTC)
	expected := []importing.Record{
		{Position: 1, URL: "https://go.dev/blog/?utm_source=x#top", Title: "The Go Blog", Folder: []string{"Bookmarks bar", "Go"}, Collection: "Backend", AddedAt: &added, Tags: []string{"go", "Concurrency"}},
		{Position: 2, URL: "https://example.com/loose", Folder: []string{}, Tags: []string{}},
	}
	if len(records) != len(expected) {
		t.Fatalf("records = %+v, want %d", records, len(expected))
	}
	for i, r := range records {
		e := expected[i]
		sameTime := (r.AddedAt == nil) == (e.AddedAt == nil) && (r.AddedAt == nil || r.AddedAt.Equal(*e.AddedAt))
		if r.Position != e.Position || r.URL != e.URL || r.Title != e.Title || r.Collection != e.Collection ||
			!slices.Equal(r.Folder, e.Folder) || !slices.Equal(r.Tags, e.Tags) || !sameTime {
			t.Errorf("record %d = %+v, want %+v", i, r, e)
		}
	}
}

func TestParse_StillReadsBookmarkFiles(t *testing.T) {
	t.Parallel()

	fromParse, err := importing.Parse(strings.NewReader(chromeExport), 100)
	if err != nil {
		t.Fatal(err)
	}
	fromHTML, _ := importing.ParseHTML(strings.NewReader(chromeExport), 100)
	if len(fromParse) != len(fromHTML) || len(fromParse) == 0 || fromParse[1].URL != fromHTML[1].URL {
		t.Errorf("Parse = %+v, want the bookmarks of ParseHTML %+v", fromParse, fromHTML)
	}
}

func TestParse_ExportOverTheLimitIsTooManyRecords(t *testing.T) {
	t.Parallel()

	if _, err := importing.Parse(strings.NewReader(readVaultExport), 1); !errors.Is(err, importing.ErrTooManyRecords) {
		t.Errorf("err = %v, want ErrTooManyRecords", err)
	}
}

func TestParse_RejectsJSONThatIsNotAnExport(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"malformed":          `{"schemaVersion": 1, "articles": [`,
		"newer version":      `{"schemaVersion": 2, "articles": []}`,
		"no version":         `{"articles": []}`,
		"articles not list":  `{"schemaVersion": 1, "articles": {}}`,
		"article not object": `{"schemaVersion": 1, "articles": [1]}`,
		"trailing data":      `{"schemaVersion": 1, "articles": []} {}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := importing.Parse(strings.NewReader(body), 100); !errors.Is(err, importing.ErrInvalidFile) {
				t.Errorf("err = %v, want ErrInvalidFile", err)
			}
		})
	}
}
