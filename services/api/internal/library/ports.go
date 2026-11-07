// Package library lists, finds and edits the articles in a user's library.
package library

import (
	"context"
	"time"

	"readvault/api/internal/tag"
)

// Repository is the driven port the library service reads through.
type Repository interface {
	// List returns up to q.Limit+1 articles of the user that match q, in
	// q.Sort order, starting after q.After when it is set.
	List(ctx context.Context, userID string, q Query) ([]Listed, error)

	// All returns every article of the user, oldest first.
	All(ctx context.Context, userID string) ([]Article, error)

	// Get returns one article of the user, or ErrNotFound.
	Get(ctx context.Context, userID, articleID string) (Article, error)

	// Update applies a validated change to one article of the user in one
	// transaction and returns the result, or ErrNotFound. A changed title
	// is marked as edited, so imports never overwrite it.
	Update(ctx context.Context, userID, articleID string, c Change) (Article, error)
}

// Change is a validated edit. Nil fields are left as they are; Tags
// replaces the whole tag set.
type Change struct {
	Title *string
	Tags  *[]tag.Tag
}

// Article is a saved bookmark with its tags and provenance.
type Article struct {
	ID            string     `json:"id"`
	OriginalURL   string     `json:"originalUrl"`
	CanonicalURL  string     `json:"canonicalUrl"`
	Title         string     `json:"title"`
	TitleEditedAt *time.Time `json:"titleEditedAt"`
	Folder        []string   `json:"folder"`
	BookmarkedAt  *time.Time `json:"bookmarkedAt"`
	Tags          []tag.Tag  `json:"tags"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// Listed is an article of a list page with the key it was sorted by when
// sorting by title, so the next page can start after it.
type Listed struct {
	Article
	TitleKey string
}

// Query is a validated list request.
type Query struct {
	Search string
	// TagSlugs keeps articles that have every one of these tags.
	TagSlugs []string
	Sort     Sort
	Limit    int
	After    *Position
}

// Position is the sort key and id of the last article of a page.
type Position struct {
	Key string
	ID  string
}

// Sort is a list order. Ties are broken by id so pages never overlap.
type Sort string

const (
	SortNewest    Sort = "created_desc"
	SortOldest    Sort = "created_asc"
	SortTitle     Sort = "title_asc"
	SortTitleDesc Sort = "title_desc"
)

// ByTitle reports whether s orders by title.
func (s Sort) ByTitle() bool { return s == SortTitle || s == SortTitleDesc }

// Descending reports whether s orders from the highest key down.
func (s Sort) Descending() bool { return s == SortNewest || s == SortTitleDesc }
