// Package http is the driving adapter: it exposes the ReadVault use cases
// over a JSON HTTP API.
package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// readinessTimeout bounds the database check behind /readyz.
const readinessTimeout = 2 * time.Second

// Pinger reports whether a backing service is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps holds the collaborators the handlers need.
type Deps struct {
	DB     Pinger
	Logger *slog.Logger

	// Imports serves the import endpoints; they are not mounted when nil.
	Imports ImportService
	// Library serves the article endpoints; they are not mounted when nil.
	Library LibraryService
	// OwnerID is the user every request acts for until authentication
	// exists. It is never taken from the client.
	OwnerID string
	// MaxUploadBytes bounds an uploaded bookmarks file.
	MaxUploadBytes int64
	// MaxImportRecords is the record limit the import service enforces,
	// reported back to clients that exceed it.
	MaxImportRecords int

	// Content serves the article text endpoints; they are not mounted when nil.
	Content ContentService
	// MaxArticleBytes bounds an uploaded article page or Markdown file.
	MaxArticleBytes int64
}

// NewHandler returns the API router wrapped in the common middleware.
func NewHandler(deps Deps) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.DiscardHandler)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /readyz", handleReadyz(deps.DB, deps.Logger))
	if deps.Imports != nil {
		h := &importHandler{
			svc:        deps.Imports,
			ownerID:    deps.OwnerID,
			maxBytes:   deps.MaxUploadBytes,
			maxRecords: deps.MaxImportRecords,
			logger:     deps.Logger,
		}
		mux.HandleFunc("POST /api/imports/preview", h.preview)
		mux.HandleFunc("POST /api/imports", h.run)
	}
	if deps.Library != nil {
		h := &articleHandler{svc: deps.Library, ownerID: deps.OwnerID, logger: deps.Logger}
		mux.HandleFunc("GET /api/articles", h.list)
		mux.HandleFunc("GET /api/articles/{id}", h.get)
		mux.HandleFunc("PATCH /api/articles/{id}", h.update)
		mux.HandleFunc("GET /api/articles/{id}/suggestions", h.suggestions)
		mux.HandleFunc("GET /api/export", h.export)
		mux.HandleFunc("GET /api/tags", h.tags)
		mux.HandleFunc("GET /api/collections", h.collections)
		mux.HandleFunc("POST /api/collections", h.createCollection)
		mux.HandleFunc("PATCH /api/collections/{id}", h.renameCollection)
		mux.HandleFunc("DELETE /api/collections/{id}", h.deleteCollection)
	}
	if deps.Content != nil {
		h := &contentHandler{svc: deps.Content, ownerID: deps.OwnerID, maxBytes: deps.MaxArticleBytes, logger: deps.Logger}
		mux.HandleFunc("GET /api/articles/{id}/content", h.get)
		mux.HandleFunc("POST /api/articles/{id}/content", h.upload)
		mux.HandleFunc("POST /api/articles/{id}/content/fetch", h.fetch)
		mux.HandleFunc("GET /api/articles/{id}/images/{imageId}", h.image)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
	})

	return withRecovery(deps.Logger, withRequestLog(deps.Logger, mux))
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleReadyz(db Pinger, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			logger.WarnContext(ctx, "readiness check failed", slog.String("error", err.Error()))
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
