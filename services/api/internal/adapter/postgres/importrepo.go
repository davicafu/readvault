package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/importing"
)

// ImportRepository persists imported articles. It implements importing.Repository.
type ImportRepository struct {
	pool *pgxpool.Pool
}

var _ importing.Repository = (*ImportRepository)(nil)

// NewImportRepository returns a repository backed by pool.
func NewImportRepository(pool *pgxpool.Pool) *ImportRepository {
	return &ImportRepository{pool: pool}
}

// ExistingCanonicalURLs implements importing.Repository.
func (r *ImportRepository) ExistingCanonicalURLs(ctx context.Context, userID string, canonicalURLs []string) (map[string]bool, error) {
	found := make(map[string]bool)
	if len(canonicalURLs) == 0 {
		return found, nil
	}

	rows, err := r.pool.Query(ctx,
		`SELECT canonical_url FROM articles WHERE user_id = $1 AND canonical_url = ANY($2)`,
		userID, canonicalURLs)
	if err != nil {
		return nil, fmt.Errorf("querying existing articles: %w", err)
	}
	urls, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("reading existing articles: %w", err)
	}
	for _, u := range urls {
		found[u] = true
	}
	return found, nil
}

// Save implements importing.Repository. The whole batch runs in one
// transaction; the unique (user_id, canonical_url) constraint is what
// prevents duplicates, including under concurrent imports.
func (r *ImportRepository) Save(ctx context.Context, userID string, articles []importing.NewArticle) ([]importing.Outcome, error) {
	outcomes := make([]importing.Outcome, len(articles))
	if len(articles) == 0 {
		return outcomes, nil
	}

	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		ids, err := insertArticles(ctx, tx, userID, articles, outcomes)
		if err != nil {
			return err
		}
		filled, err := fillExisting(ctx, tx, userID, articles, ids, outcomes)
		if err != nil {
			return err
		}
		tagged, err := linkTags(ctx, tx, userID, articles, ids)
		if err != nil {
			return err
		}
		return touchUpdated(ctx, tx, articles, ids, outcomes, filled, tagged)
	})
	if err != nil {
		return nil, fmt.Errorf("saving import batch: %w", err)
	}
	return outcomes, nil
}

// insertArticles inserts every article that does not exist yet and returns
// the id of each article by position. Existing ones are marked skipped and
// get their id from fillExisting.
func insertArticles(ctx context.Context, tx pgx.Tx, userID string, articles []importing.NewArticle, outcomes []importing.Outcome) ([]string, error) {
	batch := &pgx.Batch{}
	for _, a := range articles {
		batch.Queue(`INSERT INTO articles (user_id, original_url, canonical_url, title, original_folder, bookmarked_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (user_id, canonical_url) DO NOTHING
			RETURNING id`,
			userID, a.OriginalURL, a.CanonicalURL, a.Title, a.Folder, a.BookmarkedAt)
	}

	ids := make([]string, len(articles))
	results := tx.SendBatch(ctx, batch)
	for i := range articles {
		err := results.QueryRow().Scan(&ids[i])
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			outcomes[i] = importing.OutcomeSkipped
		case err != nil:
			_ = results.Close()
			return nil, fmt.Errorf("inserting article: %w", err)
		default:
			outcomes[i] = importing.OutcomeInserted
		}
	}
	if err := results.Close(); err != nil {
		return nil, fmt.Errorf("inserting articles: %w", err)
	}
	return ids, nil
}

