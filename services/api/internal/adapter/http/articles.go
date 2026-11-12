package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"readvault/api/internal/library"
)

// LibraryService is the driving port the article endpoints call.
type LibraryService interface {
	List(ctx context.Context, userID string, p library.ListParams) (library.Page, error)
	Get(ctx context.Context, userID, articleID string) (library.Article, error)
	Update(ctx context.Context, userID, articleID string, p library.Patch) (library.Article, error)
	Export(ctx context.Context, userID string) (library.Export, error)
	Tags(ctx context.Context, userID string) ([]library.TagCount, error)
	Collections(ctx context.Context, userID string) ([]library.Collection, error)
	CreateCollection(ctx context.Context, userID, name string) (library.Collection, error)
	RenameCollection(ctx context.Context, userID, collectionID, name string) (library.Collection, error)
	DeleteCollection(ctx context.Context, userID, collectionID string) error
	Suggest(ctx context.Context, userID, articleID string) (library.Suggestions, error)
}

type articleHandler struct {
	svc     LibraryService
	ownerID string
	logger  *slog.Logger
}

// list serves GET /api/articles?q=&tag=&tag=&sort=&limit=&cursor=
func (h *articleHandler) list(w http.ResponseWriter, r *http.Request) {
	// url.ParseQuery reports what r.URL.Query() silently drops, such as a
	// pair separated by ";" instead of "&".
	v, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeErrorDetails(w, http.StatusBadRequest, "invalid_query", "the query parameters are not valid",
			[]library.FieldProblem{{Field: "query", Code: "invalid", Message: `the query string is malformed; separate parameters with "&"`}})
		return
	}
	p := library.ListParams{Search: v.Get("q"), Tags: v["tag"], Collection: v.Get("collection"), Text: v.Get("text"), Sort: v.Get("sort"), Cursor: v.Get("cursor")}
	if raw := v.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeErrorDetails(w, http.StatusBadRequest, "invalid_query", "the query parameters are not valid",
				[]library.FieldProblem{{Field: "limit", Code: "invalid", Message: "limit must be a whole number"}})
			return
		}
		p.Limit = &n
	}

	page, err := h.svc.List(r.Context(), h.ownerID, p)
	if err != nil {
		h.fail(w, r, err, http.StatusBadRequest, "invalid_query", "the query parameters are not valid")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// tags serves GET /api/tags.
func (h *articleHandler) tags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.svc.Tags(r.Context(), h.ownerID)
	if err != nil {
		h.fail(w, r, err, http.StatusBadRequest, "invalid_query", "the query parameters are not valid")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

// suggestions serves GET /api/articles/{id}/suggestions.
func (h *articleHandler) suggestions(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.Suggest(r.Context(), h.ownerID, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err, http.StatusBadRequest, "invalid_query", "the query parameters are not valid")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// get serves GET /api/articles/{id}.
func (h *articleHandler) get(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.Get(r.Context(), h.ownerID, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err, http.StatusBadRequest, "invalid_query", "the query parameters are not valid")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// update serves PATCH /api/articles/{id} with a body such as
// {"title": "New title", "tags": ["go", "news"]}. Unknown fields are
// rejected, so the URLs cannot be changed through this endpoint.
func (h *articleHandler) update(w http.ResponseWriter, r *http.Request) {
	var p library.Patch
	if !decodeJSONBody(w, r, &p) {
		return
	}
	a, err := h.svc.Update(r.Context(), h.ownerID, r.PathValue("id"), p)
	if err != nil {
		h.fail(w, r, err, http.StatusUnprocessableEntity, "validation_failed", "the edit is not valid")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// export serves GET /api/export: the owner's whole library as a JSON
// download in the versioned library.Export format.
func (h *articleHandler) export(w http.ResponseWriter, r *http.Request) {
	e, err := h.svc.Export(r.Context(), h.ownerID)
	if err != nil {
		h.fail(w, r, err, http.StatusBadRequest, "invalid_query", "the request is not valid")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="readvault-export-%s.json"`, e.ExportedAt.Format("20060102-150405")))
	writeJSON(w, http.StatusOK, e)
}

// fail maps a service error to a response; a validation error is answered
// with the status, code and message the endpoint uses for invalid input.
func (h *articleHandler) fail(w http.ResponseWriter, r *http.Request, err error, invalidStatus int, invalidCode, invalidMessage string) {
	var verr *library.ValidationError
	switch {
	case errors.As(err, &verr):
		writeErrorDetails(w, invalidStatus, invalidCode, invalidMessage, verr.Problems)
	case errors.Is(err, library.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "article not found")
	case errors.Is(err, library.ErrCollectionNotFound):
		writeError(w, http.StatusNotFound, "not_found", "collection not found")
	case errors.Is(err, library.ErrCollectionExists):
		writeError(w, http.StatusConflict, "collection_exists", "a collection with this name already exists")
	default:
		h.logger.ErrorContext(r.Context(), "article request failed", slog.String("path", r.URL.Path), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal", "the request could not be completed; retry later")
	}
}

// collectionName is the body of POST and PATCH /api/collections.
type collectionName struct {
	Name string `json:"name"`
}

// collections serves GET /api/collections.
func (h *articleHandler) collections(w http.ResponseWriter, r *http.Request) {
	cs, err := h.svc.Collections(r.Context(), h.ownerID)
	if err != nil {
		h.fail(w, r, err, http.StatusBadRequest, "invalid_query", "the query parameters are not valid")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"collections": cs})
}

// createCollection serves POST /api/collections.
func (h *articleHandler) createCollection(w http.ResponseWriter, r *http.Request) {
	var body collectionName
	if !decodeJSONBody(w, r, &body) {
		return
	}
	c, err := h.svc.CreateCollection(r.Context(), h.ownerID, body.Name)
	if err != nil {
		h.fail(w, r, err, http.StatusUnprocessableEntity, "validation_failed", "the collection is not valid")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// renameCollection serves PATCH /api/collections/{id}.
func (h *articleHandler) renameCollection(w http.ResponseWriter, r *http.Request) {
	var body collectionName
	if !decodeJSONBody(w, r, &body) {
		return
	}
	c, err := h.svc.RenameCollection(r.Context(), h.ownerID, r.PathValue("id"), body.Name)
	if err != nil {
		h.fail(w, r, err, http.StatusUnprocessableEntity, "validation_failed", "the collection is not valid")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// deleteCollection serves DELETE /api/collections/{id}. The collection's
// articles stay in the library.
func (h *articleHandler) deleteCollection(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteCollection(r.Context(), h.ownerID, r.PathValue("id")); err != nil {
		h.fail(w, r, err, http.StatusBadRequest, "invalid_request", "the request is not valid")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
