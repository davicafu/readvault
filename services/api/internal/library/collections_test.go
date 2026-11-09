//go:build integration

package library_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"readvault/api/internal/library"
)

func collectionNames(cs []library.Collection) []string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.Name
	}
	return names
}

func problemCodes(t *testing.T, err error) []string {
	t.Helper()
	var verr *library.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want a validation error", err)
	}
	codes := make([]string, len(verr.Problems))
	for i, p := range verr.Problems {
		codes[i] = p.Field + "/" + p.Code
	}
	return codes
}

func setCollection(id *string) library.Patch {
	return library.Patch{CollectionID: library.Nullable[string]{Set: true, Value: id}}
}

func TestCollections_CreateNormalizesTheNameAndRejectsDuplicates(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	user := newUser(t, pool)

	created, err := svc.CreateCollection(t.Context(), user, "  Data   Engineering ")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Name != "Data Engineering" || created.Slug != "data-engineering" || created.Articles != 0 {
		t.Errorf("created = %+v", created)
	}

	if _, err := svc.CreateCollection(t.Context(), user, "data engineering"); !errors.Is(err, library.ErrCollectionExists) {
		t.Errorf("duplicate: err = %v, want ErrCollectionExists", err)
	}
	// Another user may use the same name.
	if _, err := svc.CreateCollection(t.Context(), newUser(t, pool), "Data Engineering"); err != nil {
		t.Errorf("other user: %v", err)
	}
}

func TestCollections_CreateRejectsInvalidNames(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	user := newUser(t, pool)

	for name, expected := range map[string]string{
		"   ":                    "name/required",
		string(make([]rune, 65)): "name/invalid", // 65 NUL runes
		"\xff":                   "name/invalid",
		"a234567890123456789012345678901234567890123456789012345678901234x": "name/too_long",
	} {
		_, err := svc.CreateCollection(t.Context(), user, name)
		if got := problemCodes(t, err); !slices.Equal(got, []string{expected}) {
			t.Errorf("name %q: problems = %v, want %v", name, got, expected)
		}
	}
}

func TestCollections_ListCountsArticlesAndShowsOnlyTheUsersOwn(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	user, other := newUser(t, pool), newUser(t, pool)
	ids := seed(t, pool, user,
		seedArticle{url: "https://a.example/1", title: "one", created: base},
		seedArticle{url: "https://a.example/2", title: "two", created: base},
	)
	reading, _ := svc.CreateCollection(t.Context(), user, "reading")
	_, _ = svc.CreateCollection(t.Context(), user, "Archive")
	_, _ = svc.CreateCollection(t.Context(), other, "theirs")
	for _, id := range ids {
		if _, err := svc.Update(t.Context(), user, id, setCollection(&reading.ID)); err != nil {
			t.Fatal(err)
		}
	}

	got, err := svc.Collections(t.Context(), user)

	if err != nil {
		t.Fatal(err)
	}
	if names := collectionNames(got); !slices.Equal(names, []string{"Archive", "reading"}) {
		t.Errorf("collections = %v, want Archive, reading (by name)", names)
	}
	if got[0].Articles != 0 || got[1].Articles != 2 {
		t.Errorf("counts = %d, %d; want 0, 2", got[0].Articles, got[1].Articles)
	}

	empty, err := svc.Collections(t.Context(), newUser(t, pool))
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("new user: %#v, %v; want an empty, non-nil list", empty, err)
	}
}

