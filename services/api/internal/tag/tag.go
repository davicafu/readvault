// Package tag holds the rules every tag follows, whether it comes from an
// import or from a user edit.
package tag

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// MaxRunes bounds the length of a tag name.
const MaxRunes = 64

var (
	// ErrEmpty means the name has no visible characters.
	ErrEmpty = errors.New("tag: empty name")
	// ErrTooLong means the name is longer than MaxRunes.
	ErrTooLong = errors.New("tag: name too long")
)

// Tag is a normalized tag: Slug is its identity per user, Name its label.
type Tag struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Normalize collapses whitespace in name and derives its slug.
func Normalize(name string) (Tag, error) {
	name = strings.Join(strings.Fields(name), " ")
	switch {
	case name == "":
		return Tag{}, ErrEmpty
	case utf8.RuneCountInString(name) > MaxRunes:
		return Tag{}, ErrTooLong
	}
	return Tag{Name: name, Slug: Slugify(name)}, nil
}

// Slugify returns the per-user identity of a tag name: lowercase words
// joined by hyphens. "Go News" and "go  news" share the slug "go-news".
func Slugify(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), "-")
}
