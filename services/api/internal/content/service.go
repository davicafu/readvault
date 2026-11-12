package content

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Limits on what an article may hold. They bound memory per request and the
// size of the database.
const (
	// maxMarkdownBytes and maxImageURLBytes match the database constraints.
	maxMarkdownBytes   = 5 << 20
	maxImageURLBytes   = 4096
	maxImages          = 30
	maxImageBytes      = 5 << 20
	maxTotalImageBytes = 30 << 20
	imageWorkers       = 4

	pageTimeout   = 10 * time.Second
	imagesTimeout = 15 * time.Second
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// storableImageTypes are sniffed from the bytes, never taken from the
// server's header. SVG is left out: it can carry scripts.
var storableImageTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}

// Service saves the text and images of articles.
type Service struct {
	repo         Repository
	fetcher      Fetcher
	maxPageBytes int64
	mirror       string
}

// Option configures a Service.
type Option func(*Service)

// WithMirror makes Fetch fall back to a reading mirror when the original page
// fails or is a paid preview. The mirror address is prefix followed by the
// original URL, as in https://freedium-mirror.cfd/https://medium.com/....
// An empty prefix means no mirror.
func WithMirror(prefix string) Option {
	return func(s *Service) { s.mirror = prefix }
}

// NewService returns a Service. maxPageBytes bounds an uploaded file and a
// downloaded page.
func NewService(repo Repository, fetcher Fetcher, maxPageBytes int64, opts ...Option) *Service {
	s := &Service{repo: repo, fetcher: fetcher, maxPageBytes: maxPageBytes}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Get returns the saved text of an article.
func (s *Service) Get(ctx context.Context, userID, articleID string) (Content, error) {
	if !uuidPattern.MatchString(articleID) {
		return Content{}, ErrNotFound
	}
	return s.repo.Get(ctx, userID, articleID)
}

// Image returns a stored image of an article.
func (s *Service) Image(ctx context.Context, userID, articleID, imageID string) (Image, error) {
	if !uuidPattern.MatchString(articleID) || !uuidPattern.MatchString(imageID) {
		return Image{}, ErrNotFound
	}
	return s.repo.Image(ctx, userID, articleID, imageID)
}

// Upload saves the text of a file the user chose: HTML is converted to
// Markdown, Markdown is kept as is. The user picked the file, so the text
// counts as complete. Body images are downloaded and stored.
func (s *Service) Upload(ctx context.Context, userID, articleID, fileName string, file io.Reader) (Content, error) {
	if !uuidPattern.MatchString(articleID) {
		return Content{}, ErrNotFound
	}
	base, err := s.articleBase(ctx, userID, articleID)
	if err != nil {
		return Content{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, s.maxPageBytes+1))
	if err != nil {
		return Content{}, fmt.Errorf("reading upload: %w", err)
	}
	if int64(len(data)) > s.maxPageBytes {
		return Content{}, ErrTooLarge
	}
	data = cleanText(data)

	var ex Extracted
	source := SourceUploadedMarkdown
	if isMarkdownFile(fileName, data) {
		ex = FromMarkdown(string(data), base)
	} else {
		source = SourceUploadedHTML
		if ex, err = FromHTML(bytes.NewReader(data), base); err != nil {
			return Content{}, err
		}
	}
	if ex.Words == 0 {
		return Content{}, ErrNoText
	}
	// Repaired text can be larger than the file: each invalid byte becomes
	// a three-byte U+FFFD.
	if len(ex.Markdown) > maxMarkdownBytes {
		return Content{}, ErrTooLarge
	}
	return s.save(ctx, userID, articleID, ex, StatusComplete, source)
}

// Fetch downloads the article from its original URL and saves its text. A
// page that says it is a paid preview is saved as incomplete. When the page
// fails or is a paid preview and a mirror is configured, the mirror's copy is
// saved instead if it holds the whole text. A failed download keeps any saved
// text, is recorded and is returned as *FetchError.
func (s *Service) Fetch(ctx context.Context, userID, articleID string) (Content, error) {
	if !uuidPattern.MatchString(articleID) {
		return Content{}, ErrNotFound
	}
	rawURL, err := s.repo.ArticleURL(ctx, userID, articleID)
	if err != nil {
		return Content{}, err
	}

	ex, fe := s.download(ctx, rawURL)
	if s.useMirror(ctx, rawURL, ex, fe) {
		if mex, mfe := s.download(ctx, s.mirror+rawURL); mfe == nil && !mex.Paywalled {
			ex, fe = mex, nil
		}
	}
	if fe != nil {
		if serr := s.repo.SaveFailure(ctx, userID, articleID, fe.Problem); serr != nil {
			return Content{}, serr
		}
		return Content{}, fe
	}

	status := StatusComplete
	if ex.Paywalled {
		status = StatusIncomplete
	}
	return s.save(ctx, userID, articleID, ex, status, SourceFetched)
}

// downloadMissing fetches the article's text unless something was saved
// or a download recorded already, by the person or an earlier download.
func (s *Service) downloadMissing(ctx context.Context, userID, articleID string) error {
	_, err := s.repo.Get(ctx, userID, articleID)
	switch {
	case err == nil:
		return nil
	case !errors.Is(err, ErrNoContent):
		return err
	}
	_, err = s.Fetch(ctx, userID, articleID)
	return err
}

// useMirror says whether the mirror may help: a mirror is configured, the
// download was not cancelled, and the site answered without the article (an
// HTTP error such as 403, a page without text, or a paid preview). An
// address that did not resolve or answer, or that the fetcher refused, may
// be a private or intranet host and never leaves the machine. An address
// already on the mirror is not sent to it again.
func (s *Service) useMirror(ctx context.Context, rawURL string, ex Extracted, fe *FetchError) bool {
	switch {
	case s.mirror == "" || ctx.Err() != nil || strings.HasPrefix(rawURL, s.mirror):
		return false
	case fe != nil:
		return fe.Problem == ProblemHTTPStatus || fe.Problem == ProblemNoText
	default:
		return ex.Paywalled
	}
}

// download fetches one page and extracts its text.
func (s *Service) download(ctx context.Context, rawURL string) (Extracted, *FetchError) {
	pageCtx, cancel := context.WithTimeout(ctx, pageTimeout)
	page, err := s.fetcher.Fetch(pageCtx, rawURL, s.maxPageBytes, []string{"text/html", "application/xhtml+xml"})
	cancel()
	if err != nil {
		return Extracted{}, asFetchError(err)
	}
	base, _ := url.Parse(page.URL)
	ex, err := FromHTML(bytes.NewReader(cleanText(page.Body)), base)
	switch {
	case err != nil:
		return Extracted{}, &FetchError{Problem: ProblemNotHTML, Err: err}
	case ex.Words == 0:
		return Extracted{}, &FetchError{Problem: ProblemNoText}
	case len(ex.Markdown) > maxMarkdownBytes:
		return Extracted{}, &FetchError{Problem: ProblemTooLarge}
	}
	return ex, nil
}

// asFetchError keeps a *FetchError and classifies any other download error,
// so a failure is never reported as a problem it is not.
func asFetchError(err error) *FetchError {
	var fe *FetchError
	switch {
	case errors.As(err, &fe):
		return fe
	case errors.Is(err, context.DeadlineExceeded):
		return &FetchError{Problem: ProblemTimeout, Err: err}
	default:
		return &FetchError{Problem: ProblemUnreachable, Err: err}
	}
}

func (s *Service) articleBase(ctx context.Context, userID, articleID string) (*url.URL, error) {
	rawURL, err := s.repo.ArticleURL(ctx, userID, articleID)
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil
	}
	return base, nil
}

