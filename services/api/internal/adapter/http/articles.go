package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"readvault/api/internal/library"
)

// LibraryService is the driving port the article endpoints call.
type LibraryService interface {
	List(ctx context.Context, userID string, p library.ListParams) (library.Page, error)
	Get(ctx context.Context, userID, articleID string) (library.Article, error)
}

type articleHandler struct {
	svc     LibraryService
	ownerID string
	logger  *slog.Logger
}

// list serves GET /api/articles?q=&tag=&tag=&sort=&limit=&cursor=
func (h *articleHandler) list(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	p := library.ListParams{Search: v.Get("q"), Tags: v["tag"], Sort: v.Get("sort"), Cursor: v.Get("cursor")}
	if raw := v.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeErrorDetails(w, http.StatusBadRequest, "invalid_query", "the query parameters are not valid",
				[]library.FieldProblem{{Field: "limit", Code: "invalid", Message: "limit must be a whole number"}})
			return
		}
		p.Limit = n
	}

	page, err := h.svc.List(r.Context(), h.ownerID, p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// get serves GET /api/articles/{id}.
func (h *articleHandler) get(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.Get(r.Context(), h.ownerID, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *articleHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var verr *library.ValidationError
	switch {
	case errors.As(err, &verr):
		writeErrorDetails(w, http.StatusBadRequest, "invalid_query", "the query parameters are not valid", verr.Problems)
	case errors.Is(err, library.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "article not found")
	default:
		h.logger.ErrorContext(r.Context(), "article request failed", slog.String("path", r.URL.Path), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal", "the request could not be completed; retry later")
	}
}
