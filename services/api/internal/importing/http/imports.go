// Package http is the importing module's driving adapter: preview and run
// an import of a bookmarks file or a ReadVault export.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"readvault/api/internal/importing"

	"readvault/api/internal/platform/httpapi"
)

// maxChoicesBytes bounds the "choices" field of an import.
const maxChoicesBytes = 1 << 20

// Service is the driving port the import endpoints call.
type Service interface {
	Preview(ctx context.Context, userID string, file io.Reader) (importing.Preview, error)
	Import(ctx context.Context, userID string, choices importing.Choices, file io.Reader) (importing.Report, error)
}

type Handler struct {
	svc        Service
	ownerID    string
	maxBytes   int64
	maxRecords int
	logger     *slog.Logger
}

// New returns the import endpoints, acting for ownerID. maxBytes bounds an
// uploaded file; maxRecords is the record limit the service enforces,
// reported back to clients that exceed it.
func New(svc Service, ownerID string, maxBytes int64, maxRecords int, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Handler{svc: svc, ownerID: ownerID, maxBytes: maxBytes, maxRecords: maxRecords, logger: logger}
}

// Register mounts the endpoints on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/imports/preview", h.preview)
	mux.HandleFunc("POST /api/imports", h.run)
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	file, _, ok := httpapi.OpenUpload(w, r, h.maxBytes, importUpload)
	if !ok {
		return
	}
	p, err := h.svc.Preview(r.Context(), h.ownerID, file)
	if err != nil {
		h.fail(w, r, file, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, p)
}

func (h *Handler) run(w http.ResponseWriter, r *http.Request) {
	fields := map[string][]byte{}
	file, _, ok := httpapi.OpenUploadWithFields(w, r, h.maxBytes, importUpload, map[string]int64{"choices": maxChoicesBytes}, fields)
	if !ok {
		return
	}
	choices, err := decodeChoices(fields["choices"])
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_choices", err.Error())
		return
	}
	report, err := h.svc.Import(r.Context(), h.ownerID, choices, file)
	var choiceErr *importing.ChoiceError
	switch {
	case errors.As(err, &choiceErr):
		httpapi.WriteErrorDetails(w, http.StatusUnprocessableEntity, "invalid_choices", choiceErr.Message, map[string]string{"field": choiceErr.Field})
	case err != nil:
		h.fail(w, r, file, err)
	default:
		httpapi.WriteJSON(w, http.StatusOK, report)
	}
}

// choicesBody is the JSON of the "choices" field. A bookmark's collection
// can be absent (keep the suggestion), null (none) or a name.
type choicesBody struct {
	Collections map[string]*string `json:"collections"`
	Bookmarks   map[int]struct {
		Collection nullable  `json:"collection"`
		Tags       *[]string `json:"tags"`
	} `json:"bookmarks"`
}

// nullable is a JSON string field that can be absent, null or a value; Set
// tells absent from null.
type nullable struct {
	Set   bool
	Value *string
}

// UnmarshalJSON runs only when the field is present, including when it is null.
func (n *nullable) UnmarshalJSON(b []byte) error {
	n.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		n.Value = nil
		return nil
	}
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}

func decodeChoices(raw []byte) (importing.Choices, error) {
	if raw == nil {
		return importing.Choices{}, nil
	}
	if len(raw) > maxChoicesBytes {
		return importing.Choices{}, fmt.Errorf("the choices are larger than %d bytes", maxChoicesBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var body choicesBody
	if err := dec.Decode(&body); err != nil {
		return importing.Choices{}, errors.New(`the "choices" field is not valid JSON of the documented shape`)
	}
	c := importing.Choices{Collections: body.Collections}
	if body.Bookmarks != nil {
		c.Bookmarks = make(map[int]importing.BookmarkChoice, len(body.Bookmarks))
	}
	for pos, b := range body.Bookmarks {
		choice := importing.BookmarkChoice{SetCollection: b.Collection.Set, Collection: b.Collection.Value}
		if b.Tags != nil {
			choice.SetTags = true
			choice.Tags = *b.Tags
		}
		c.Bookmarks[pos] = choice
	}
	return c, nil
}

var importUpload = httpapi.UploadKind{Noun: "bookmarks file", TooLarge: "the file is larger than an import allows"}

// fail maps a service error to a response. Read errors of the upload are
// the client's; anything else is logged and reported without details.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, file *httpapi.Upload, err error) {
	switch {
	case errors.Is(err, importing.ErrTooManyRecords):
		httpapi.WriteErrorDetails(w, http.StatusUnprocessableEntity, "too_many_records",
			"the file has more bookmarks than an import allows", map[string]int{"maxRecords": h.maxRecords})
	case errors.Is(err, importing.ErrInvalidFile):
		httpapi.WriteError(w, http.StatusUnprocessableEntity, "invalid_file",
			"the file is a JSON file but not a ReadVault export this version can read")
	case file.Err() != nil:
		httpapi.WriteUploadError(w, file.Err(), h.maxBytes, importUpload)
	default:
		h.logger.ErrorContext(r.Context(), "import request failed", slog.String("path", r.URL.Path), slog.String("error", err.Error()))
		httpapi.WriteError(w, http.StatusInternalServerError, "internal", "the import could not be completed; retry later")
	}
}