func (s *Service) save(ctx context.Context, userID, articleID string, ex Extracted, status Status, source Source) (Content, error) {
	images, missing := s.downloadImages(ctx, ex.Images)
	return s.repo.Save(ctx, userID, articleID, NewContent{
		Status: status, Source: source, Markdown: ex.Markdown, Words: ex.Words,
		Images: images, MissingImages: missing,
	})
}

// downloadImages fetches up to maxImages body images with a few workers and
// keeps those whose bytes are a raster image, within maxTotalImageBytes in
// all: memory holds at most the kept images plus one download per worker.
// It returns them in text order and how many could not be kept. It never
// fails the save.
func (s *Service) downloadImages(ctx context.Context, refs []ImageRef) ([]NewImage, int) {
	missing := 0
	var usable []ImageRef
	for _, ref := range refs {
		if len(ref.URL) > maxImageURLBytes || len(usable) == maxImages {
			missing++
			continue
		}
		usable = append(usable, ref)
	}
	ctx, cancel := context.WithTimeout(ctx, imagesTimeout)
	defer cancel()

	results := make([]*NewImage, len(usable))
	var mu sync.Mutex
	total := 0
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(imageWorkers, len(usable)) {
		wg.Go(func() {
			for i := range jobs {
				got, err := s.fetcher.Fetch(ctx, usable[i].URL, maxImageBytes, []string{"image/*", "application/octet-stream", "binary/octet-stream"})
				if err != nil {
					continue
				}
				ct := http.DetectContentType(got.Body)
				if !storableImageTypes[ct] {
					continue
				}
				mu.Lock()
				if total+len(got.Body) <= maxTotalImageBytes {
					total += len(got.Body)
					results[i] = &NewImage{SourceURL: usable[i].URL, ContentType: ct, Data: got.Body}
				}
				mu.Unlock()
			}
		})
	}
	for i := range usable {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	var images []NewImage
	for _, img := range results {
		if img == nil {
			missing++
			continue
		}
		images = append(images, *img)
	}
	return images, missing
}

// isMarkdownFile decides by the file name, then by whether the bytes look
// like an HTML document.
func isMarkdownFile(name string, data []byte) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".markdown", ".txt":
		return true
	case ".html", ".htm", ".xhtml":
		return false
	}
	head := strings.ToLower(string(data[:min(len(data), 2048)]))
	for _, tag := range []string{"<!doctype html", "<html", "<body", "<article", "<p>", "<div"} {
		if strings.Contains(head, tag) {
			return false
		}
	}
	return true
}

// cleanText makes text storable in Postgres: invalid UTF-8 becomes U+FFFD
// and NUL bytes are dropped.
func cleanText(b []byte) []byte {
	if utf8.Valid(b) && bytes.IndexByte(b, 0) < 0 {
		return b
	}
	return bytes.ReplaceAll(bytes.ToValidUTF8(b, []byte("�")), []byte{0}, nil)
}
