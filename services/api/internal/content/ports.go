package content

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Status says how much of an article the saved text holds.
type Status string

const (
	// StatusComplete is the whole text.
	StatusComplete Status = "complete"
	// StatusIncomplete is a paid preview: the page said it is not free.
	StatusIncomplete Status = "incomplete"
	// StatusFailed means the download failed and no text was saved yet.
	StatusFailed Status = "failed"
)

// Source says where the saved text came from.
type Source string

const (
	SourceFetched          Source = "fetched"
	SourceUploadedHTML     Source = "uploaded_html"
	SourceUploadedMarkdown Source = "uploaded_markdown"
)

// ImageInfo describes a stored image. SourceURL is the address the Markdown
// refers to.
type ImageInfo struct {
	ID        string `json:"id"`
	SourceURL string `json:"sourceUrl"`
}

// Content is the saved text of an article.
type Content struct {
	ArticleID string `json:"articleId"`
	Status    Status `json:"status"`
	Source    Source `json:"source"`
	Markdown  string `json:"markdown"`
	Words     int    `json:"words"`
	// Problem is the code of the last failed download, when there is one.
	Problem       *string     `json:"problem"`
	Images        []ImageInfo `json:"images"`
	MissingImages int         `json:"missingImages"`
	SavedAt       time.Time   `json:"savedAt"`
}

// Image is a stored image with its bytes.
type Image struct {
	ContentType string
	Data        []byte
}

// NewImage is a downloaded image ready to store.
type NewImage struct {
	SourceURL   string
	ContentType string
	Data        []byte
}

// NewContent is a text ready to store; it replaces the previous one.
type NewContent struct {
	Status        Status
	Source        Source
	Markdown      string
	Words         int
	Images        []NewImage
	MissingImages int
}

var (
	// ErrNotFound means the article (or image) does not exist for the user.
	ErrNotFound = errors.New("not found")
	// ErrNoContent means no text was saved for the article.
	ErrNoContent = errors.New("no content saved")
	// ErrTooLarge means an upload or a downloaded page is over the limit.
	ErrTooLarge = errors.New("content too large")
	// ErrNoText means the file or page holds no article text to save.
	ErrNoText = errors.New("no article text found")
)

// Repository stores article texts and images. Every method is scoped to the
// user; another user's article is ErrNotFound.
type Repository interface {
	// ArticleURL returns the original URL of the user's article.
	ArticleURL(ctx context.Context, userID, articleID string) (string, error)
	// Save replaces the article's text and images.
	Save(ctx context.Context, userID, articleID string, c NewContent) (Content, error)
	// SaveFailure records a failed download. Saved text is kept; without
	// one, a failed entry is stored so the library can show a warning.
	SaveFailure(ctx context.Context, userID, articleID, problem string) error
	Get(ctx context.Context, userID, articleID string) (Content, error)
	Image(ctx context.Context, userID, articleID, imageID string) (Image, error)
	// DownloadSummary counts the user's articles by the state of their text.
	DownloadSummary(ctx context.Context, userID string) (DownloadSummary, error)
	// QueueMissing queues the text download of every article of the user
	// that has no text and no recorded download and is not queued yet, and
	// returns how many it queued.
	QueueMissing(ctx context.Context, userID string) (int, error)
	// RemoveTexts deletes the saved text, images and recorded failure of
	// the user's articles, of one collection's when collectionID is not "",
	// drops their queued downloads, and returns how many entries it
	// deleted. An unknown collection is ErrNotFound.
	RemoveTexts(ctx context.Context, userID, collectionID string) (int, error)
}

// DownloadSummary counts a user's articles by the state of their text.
type DownloadSummary struct {
	// Queued are waiting for the background download.
	Queued int `json:"queued"`
	// Missing have no text, no recorded download and are not queued.
	Missing int `json:"missing"`
	// Failed have a recorded download failure and no text.
	Failed int `json:"failed"`
	// Incomplete have only a paid preview.
	Incomplete int `json:"incomplete"`
}

// Fetched is a downloaded resource.
type Fetched struct {
	// URL is the final address, after redirects.
	URL         string
	ContentType string
	Body        []byte
}

// Fetcher downloads public web resources. accept lists the media types
// allowed (a type ending in "/*" allows its family).
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string, maxBytes int64, accept []string) (Fetched, error)
}

// Download problem codes, stable for clients.
const (
	ProblemBlockedAddress = "blocked_address"
	ProblemUnreachable    = "unreachable"
	ProblemTimeout        = "timeout"
	ProblemHTTPStatus     = "http_status"
	ProblemNotHTML        = "not_html"
	ProblemTooLarge       = "too_large"
	ProblemNoText         = "no_text"
	// ProblemNoCopy: no other copy of the article holds its whole text.
	ProblemNoCopy = "no_copy"
)

// FetchError is a failed download, with a stable problem code.
type FetchError struct {
	Problem string
	// Status is the HTTP status when Problem is ProblemHTTPStatus.
	Status int
	Err    error
}

func (e *FetchError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("download failed: %s %d", e.Problem, e.Status)
	}
	if e.Err != nil {
		return fmt.Sprintf("download failed: %s: %v", e.Problem, e.Err)
	}
	return "download failed: " + e.Problem
}

func (e *FetchError) Unwrap() error { return e.Err }

// DownloadJob is one article waiting for its text to be downloaded.
type DownloadJob struct {
	UserID    string
	ArticleID string
}

// DownloadQueue hands out queued text downloads to the Worker.
type DownloadQueue interface {
	// Claim takes the next queued download for lease. A claim that runs
	// out before Done is handed out again, a few times at most. ok is false
	// when nothing is waiting.
	Claim(ctx context.Context, lease time.Duration) (job DownloadJob, ok bool, err error)
	// Done removes the download from the queue.
	Done(ctx context.Context, job DownloadJob) error
}
