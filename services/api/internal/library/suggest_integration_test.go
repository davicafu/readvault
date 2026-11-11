//go:build integration

package library_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"readvault/api/internal/library"
)

func putInCollection(t *testing.T, pool *pgxpool.Pool, userID, name string, articleIDs ...string) {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(), `INSERT INTO collections (user_id, name, slug) VALUES ($1, $2, lower($2)) RETURNING id`,
		userID, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE articles SET collection_id = $1 WHERE id = ANY($2::uuid[])`, id, articleIDs); err != nil {
		t.Fatal(err)
	}
}

func TestSuggestions_ComeFromTheSavedTextAndTheUsersLibrary(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ctx := t.Context()

	ids := seed(t, pool, owner,
		seedArticle{url: "https://a.example/1", title: "Kafka consumer groups explained", created: base, tags: []string{"kafka"}},
		seedArticle{url: "https://a.example/2", title: "Kafka partitions and rebalancing", created: base, tags: []string{"kafka"}},
		seedArticle{url: "https://a.example/3", title: "Go error handling", created: base, tags: []string{"go"}},
		// Saved from a link: the title is the address, the text says what it is about.
		seedArticle{url: "https://blog.example.com/p/8812", title: "https://blog.example.com/p/8812", created: base},
	)
	putInCollection(t, pool, owner, "Streaming", ids[0], ids[1])
	putInCollection(t, pool, owner, "Backend", ids[2])
	if _, err := pool.Exec(ctx, `INSERT INTO article_contents (article_id, user_id, status, source, markdown, words)
		VALUES ($1, $2, 'complete', 'fetched', $3, 12)`, ids[3], owner,
		"# Consumer groups\n\nKafka assigns partitions to consumers. Rebalancing in Kafka moves them."); err != nil {
		t.Fatal(err)
	}

	// Another user's library is never read.
	other := newUser(t, pool)
	theirs := seed(t, pool, other,
		seedArticle{url: "https://b.example/1", title: "Kafka consumer partitions rebalancing", created: base, tags: []string{"consumers"}},
		seedArticle{url: "https://b.example/2", title: "Kafka consumer partitions", created: base, tags: []string{"consumers"}},
		seedArticle{url: "https://b.example/3", title: "Kafka partitions rebalancing", created: base, tags: []string{"consumers"}},
	)
	putInCollection(t, pool, other, "Theirs", theirs...)

	got, err := svc.Suggest(ctx, owner, ids[3])
	if err != nil {
		t.Fatalf("Suggest() error: %v", err)
	}
	if got.Collection == nil || got.Collection.Name != "Streaming" {
		t.Errorf("collection = %+v, want Streaming", got.Collection)
	}
	var slugs []string
	for _, tg := range got.Tags {
		slugs = append(slugs, tg.Slug)
	}
	if !slices.Equal(slugs, []string{"kafka"}) {
		t.Errorf("tags = %v, want [kafka] (only the user's own tags)", slugs)
	}
}

func TestSuggestions_OnlyForTheUsersOwnArticles(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	other := newUser(t, pool)
	theirs := seed(t, pool, other, seedArticle{url: "https://b.example/1", title: "Theirs", created: base})

	for name, id := range map[string]string{
		"another user's article": theirs[0],
		"unknown id":             "6b0c7f0e-8d7a-4c7e-9b1d-2f3a4b5c6d7e",
		"malformed id":           "not-a-uuid",
	} {
		if _, err := svc.Suggest(t.Context(), owner, id); !errors.Is(err, library.ErrNotFound) {
			t.Errorf("%s: Suggest() error = %v, want ErrNotFound", name, err)
		}
	}
}
