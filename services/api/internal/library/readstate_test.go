//go:build integration

package library_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"readvault/api/internal/library"
)

func read(v bool) library.Nullable[bool] { return library.Nullable[bool]{Set: true, Value: &v} }

func TestUpdate_ReadMustBeTrueOrFalse(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ids := seed(t, pool, owner, seedArticle{url: "https://a.example/one", title: "One", created: base})
	before := snapshot(t, pool)

	// {"read": null}, alone or with another field, is refused and changes nothing.
	for _, p := range []library.Patch{
		{Read: library.Nullable[bool]{Set: true}},
		{Title: ptr("New"), Read: library.Nullable[bool]{Set: true}},
	} {
		_, err := svc.Update(t.Context(), owner, ids[0], p)
		var ve *library.ValidationError
		if !errors.As(err, &ve) || len(ve.Problems) != 1 || ve.Problems[0].Field != "read" || ve.Problems[0].Code != "invalid" {
			t.Errorf("%+v: %v, want a read/invalid problem", p, err)
		}
	}
	if after := snapshot(t, pool); after != before {
		t.Error("a refused read state changed the library")
	}
}

func TestUpdate_MarksAnArticleReadAndUnread(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ids := seed(t, pool, owner, seedArticle{url: "https://a.example/one", title: "One", created: base})

	got, err := svc.Update(t.Context(), owner, ids[0], library.Patch{Read: read(true)})
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if got.ReadAt == nil {
		t.Fatal("readAt is nil after marking the article read")
	}
	// Reading is not editing: the article keeps its updatedAt and edit marks.
	if !got.UpdatedAt.Equal(base) || got.TitleEditedAt != nil || got.TagsEditedAt != nil {
		t.Errorf("updatedAt %v, titleEditedAt %v, tagsEditedAt %v: marking read must not count as an edit",
			got.UpdatedAt, got.TitleEditedAt, got.TagsEditedAt)
	}

	// Marking it read again keeps the first time it was read.
	first := *got.ReadAt
	again, err := svc.Update(t.Context(), owner, ids[0], library.Patch{Read: read(true)})
	if err != nil || again.ReadAt == nil || !again.ReadAt.Equal(first) {
		t.Errorf("marking read twice: readAt %v (err %v), want %v", again.ReadAt, err, first)
	}

	unread, err := svc.Update(t.Context(), owner, ids[0], library.Patch{Read: read(false)})
	if err != nil || unread.ReadAt != nil {
		t.Errorf("mark unread: readAt %v (err %v), want nil", unread.ReadAt, err)
	}
	stored, err := svc.Get(t.Context(), owner, ids[0])
	if err != nil || stored.ReadAt != nil {
		t.Errorf("stored readAt %v (err %v), want nil", stored.ReadAt, err)
	}
}

func TestUpdate_ReadOnAnotherUsersArticleIsNotFound(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	other := newUser(t, pool)
	ids := seed(t, pool, other, seedArticle{url: "https://a.example/theirs", title: "Theirs", created: base})

	if _, err := svc.Update(t.Context(), owner, ids[0], library.Patch{Read: read(true)}); !errors.Is(err, library.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	var readAt *time.Time
	if err := pool.QueryRow(t.Context(), `SELECT read_at FROM articles WHERE id = $1`, ids[0]).Scan(&readAt); err != nil || readAt != nil {
		t.Errorf("their article's read_at = %v (err %v), want nil", readAt, err)
	}
}

func TestList_FiltersReadAndUnreadArticles(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ids := seed(t, pool, owner,
		seedArticle{url: "https://a.example/read", title: "read", created: base},
		seedArticle{url: "https://a.example/unread", title: "unread", created: base.Add(time.Hour)},
	)
	if _, err := svc.Update(t.Context(), owner, ids[0], library.Patch{Read: read(true)}); err != nil {
		t.Fatal(err)
	}

	for read, expected := range map[string][]string{
		"true":  {"read"},
		"false": {"unread"},
		"":      {"read", "unread"},
	} {
		page, err := svc.List(t.Context(), owner, library.ListParams{Read: read, Sort: "created_asc"})
		if err != nil {
			t.Fatalf("read=%q: %v", read, err)
		}
		var got []string
		for _, a := range page.Items {
			got = append(got, a.Title)
		}
		if !slices.Equal(got, expected) {
			t.Errorf("read=%q: %v, want %v", read, got, expected)
		}
	}

	_, err := svc.List(t.Context(), owner, library.ListParams{Read: "yes"})
	var ve *library.ValidationError
	if !errors.As(err, &ve) || len(ve.Problems) != 1 || ve.Problems[0].Field != "read" || ve.Problems[0].Code != "invalid" {
		t.Errorf("read=yes: %v, want a read/invalid problem", err)
	}
}

func TestExport_IncludesWhenEachArticleWasRead(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	ids := seed(t, pool, owner,
		seedArticle{url: "https://a.example/read", title: "read", created: base},
		seedArticle{url: "https://a.example/unread", title: "unread", created: base.Add(time.Hour)},
	)
	if _, err := svc.Update(t.Context(), owner, ids[0], library.Patch{Read: read(true)}); err != nil {
		t.Fatal(err)
	}

	export, err := svc.Export(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(export.Articles) != 2 || export.Articles[0].ReadAt == nil || export.Articles[1].ReadAt != nil {
		t.Errorf("exported readAt: %+v", export.Articles)
	}
}
