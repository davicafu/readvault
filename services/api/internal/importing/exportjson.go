package importing

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// ErrInvalidFile means a JSON file that is not a ReadVault export this
// version can read.
var ErrInvalidFile = errors.New("importing: not a readvault export")

// exportSchemaVersion is the ReadVault export format this importer reads
// (library.ExportSchemaVersion).
const exportSchemaVersion = 1

// Parse reads an uploaded file: a ReadVault JSON export (GET /api/export),
// recognised by its first character "{", or else a browser bookmarks file.
// Restoring an export goes through the same rules as a bookmarks import, so
// articles missing from the library are added, a lost collection is created
// again and articles without a collection go back to theirs.
func Parse(r io.Reader, maxRecords int) ([]Record, error) {
	br := bufio.NewReader(r)
	for {
		b, err := br.Peek(1)
		if err != nil || !isSpace(b[0]) {
			if err == nil && b[0] == '{' {
				return ParseExport(br, maxRecords)
			}
			return ParseHTML(br, maxRecords)
		}
		_, _ = br.ReadByte()
	}
}

func isSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }

// exportArticle holds the fields of an exported article that an import
// uses; ids, times of edits and the text status are the library's own.
type exportArticle struct {
	OriginalURL  string     `json:"originalUrl"`
	Title        string     `json:"title"`
	Folder       []string   `json:"folder"`
	BookmarkedAt *time.Time `json:"bookmarkedAt"`
	ReadAt       *time.Time `json:"readAt"`
	Tags         []struct {
		Name string `json:"name"`
	} `json:"tags"`
	Collection *struct {
		Name string `json:"name"`
	} `json:"collection"`
}

// ParseExport reads a ReadVault JSON export, one article at a time, into
// records in file order. Fields it does not use are skipped.
func ParseExport(r io.Reader, maxRecords int) ([]Record, error) {
	dec := json.NewDecoder(r)
	if err := expectDelim(dec, '{'); err != nil {
		return nil, err
	}
	records := []Record{}
	version, seenArticles := 0, false
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, invalid(err)
		}
		switch key {
		case "schemaVersion":
			if err := dec.Decode(&version); err != nil {
				return nil, invalid(err)
			}
		case "articles":
			seenArticles = true
			if records, err = decodeArticles(dec, maxRecords); err != nil {
				return nil, err
			}
		default:
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return nil, invalid(err)
			}
		}
	}
	if err := expectDelim(dec, '}'); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: data after the export", ErrInvalidFile)
	}
	if version != exportSchemaVersion || !seenArticles {
		return nil, fmt.Errorf("%w: schemaVersion %d", ErrInvalidFile, version)
	}
	return records, nil
}

func decodeArticles(dec *json.Decoder, maxRecords int) ([]Record, error) {
	if err := expectDelim(dec, '['); err != nil {
		return nil, err
	}
	records := []Record{}
	for dec.More() {
		if len(records) >= maxRecords {
			return nil, ErrTooManyRecords
		}
		var a exportArticle
		if err := dec.Decode(&a); err != nil {
			return nil, invalid(err)
		}
		rec := Record{Position: len(records) + 1, URL: a.OriginalURL, Title: a.Title, Folder: a.Folder, Tags: []string{}}
		if rec.Folder == nil {
			rec.Folder = []string{}
		}
		if a.BookmarkedAt != nil {
			t := a.BookmarkedAt.UTC()
			rec.AddedAt = &t
		}
		if a.ReadAt != nil {
			t := a.ReadAt.UTC()
			rec.ReadAt = &t
		}
		for _, t := range a.Tags {
			rec.Tags = append(rec.Tags, t.Name)
		}
		if a.Collection != nil {
			rec.Collection = a.Collection.Name
		}
		records = append(records, rec)
	}
	if err := expectDelim(dec, ']'); err != nil {
		return nil, err
	}
	return records, nil
}

func expectDelim(dec *json.Decoder, want json.Delim) error {
	tok, err := dec.Token()
	if err != nil {
		return invalid(err)
	}
	if d, ok := tok.(json.Delim); !ok || d != want {
		return fmt.Errorf("%w: expected %q", ErrInvalidFile, want)
	}
	return nil
}

func invalid(err error) error { return fmt.Errorf("%w: %v", ErrInvalidFile, err) }
