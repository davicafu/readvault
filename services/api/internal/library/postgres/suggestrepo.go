package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"readvault/api/internal/library"
	"readvault/api/internal/tag"
)

// Suggestions read bounded data: the most recent suggestPeers articles and
// the first suggestTextChars characters of the saved text.
const (
	suggestPeers     = 10000
	suggestTextChars = 200000
)

// SuggestionData implements library.Repository.
func (r *Repository) SuggestionData(ctx context.Context, userID, articleID string) (library.SuggestTarget, []library.SuggestPeer, error) {
	var t library.SuggestTarget
	var names, slugs []string
	err := r.pool.QueryRow(ctx, `SELECT a.id, a.title, a.original_url,
		COALESCE((SELECT left(c.markdown, $3) FROM article_contents c WHERE c.article_id = a.id), ''),
		ARRAY(SELECT t.name FROM article_tags at JOIN tags t ON t.id = at.tag_id WHERE at.article_id = a.id ORDER BY t.slug),
		ARRAY(SELECT t.slug FROM article_tags at JOIN tags t ON t.id = at.tag_id WHERE at.article_id = a.id ORDER BY t.slug)
		FROM articles a WHERE a.user_id = $1 AND a.id = $2`, userID, articleID, suggestTextChars).
		Scan(&t.ID, &t.Title, &t.URL, &t.Text, &names, &slugs)
	if errors.Is(err, pgx.ErrNoRows) {
		return library.SuggestTarget{}, nil, library.ErrNotFound
	}
	if err != nil {
		return library.SuggestTarget{}, nil, fmt.Errorf("reading suggestion target: %w", err)
	}
	t.Tags = zipTags(names, slugs)

	rows, err := r.pool.Query(ctx, `SELECT a.id, a.title, a.collection_id,
		ARRAY(SELECT t.name FROM article_tags at JOIN tags t ON t.id = at.tag_id WHERE at.article_id = a.id ORDER BY t.slug),
		ARRAY(SELECT t.slug FROM article_tags at JOIN tags t ON t.id = at.tag_id WHERE at.article_id = a.id ORDER BY t.slug)
		FROM articles a WHERE a.user_id = $1
		ORDER BY a.created_at DESC, a.id LIMIT $2`, userID, suggestPeers)
	if err != nil {
		return library.SuggestTarget{}, nil, fmt.Errorf("querying suggestion peers: %w", err)
	}
	peers, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (library.SuggestPeer, error) {
		var p library.SuggestPeer
		var names, slugs []string
		err := row.Scan(&p.ID, &p.Title, &p.CollectionID, &names, &slugs)
		p.Tags = zipTags(names, slugs)
		return p, err
	})
	if err != nil {
		return library.SuggestTarget{}, nil, fmt.Errorf("reading suggestion peers: %w", err)
	}
	return t, peers, nil
}

func zipTags(names, slugs []string) []tag.Tag {
	tags := make([]tag.Tag, 0, len(names))
	for i := range min(len(names), len(slugs)) {
		tags = append(tags, tag.Tag{Name: names[i], Slug: slugs[i]})
	}
	return tags
}
