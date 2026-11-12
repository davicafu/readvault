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
const selectArticle = `SELECT a.id, a.original_url, a.canonical_url, a.title, a.title_edited_at, a.tags_edited_at,
	a.original_folder, a.bookmarked_at, a.created_at, a.updated_at,
	(SELECT c.status FROM article_contents c WHERE c.article_id = a.id),
	ARRAY(SELECT t.name FROM article_tags at JOIN tags t ON t.id = at.tag_id WHERE at.article_id = a.id ORDER BY t.slug),
	ARRAY(SELECT t.slug FROM article_tags at JOIN tags t ON t.id = at.tag_id WHERE at.article_id = a.id ORDER BY t.slug),
	lower(a.title),
	col.id, col.name, col.slug
	FROM articles a LEFT JOIN collections col ON col.id = a.collection_id`

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
	if q.CollectionSlug != "" {
		where = append(where, fmt.Sprintf("a.collection_id = (SELECT id FROM collections WHERE user_id = $1 AND slug = %s)", arg(q.CollectionSlug)))
	}
	switch q.Text {
	case "":
	case library.TextMissing:
		where = append(where, "NOT EXISTS (SELECT 1 FROM article_contents c WHERE c.article_id = a.id)")
	default:
		where = append(where, fmt.Sprintf("EXISTS (SELECT 1 FROM article_contents c WHERE c.article_id = a.id AND c.status = %s)", arg(string(q.Text))))
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

// All implements library.Repository.
func (r *LibraryRepository) All(ctx context.Context, userID string) ([]library.Article, error) {
	rows, err := r.pool.Query(ctx, selectArticle+" WHERE a.user_id = $1 ORDER BY a.created_at, a.id", userID)
	if err != nil {
		return nil, fmt.Errorf("querying articles: %w", err)
	}
	listed, err := pgx.CollectRows(rows, scanListed)
	if err != nil {
		return nil, fmt.Errorf("reading articles: %w", err)
	}
	articles := make([]library.Article, len(listed))
	for i, l := range listed {
		articles[i] = l.Article
	}
	return articles, nil
}

// Tags implements library.Repository.
func (r *LibraryRepository) Tags(ctx context.Context, userID string) ([]library.TagCount, error) {
	rows, err := r.pool.Query(ctx, `SELECT t.name, t.slug, count(*) AS articles
		FROM tags t JOIN article_tags at ON at.tag_id = t.id
		WHERE t.user_id = $1
		GROUP BY t.id
		ORDER BY articles DESC, lower(t.name), t.slug`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying tags: %w", err)
	}
	tags, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (library.TagCount, error) {
		var c library.TagCount
		err := row.Scan(&c.Name, &c.Slug, &c.Articles)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("reading tags: %w", err)
	}
	return tags, nil
}

// Get implements library.Repository.
func (r *LibraryRepository) Get(ctx context.Context, userID, articleID string) (library.Article, error) {
	return getArticle(ctx, r.pool, userID, articleID)
}

// querier is what a pool and a transaction share for reads.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func getArticle(ctx context.Context, q querier, userID, articleID string) (library.Article, error) {
	rows, err := q.Query(ctx, selectArticle+" WHERE a.user_id = $1 AND a.id = $2", userID, articleID)
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
	var colID, colName, colSlug *string
	err := row.Scan(&l.ID, &l.OriginalURL, &l.CanonicalURL, &l.Title, &l.TitleEditedAt, &l.TagsEditedAt,
		&l.Folder, &l.BookmarkedAt, &l.CreatedAt, &l.UpdatedAt, &l.ContentStatus, &names, &slugs, &l.TitleKey,
		&colID, &colName, &colSlug)
	if colID != nil && colName != nil && colSlug != nil {
		l.Collection = &library.CollectionRef{ID: *colID, Name: *colName, Slug: *colSlug}
	}
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

// Update implements library.Repository. The article row is locked first, so
// concurrent edits of the same article apply one after the other.
func (r *LibraryRepository) Update(ctx context.Context, userID, articleID string, c library.Change) (library.Article, error) {
	var updated library.Article
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var title string
		var collectionID *string
		err := tx.QueryRow(ctx, `SELECT title, collection_id FROM articles WHERE user_id = $1 AND id = $2 FOR UPDATE`, userID, articleID).Scan(&title, &collectionID)
		if errors.Is(err, pgx.ErrNoRows) {
			return library.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("locking article: %w", err)
		}

		changed := false
		if c.Title != nil && *c.Title != title {
			if _, err := tx.Exec(ctx, `UPDATE articles SET title = $2, title_edited_at = now() WHERE id = $1`, articleID, *c.Title); err != nil {
				return fmt.Errorf("updating title: %w", err)
			}
			changed = true
		}
		if c.Tags != nil {
			tagsChanged, err := replaceTags(ctx, tx, userID, articleID, *c.Tags)
			if err != nil {
				return err
			}
			// Only a changed set counts as the user's: the web app sends the
			// tags with every save, including title-only edits.
			if tagsChanged {
				if _, err := tx.Exec(ctx, `UPDATE articles SET tags_edited_at = now() WHERE id = $1`, articleID); err != nil {
					return fmt.Errorf("marking tags edited: %w", err)
				}
			}
			changed = changed || tagsChanged
		}
		if c.SetCollection && !samePtr(c.CollectionID, collectionID) {
			// The (collection_id, user_id) foreign key rejects another
			// user's collection as well as a missing one.
			if _, err := tx.Exec(ctx, `UPDATE articles SET collection_id = $2 WHERE id = $1`, articleID, c.CollectionID); err != nil {
				if isViolation(err, pgerrcode.ForeignKeyViolation) {
					return library.ErrCollectionNotFound
				}
				return fmt.Errorf("moving article to collection: %w", err)
			}
			changed = true
		}
		if changed {
			if _, err := tx.Exec(ctx, `UPDATE articles SET updated_at = now() WHERE id = $1`, articleID); err != nil {
				return fmt.Errorf("touching article: %w", err)
			}
		}
		// Read the result inside the transaction, so the response is the
		// state this edit wrote, not a later writer's.
		updated, err = getArticle(ctx, tx, userID, articleID)
		return err
	})
	if err != nil {
		return library.Article{}, err
	}
	return updated, nil
}

// replaceTags makes tags the article's whole tag set. Missing tags are
// created; an existing tag keeps its stored spelling. It reports whether
// any link was added or removed.
func replaceTags(ctx context.Context, tx pgx.Tx, userID, articleID string, tags []tag.Tag) (bool, error) {
	slugs := make([]string, len(tags))
	for i, t := range tags {
		slugs[i] = t.Slug
	}

	removed, err := tx.Exec(ctx, `DELETE FROM article_tags at USING tags t
		WHERE at.tag_id = t.id AND at.article_id = $1 AND NOT (t.slug = ANY($2::text[]))`, articleID, slugs)
	if err != nil {
		return false, fmt.Errorf("removing tags: %w", err)
	}
	if len(tags) == 0 {
		return removed.RowsAffected() > 0, nil
	}
	if err := createTags(ctx, tx, userID, tags); err != nil {
		return false, err
	}
	added, err := tx.Exec(ctx, `INSERT INTO article_tags (user_id, article_id, tag_id)
		SELECT $1, $2, id FROM tags WHERE user_id = $1 AND slug = ANY($3::text[])
		ON CONFLICT (article_id, tag_id) DO NOTHING`, userID, articleID, slugs)
	if err != nil {
		return false, fmt.Errorf("linking tags: %w", err)
	}
	return removed.RowsAffected() > 0 || added.RowsAffected() > 0, nil
}
