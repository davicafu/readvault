package http

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"readvault/api/internal/content"
)

// ContentService is the driving port the article text endpoints call.
type ContentService interface {
	Get(ctx context.Context, userID, articleID string) (content.Content, error)
	Upload(ctx context.Context, userID, articleID, fileName string, file io.Reader) (content.Content, error)
	Fetch(ctx context.Context, userID, articleID string) (content.Content, error)
	Image(ctx context.Context, userID, articleID, imageID string) (content.Image, error)
	DownloadSummary(ctx context.Context, userID string) (content.DownloadSummary, error)
	QueueMissing(ctx context.Context, userID string) (int, error)
}

var articleUpload = uploadKind{noun: "article page or Markdown file", tooLarge: "the file is larger than an article allows"}

type contentHandler struct {
	svc      ContentService
	ownerID  string
	maxBytes int64
	logger   *slog.Logger
}

// downloads serves GET /api/content-downloads: how many articles wait for
// their text, have none, failed or are incomplete.
func (h *contentHandler) downloads(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.DownloadSummary(r.Context(), h.ownerID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// queueMissing serves POST /api/content-downloads: queue the background
// download of every article never downloaded.
func (h *contentHandler) queueMissing(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.QueueMissing(r.Context(), h.ownerID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.logger.InfoContext(r.Context(), "missing texts queued", slog.Int("queued", n))
	writeJSON(w, http.StatusAccepted, map[string]int{"queued": n})
}

// get serves GET /api/articles/{id}/content.
func (h *contentHandler) get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Get(r.Context(), h.ownerID, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// upload serves POST /api/articles/{id}/content with an HTML or Markdown
// file in the "file" field of a multipart form.
func (h *contentHandler) upload(w http.ResponseWriter, r *http.Request) {
	file, name, ok := openUpload(w, r, h.maxBytes, articleUpload)
	if !ok {
		return
	}
	c, err := h.svc.Upload(r.Context(), h.ownerID, r.PathValue("id"), name, file)
	if err != nil {
		if file.err != nil {
			writeUploadError(w, file.err, h.maxBytes, articleUpload)
			return
		}
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// fetch serves POST /api/articles/{id}/content/fetch: download the article
// from its original URL.
func (h *contentHandler) fetch(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Fetch(r.Context(), h.ownerID, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// image serves GET /api/articles/{id}/images/{imageId}. Images are sniffed
// raster formats; the headers stop a browser from running anything in them.
func (h *contentHandler) image(w http.ResponseWriter, r *http.Request) {
	img, err := h.svc.Image(r.Context(), h.ownerID, r.PathValue("id"), r.PathValue("imageId"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", img.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(img.Data)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(img.Data)
}

func (h *contentHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fe *content.FetchError
	switch {
	case errors.Is(err, content.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "article not found")
	case errors.Is(err, content.ErrNoContent):
		writeError(w, http.StatusNotFound, "content_not_found", "no text has been saved for this article")
	case errors.Is(err, content.ErrNoText):
		writeError(w, http.StatusUnprocessableEntity, "no_text", "the file holds no article text")
	case errors.Is(err, content.ErrTooLarge):
		writeErrorDetails(w, http.StatusRequestEntityTooLarge, "file_too_large", articleUpload.tooLarge,
			map[string]int64{"maxBytes": h.maxBytes})
	case errors.As(err, &fe):
		details := map[string]any{"problem": fe.Problem}
		if fe.Status != 0 {
			details["status"] = fe.Status
		}
		h.logger.InfoContext(r.Context(), "article download failed", slog.String("problem", fe.Problem), slog.String("error", fe.Error()))
		writeErrorDetails(w, http.StatusBadGateway, "fetch_failed", "the article could not be downloaded", details)
	default:
		h.logger.ErrorContext(r.Context(), "content request failed", slog.String("path", r.URL.Path), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal", "the request could not be completed; retry later")
	}
}