func TestCollections_AnArticleIsInAtMostOneCollection(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	user := newUser(t, pool)
	id := seed(t, pool, user, seedArticle{url: "https://a.example/1", title: "one", created: base, tags: []string{"go", "archive"}})[0]
	reading, _ := svc.CreateCollection(t.Context(), user, "Reading")
	archive, _ := svc.CreateCollection(t.Context(), user, "Archive")

	a, err := svc.Update(t.Context(), user, id, setCollection(&reading.ID))
	if err != nil {
		t.Fatal(err)
	}
	if a.Collection == nil || *a.Collection != (library.CollectionRef{ID: reading.ID, Name: "Reading", Slug: "reading"}) {
		t.Errorf("after assigning: collection = %+v", a.Collection)
	}

	// Moving replaces the collection; tags are untouched.
	a, err = svc.Update(t.Context(), user, id, setCollection(&archive.ID))
	if err != nil {
		t.Fatal(err)
	}
	if a.Collection == nil || a.Collection.ID != archive.ID || len(a.Tags) != 2 {
		t.Errorf("after moving: collection = %+v, tags = %v", a.Collection, a.Tags)
	}
	got, _ := svc.Collections(t.Context(), user)
	if got[0].Articles != 1 || got[1].Articles != 0 {
		t.Errorf("counts after moving = %d (Archive), %d (Reading); want 1, 0", got[0].Articles, got[1].Articles)
	}

	a, err = svc.Update(t.Context(), user, id, setCollection(nil))
	if err != nil || a.Collection != nil {
		t.Errorf("after removing: collection = %+v, err %v", a.Collection, err)
	}

	// A title-only edit leaves the collection as it is.
	_, _ = svc.Update(t.Context(), user, id, setCollection(&reading.ID))
	title := "renamed"
	a, err = svc.Update(t.Context(), user, id, library.Patch{Title: &title})
	if err != nil || a.Collection == nil || a.Collection.ID != reading.ID {
		t.Errorf("after a title edit: collection = %+v, err %v", a.Collection, err)
	}
}

func TestCollections_AssigningAnUnknownCollectionIsRejected(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	user, other := newUser(t, pool), newUser(t, pool)
	id := seed(t, pool, user, seedArticle{url: "https://a.example/1", title: "one", created: base})[0]
	theirs, _ := svc.CreateCollection(t.Context(), other, "theirs")

	for _, cid := range []string{theirs.ID, "00000000-0000-0000-0000-00000000dead", "not-a-uuid"} {
		_, err := svc.Update(t.Context(), user, id, setCollection(&cid))
		if got := problemCodes(t, err); !slices.Equal(got, []string{"collectionId/not_found"}) {
			t.Errorf("collection %q: problems = %v", cid, got)
		}
	}
	a, _ := svc.Get(t.Context(), user, id)
	if a.Collection != nil {
		t.Errorf("a rejected edit changed the collection to %+v", a.Collection)
	}
}

func TestCollections_RenameAndDelete(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	user, other := newUser(t, pool), newUser(t, pool)
	id := seed(t, pool, user, seedArticle{url: "https://a.example/1", title: "one", created: base})[0]
	reading, _ := svc.CreateCollection(t.Context(), user, "Reading")
	_, _ = svc.CreateCollection(t.Context(), user, "Archive")
	_, _ = svc.Update(t.Context(), user, id, setCollection(&reading.ID))

	renamed, err := svc.RenameCollection(t.Context(), user, reading.ID, "To read")
	if err != nil || renamed.Name != "To read" || renamed.Slug != "to-read" || renamed.Articles != 1 {
		t.Errorf("renamed = %+v, err %v", renamed, err)
	}
	if _, err := svc.RenameCollection(t.Context(), user, reading.ID, "archive"); !errors.Is(err, library.ErrCollectionExists) {
		t.Errorf("rename onto an existing name: err = %v", err)
	}
	if _, err := svc.RenameCollection(t.Context(), other, reading.ID, "mine"); !errors.Is(err, library.ErrCollectionNotFound) {
		t.Errorf("another user's rename: err = %v", err)
	}
	if err := svc.DeleteCollection(t.Context(), other, reading.ID); !errors.Is(err, library.ErrCollectionNotFound) {
		t.Errorf("another user's delete: err = %v", err)
	}

	if err := svc.DeleteCollection(t.Context(), user, reading.ID); err != nil {
		t.Fatal(err)
	}
	a, err := svc.Get(t.Context(), user, id)
	if err != nil || a.Collection != nil {
		t.Errorf("after deleting the collection: article %+v, err %v; want it kept without a collection", a, err)
	}
	if err := svc.DeleteCollection(t.Context(), user, reading.ID); !errors.Is(err, library.ErrCollectionNotFound) {
		t.Errorf("second delete: err = %v", err)
	}
	if err := svc.DeleteCollection(t.Context(), user, "not-a-uuid"); !errors.Is(err, library.ErrCollectionNotFound) {
		t.Errorf("malformed id: err = %v", err)
	}
}

