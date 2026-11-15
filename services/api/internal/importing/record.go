// Package importing turns browser bookmark exports into
// validated import candidates and persists them idempotently.
package importing

import (
	"errors"
	"time"
)

// ErrTooManyRecords means the file holds more records than the allowed limit.
var ErrTooManyRecords = errors.New("importing: too many records")

// Record is one raw entry extracted from an uploaded file, before validation.
// Position is 1-based and stable, so problems can be reported per record.
type Record struct {
	Position int
	URL      string
	Title    string
	Folder   []string
	// Collection is the suggested collection: the first folder below the
	// browser's root folders, or "" when there is none.
	Collection string
	AddedAt    *time.Time
	// ReadAt is when the article was read, from a ReadVault export; nil
	// for bookmark files and unread articles.
	ReadAt *time.Time
	Tags   []string
}
