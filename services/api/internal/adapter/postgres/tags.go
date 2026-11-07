package postgres

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"readvault/api/internal/tag"
)

// createTags creates the tags the user does not have yet; an existing tag
// keeps its stored spelling. Every writer inserts tags in slug order, so
// two transactions adding the same new tags wait for each other instead of
// deadlocking on the unique (user_id, slug) index.
func createTags(ctx context.Context, tx pgx.Tx, userID string, tags []tag.Tag) error {
	sorted := slices.SortedFunc(slices.Values(tags), func(a, b tag.Tag) int { return strings.Compare(a.Slug, b.Slug) })
	names := make([]string, len(sorted))
	slugs := make([]string, len(sorted))
	for i, t := range sorted {
		names[i], slugs[i] = t.Name, t.Slug
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tags (user_id, name, slug)
		SELECT $1, t.name, t.slug FROM unnest($2::text[], $3::text[]) WITH ORDINALITY AS t (name, slug, ord)
		ORDER BY t.ord
		ON CONFLICT (user_id, slug) DO NOTHING`, userID, names, slugs); err != nil {
		return fmt.Errorf("creating tags: %w", err)
	}
	return nil
}