func TestList_FiltersByCollection(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	user := newUser(t, pool)
	ids := seed(t, pool, user,
		seedArticle{url: "https://a.example/1", title: "in reading", created: base, tags: []string{"go"}},
		seedArticle{url: "https://a.example/2", title: "also reading", created: base.Add(time.Hour)},
		seedArticle{url: "https://a.example/3", title: "loose", created: base.Add(2 * time.Hour), tags: []string{"go"}},
	)
	reading, _ := svc.CreateCollection(t.Context(), user, "Reading")
	for _, id := range ids[:2] {
		_, _ = svc.Update(t.Context(), user, id, setCollection(&reading.ID))
	}

	tests := []struct {
		params   library.ListParams
		expected []string
	}{
		{library.ListParams{Collection: "reading"}, []string{"also reading", "in reading"}},
		{library.ListParams{Collection: "Reading"}, []string{"also reading", "in reading"}},
		{library.ListParams{Collection: "reading", Tags: []string{"go"}}, []string{"in reading"}},
		{library.ListParams{Collection: "missing"}, []string{}},
		{library.ListParams{}, []string{"loose", "also reading", "in reading"}},
	}
	for _, tt := range tests {
		page, err := svc.List(t.Context(), user, tt.params)
		if err != nil {
			t.Fatalf("%+v: %v", tt.params, err)
		}
		if got := titles(page.Items); !slices.Equal(got, tt.expected) {
			t.Errorf("%+v = %v, want %v", tt.params, got, tt.expected)
		}
	}
}

func TestCollections_RenameChecksTheIDBeforeTheName(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)

	_, err := svc.RenameCollection(t.Context(), newUser(t, pool), "not-a-uuid", "   ")

	if !errors.Is(err, library.ErrCollectionNotFound) {
		t.Errorf("err = %v, want ErrCollectionNotFound", err)
	}
}

func TestList_ABlankCollectionFilterIsIgnored(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	user := newUser(t, pool)
	seed(t, pool, user, seedArticle{url: "https://a.example/1", title: "one", created: base})

	page, err := svc.List(t.Context(), user, library.ListParams{Collection: "   "})

	if err != nil || !slices.Equal(titles(page.Items), []string{"one"}) {
		t.Errorf("items = %v, err %v; want the whole library", titles(page.Items), err)
	}
}

func TestExport_IncludesEachArticlesCollection(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t)
	user := newUser(t, pool)
	ids := seed(t, pool, user,
		seedArticle{url: "https://a.example/1", title: "filed", created: base},
		seedArticle{url: "https://a.example/2", title: "loose", created: base.Add(time.Hour)},
	)
	reading, _ := svc.CreateCollection(t.Context(), user, "Reading")
	_, _ = svc.Update(t.Context(), user, ids[0], setCollection(&reading.ID))

	exp, err := svc.Export(t.Context(), user)

	if err != nil {
		t.Fatal(err)
	}
	if c := exp.Articles[0].Collection; c == nil || c.ID != reading.ID || c.Name != "Reading" {
		t.Errorf("filed article collection = %+v", c)
	}
	if exp.Articles[1].Collection != nil {
		t.Errorf("loose article collection = %+v, want nil", exp.Articles[1].Collection)
	}
}
