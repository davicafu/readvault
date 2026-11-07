package importing

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"readvault/api/internal/urlnorm"
)

const (
	maxURLBytes   = 2048
	maxTitleRunes = 500
	maxTagRunes   = 64
)

// analyzed pairs a candidate with the article it would become when valid.
type analyzed struct {
	Candidate
	article NewArticle
}

// analyze validates records and flags duplicates within the file. The first
// occurrence of a canonical URL wins; later ones are duplicate_in_file.
// It does not know which URLs already exist in the library.
func analyze(records []Record) []analyzed {
	seen := make(map[string]bool, len(records))
	out := make([]analyzed, 0, len(records))

	for _, rec := range records {
		a := analyzed{Candidate: Candidate{
			Position:    rec.Position,
			Status:      StatusValid,
			OriginalURL: rec.URL,
			Title:       rec.Title,
			Folder:      rec.Folder,
			Tags:        []string{},
			Errors:      []Problem{},
			Warnings:    []Problem{},
		}}

		canonical, problem := canonicalize(rec.URL)
		switch {
		case problem != nil:
			a.Status = StatusInvalid
			a.Errors = append(a.Errors, *problem)
		case seen[canonical]:
			a.Status = StatusDuplicateInFile
			a.CanonicalURL = canonical
			a.Warnings = append(a.Warnings, Problem{Code: "duplicate_in_file", Message: "the same URL appears earlier in the file"})
		default:
			seen[canonical] = true
			a.CanonicalURL = canonical
		}

		switch {
		case a.Title == "":
			a.Warnings = append(a.Warnings, Problem{Code: "missing_title", Message: "the bookmark has no title"})
		case utf8.RuneCountInString(a.Title) > maxTitleRunes:
			a.Title = string([]rune(a.Title)[:maxTitleRunes])
			a.Warnings = append(a.Warnings, Problem{Code: "title_truncated", Message: fmt.Sprintf("the title was cut to %d characters", maxTitleRunes)})
		}

		tags, tagProblems := normalizeTags(rec.Tags)
		a.Warnings = append(a.Warnings, tagProblems...)
		for _, tag := range tags {
			a.Tags = append(a.Tags, tag.Name)
		}

		a.article = NewArticle{
			OriginalURL:  rec.URL,
			CanonicalURL: a.CanonicalURL,
			Title:        a.Title,
			Folder:       rec.Folder,
			BookmarkedAt: rec.AddedAt,
			Tags:         tags,
		}
		out = append(out, a)
	}
	return out
}

func canonicalize(raw string) (string, *Problem) {
	switch {
	case raw == "":
		return "", &Problem{Code: "missing_url", Message: "the bookmark has no URL"}
	case len(raw) > maxURLBytes:
		return "", &Problem{Code: "url_too_long", Message: fmt.Sprintf("the URL is longer than %d bytes", maxURLBytes)}
	}

	canonical, err := urlnorm.Canonicalize(raw)
	switch {
	case errors.Is(err, urlnorm.ErrUnsupportedScheme):
		return "", &Problem{Code: "unsupported_scheme", Message: "only http and https URLs can be imported"}
	case err != nil:
		return "", &Problem{Code: "invalid_url", Message: "the URL is not valid"}
	case len(canonical) > maxURLBytes:
		return "", &Problem{Code: "url_too_long", Message: fmt.Sprintf("the URL is longer than %d bytes", maxURLBytes)}
	}
	return canonical, nil
}

// normalizeTags collapses whitespace, drops over-long tags and removes
// tags that share a slug, keeping the first spelling.
func normalizeTags(names []string) ([]Tag, []Problem) {
	tags := []Tag{}
	var problems []Problem
	for _, name := range names {
		name = strings.Join(strings.Fields(name), " ")
		if name == "" {
			continue
		}
		if utf8.RuneCountInString(name) > maxTagRunes {
			problems = append(problems, Problem{Code: "tag_too_long", Message: fmt.Sprintf("a tag longer than %d characters was skipped", maxTagRunes)})
			continue
		}
		slug := Slugify(name)
		if slices.ContainsFunc(tags, func(t Tag) bool { return t.Slug == slug }) {
			continue
		}
		tags = append(tags, Tag{Name: name, Slug: slug})
	}
	return tags, problems
}

// Slugify returns the per-user identity of a tag name: lowercase words
// joined by hyphens. "Go News" and "go  news" share the slug "go-news".
func Slugify(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), "-")
}

// FillMissing completes an existing article with imported metadata without
// overwriting anything the user or an earlier import already set: a title is
// filled only when it is empty and was never edited; folder and bookmark
// date only when absent. It reports whether anything changed.
func FillMissing(existing StoredArticle, incoming NewArticle) (StoredArticle, bool) {
	filled := existing
	changed := false

	if existing.Title == "" && existing.TitleEditedAt == nil && incoming.Title != "" {
		filled.Title = incoming.Title
		changed = true
	}
	if len(existing.Folder) == 0 && len(incoming.Folder) > 0 {
		filled.Folder = incoming.Folder
		changed = true
	}
	if existing.BookmarkedAt == nil && incoming.BookmarkedAt != nil {
		filled.BookmarkedAt = incoming.BookmarkedAt
		changed = true
	}
	return filled, changed
}
