package library

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"readvault/api/internal/tag"
)

var (
	// ErrCollectionNotFound means the user has no collection with that id.
	ErrCollectionNotFound = errors.New("library: collection not found")
	// ErrCollectionExists means the user has a collection with the same
	// slug.
	ErrCollectionExists = errors.New("library: collection already exists")
)

var collectionNotFound = FieldProblem{"collectionId", "not_found", "there is no collection with that id"}

// Nullable is a JSON field that can be absent, null or a value. Set is
// false when the field was absent; Value is nil when it was null.
type Nullable[T any] struct {
	Set   bool
	Value *T
}

// UnmarshalJSON implements json.Unmarshaler. It runs only when the field is
// present, including when it is null.
func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		n.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}

// Collections lists the user's collections by name, with how many articles
// each holds.
func (s *Service) Collections(ctx context.Context, userID string) ([]Collection, error) {
	cs, err := s.repo.Collections(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing collections: %w", err)
	}
	if cs == nil {
		cs = []Collection{}
	}
	return cs, nil
}

// CreateCollection adds a collection. Names follow the tag rules: spaces
// are collapsed, up to 64 characters, and names that share a slug ("Go
// News", "go news") are the same collection.
func (s *Service) CreateCollection(ctx context.Context, userID, name string) (Collection, error) {
	n, err := parseCollectionName(name)
	if err != nil {
		return Collection{}, err
	}
	c, err := s.repo.CreateCollection(ctx, userID, n)
	if err != nil && !errors.Is(err, ErrCollectionExists) {
		return Collection{}, fmt.Errorf("creating collection: %w", err)
	}
	return c, err
}

// RenameCollection renames one of the user's collections.
func (s *Service) RenameCollection(ctx context.Context, userID, collectionID, name string) (Collection, error) {
	if !uuidPattern.MatchString(collectionID) {
		return Collection{}, ErrCollectionNotFound
	}
	n, err := parseCollectionName(name)
	if err != nil {
		return Collection{}, err
	}
	c, err := s.repo.RenameCollection(ctx, userID, collectionID, n)
	if err != nil && !errors.Is(err, ErrCollectionNotFound) && !errors.Is(err, ErrCollectionExists) {
		return Collection{}, fmt.Errorf("renaming collection: %w", err)
	}
	return c, err
}

// DeleteCollection deletes one of the user's collections. Its articles stay
// in the library, without a collection.
func (s *Service) DeleteCollection(ctx context.Context, userID, collectionID string) error {
	if !uuidPattern.MatchString(collectionID) {
		return ErrCollectionNotFound
	}
	err := s.repo.DeleteCollection(ctx, userID, collectionID)
	if err != nil && !errors.Is(err, ErrCollectionNotFound) {
		return fmt.Errorf("deleting collection: %w", err)
	}
	return err
}

func parseCollectionName(name string) (tag.Tag, error) {
	if !validText(name) {
		return tag.Tag{}, &ValidationError{Problems: []FieldProblem{{"name", "invalid", "the name must be valid UTF-8 text"}}}
	}
	n, err := tag.Normalize(name)
	switch {
	case errors.Is(err, tag.ErrEmpty):
		return tag.Tag{}, &ValidationError{Problems: []FieldProblem{{"name", "required", "the name cannot be empty"}}}
	case errors.Is(err, tag.ErrTooLong):
		return tag.Tag{}, &ValidationError{Problems: []FieldProblem{{"name", "too_long", fmt.Sprintf("the name is longer than %d characters", tag.MaxRunes)}}}
	case err != nil:
		return tag.Tag{}, err
	}
	return n, nil
}
