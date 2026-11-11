package importing

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"
)

// saveBatchSize bounds how many articles share one transaction, so a failure
// only affects its batch and long imports do not hold one huge transaction.
const saveBatchSize = 500

// Status classifies a candidate in a preview.
type Status string

const (
	StatusValid           Status = "valid"
	StatusInvalid         Status = "invalid"
	StatusDuplicateInFile Status = "duplicate_in_file"
	StatusExisting        Status = "existing"
)

// Problem is a per-record warning or error. Code is stable for clients.
type Problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Candidate is one record of the uploaded file after validation.
type Candidate struct {
	Position     int       `json:"position"`
	Status       Status    `json:"status"`
	OriginalURL  string    `json:"originalUrl"`
	CanonicalURL string    `json:"canonicalUrl,omitempty"`
	Title        string    `json:"title"`
	Folder       []string  `json:"folder"`
	Collection   *string   `json:"collection"`
	Tags         []string  `json:"tags"`
	Errors       []Problem `json:"errors"`
	Warnings     []Problem `json:"warnings"`
}

// PreviewTotals summarizes a preview. Valid counts new, importable records;
// records already in the library count as Existing instead.
type PreviewTotals struct {
	Detected        int `json:"detected"`
	Valid           int `json:"valid"`
	Invalid         int `json:"invalid"`
	DuplicateInFile int `json:"duplicateInFile"`
	Existing        int `json:"existing"`
}

// SuggestedCollection is a collection the bookmarks of a file suggest, with
// how many bookmarks to import would go in it and whether the user already
// has it.
type SuggestedCollection struct {
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Bookmarks int    `json:"bookmarks"`
	Existing  bool   `json:"existing"`
}

// Preview is what an import would do, without writing anything.
type Preview struct {
	Totals      PreviewTotals         `json:"totals"`
	Collections []SuggestedCollection `json:"collections"`
	Candidates  []Candidate           `json:"candidates"`
}

// ReportTotals counts what an import did.
type ReportTotals struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
	Skipped  int `json:"skipped"`
	Failed   int `json:"failed"`
}

// ReportItem explains a record that was not inserted.
type ReportItem struct {
	Position    int       `json:"position"`
	OriginalURL string    `json:"originalUrl"`
	Outcome     Outcome   `json:"outcome"`
	Problems    []Problem `json:"problems,omitempty"`
}

// Report is the result of an executed import.
type Report struct {
	ImportID string       `json:"importId"`
	Totals   ReportTotals `json:"totals"`
	Items    []ReportItem `json:"items"`
}

// Service previews and executes bookmark imports.
type Service struct {
	repo       Repository
	maxRecords int
	logger     *slog.Logger
}

// NewService returns an importing service. maxRecords bounds every file.
func NewService(repo Repository, maxRecords int) *Service {
	return &Service{repo: repo, maxRecords: maxRecords, logger: slog.Default()}
}

// Preview parses and validates a bookmark HTML file without writing anything.
func (s *Service) Preview(ctx context.Context, userID string, file io.Reader) (Preview, error) {
	records, err := ParseHTML(file, s.maxRecords)
	if err != nil {
		return Preview{}, err
	}
	items := analyze(records)

	var urls []string
	for _, it := range items {
		if it.Status == StatusValid {
			urls = append(urls, it.CanonicalURL)
		}
	}
	existing, err := s.repo.ExistingCanonicalURLs(ctx, userID, urls)
	if err != nil {
		return Preview{}, fmt.Errorf("checking existing articles: %w", err)
	}

	p := Preview{Collections: []SuggestedCollection{}, Candidates: make([]Candidate, 0, len(items))}
	suggested := map[string]int{}
	p.Totals.Detected = len(items)
	for _, it := range items {
		c := it.Candidate
		if c.Status == StatusValid && existing[c.CanonicalURL] {
			c.Status = StatusExisting
		}
		switch c.Status {
		case StatusValid:
			p.Totals.Valid++
		case StatusInvalid:
			p.Totals.Invalid++
		case StatusDuplicateInFile:
			p.Totals.DuplicateInFile++
		case StatusExisting:
			p.Totals.Existing++
		}
		p.Candidates = append(p.Candidates, c)

		col := it.article.Collection
		if col == nil || c.Status == StatusInvalid || c.Status == StatusDuplicateInFile {
			continue
		}
		if i, ok := suggested[col.Slug]; ok {
			p.Collections[i].Bookmarks++
			continue
		}
		suggested[col.Slug] = len(p.Collections)
		p.Collections = append(p.Collections, SuggestedCollection{Name: col.Name, Slug: col.Slug, Bookmarks: 1})
	}

	slugs := make([]string, len(p.Collections))
	for i, c := range p.Collections {
		slugs[i] = c.Slug
	}
	have, err := s.repo.ExistingCollections(ctx, userID, slugs)
	if err != nil {
		return Preview{}, fmt.Errorf("checking existing collections: %w", err)
	}
	for i := range p.Collections {
		p.Collections[i].Existing = have[p.Collections[i].Slug]
	}
	slices.SortFunc(p.Collections, func(a, b SuggestedCollection) int { return strings.Compare(a.Slug, b.Slug) })
	return p, nil
}

