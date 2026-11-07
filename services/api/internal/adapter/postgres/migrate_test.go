//go:build integration

package postgres_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/adapter/postgres"
	"readvault/api/internal/testdb"
)

func TestMigrations_UpDownUp(t *testing.T) {
	t.Parallel()
	dbURL := testdb.Empty(t)

	if err := postgres.MigrateUp(dbURL); err != nil {
		t.Fatalf("first up: %v", err)
	}
	if err := postgres.MigrateUp(dbURL); err != nil {
		t.Fatalf("up is not idempotent: %v", err)
	}
	if err := postgres.MigrateDown(dbURL); err != nil {
		t.Fatalf("down: %v", err)
	}
	if err := postgres.MigrateUp(dbURL); err != nil {
		t.Fatalf("up after down: %v", err)
	}
}

func TestSchema_SeedsDevelopmentOwner(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)

	var email *string
	err := pool.QueryRow(t.Context(), "SELECT email FROM users WHERE id = $1", postgres.DevOwnerID).Scan(&email)
	if err != nil {
		t.Fatalf("development owner not found: %v", err)
	}
	if email != nil {
		t.Errorf("development owner email = %q, want NULL until authentication exists", *email)
	}
}

func TestSchema_Invariants(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	ctx := t.Context()

	const otherUser = "00000000-0000-0000-0000-0000000000ff"
	mustExec(t, pool, `INSERT INTO users (id) VALUES ($1)`, otherUser)
	mustExec(t, pool, `INSERT INTO articles (id, user_id, original_url, canonical_url)
		VALUES ('10000000-0000-0000-0000-000000000001', $1, 'https://Example.com/a#x', 'https://example.com/a')`, postgres.DevOwnerID)
	mustExec(t, pool, `INSERT INTO tags (id, user_id, name, slug)
		VALUES ('20000000-0000-0000-0000-000000000001', $1, 'Go', 'go')`, otherUser)

	tests := []struct {
		name           string
		sql            string
		args           []any
		expectedSQLErr string
	}{
		{
			name: "canonical url unique per user",
			sql: `INSERT INTO articles (user_id, original_url, canonical_url)
				VALUES ($1, 'https://example.com/a', 'https://example.com/a')`,
			args:           []any{postgres.DevOwnerID},
			expectedSQLErr: "23505",
		},
		{
			name: "article requires an existing user",
			sql: `INSERT INTO articles (user_id, original_url, canonical_url)
				VALUES ('00000000-0000-0000-0000-0000000000aa', 'https://example.com/c', 'https://example.com/c')`,
			expectedSQLErr: "23503",
		},
		{
			name:           "tag slug unique per user",
			sql:            `INSERT INTO tags (user_id, name, slug) VALUES ($1, 'GO', 'go')`,
			args:           []any{otherUser},
			expectedSQLErr: "23505",
		},
		{
			name: "article cannot use another user's tag",
			sql: `INSERT INTO article_tags (user_id, article_id, tag_id)
				VALUES ($1, '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000001')`,
			args:           []any{postgres.DevOwnerID},
			expectedSQLErr: "23503",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pool.Exec(ctx, tt.sql, tt.args...)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != tt.expectedSQLErr {
				t.Errorf("error = %v, want SQLSTATE %s", err, tt.expectedSQLErr)
			}
		})
	}

	t.Run("same canonical url allowed for another user", func(t *testing.T) {
		mustExec(t, pool, `INSERT INTO articles (user_id, original_url, canonical_url)
			VALUES ($1, 'https://example.com/a', 'https://example.com/a')`, otherUser)
	})

	t.Run("deleting an article removes its tag links", func(t *testing.T) {
		mustExec(t, pool, `INSERT INTO tags (id, user_id, name, slug)
			VALUES ('20000000-0000-0000-0000-000000000002', $1, 'Go', 'go')`, postgres.DevOwnerID)
		mustExec(t, pool, `INSERT INTO article_tags (user_id, article_id, tag_id)
			VALUES ($1, '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000002')`, postgres.DevOwnerID)
		mustExec(t, pool, `DELETE FROM articles WHERE id = '10000000-0000-0000-0000-000000000001'`)

		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM article_tags`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("article_tags rows = %d, want 0", n)
		}
	})
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}
