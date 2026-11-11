package importing_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"readvault/api/internal/importing"
)

const chromeExport = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<!-- This is an automatically generated file. -->
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
    <DT><H3 ADD_DATE="1700000000" PERSONAL_TOOLBAR_FOLDER="true">Bookmarks bar</H3>
    <DL><p>
        <DT><A HREF="https://medium.com/@a/go-tips-1" ADD_DATE="1700000100">Go tips</A>
        <DT><H3>Engineering</H3>
        <DL><p>
            <DT><A HREF="https://medium.com/@b/postgres-indexes" ADD_DATE="1700000200" TAGS="db,postgres">Postgres &amp; índices</A>
        </DL><p>
        <DT><A HREF="https://example.com/after-nested">After nested</A>
    </DL><p>
    <DT><A HREF="https://example.com/root-level">Root level</A>
</DL><p>
`

func TestParseHTML(t *testing.T) {
	t.Parallel()

	records, err := importing.ParseHTML(strings.NewReader(chromeExport), 100)
	if err != nil {
		t.Fatalf("ParseHTML() error: %v", err)
	}

	expected := []importing.Record{
		{
			Position: 1,
			URL:      "https://medium.com/@a/go-tips-1",
			Title:    "Go tips",
			Folder:   []string{"Bookmarks bar"},
			AddedAt:  new(time.Unix(1700000100, 0).UTC()),
		},
		{
			Position:   2,
			URL:        "https://medium.com/@b/postgres-indexes",
			Title:      "Postgres & índices",
			Folder:     []string{"Bookmarks bar", "Engineering"},
			Collection: "Engineering",
			AddedAt:    new(time.Unix(1700000200, 0).UTC()),
			Tags:       []string{"db", "postgres"},
		},
		{
			Position: 3,
			URL:      "https://example.com/after-nested",
			Title:    "After nested",
			Folder:   []string{"Bookmarks bar"},
		},
		{
			Position: 4,
			URL:      "https://example.com/root-level",
			Title:    "Root level",
			Folder:   []string{},
		},
	}

	assertRecords(t, records, expected)
}

// A bookmark's collection is its first folder below the browser's own root
// folders (bookmarks bar, other bookmarks), never a deeper one.
func TestParseHTML_SuggestsTheFirstFolderBelowTheBrowserRoots(t *testing.T) {
	t.Parallel()

	input := `<DL><p>
		<DT><H3 PERSONAL_TOOLBAR_FOLDER="true">Barra de marcadores</H3>
		<DL><p>
			<DT><H3>Reading</H3>
			<DL><p>
				<DT><H3>Go</H3>
				<DL><p>
					<DT><A HREF="https://go.dev/doc">Go docs</A>
				</DL><p>
				<DT><A HREF="https://example.com/read">Read later</A>
			</DL><p>
			<DT><A HREF="https://example.com/bar">On the bar</A>
		</DL><p>
		<DT><H3 UNFILED_BOOKMARKS_FOLDER="true">Other Bookmarks</H3>
		<DL><p>
			<DT><A HREF="https://example.com/other">Other</A>
		</DL><p>
		<DT><H3>Streaming</H3>
		<DL><p>
			<DT><A HREF="https://kafka.apache.org">Kafka</A>
		</DL><p>
	</DL>`

	records, err := importing.ParseHTML(strings.NewReader(input), 100)
	if err != nil {
		t.Fatalf("ParseHTML() error: %v", err)
	}

	expected := []importing.Record{
		{Position: 1, URL: "https://go.dev/doc", Title: "Go docs", Folder: []string{"Barra de marcadores", "Reading", "Go"}, Collection: "Reading"},
		{Position: 2, URL: "https://example.com/read", Title: "Read later", Folder: []string{"Barra de marcadores", "Reading"}, Collection: "Reading"},
		{Position: 3, URL: "https://example.com/bar", Title: "On the bar", Folder: []string{"Barra de marcadores"}},
		{Position: 4, URL: "https://example.com/other", Title: "Other", Folder: []string{"Other Bookmarks"}},
		{Position: 5, URL: "https://kafka.apache.org", Title: "Kafka", Folder: []string{"Streaming"}, Collection: "Streaming"},
	}

	assertRecords(t, records, expected)
}

func TestParseHTML_ToleratesMalformedEntries(t *testing.T) {
	t.Parallel()

	input := `<DL><p>
		<DT><A HREF="https://example.com/no-title"></A>
		<DT><A>no href at all</A>
		<DT><A HREF="javascript:alert(1)">evil</A>
		<DT><A HREF="https://example.com/bad-date" ADD_DATE="yesterday">Bad date</A>
		<DT><A HREF="https://example.com/unclosed">Unclosed
	`

	records, err := importing.ParseHTML(strings.NewReader(input), 100)
	if err != nil {
		t.Fatalf("ParseHTML() error: %v", err)
	}

	expected := []importing.Record{
		{Position: 1, URL: "https://example.com/no-title", Folder: []string{}},
		{Position: 2, URL: "", Title: "no href at all", Folder: []string{}},
		{Position: 3, URL: "javascript:alert(1)", Title: "evil", Folder: []string{}},
		{Position: 4, URL: "https://example.com/bad-date", Title: "Bad date", Folder: []string{}},
		{Position: 5, URL: "https://example.com/unclosed", Title: "Unclosed", Folder: []string{}},
	}

	assertRecords(t, records, expected)
}

func TestParseHTML_DoesNotInterpretScriptContent(t *testing.T) {
	t.Parallel()

	input := `<DL><p>
		<DT><A HREF="https://example.com/x"><script>alert(1)</script>Title</A>
		<script>document.write('<A HREF="https://injected.example">x</A>')</script>
	</DL>`

	records, err := importing.ParseHTML(strings.NewReader(input), 100)
	if err != nil {
		t.Fatalf("ParseHTML() error: %v", err)
	}

	expected := []importing.Record{
		{Position: 1, URL: "https://example.com/x", Title: "Title", Folder: []string{}},
	}

	assertRecords(t, records, expected)
}

func TestParseHTML_TooManyRecords(t *testing.T) {
	t.Parallel()

	input := strings.Repeat(`<DT><A HREF="https://example.com/a">a</A>`, 3)

	_, err := importing.ParseHTML(strings.NewReader(input), 2)
	if !errors.Is(err, importing.ErrTooManyRecords) {
		t.Errorf("ParseHTML() error = %v, want %v", err, importing.ErrTooManyRecords)
	}
}

func FuzzParseHTML(f *testing.F) {
	f.Add(chromeExport)
	f.Add("<DL><DL><DL></DL>")
	f.Add("</DL></DL><A HREF=x>")

	f.Fuzz(func(t *testing.T, input string) {
		records, err := importing.ParseHTML(strings.NewReader(input), 1000)
		if err != nil && !errors.Is(err, importing.ErrTooManyRecords) {
			t.Fatalf("unexpected error: %v", err)
		}
		for i, r := range records {
			if r.Position != i+1 {
				t.Fatalf("record %d has position %d", i, r.Position)
			}
		}
	})
}

func assertRecords(t *testing.T, got, expected []importing.Record) {
	t.Helper()

	if len(got) != len(expected) {
		t.Fatalf("got %d records, want %d: %+v", len(got), len(expected), got)
	}
	for i := range expected {
		g, e := got[i], expected[i]
		sameTime := (g.AddedAt == nil && e.AddedAt == nil) ||
			(g.AddedAt != nil && e.AddedAt != nil && g.AddedAt.Equal(*e.AddedAt))
		same := g.Position == e.Position &&
			g.URL == e.URL &&
			g.Title == e.Title &&
			slices.Equal(g.Folder, e.Folder) &&
			slices.Equal(g.Tags, e.Tags) &&
			g.Collection == e.Collection &&
			sameTime
		if !same {
			t.Errorf("record %d:\n got  %+v\n want %+v", i, g, e)
		}
	}
}
