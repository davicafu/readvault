package importing

import (
	"context"
	"time"
)

// Repository is the driven port the importing service persists through.
type Repository interface {
	// ExistingCanonicalURLs reports which of the given canonical URLs the user
	// already has in the library.
	ExistingCanonicalURLs(ctx context.Context, userID string, canonicalURLs []string) (map[string]bool, error)

	// Save stores a batch of new articles in one transaction. An article
	// whose canonical URL already exists is never duplicated: the adapter
	// locks it and applies FillMissing, adding any new tags without removing
	// existing ones. It returns one Outcome per article, in order.
	Save(ctx context.Context, userID string, articles []NewArticle) ([]Outcome, error)
}

// NewArticle is a valid candidate ready to be persisted.
type NewArticle struct {
	OriginalURL  string
	CanonicalURL string
	Title        string
	Folder       []string
	BookmarkedAt *time.Time
	Tags         []Tag
}

// Tag is a normalized tag: Slug is its identity per user, Name its label.
type Tag struct {
	Name string
	Slug string
}

// StoredArticle is the part of an existing article an import may complete.
type StoredArticle struct {
	Title         string
	TitleEditedAt *time.Time
	Folder        []string
	BookmarkedAt  *time.Time
}

// Outcome is what an import did with one article.
type Outcome string

const (
	// OutcomeInserted means a new article was created.
	OutcomeInserted Outcome = "inserted"
	// OutcomeUpdated means an existing article got missing metadata or new tags.
	OutcomeUpdated Outcome = "updated"
	// OutcomeSkipped means the article already existed and nothing changed.
	OutcomeSkipped Outcome = "skipped"
	// OutcomeFailed means the article could not be imported.
	OutcomeFailed Outcome = "failed"
)
