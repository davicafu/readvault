package library

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"readvault/api/internal/tag"
)

const (
	defaultLimit   = 25
	maxLimit       = 100
	maxSearchRunes = 200
	maxFilterTags  = 10
	maxArticleTags = 50
	// maxTitleRunes matches the articles.title check constraint.
	maxTitleRunes = 500
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ErrNotFound means the article does not exist for this user.
var ErrNotFound = errors.New("library: article not found")

// TextState is the state of an article's saved text, for filtering.
type TextState string

const (
	TextComplete   TextState = "complete"
	TextIncomplete TextState = "incomplete"
	TextFailed     TextState = "failed"
	// TextMissing means no text was saved and no download recorded.
	TextMissing TextState = "missing"
)

// ListParams is a list request as the client sent it.
type ListParams struct {
	Search     string
	Tags       []string
	Collection string
	// Text is complete, incomplete, failed or missing (no text saved and
	// no download recorded); empty keeps all.
	Text string
	Sort string
	// Limit is nil when the client sent none.
	Limit  *int
	Cursor string
}

// Page is one page of a list.
type Page struct {
	Items      []Article `json:"items"`
	NextCursor *string   `json:"nextCursor"`
}

// FieldProblem explains why one input field was rejected.
type FieldProblem struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidationError lists every rejected input field.
type ValidationError struct {
	Problems []FieldProblem
}

func (e *ValidationError) Error() string { return "library: invalid input" }

// Service reads the library.
type Service struct {
	repo Repository
}

// NewService returns a library service.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// ExportSchemaVersion is the version of the Export format. Bump it on any
// change that is not a new optional field.
const ExportSchemaVersion = 1

// Export is a full copy of a user's library.
type Export struct {
	SchemaVersion int       `json:"schemaVersion"`
	ExportedAt    time.Time `json:"exportedAt"`
	Articles      []Article `json:"articles"`
}

// Export returns every article of the user, oldest first.
func (s *Service) Export(ctx context.Context, userID string) (Export, error) {
	articles, err := s.repo.All(ctx, userID)
	if err != nil {
		return Export{}, fmt.Errorf("exporting articles: %w", err)
	}
	if articles == nil {
		articles = []Article{}
	}
	return Export{SchemaVersion: ExportSchemaVersion, ExportedAt: time.Now().UTC(), Articles: articles}, nil
}

// Tags lists the user's tags with the number of articles of each, most
// articles first. Tags without articles are left out.
func (s *Service) Tags(ctx context.Context, userID string) ([]TagCount, error) {
	tags, err := s.repo.Tags(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	if tags == nil {
		tags = []TagCount{}
	}
	return tags, nil
}

// Patch is an edit as the client sent it. Nil fields are left as they are.
type Patch struct {
	Title *string   `json:"title"`
	Tags  *[]string `json:"tags"`
	// CollectionID moves the article to a collection; null removes it from
	// its collection.
	CollectionID Nullable[string] `json:"collectionId"`
}

// Update edits the title and tags of one of the user's articles. The title
// is trimmed and its spaces collapsed; tags replace the whole set and follow
// the same rules as imported tags. The URLs are never changed.
func (s *Service) Update(ctx context.Context, userID, articleID string, p Patch) (Article, error) {
	c, err := parsePatch(p)
	if err != nil {
		return Article{}, err
	}
	if !uuidPattern.MatchString(articleID) {
		return Article{}, ErrNotFound
	}
	a, err := s.repo.Update(ctx, userID, articleID, c)
	if errors.Is(err, ErrCollectionNotFound) {
		return Article{}, &ValidationError{Problems: []FieldProblem{collectionNotFound}}
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Article{}, fmt.Errorf("updating article: %w", err)
	}
	return a, err
}

func parsePatch(p Patch) (Change, error) {
	if p.Title == nil && p.Tags == nil && !p.CollectionID.Set {
		return Change{}, &ValidationError{Problems: []FieldProblem{{"body", "empty", "send a title, tags, collectionId or several"}}}
	}
	var problems []FieldProblem
	var c Change

	if p.Title != nil {
		title := strings.Join(strings.Fields(*p.Title), " ")
		switch {
		case !validText(title):
			problems = append(problems, FieldProblem{"title", "invalid", "the title must be valid UTF-8 text"})
		case title == "":
			problems = append(problems, FieldProblem{"title", "required", "the title cannot be empty"})
		case utf8.RuneCountInString(title) > maxTitleRunes:
			problems = append(problems, FieldProblem{"title", "too_long", fmt.Sprintf("the title is longer than %d characters", maxTitleRunes)})
		default:
			c.Title = &title
		}
	}

	if p.Tags != nil {
		tags, problem := parseTags(*p.Tags)
		if problem != nil {
			problems = append(problems, *problem)
		}
		c.Tags = &tags
	}

	if p.CollectionID.Set {
		c.SetCollection = true
		if id := p.CollectionID.Value; id != nil {
			if !uuidPattern.MatchString(*id) {
				problems = append(problems, collectionNotFound)
			}
			c.CollectionID = id
		}
	}

	if problems != nil {
		return Change{}, &ValidationError{Problems: problems}
	}
	return c, nil
}

// parseTags normalizes a full tag set and drops tags that share a slug.
func parseTags(names []string) ([]tag.Tag, *FieldProblem) {
	if len(names) > maxArticleTags {
		return nil, &FieldProblem{"tags", "too_many", fmt.Sprintf("an article can have at most %d tags", maxArticleTags)}
	}
	if slices.ContainsFunc(names, func(n string) bool { return !validText(n) }) {
		return nil, &FieldProblem{"tags", "invalid", "tags must be valid UTF-8 text"}
	}
	tags := []tag.Tag{}
	for _, name := range names {
		t, err := tag.Normalize(name)
		switch {
		case errors.Is(err, tag.ErrEmpty):
			return nil, &FieldProblem{"tags", "empty", "a tag cannot be empty"}
		case errors.Is(err, tag.ErrTooLong):
			return nil, &FieldProblem{"tags", "too_long", fmt.Sprintf("a tag is longer than %d characters", tag.MaxRunes)}
		}
		if !slices.ContainsFunc(tags, func(seen tag.Tag) bool { return seen.Slug == t.Slug }) {
			tags = append(tags, t)
		}
	}
	return tags, nil
}

// List returns one page of the user's articles. A zero Limit means the
// default page size; an empty Sort means newest first.
func (s *Service) List(ctx context.Context, userID string, p ListParams) (Page, error) {
	q, err := parseQuery(p)
	if err != nil {
		return Page{}, err
	}
	rows, err := s.repo.List(ctx, userID, q)
	if err != nil {
		return Page{}, fmt.Errorf("listing articles: %w", err)
	}

	page := Page{Items: make([]Article, 0, min(len(rows), q.Limit))}
	for _, r := range rows[:min(len(rows), q.Limit)] {
		page.Items = append(page.Items, r.Article)
	}
	if len(rows) > q.Limit {
		last := rows[q.Limit-1]
		key := last.TitleKey
		if !q.Sort.ByTitle() {
			key = last.CreatedAt.UTC().Format(time.RFC3339Nano)
		}
		next := encodeCursor(cursor{Sort: q.Sort, Key: key, ID: last.ID})
		page.NextCursor = &next
	}
	return page, nil
}

// Get returns one of the user's articles. A malformed id is not found.
func (s *Service) Get(ctx context.Context, userID, articleID string) (Article, error) {
	if !uuidPattern.MatchString(articleID) {
		return Article{}, ErrNotFound
	}
	a, err := s.repo.Get(ctx, userID, articleID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Article{}, fmt.Errorf("getting article: %w", err)
	}
	return a, err
}

// Suggest suggests a collection and tags for one of the user's articles from
// its content, among the user's own collections and tags. See Suggest.
func (s *Service) Suggest(ctx context.Context, userID, articleID string) (Suggestions, error) {
	if !uuidPattern.MatchString(articleID) {
		return Suggestions{}, ErrNotFound
	}
	target, peers, err := s.repo.SuggestionData(ctx, userID, articleID)
	if errors.Is(err, ErrNotFound) {
		return Suggestions{}, err
	}
	if err != nil {
		return Suggestions{}, fmt.Errorf("loading suggestion data: %w", err)
	}
	counts, err := s.repo.Tags(ctx, userID)
	if err != nil {
		return Suggestions{}, fmt.Errorf("listing tags: %w", err)
	}
	collections, err := s.repo.Collections(ctx, userID)
	if err != nil {
		return Suggestions{}, fmt.Errorf("listing collections: %w", err)
	}

	in := SuggestInput{Target: target, Peers: peers}
	for _, t := range counts {
		in.Tags = append(in.Tags, t.Tag)
	}
	for _, c := range collections {
		in.Collections = append(in.Collections, CollectionRef{ID: c.ID, Name: c.Name, Slug: c.Slug})
	}
	return Suggest(in), nil
}

func parseQuery(p ListParams) (Query, error) {
	var problems []FieldProblem
	q := Query{Search: p.Search, Sort: Sort(p.Sort)}
	if p.Limit != nil {
		q.Limit = *p.Limit
	}

	if !validText(q.Search) {
		problems = append(problems, FieldProblem{"q", "invalid", "the search must be valid UTF-8 text"})
	}
	if slices.ContainsFunc(p.Tags, func(t string) bool { return !validText(t) }) {
		problems = append(problems, FieldProblem{"tag", "invalid", "tags must be valid UTF-8 text"})
	}

	if validText(q.Search) && utf8.RuneCountInString(q.Search) > maxSearchRunes {
		problems = append(problems, FieldProblem{"q", "too_long", fmt.Sprintf("the search is longer than %d characters", maxSearchRunes)})
	}

	if !validText(p.Collection) {
		problems = append(problems, FieldProblem{"collection", "invalid", "the collection must be valid UTF-8 text"})
	} else {
		q.CollectionSlug = tag.Slugify(p.Collection)
	}

	for _, name := range p.Tags {
		slug := tag.Slugify(name)
		if slug != "" && !slices.Contains(q.TagSlugs, slug) {
			q.TagSlugs = append(q.TagSlugs, slug)
		}
	}
	if len(q.TagSlugs) > maxFilterTags {
		problems = append(problems, FieldProblem{"tag", "too_many", fmt.Sprintf("filter by at most %d tags", maxFilterTags)})
	}

	switch TextState(p.Text) {
	case "", TextComplete, TextIncomplete, TextFailed, TextMissing:
		q.Text = TextState(p.Text)
	default:
		problems = append(problems, FieldProblem{"text", "invalid", "text must be complete, incomplete, failed or missing"})
	}

	switch q.Sort {
	case "":
		q.Sort = SortNewest
	case SortNewest, SortOldest, SortTitle, SortTitleDesc:
	default:
		problems = append(problems, FieldProblem{"sort", "invalid", "sort must be created_desc, created_asc, title_asc or title_desc"})
	}

	switch {
	case p.Limit == nil:
		q.Limit = defaultLimit
	case q.Limit < 1 || q.Limit > maxLimit:
		problems = append(problems, FieldProblem{"limit", "out_of_range", fmt.Sprintf("limit must be between 1 and %d", maxLimit)})
	}

	if p.Cursor != "" {
		c, ok := decodeCursor(p.Cursor)
		if !ok || c.Sort != q.Sort {
			problems = append(problems, FieldProblem{"cursor", "invalid", "the cursor is not valid for this list; start from the first page"})
		} else {
			q.After = &Position{Key: c.Key, ID: c.ID}
		}
	}

	if problems != nil {
		return Query{}, &ValidationError{Problems: problems}
	}
	return q, nil
}

// cursor marks where the next page starts. It is opaque to clients.
type cursor struct {
	Sort Sort   `json:"s"`
	Key  string `json:"k"`
	ID   string `json:"i"`
}

func encodeCursor(c cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(raw string) (cursor, bool) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return cursor{}, false
	}
	var c cursor
	if err := json.Unmarshal(b, &c); err != nil || !uuidPattern.MatchString(c.ID) {
		return cursor{}, false
	}
	if !c.Sort.ByTitle() {
		if _, err := time.Parse(time.RFC3339Nano, c.Key); err != nil {
			return cursor{}, false
		}
	}
	return c, true
}

// validText reports whether s can be stored as Postgres text: valid UTF-8
// without NUL bytes.
func validText(s string) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
