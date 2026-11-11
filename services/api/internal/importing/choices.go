package importing

import (
	"errors"
	"fmt"
	"strconv"

	"readvault/api/internal/tag"
)

// Choices are the person's decisions on a preview. The zero value keeps
// every suggestion.
type Choices struct {
	// Collections renames a suggested collection for all its bookmarks,
	// keyed by the suggested name; a nil value drops the suggestion.
	Collections map[string]*string
	// Bookmarks overrides single bookmarks, keyed by their position.
	Bookmarks map[int]BookmarkChoice
}

// BookmarkChoice overrides the collection or the tags of one bookmark.
type BookmarkChoice struct {
	// SetCollection replaces the collection with Collection; a nil
	// Collection means none.
	SetCollection bool
	Collection    *string
	// SetTags replaces the tags of the file with Tags.
	SetTags bool
	Tags    []string
}

// ChoiceError means a choice breaks the rules. Field names it, such as
// "collections.Go" or "bookmarks.12.tags".
type ChoiceError struct {
	Field   string
	Message string
}

func (e *ChoiceError) Error() string {
	return fmt.Sprintf("importing: invalid choice %s: %s", e.Field, e.Message)
}

// resolvedChoices are Choices checked against the file and normalized.
type resolvedChoices struct {
	// renames maps a suggested slug to its new collection; nil drops it.
	renames   map[string]*tag.Tag
	bookmarks map[int]resolvedBookmark
}

type resolvedBookmark struct {
	setCollection bool
	collection    *tag.Tag
	setTags       bool
	tags          []tag.Tag
}

// resolve checks the choices against the bookmarks that will be imported,
// before anything is written. A choice about a bookmark or a suggestion
// that is not in the file is an error, not silently ignored.
func (c Choices) resolve(items []analyzed) (resolvedChoices, error) {
	suggested := map[string]bool{}
	importable := map[int]bool{}
	for _, it := range items {
		if it.Status == StatusInvalid || it.Status == StatusDuplicateInFile {
			continue
		}
		importable[it.Position] = true
		if it.article.Collection != nil {
			suggested[it.article.Collection.Slug] = true
		}
	}

	r := resolvedChoices{renames: map[string]*tag.Tag{}, bookmarks: map[int]resolvedBookmark{}}
	for key, name := range c.Collections {
		field := "collections." + key
		k, err := tag.Normalize(key)
		if err != nil || !suggested[k.Slug] {
			return r, &ChoiceError{field, "no bookmark in the file has this suggested collection"}
		}
		if name == nil {
			r.renames[k.Slug] = nil
			continue
		}
		n, err := collectionName(*name, field)
		if err != nil {
			return r, err
		}
		r.renames[k.Slug] = &n
	}

	for pos, b := range c.Bookmarks {
		field := "bookmarks." + strconv.Itoa(pos)
		if !importable[pos] {
			return r, &ChoiceError{field, "there is no bookmark to import at this position"}
		}
		rb := resolvedBookmark{setCollection: b.SetCollection, setTags: b.SetTags}
		if b.SetCollection && b.Collection != nil {
			n, err := collectionName(*b.Collection, field+".collection")
			if err != nil {
				return r, err
			}
			rb.collection = &n
		}
		if b.SetTags {
			tags, problems := normalizeTags(b.Tags)
			if len(problems) > 0 {
				return r, &ChoiceError{field + ".tags", fmt.Sprintf("a tag is longer than %d characters", tag.MaxRunes)}
			}
			rb.tags = tags
		}
		r.bookmarks[pos] = rb
	}
	return r, nil
}

// apply sets the collection and tags the article will be saved with.
func (r resolvedChoices) apply(it *analyzed) {
	if c := it.article.Collection; c != nil {
		if renamed, ok := r.renames[c.Slug]; ok {
			it.article.Collection = renamed
		}
	}
	b, ok := r.bookmarks[it.Position]
	if !ok {
		return
	}
	if b.setCollection {
		it.article.Collection = b.collection
	}
	if b.setTags {
		it.article.Tags = b.tags
	}
}

func collectionName(name, field string) (tag.Tag, error) {
	n, err := tag.Normalize(name)
	switch {
	case errors.Is(err, tag.ErrEmpty):
		return tag.Tag{}, &ChoiceError{field, "a collection name cannot be empty; use null for no collection"}
	case err != nil:
		return tag.Tag{}, &ChoiceError{field, fmt.Sprintf("a collection name is at most %d characters", tag.MaxRunes)}
	}
	return n, nil
}