// Import persists the valid, unique records of a bookmark HTML file with
// their suggested collection and tags, as changed by choices. It is
// idempotent: re-importing the same file creates nothing new. Invalid
// choices return a *ChoiceError before anything is written. A failing batch
// is reported as failed without discarding the batches that succeeded.
func (s *Service) Import(ctx context.Context, userID string, choices Choices, file io.Reader) (Report, error) {
	records, err := ParseHTML(file, s.maxRecords)
	if err != nil {
		return Report{}, err
	}
	items := analyze(records)
	resolved, err := choices.resolve(items)
	if err != nil {
		return Report{}, err
	}
	for i := range items {
		resolved.apply(&items[i])
	}

	report := Report{ImportID: newID(), Items: []ReportItem{}}
	logger := s.logger.With(slog.String("import_id", report.ImportID))
	start := time.Now()

	var pending []analyzed
	for _, it := range items {
		switch it.Status {
		case StatusInvalid:
			report.add(it.Candidate, OutcomeFailed, it.Errors)
		case StatusDuplicateInFile:
			report.add(it.Candidate, OutcomeSkipped, it.Warnings)
		default:
			pending = append(pending, it)
		}
	}

	for batch := range slices.Chunk(pending, saveBatchSize) {
		articles := make([]NewArticle, len(batch))
		for i, it := range batch {
			articles[i] = it.article
		}

		outcomes, err := s.repo.Save(ctx, userID, articles)
		if err != nil {
			if ctx.Err() != nil {
				return Report{}, fmt.Errorf("import %s interrupted: %w", report.ImportID, ctx.Err())
			}
			logger.ErrorContext(ctx, "import batch failed", slog.Int("batch_size", len(batch)), slog.String("error", err.Error()))
			for _, it := range batch {
				report.add(it.Candidate, OutcomeFailed, []Problem{{Code: "storage_error", Message: "the bookmark could not be saved; retry the import"}})
			}
			continue
		}
		for i, it := range batch {
			report.add(it.Candidate, outcomes[i], nil)
		}
	}

	logger.InfoContext(ctx, "import finished",
		slog.Int("records", len(items)),
		slog.Int("inserted", report.Totals.Inserted),
		slog.Int("updated", report.Totals.Updated),
		slog.Int("skipped", report.Totals.Skipped),
		slog.Int("failed", report.Totals.Failed),
		slog.Duration("duration", time.Since(start)),
	)
	return report, nil
}

func (r *Report) add(c Candidate, outcome Outcome, problems []Problem) {
	switch outcome {
	case OutcomeInserted:
		r.Totals.Inserted++
		return
	case OutcomeUpdated:
		r.Totals.Updated++
	case OutcomeSkipped:
		r.Totals.Skipped++
	case OutcomeFailed:
		r.Totals.Failed++
	}
	r.Items = append(r.Items, ReportItem{Position: c.Position, OriginalURL: c.OriginalURL, Outcome: outcome, Problems: problems})
}

// newID returns a random UUID (version 4) identifying one import in logs and reports.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
