// Package http is the content module's driving adapter: article texts,
// their images and the background downloads.
package http

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"readvault/api/internal/content"

	"readvault/api/internal/platform/httpapi"
)

// Service is the driving port the article text endpoints call.
type Service interface {
	Get(ctx context.Context, userID, articleID string) (content.Content, error)
	Upload(ctx context.Context, userID, articleID, fileName string, file io.Reader) (content.Content, error)
	Fetch(ctx context.Context, userID, articleID string) (content.Content, error)
	FetchCopy(ctx context.Context, userID, articleID string) (content.Content, error)
	Image(ctx context.Context, userID, articleID, imageID string) (content.Image, error)
	DownloadSummary(ctx context.Context, userID string) (content.DownloadSummary, error)
	QueueMissing(ctx context.Context, userID string) (int, error)
	RemoveTexts(ctx context.Context, userID, collectionID string) (int, error)
	RemoveText(ctx context.Context, userID, articleID string) (int, error)
}

var articleUpload = httpapi.UploadKind{Noun: "article page or Markdown file", TooLarge: "the file is larger than an article allows"}

type Handler struct {
	svc      Service
	ownerID  string
	maxBytes int64
	logger   *slog.Logger
}

// New returns the article text endpoints, acting for ownerID. maxBytes
// bounds an uploaded article page or Markdown file.
func New(svc Service, ownerID string, maxBytes int64, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Handler{svc: svc, ownerID: ownerID, maxBytes: maxBytes, logger: logger}
}

// Register mounts the endpoints on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/articles/{id}/content", h.get)
	mux.HandleFunc("POST /api/articles/{id}/content", h.upload)
	mux.HandleFunc("POST /api/articles/{id}/content/fetch", h.fetch)
	mux.HandleFunc("GET /api/content-downloads", h.downloads)
	mux.HandleFunc("POST /api/content-downloads", h.queueMissing)
	mux.HandleFunc("DELETE /api/contents", h.removeTexts)
	mux.HandleFunc("DELETE /api/collections/{id}/contents", h.removeTexts)
	mux.HandleFunc("DELETE /api/articles/{id}/content", h.removeText)
	mux.HandleFunc("GET /api/articles/{id}/images/{imageId}", h.image)
}

// downloads serves GET /api/content-downloads: how many articles wait for
// their text, have none, failed or are incomplete.
func (h *Handler) downloads(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.DownloadSummary(r.Context(), h.ownerID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, s)
}

// queueMissing serves POST /api/content-downloads: queue the background
// download of every article never downloaded.
func (h *Handler) queueMissing(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.QueueMissing(r.Context(), h.ownerID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.logger.InfoContext(r.Context(), "missing texts queued", slog.Int("queued", n))
	httpapi.WriteJSON(w, http.StatusAccepted, map[string]int{"queued": n})
}

// removeTexts serves DELETE /api/contents and DELETE
// /api/collections/{id}/contents: delete the saved texts of every article,
// or of one collection's, keeping the articles.
func (h *Handler) removeTexts(w http.ResponseWriter, r *http.Request) {
	collectionID := r.PathValue("id")
	n, err := h.svc.RemoveTexts(r.Context(), h.ownerID, collectionID)
	if errors.Is(err, content.ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "collection not found")
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.logger.InfoContext(r.Context(), "saved texts removed", slog.Int("removed", n), slog.String("collection", collectionID))
	httpapi.WriteJSON(w, http.StatusOK, map[string]int{"removed": n})
}

// removeText serves DELETE /api/articles/{id}/content: delete one
// article's saved text, keeping the article.
func (h *Handler) removeText(w http.ResponseWriter, r *http.Request) {
	articleID := r.PathValue("id")
	n, err := h.svc.RemoveText(r.Context(), h.ownerID, articleID)
	if errors.Is(err, content.ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "article not found")
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.logger.InfoContext(r.Context(), "saved text removed", slog.Int("removed", n), slog.String("article", articleID))
	httpapi.WriteJSON(w, http.StatusOK, map[string]int{"removed": n})
}

// get serves GET /api/articles/{id}/content.
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Get(r.Context(), h.ownerID, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, c)
}

// upload serves POST /api/articles/{id}/content with an HTML or Markdown
// file in the "file" field of a multipart form.
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	file, name, ok := httpapi.OpenUpload(w, r, h.maxBytes, articleUpload)
	if !ok {
		return
	}
	c, err := h.svc.Upload(r.Context(), h.ownerID, r.PathValue("id"), name, file)
	if err != nil {
		if file.Err() != nil {
			httpapi.WriteUploadError(w, file.Err(), h.maxBytes, articleUpload)
			return
		}
		h.fail(w, r, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, c)
}

// fetch serves POST /api/articles/{id}/content/fetch: download the article
// from its original URL, or with ?from=copy from another copy only, without
// asking the original site.
func (h *Handler) fetch(w http.ResponseWriter, r *http.Request) {
	fetch := h.svc.Fetch
	switch r.URL.Query().Get("from") {
	case "":
	case "copy":
		fetch = h.svc.FetchCopy
	default:
		httpapi.WriteErrorDetails(w, http.StatusBadRequest, "invalid_query", "the query parameters are not valid",
			[]map[string]string{{"field": "from", "code": "invalid", "message": `from must be "copy" or absent`}})
		return
	}
	c, err := fetch(r.Context(), h.ownerID, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, c)
}

// image serves GET /api/articles/{id}/images/{imageId}. Images are sniffed
// raster formats; the headers stop a browser from running anything in them.
func (h *Handler) image(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fe *content.FetchError
	switch {
	case errors.Is(err, content.ErrNotFound):
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "article not found")
	case errors.Is(err, content.ErrNoContent):
		httpapi.WriteError(w, http.StatusNotFound, "content_not_found", "no text has been saved for this article")
	case errors.Is(err, content.ErrNoText):
		httpapi.WriteError(w, http.StatusUnprocessableEntity, "no_text", "the file holds no article text")
	case errors.Is(err, content.ErrTooLarge):
		httpapi.WriteErrorDetails(w, http.StatusRequestEntityTooLarge, "file_too_large", articleUpload.TooLarge,
			map[string]int64{"maxBytes": h.maxBytes})
	case errors.As(err, &fe):
		details := map[string]any{"problem": fe.Problem}
		if fe.Status != 0 {
			details["status"] = fe.Status
		}
		h.logger.InfoContext(r.Context(), "article download failed", slog.String("problem", fe.Problem), slog.String("error", fe.Error()))
		httpapi.WriteErrorDetails(w, http.StatusBadGateway, "fetch_failed", "the article could not be downloaded", details)
	default:
		h.logger.ErrorContext(r.Context(), "content request failed", slog.String("path", r.URL.Path), slog.String("error", err.Error()))
		httpapi.WriteError(w, http.StatusInternalServerError, "internal", "the request could not be completed; retry later")
	}
}
