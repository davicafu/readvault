package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/library"
	"readvault/api/internal/tag"
)

// LibraryRepository reads the library. It implements library.Repository.
type LibraryRepository struct {
	pool *pgxpool.Pool
}

var _ library.Repository = (*LibraryRepository)(nil)

// NewLibraryRepository returns a repository backed by pool.
func NewLibraryRepository(pool *pgxpool.Pool) *LibraryRepository {
	return &LibraryRepository{pool: pool}
}

// selectArticle reads an article with its tags, sorted by slug, and the
// title key the title sorts use.
const selectArticle = `SELECT a.id, a.original_url, a.canonical_url, a.title, a.title_edited_at,
	a.original_folder, a.bookmarked_at, a.created_at, a.updated_at,
	ARRAY(SELECT t.name FROM article_tags at JOIN tags t ON t.id = at.tag_id WHERE at.article_id = a.id ORDER BY t.slug),
	ARRAY(SELECT t.slug FROM article_tags at JOIN tags t ON t.id = at.tag_id WHERE at.article_id = a.id ORDER BY t.slug),
	lower(a.title)
	FROM articles a`

// List implements library.Repository. Pages use keyset pagination on the
// (sort key, id) indexes, so deep pages cost the same as the first one.
func (r *LibraryRepository) List(ctx context.Context, userID string, q library.Query) ([]library.Listed, error) {
	args := []any{userID}
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	where := []string{"a.user_id = $1"}

	if q.Search != "" {
		text, pattern := arg(q.Search), arg("%"+escapeLike(q.Search)+"%")
		where = append(where, fmt.Sprintf(`(a.search @@ websearch_to_tsquery('simple', %[1]s)
			OR a.title ILIKE %[2]s OR a.canonical_url ILIKE %[2]s
			OR EXISTS (SELECT 1 FROM article_tags at JOIN tags t ON t.id = at.tag_id
				WHERE at.article_id = a.id AND t.name ILIKE %[2]s))`, text, pattern))
	}
	if len(q.TagSlugs) > 0 {
		slugs := arg(q.TagSlugs)
		where = append(where, fmt.Sprintf(`(SELECT count(*) FROM article_tags at JOIN tags t ON t.id = at.tag_id
			WHERE at.article_id = a.id AND t.slug = ANY(%[1]s::text[])) = cardinality(%[1]s::text[])`, slugs))
	}

	key, keyType := "a.created_at", "timestamptz"
	if q.Sort.ByTitle() {
		key, keyType = "lower(a.title)", "text"
	}
	dir, op := "ASC", ">"
	if q.Sort.Descending() {
		dir, op = "DESC", "<"
	}
	if q.After != nil {
		where = append(where, fmt.Sprintf("(%s, a.id) %s (%s::%s, %s::uuid)", key, op, arg(q.After.Key), keyType, arg(q.After.ID)))
	}

	sql := fmt.Sprintf("%s WHERE %s ORDER BY %s %s, a.id %s LIMIT %s",
		selectArticle, strings.Join(where, " AND "), key, dir, dir, arg(q.Limit+1))
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("querying articles: %w", err)
	}
	listed, err := pgx.CollectRows(rows, scanListed)
	if err != nil {
		return nil, fmt.Errorf("reading articles: %w", err)
	}
	return listed, nil
}

// Get implements library.Repository.
func (r *LibraryRepository) Get(ctx context.Context, userID, articleID string) (library.Article, error) {
	rows, err := r.pool.Query(ctx, selectArticle+" WHERE a.user_id = $1 AND a.id = $2", userID, articleID)
	if err != nil {
		return library.Article{}, fmt.Errorf("querying article: %w", err)
	}
	l, err := pgx.CollectExactlyOneRow(rows, scanListed)
	if errors.Is(err, pgx.ErrNoRows) {
		return library.Article{}, library.ErrNotFound
	}
	if err != nil {
		return library.Article{}, fmt.Errorf("reading article: %w", err)
	}
	return l.Article, nil
}

func scanListed(row pgx.CollectableRow) (library.Listed, error) {
	var l library.Listed
	var names, slugs []string
	err := row.Scan(&l.ID, &l.OriginalURL, &l.CanonicalURL, &l.Title, &l.TitleEditedAt,
		&l.Folder, &l.BookmarkedAt, &l.CreatedAt, &l.UpdatedAt, &names, &slugs, &l.TitleKey)
	if l.Folder == nil {
		l.Folder = []string{}
	}
	l.Tags = make([]tag.Tag, len(names))
	for i := range names {
		l.Tags[i] = tag.Tag{Name: names[i], Slug: slugs[i]}
	}
	return l, err
}

// escapeLike makes s match literally inside a LIKE pattern.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
