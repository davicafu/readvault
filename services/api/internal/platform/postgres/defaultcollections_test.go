//go:build integration

package postgres_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"readvault/api/internal/platform/postgres"
	"readvault/api/internal/tag"
	"readvault/api/internal/testdb"
)

var defaultCollections = []string{
	"AI and tools",
	"Architecture and system design",
	"Backend",
	"Data engineering",
	"Databases and SQL",
	"DevOps, cloud and Linux",
	"Frontend and visualization",
	"Machine learning and analytics",
	"Streaming and Kafka",
}

type collectionRow struct{ name, slug string }

func collectionsOf(t *testing.T, conn *pgx.Conn, userID string) []collectionRow {
	t.Helper()
	rows, err := conn.Query(t.Context(), `SELECT name, slug FROM collections WHERE user_id = $1 ORDER BY name COLLATE "C"`, userID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (collectionRow, error) {
		var c collectionRow
		return c, r.Scan(&c.name, &c.slug)
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestMigration_DefaultCollectionsInEnglish(t *testing.T) {
	t.Parallel()
	dbURL := testdb.Empty(t)
	if err := postgres.MigrateTo(dbURL, 5); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)

	// The owner has the Spanish collections, one article in one of them, and
	// a collection of their own; another user has none.
	const other = "00000000-0000-0000-0000-0000000000ff"
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO users (id) VALUES ($1)`, other)
	for _, name := range []string{"Arquitectura y system design", "Streaming y Kafka", "Ingeniería de datos", "Mis cosas"} {
		exec(`INSERT INTO collections (user_id, name, slug) VALUES ($1, $2, $3)`, postgres.DevOwnerID, name, tag.Slugify(name))
	}
	exec(`INSERT INTO articles (user_id, original_url, canonical_url, collection_id)
		SELECT $1, 'https://example.com/a', 'https://example.com/a', id FROM collections WHERE slug = 'streaming-y-kafka'`, postgres.DevOwnerID)

	if err := postgres.MigrateUp(dbURL); err != nil {
		t.Fatal(err)
	}

	want := make([]collectionRow, 0, len(defaultCollections))
	for _, n := range defaultCollections {
		want = append(want, collectionRow{n, tag.Slugify(n)})
	}
	if got := collectionsOf(t, conn, other); !slices.Equal(got, want) {
		t.Errorf("other user's collections = %v, want the defaults %v", got, want)
	}
	ownerWant := append(slices.Clone(want), collectionRow{"Mis cosas", "mis-cosas"})
	slices.SortFunc(ownerWant, func(a, b collectionRow) int {
		if a.name < b.name {
			return -1
		}
		return 1
	})
	if got := collectionsOf(t, conn, postgres.DevOwnerID); !slices.Equal(got, ownerWant) {
		t.Errorf("owner's collections = %v, want the defaults (renamed, not duplicated) plus their own %v", got, ownerWant)
	}
	var name string
	if err := conn.QueryRow(ctx, `SELECT c.name FROM articles a JOIN collections c ON c.id = a.collection_id
		WHERE a.canonical_url = 'https://example.com/a'`).Scan(&name); err != nil || name != "Streaming and Kafka" {
		t.Errorf("article's collection = %q, %v; want it kept in the renamed collection", name, err)
	}

	// Down keeps the user's collections; up again adds nothing twice.
	if err := postgres.MigrateDown(dbURL); err != nil {
		t.Fatal(err)
	}
	if err := postgres.MigrateUp(dbURL); err != nil {
		t.Fatal(err)
	}
	if got := collectionsOf(t, conn, postgres.DevOwnerID); !slices.Equal(got, ownerWant) {
		t.Errorf("after down and up: %v, want %v", got, ownerWant)
	}
}
