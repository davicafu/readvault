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
	// maxPDFBytes matches the database constraint. A PDF has its own limit,
	// larger than a page's: it holds its images.
	maxPDFBytes = 50 << 20

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
	mirrorServes func(ctx context.Context, rawURL string) bool
	archive      string
}

// Option configures a Service.
type Option func(*Service)

// WithMirror makes Fetch fall back to a reading mirror when the original page
// fails or is a paid preview. The mirror address is prefix followed by the
// original URL, as in https://freedium-mirror.cfd/https://medium.com/....
// serves says which articles the mirror can read (Freedium reads Medium and
// some news sites); others are never sent to it. An empty prefix means no mirror.
func WithMirror(prefix string, serves func(ctx context.Context, rawURL string) bool) Option {
	return func(s *Service) {
		s.mirror = prefix
		s.mirrorServes = serves
	}
}

// WithArchive makes Fetch fall back to an archived copy of the page, after
// the mirror, for any site. The archive address is prefix followed by the
// original URL, as in https://web.archive.org/web/2id_/https://example.com/...
// An empty prefix means no archive.
func WithArchive(prefix string) Option {
	return func(s *Service) { s.archive = prefix }
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

// PDF returns the article's saved PDF, named after the file in its address.
func (s *Service) PDF(ctx context.Context, userID, articleID string) (PDF, error) {
	if !uuidPattern.MatchString(articleID) {
		return PDF{}, ErrNotFound
	}
	rawURL, err := s.repo.ArticleURL(ctx, userID, articleID)
	if err != nil {
		return PDF{}, err
	}
	data, err := s.repo.PDF(ctx, userID, articleID)
	if err != nil {
		return PDF{}, err
	}
	name := pdfName(rawURL)
	if name == "" {
		name = "article"
	}
	return PDF{Name: name + ".pdf", Data: data}, nil
}

// pdfName is the name of the file an address points to, without its .pdf
// extension, or "" when it names no file.
func pdfName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	name := path.Base(u.Path)
	if name == "/" || name == "." {
		return ""
	}
	if ext := path.Ext(name); strings.EqualFold(ext, ".pdf") {
		name = name[:len(name)-len(ext)]
	}
	return strings.TrimSpace(name)
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

// Fetch downloads the article from its original URL and saves its text. An
// address that serves a PDF has the PDF saved as it is. A
// page that says it is a paid preview is saved as incomplete. When the page
// fails or is a paid preview, another copy is saved instead if one holds the
// whole text: the mirror's for the pages it serves, then the archive's. A failed download keeps any saved
// text, is recorded and is returned as *FetchError.
func (s *Service) Fetch(ctx context.Context, userID, articleID string) (Content, error) {
	if !uuidPattern.MatchString(articleID) {
		return Content{}, ErrNotFound
	}
	rawURL, err := s.repo.ArticleURL(ctx, userID, articleID)
	if err != nil {
		return Content{}, err
	}

	ex, fe := s.download(ctx, rawURL, nil)
	if s.mayAskCopies(ctx, rawURL, ex, fe) {
		if cex, ok := s.fromCopies(ctx, rawURL); ok {
			ex, fe = cex, nil
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

// FetchCopy saves the text of another copy of the article, the mirror's or
// the archive's, without asking the original site: for a text that came out
// cut short. When no copy holds the whole text it returns a *FetchError
// with ProblemNoCopy and keeps whatever was saved, recording nothing.
func (s *Service) FetchCopy(ctx context.Context, userID, articleID string) (Content, error) {
	if !uuidPattern.MatchString(articleID) {
		return Content{}, ErrNotFound
	}
	rawURL, err := s.repo.ArticleURL(ctx, userID, articleID)
	if err != nil {
		return Content{}, err
	}
	if s.isCopyAddress(rawURL) {
		return Content{}, &FetchError{Problem: ProblemNoCopy}
	}
	ex, ok := s.fromCopies(ctx, rawURL)
	if !ok {
		return Content{}, &FetchError{Problem: ProblemNoCopy}
	}
	return s.save(ctx, userID, articleID, ex, StatusComplete, SourceFetched)
}

// DownloadSummary counts the user's articles by the state of their text.
func (s *Service) DownloadSummary(ctx context.Context, userID string) (DownloadSummary, error) {
	return s.repo.DownloadSummary(ctx, userID)
}

// QueueMissing queues the background download of every article of the user
// never downloaded. Failed and incomplete ones are left to the person, from
// the article's page, so a site that refused is not asked again unasked.
func (s *Service) QueueMissing(ctx context.Context, userID string) (int, error) {
	return s.repo.QueueMissing(ctx, userID)
}

// RemoveTexts deletes the saved texts of the user's articles, or of one
// collection's when collectionID is not "", to free space. The articles,
// their tags and collections stay; their queued downloads are dropped so
// the texts are not downloaded again behind the user's back. It returns how
// many saved entries (texts and recorded failures) were deleted.
//
// A download already running when the texts are removed may still save its
// text afterwards.
func (s *Service) RemoveTexts(ctx context.Context, userID, collectionID string) (int, error) {
	if collectionID != "" && !uuidPattern.MatchString(collectionID) {
		return 0, ErrNotFound
	}
	return s.repo.RemoveTexts(ctx, userID, collectionID)
}

// RemoveText deletes the saved text of one article, with its images and
// recorded failure, and drops its queued download; the article stays. It
// returns how many saved entries were deleted (0 or 1). An unknown article
// is ErrNotFound.
func (s *Service) RemoveText(ctx context.Context, userID, articleID string) (int, error) {
	if !uuidPattern.MatchString(articleID) {
		return 0, ErrNotFound
	}
	return s.repo.RemoveText(ctx, userID, articleID)
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

// mayAskCopies says whether another copy may help: the download was not
// cancelled and the site answered without the article (an HTTP error such
// as 403, a page without text, or a paid preview). An address that did not
// resolve or answer, or that the fetcher refused, may be a private or
// intranet host and never leaves the machine. An address that is already a
// copy is not sent for a copy again.
func (s *Service) mayAskCopies(ctx context.Context, rawURL string, ex Extracted, fe *FetchError) bool {
	switch {
	case ctx.Err() != nil || s.isCopyAddress(rawURL):
		return false
	case fe != nil && fe.Problem != ProblemHTTPStatus && fe.Problem != ProblemNoText:
		return false
	case fe == nil && !ex.Paywalled:
		return false
	}
	return true
}

// fromCopies downloads the whole text from another copy: the mirror's, for
// the pages it serves, then the archive's. A copy that is a paid preview too
// does not count.
func (s *Service) fromCopies(ctx context.Context, rawURL string) (Extracted, bool) {
	if s.mirror != "" && (s.mirrorServes == nil || s.mirrorServes(ctx, rawURL)) {
		if ex, fe := s.download(ctx, s.mirror+rawURL, nil); fe == nil && !ex.Paywalled && !ex.mirrorError {
			ex.Title = withoutSiteName(ex.Title, s.mirror)
			return ex, true
		}
	}
	if s.archive != "" && ctx.Err() == nil {
		// The archived page is the original as it was: its relative links
		// and images belong to the original site.
		base, _ := url.Parse(rawURL)
		if ex, fe := s.download(ctx, s.archive+rawURL, base); fe == nil && !ex.Paywalled {
			return ex, true
		}
	}
	return Extracted{}, false
}

// withoutSiteName drops a site's name that a page appends to its title
// ("Queues - Freedium"): the last " - " or " | " part when it is the site's
// name, the first word of its host (freedium-mirror.cfd gives "freedium"),
// compared without case.
func withoutSiteName(title, siteURL string) string {
	u, err := url.Parse(siteURL)
	if err != nil {
		return title
	}
	i := max(strings.LastIndex(title, " - "), strings.LastIndex(title, " | "))
	if i <= 0 {
		return title
	}
	name := strings.TrimSpace(title[i+3:])
	site, _, _ := strings.Cut(strings.TrimPrefix(u.Hostname(), "www."), ".")
	site, _, _ = strings.Cut(site, "-")
	if site != "" && strings.EqualFold(name, site) {
		return strings.TrimSpace(title[:i])
	}
	return title
}

// mirrorErrorData finds the error Freedium embeds in its page data when it
// cannot read an article (not found, render error, site blocked); it still
// answers 200 with a page of text. A readable article has error:null.
var mirrorErrorData = regexp.MustCompile(`\berror:\{status:\d{3},`)

func (s *Service) isCopyAddress(rawURL string) bool {
	return (s.mirror != "" && strings.HasPrefix(rawURL, s.mirror)) || (s.archive != "" && strings.HasPrefix(rawURL, s.archive))
}

// download fetches one page and extracts its text. Links and images are
// resolved against base, or against the page's own address when base is nil.
func (s *Service) download(ctx context.Context, rawURL string, base *url.URL) (Extracted, *FetchError) {
	pageCtx, cancel := context.WithTimeout(ctx, pageTimeout)
	page, err := s.fetcher.Fetch(pageCtx, rawURL, max(s.maxPageBytes, maxPDFBytes),
		[]string{"text/html", "application/xhtml+xml", "application/pdf", "application/octet-stream"})
	cancel()
	if err != nil {
		return Extracted{}, asFetchError(err)
	}
	// The bytes decide, not the server's header: a PDF is often sent as
	// application/octet-stream.
	if http.DetectContentType(page.Body) == "application/pdf" {
		return Extracted{Title: pdfName(rawURL), PDF: page.Body}, nil
	}
	if page.ContentType == "application/pdf" || page.ContentType == "application/octet-stream" {
		return Extracted{}, &FetchError{Problem: ProblemNotHTML, Err: fmt.Errorf("media type %q is not a PDF", page.ContentType)}
	}
	if int64(len(page.Body)) > s.maxPageBytes {
		return Extracted{}, &FetchError{Problem: ProblemTooLarge}
	}
	if base == nil {
		base, _ = url.Parse(page.URL)
	}
	ex, err := FromHTML(bytes.NewReader(cleanText(page.Body)), base)
	ex.mirrorError = mirrorErrorData.Match(page.Body)
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
	if ex.PDF != nil {
		return s.repo.Save(ctx, userID, articleID, NewContent{Title: ex.Title, Status: status, Source: source, PDF: ex.PDF})
	}
	images, missing := s.downloadImages(ctx, ex.Images)
	return s.repo.Save(ctx, userID, articleID, NewContent{
		Title: ex.Title, Status: status, Source: source, Markdown: ex.Markdown, Words: ex.Words,
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
