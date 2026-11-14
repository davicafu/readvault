//go:build integration

// Package testdb gives integration tests an isolated, migrated PostgreSQL
// database. It needs TEST_DATABASE_URL pointing at a server where the user
// may create databases (the compose Postgres works).
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/platform/postgres"
)

// New creates an empty database, applies every migration and returns a pool
// to it, with an empty library: the default collections the migrations
// create are removed, so tests start from nothing. The database is dropped
// when the test ends.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := Empty(t)
	if err := postgres.MigrateUp(dbURL); err != nil {
		t.Fatalf("migrating test database: %v", err)
	}
	pool, err := postgres.Open(t.Context(), dbURL)
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(t.Context(), "DELETE FROM collections"); err != nil {
		t.Fatalf("emptying the default collections: %v", err)
	}
	return pool
}

// Empty creates an empty database and returns its URL. The database is
// dropped when the test ends.
func Empty(t *testing.T) string {
	t.Helper()

	admin := os.Getenv("TEST_DATABASE_URL")
	if admin == "" {
		admin = os.Getenv("DATABASE_URL")
	}
	if admin == "" {
		t.Fatal("TEST_DATABASE_URL (or DATABASE_URL) must be set for integration tests")
	}

	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	name := "rv_test_" + hex.EncodeToString(suffix)

	ctx := t.Context()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatalf("connecting to admin database: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("creating test database: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		conn, err := pgx.Connect(ctx, admin)
		if err != nil {
			t.Errorf("connecting to drop test database: %v", err)
			return
		}
		defer conn.Close(ctx)
		if _, err := conn.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("dropping test database: %v", err)
		}
	})

	u, err := url.Parse(admin)
	if err != nil {
		t.Fatalf("parsing admin url: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}
