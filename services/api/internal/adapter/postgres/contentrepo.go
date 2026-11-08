package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/content"
)

// ContentRepository stores article texts and images in Postgres.
type ContentRepository struct {
	pool *pgxpool.Pool
}

// NewContentRepository returns a repository over pool.
func NewContentRepository(pool *pgxpool.Pool) *ContentRepository {
	return &ContentRepository{pool: pool}
}

var _ content.Repository = (*ContentRepository)(nil)

// ArticleURL implements content.Repository.
func (r *ContentRepository) ArticleURL(ctx context.Context, userID, articleID string) (string, error) {
	var u string
	err := r.pool.QueryRow(ctx, `SELECT original_url FROM articles WHERE user_id = $1 AND id = $2`, userID, articleID).Scan(&u)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", content.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("loading article url: %w", err)
	}
	return u, nil
}

// Save implements content.Repository. It locks the article row first, the
// order every other writer of an article uses, then replaces the text and
// its images in one transaction.
func (r *ContentRepository) Save(ctx context.Context, userID, articleID string, c content.NewContent) (content.Content, error) {
	var saved content.Content
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := lockArticle(ctx, tx, userID, articleID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO article_contents
				(article_id, user_id, status, source, markdown, words, problem, missing_images, saved_at)
			VALUES ($1, $2, $3, $4, $5, $6, NULL, $7, now())
			ON CONFLICT (article_id) DO UPDATE SET status = EXCLUDED.status, source = EXCLUDED.source,
				markdown = EXCLUDED.markdown, words = EXCLUDED.words, problem = NULL,
				missing_images = EXCLUDED.missing_images, saved_at = now()`,
			articleID, userID, c.Status, c.Source, c.Markdown, c.Words, c.MissingImages); err != nil {
			return fmt.Errorf("saving content: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM article_images WHERE article_id = $1`, articleID); err != nil {
			return fmt.Errorf("replacing images: %w", err)
		}
		for i, img := range c.Images {
			if _, err := tx.Exec(ctx, `INSERT INTO article_images (article_id, user_id, position, source_url, content_type, data)
				VALUES ($1, $2, $3, $4, $5, $6)`, articleID, userID, i, img.SourceURL, img.ContentType, img.Data); err != nil {
				return fmt.Errorf("saving image: %w", err)
			}
		}
		var err error
		saved, err = getContent(ctx, tx, userID, articleID)
		return err
	})
	return saved, err
}

// SaveFailure implements content.Repository.
func (r *ContentRepository) SaveFailure(ctx context.Context, userID, articleID, problem string) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := lockArticle(ctx, tx, userID, articleID); err != nil {
			return err
		}
		// Saved text stays; only the problem is recorded on it.
		if _, err := tx.Exec(ctx, `INSERT INTO article_contents (article_id, user_id, status, source, problem, saved_at)
			VALUES ($1, $2, 'failed', 'fetched', $3, now())
			ON CONFLICT (article_id) DO UPDATE SET problem = EXCLUDED.problem`,
			articleID, userID, problem); err != nil {
			return fmt.Errorf("saving download failure: %w", err)
		}
		return nil
	})
}

func lockArticle(ctx context.Context, tx pgx.Tx, userID, articleID string) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM articles WHERE user_id = $1 AND id = $2 FOR UPDATE`, userID, articleID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return content.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("locking article: %w", err)
	}
	return nil
}

// Get implements content.Repository.
func (r *ContentRepository) Get(ctx context.Context, userID, articleID string) (content.Content, error) {
	if _, err := r.ArticleURL(ctx, userID, articleID); err != nil {
		return content.Content{}, err
	}
	return getContent(ctx, r.pool, userID, articleID)
}

// rowQuerier is satisfied by both the pool and a transaction.
type rowQuerier interface {
	querier
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func getContent(ctx context.Context, q rowQuerier, userID, articleID string) (content.Content, error) {
	c := content.Content{ArticleID: articleID, Images: []content.ImageInfo{}}
	err := q.QueryRow(ctx, `SELECT status, source, markdown, words, problem, missing_images, saved_at
		FROM article_contents WHERE user_id = $1 AND article_id = $2`, userID, articleID).
		Scan(&c.Status, &c.Source, &c.Markdown, &c.Words, &c.Problem, &c.MissingImages, &c.SavedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return content.Content{}, content.ErrNoContent
	}
	if err != nil {
		return content.Content{}, fmt.Errorf("loading content: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT id, source_url FROM article_images
		WHERE user_id = $1 AND article_id = $2 ORDER BY position`, userID, articleID)
	if err != nil {
		return content.Content{}, fmt.Errorf("loading images: %w", err)
	}
	var img content.ImageInfo
	if _, err := pgx.ForEachRow(rows, []any{&img.ID, &img.SourceURL}, func() error {
		c.Images = append(c.Images, img)
		return nil
	}); err != nil {
		return content.Content{}, fmt.Errorf("loading images: %w", err)
	}
	return c, nil
}

// Image implements content.Repository.
func (r *ContentRepository) Image(ctx context.Context, userID, articleID, imageID string) (content.Image, error) {
	var img content.Image
	err := r.pool.QueryRow(ctx, `SELECT content_type, data FROM article_images
		WHERE user_id = $1 AND article_id = $2 AND id = $3`, userID, articleID, imageID).Scan(&img.ContentType, &img.Data)
	if errors.Is(err, pgx.ErrNoRows) {
		return content.Image{}, content.ErrNotFound
	}
	if err != nil {
		return content.Image{}, fmt.Errorf("loading image: %w", err)
	}
	return img, nil
}
