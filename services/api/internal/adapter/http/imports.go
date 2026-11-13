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
	"readvault/api/internal/library"
)

// multipartOverhead is the room left in the request body for multipart
// boundaries and headers on top of the file itself.
const multipartOverhead = 64 << 10

// maxChoicesBytes bounds the "choices" field of an import.
const maxChoicesBytes = 1 << 20

// errFileTooLarge means the uploaded file exceeded MaxUploadBytes.
var errFileTooLarge = errors.New("uploaded file is too large")

// ImportService is the driving port the import endpoints call.
type ImportService interface {
	Preview(ctx context.Context, userID string, file io.Reader) (importing.Preview, error)
	Import(ctx context.Context, userID string, choices importing.Choices, file io.Reader) (importing.Report, error)
}

type importHandler struct {
	svc        ImportService
	ownerID    string
	maxBytes   int64
	maxRecords int
	logger     *slog.Logger
}

func (h *importHandler) preview(w http.ResponseWriter, r *http.Request) {
	file, _, ok := openUpload(w, r, h.maxBytes, importUpload)
	if !ok {
		return
	}
	p, err := h.svc.Preview(r.Context(), h.ownerID, file)
	if err != nil {
		h.fail(w, r, file, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *importHandler) run(w http.ResponseWriter, r *http.Request) {
	fields := map[string][]byte{}
	file, _, ok := openUploadWithFields(w, r, h.maxBytes, importUpload, map[string]int64{"choices": maxChoicesBytes}, fields)
	if !ok {
		return
	}
	choices, err := decodeChoices(fields["choices"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_choices", err.Error())
		return
	}
	report, err := h.svc.Import(r.Context(), h.ownerID, choices, file)
	var choiceErr *importing.ChoiceError
	switch {
	case errors.As(err, &choiceErr):
		writeErrorDetails(w, http.StatusUnprocessableEntity, "invalid_choices", choiceErr.Message, map[string]string{"field": choiceErr.Field})
	case err != nil:
		h.fail(w, r, file, err)
	default:
		writeJSON(w, http.StatusOK, report)
	}
}

// choicesBody is the JSON of the "choices" field. A bookmark's collection
// can be absent (keep the suggestion), null (none) or a name.
type choicesBody struct {
	Collections map[string]*string `json:"collections"`
	Bookmarks   map[int]struct {
		Collection library.Nullable[string] `json:"collection"`
		Tags       *[]string                `json:"tags"`
	} `json:"bookmarks"`
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

// uploadKind names what an endpoint receives, for its error messages.
type uploadKind struct {
	// noun completes "send the <noun> as multipart/form-data".
	noun string
	// tooLarge is the file_too_large message.
	tooLarge string
}

var importUpload = uploadKind{noun: "bookmarks file", tooLarge: "the file is larger than an import allows"}

// openUpload streams the "file" part of a multipart form and returns it with
// the file name the client sent. The file is never buffered to disk: the
// service reads it once.
func openUpload(w http.ResponseWriter, r *http.Request, maxBytes int64, kind uploadKind) (*upload, string, bool) {
	return openUploadWithFields(w, r, maxBytes, kind, nil, nil)
}

// openUploadWithFields is openUpload that also reads the small fields named
// in limits into values, up to one byte past each limit so callers can tell
// an over-long field. Those fields must come before the file, since the file
// is streamed; other parts are skipped.
func openUploadWithFields(w http.ResponseWriter, r *http.Request, maxBytes int64, kind uploadKind, limits map[string]int64, values map[string][]byte) (*upload, string, bool) {
	limit := maxBytes + multipartOverhead
	for _, n := range limits {
		limit += n + 1
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upload", "send the "+kind.noun+` as multipart/form-data in the "file" field`)
		return nil, "", false
	}
	for parts := 0; ; parts++ {
		part, err := mr.NextPart()
		switch {
		case errors.Is(err, io.EOF) && parts == 0:
			// No part at all: the body is not the form its header announced.
			writeError(w, http.StatusBadRequest, "invalid_upload", "send the "+kind.noun+` as multipart/form-data in the "file" field`)
			return nil, "", false
		case errors.Is(err, io.EOF):
			writeError(w, http.StatusBadRequest, "missing_file", `the form has no "file" field`)
			return nil, "", false
		case err != nil:
			writeUploadError(w, err, maxBytes, kind)
			return nil, "", false
		case part.FormName() == "file":
			return &upload{r: io.LimitReader(part, maxBytes+1), max: maxBytes}, part.FileName(), true
		case limits[part.FormName()] > 0:
			b, err := io.ReadAll(io.LimitReader(part, limits[part.FormName()]+1))
			if err != nil {
				writeUploadError(w, err, maxBytes, kind)
				return nil, "", false
			}
			values[part.FormName()] = b
		}
	}
}

// fail maps a service error to a response. Read errors of the upload are
// the client's; anything else is logged and reported without details.
func (h *importHandler) fail(w http.ResponseWriter, r *http.Request, file *upload, err error) {
	switch {
	case errors.Is(err, importing.ErrTooManyRecords):
		writeErrorDetails(w, http.StatusUnprocessableEntity, "too_many_records",
			"the file has more bookmarks than an import allows", map[string]int{"maxRecords": h.maxRecords})
	case errors.Is(err, importing.ErrInvalidFile):
		writeError(w, http.StatusUnprocessableEntity, "invalid_file",
			"the file is a JSON file but not a ReadVault export this version can read")
	case file.err != nil:
		writeUploadError(w, file.err, h.maxBytes, importUpload)
	default:
		h.logger.ErrorContext(r.Context(), "import request failed", slog.String("path", r.URL.Path), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal", "the import could not be completed; retry later")
	}
}

func writeUploadError(w http.ResponseWriter, err error, maxBytes int64, kind uploadKind) {
	var maxBytesErr *http.MaxBytesError
	if errors.Is(err, errFileTooLarge) || errors.As(err, &maxBytesErr) {
		writeErrorDetails(w, http.StatusRequestEntityTooLarge, "file_too_large",
			kind.tooLarge, map[string]int64{"maxBytes": maxBytes})
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_upload", "the uploaded form could not be read")
}

// upload reads the file part, fails once it passes max bytes, and keeps the
// first read error so failures of the upload are told apart from the service's.
type upload struct {
	r    io.Reader
	max  int64
	read int64
	err  error
}

func (u *upload) Read(p []byte) (int, error) {
	n, err := u.r.Read(p)
	u.read += int64(n)
	if u.read > u.max {
		u.err = errFileTooLarge
		return n, u.err
	}
	if err != nil && !errors.Is(err, io.EOF) && u.err == nil {
		u.err = err
	}
	return n, err
}