// fillExisting locks the articles that already existed and completes their
// missing metadata with importing.FillMissing. It returns the positions it changed.
func fillExisting(ctx context.Context, tx pgx.Tx, userID string, articles []importing.NewArticle, ids []string, outcomes []importing.Outcome) (map[int]bool, error) {
	byURL := map[string]int{}
	var urls []string
	for i, a := range articles {
		if outcomes[i] == importing.OutcomeSkipped {
			byURL[a.CanonicalURL] = i
			urls = append(urls, a.CanonicalURL)
		}
	}
	changed := map[int]bool{}
	if len(urls) == 0 {
		return changed, nil
	}

	type stored struct {
		ID           string
		CanonicalURL string
		importing.StoredArticle
	}
	rows, err := tx.Query(ctx, `SELECT id, canonical_url, title, title_edited_at, original_folder, bookmarked_at
		FROM articles WHERE user_id = $1 AND canonical_url = ANY($2)
		ORDER BY id FOR UPDATE`, userID, urls)
	if err != nil {
		return nil, fmt.Errorf("locking existing articles: %w", err)
	}
	existing, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (stored, error) {
		var s stored
		err := row.Scan(&s.ID, &s.CanonicalURL, &s.Title, &s.TitleEditedAt, &s.Folder, &s.BookmarkedAt)
		return s, err
	})
	if err != nil {
		return nil, fmt.Errorf("reading existing articles: %w", err)
	}

	batch := &pgx.Batch{}
	for _, s := range existing {
		i := byURL[s.CanonicalURL]
		ids[i] = s.ID
		filled, ok := importing.FillMissing(s.StoredArticle, articles[i])
		if !ok {
			continue
		}
		changed[i] = true
		batch.Queue(`UPDATE articles SET title = $2, original_folder = $3, bookmarked_at = $4, updated_at = now()
			WHERE id = $1`, s.ID, filled.Title, filled.Folder, filled.BookmarkedAt)
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return nil, fmt.Errorf("completing existing articles: %w", err)
	}
	return changed, nil
}

// linkTags creates missing tags and links them to the articles. Existing
// links are never removed. It returns the positions that gained a tag.
func linkTags(ctx context.Context, tx pgx.Tx, userID string, articles []importing.NewArticle, ids []string) (map[int]bool, error) {
	var names, slugs []string
	seen := map[string]bool{}
	for _, a := range articles {
		for _, t := range a.Tags {
			if !seen[t.Slug] {
				seen[t.Slug] = true
				names = append(names, t.Name)
				slugs = append(slugs, t.Slug)
			}
		}
	}
	gained := map[int]bool{}
	if len(slugs) == 0 {
		return gained, nil
	}

	if _, err := tx.Exec(ctx, `INSERT INTO tags (user_id, name, slug)
		SELECT $1, name, slug FROM unnest($2::text[], $3::text[]) AS t (name, slug)
		ON CONFLICT (user_id, slug) DO NOTHING`, userID, names, slugs); err != nil {
		return nil, fmt.Errorf("creating tags: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT slug, id FROM tags WHERE user_id = $1 AND slug = ANY($2)`, userID, slugs)
	if err != nil {
		return nil, fmt.Errorf("loading tags: %w", err)
	}
	tagIDs := map[string]string{}
	var slug, id string
	if _, err := pgx.ForEachRow(rows, []any{&slug, &id}, func() error {
		tagIDs[slug] = id
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reading tags: %w", err)
	}

	var articleIDs, linkTagIDs []string
	position := map[string]int{}
	for i, a := range articles {
		position[ids[i]] = i
		for _, t := range a.Tags {
			articleIDs = append(articleIDs, ids[i])
			linkTagIDs = append(linkTagIDs, tagIDs[t.Slug])
		}
	}
	rows, err = tx.Query(ctx, `INSERT INTO article_tags (user_id, article_id, tag_id)
		SELECT $1, article_id, tag_id FROM unnest($2::uuid[], $3::uuid[]) AS l (article_id, tag_id)
		ON CONFLICT (article_id, tag_id) DO NOTHING
		RETURNING article_id`, userID, articleIDs, linkTagIDs)
	if err != nil {
		return nil, fmt.Errorf("linking tags: %w", err)
	}
	var articleID string
	if _, err := pgx.ForEachRow(rows, []any{&articleID}, func() error {
		gained[position[articleID]] = true
		return nil
	}); err != nil {
		return nil, fmt.Errorf("linking tags: %w", err)
	}
	return gained, nil
}

// touchUpdated marks existing articles that changed as updated.
func touchUpdated(ctx context.Context, tx pgx.Tx, articles []importing.NewArticle, ids []string, outcomes []importing.Outcome, filled, tagged map[int]bool) error {
	var touched []string
	for i := range articles {
		if outcomes[i] != importing.OutcomeSkipped || !(filled[i] || tagged[i]) {
			continue
		}
		outcomes[i] = importing.OutcomeUpdated
		if !filled[i] {
			touched = append(touched, ids[i])
		}
	}
	if len(touched) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE articles SET updated_at = $2 WHERE id = ANY($1::uuid[])`, touched, time.Now()); err != nil {
		return fmt.Errorf("touching tagged articles: %w", err)
	}
	return nil
}
