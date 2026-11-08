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
	p := library.ListParams{Search: v.Get("q"), Tags: v["tag"], Sort: v.Get("sort"), Cursor: v.Get("cursor")}
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
	default:
		h.logger.ErrorContext(r.Context(), "article request failed", slog.String("path", r.URL.Path), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal", "the request could not be completed; retry later")
	}
}
