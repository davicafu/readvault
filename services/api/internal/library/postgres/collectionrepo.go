package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"readvault/api/internal/library"
	"readvault/api/internal/tag"
)

// pgerrcode holds the SQLSTATE codes this package checks.
var pgerrcode = struct{ UniqueViolation, ForeignKeyViolation string }{"23505", "23503"}

func isViolation(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

func samePtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return strings.EqualFold(*a, *b)
}

const selectCollection = `SELECT c.id, c.name, c.slug,
	(SELECT count(*) FROM articles a WHERE a.collection_id = c.id)
	FROM collections c`

func scanCollection(row pgx.CollectableRow) (library.Collection, error) {
	var c library.Collection
	err := row.Scan(&c.ID, &c.Name, &c.Slug, &c.Articles)
	return c, err
}

// Collections implements library.Repository.
func (r *Repository) Collections(ctx context.Context, userID string) ([]library.Collection, error) {
	rows, err := r.pool.Query(ctx, selectCollection+` WHERE c.user_id = $1 ORDER BY lower(c.name), c.slug`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying collections: %w", err)
	}
	cs, err := pgx.CollectRows(rows, scanCollection)
	if err != nil {
		return nil, fmt.Errorf("reading collections: %w", err)
	}
	return cs, nil
}

// CreateCollection implements library.Repository.
func (r *Repository) CreateCollection(ctx context.Context, userID string, name tag.Tag) (library.Collection, error) {
	c := library.Collection{Name: name.Name, Slug: name.Slug}
	err := r.pool.QueryRow(ctx, `INSERT INTO collections (user_id, name, slug) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, slug) DO NOTHING RETURNING id`, userID, name.Name, name.Slug).Scan(&c.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return library.Collection{}, library.ErrCollectionExists
	}
	if err != nil {
		return library.Collection{}, fmt.Errorf("inserting collection: %w", err)
	}
	return c, nil
}

// RenameCollection implements library.Repository.
func (r *Repository) RenameCollection(ctx context.Context, userID, collectionID string, name tag.Tag) (library.Collection, error) {
	res, err := r.pool.Exec(ctx, `UPDATE collections SET name = $3, slug = $4, updated_at = now()
		WHERE user_id = $1 AND id = $2`, userID, collectionID, name.Name, name.Slug)
	switch {
	case isViolation(err, pgerrcode.UniqueViolation):
		return library.Collection{}, library.ErrCollectionExists
	case err != nil:
		return library.Collection{}, fmt.Errorf("renaming collection: %w", err)
	case res.RowsAffected() == 0:
		return library.Collection{}, library.ErrCollectionNotFound
	}
	rows, err := r.pool.Query(ctx, selectCollection+` WHERE c.user_id = $1 AND c.id = $2`, userID, collectionID)
	if err != nil {
		return library.Collection{}, fmt.Errorf("querying collection: %w", err)
	}
	c, err := pgx.CollectExactlyOneRow(rows, scanCollection)
	if errors.Is(err, pgx.ErrNoRows) {
		// Deleted between the rename and this read.
		return library.Collection{}, library.ErrCollectionNotFound
	}
	if err != nil {
		return library.Collection{}, fmt.Errorf("reading collection: %w", err)
	}
	return c, nil
}

// Delete implements library.Repository. The foreign keys remove the
// article's tags, saved text, images and queued download with it.
func (r *Repository) Delete(ctx context.Context, userID, articleID string) error {
	res, err := r.pool.Exec(ctx, `DELETE FROM articles WHERE user_id = $1 AND id = $2`, userID, articleID)
	if err != nil {
		return fmt.Errorf("deleting article: %w", err)
	}
	if res.RowsAffected() == 0 {
		return library.ErrNotFound
	}
	return nil
}

// DeleteCollection implements library.Repository. The foreign key clears
// collection_id on the collection's articles.
func (r *Repository) DeleteCollection(ctx context.Context, userID, collectionID string) error {
	res, err := r.pool.Exec(ctx, `DELETE FROM collections WHERE user_id = $1 AND id = $2`, userID, collectionID)
	if err != nil {
		return fmt.Errorf("deleting collection: %w", err)
	}
	if res.RowsAffected() == 0 {
		return library.ErrCollectionNotFound
	}
	return nil
}
