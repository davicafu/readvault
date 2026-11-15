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

	// Tags returns the user's tags that have articles, with how many each
	// has: most articles first, then by name.
	Tags(ctx context.Context, userID string) ([]TagCount, error)

	// Collections returns the user's collections by name, with their
	// article counts.
	Collections(ctx context.Context, userID string) ([]Collection, error)
	// CreateCollection stores a new collection, or returns
	// ErrCollectionExists when the user has one with the same slug.
	CreateCollection(ctx context.Context, userID string, name tag.Tag) (Collection, error)
	// RenameCollection renames one collection of the user, or returns
	// ErrCollectionNotFound or ErrCollectionExists.
	RenameCollection(ctx context.Context, userID, collectionID string, name tag.Tag) (Collection, error)
	// Delete deletes one article of the user with its tags, saved text,
	// images and queued download; ErrNotFound when the user has no such
	// article.
	Delete(ctx context.Context, userID, articleID string) error
	// DeleteCollection deletes one collection of the user; its articles
	// stay, without a collection. Returns ErrCollectionNotFound.
	DeleteCollection(ctx context.Context, userID, collectionID string) error

	// SuggestionData returns one article of the user as a suggestion target,
	// with its saved text, and the user's other articles to compare it with,
	// most recently added first and bounded; or ErrNotFound.
	SuggestionData(ctx context.Context, userID, articleID string) (SuggestTarget, []SuggestPeer, error)
}

// Collection groups articles. An article is in at most one collection.
type Collection struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Articles int    `json:"articles"`
}

// CollectionRef names the collection an article is in.
type CollectionRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// TagCount is a tag and the number of the user's articles that carry it.
type TagCount struct {
	tag.Tag
	Articles int `json:"articles"`
}

// Change is a validated edit. Nil fields are left as they are; Tags
// replaces the whole tag set.
type Change struct {
	Title *string
	Tags  *[]tag.Tag
	// SetCollection moves the article to CollectionID, or out of any
	// collection when CollectionID is nil. The repository returns
	// ErrCollectionNotFound when the user has no such collection.
	SetCollection bool
	CollectionID  *string
	// Read marks the article read (true) or unread (false); nil leaves it.
	// Marking a read article read again keeps when it was first read.
	Read *bool
}

// Article is a saved bookmark with its tags and provenance.
type Article struct {
	ID            string     `json:"id"`
	OriginalURL   string     `json:"originalUrl"`
	CanonicalURL  string     `json:"canonicalUrl"`
	Title         string     `json:"title"`
	TitleEditedAt *time.Time `json:"titleEditedAt"`
	// TagsEditedAt is set when the user replaced the tags; imports then add
	// no tags to the article.
	TagsEditedAt *time.Time `json:"tagsEditedAt"`
	// ContentStatus is the state of the saved text: "complete",
	// "incomplete" or "failed"; nil when no text was saved.
	ContentStatus *string    `json:"contentStatus"`
	Folder        []string   `json:"folder"`
	BookmarkedAt  *time.Time `json:"bookmarkedAt"`
	Tags          []tag.Tag  `json:"tags"`
	// Collection is nil when the article is in none.
	Collection *CollectionRef `json:"collection"`
	// ReadAt is when the article was read; nil while it is unread.
	ReadAt    *time.Time `json:"readAt"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
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
	// CollectionSlug keeps the articles of that collection when set.
	CollectionSlug string
	// Text keeps the articles whose saved text is in this state; empty
	// keeps all.
	Text TextState
	// Read keeps only read (true) or unread (false) articles; nil keeps all.
	Read  *bool
	Sort  Sort
	Limit int
	After *Position
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
