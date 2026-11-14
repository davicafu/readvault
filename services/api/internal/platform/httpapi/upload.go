package httpapi

import (
	"errors"
	"io"
	"net/http"
)

// multipartOverhead is the room left in the request body for multipart
// boundaries and headers on top of the file itself.
const multipartOverhead = 64 << 10

// errFileTooLarge means the uploaded file exceeded the endpoint's limit.
var errFileTooLarge = errors.New("uploaded file is too large")

// UploadKind names what an endpoint receives, for its error messages.
type UploadKind struct {
	// Noun completes "send the <noun> as multipart/form-data".
	Noun string
	// TooLarge is the file_too_large message.
	TooLarge string
}

// OpenUpload streams the "file" part of a multipart form and returns it with
// the file name the client sent. The file is never buffered to disk: the
// service reads it once.
func OpenUpload(w http.ResponseWriter, r *http.Request, maxBytes int64, kind UploadKind) (*Upload, string, bool) {
	return OpenUploadWithFields(w, r, maxBytes, kind, nil, nil)
}

// OpenUploadWithFields is OpenUpload that also reads the small fields named
// in limits into values, up to one byte past each limit so callers can tell
// an over-long field. Those fields must come before the file, since the file
// is streamed; other parts are skipped.
func OpenUploadWithFields(w http.ResponseWriter, r *http.Request, maxBytes int64, kind UploadKind, limits map[string]int64, values map[string][]byte) (*Upload, string, bool) {
	limit := maxBytes + multipartOverhead
	for _, n := range limits {
		limit += n + 1
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	mr, err := r.MultipartReader()
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_upload", "send the "+kind.Noun+` as multipart/form-data in the "file" field`)
		return nil, "", false
	}
	for parts := 0; ; parts++ {
		part, err := mr.NextPart()
		switch {
		case errors.Is(err, io.EOF) && parts == 0:
			// No part at all: the body is not the form its header announced.
			WriteError(w, http.StatusBadRequest, "invalid_upload", "send the "+kind.Noun+` as multipart/form-data in the "file" field`)
			return nil, "", false
		case errors.Is(err, io.EOF):
			WriteError(w, http.StatusBadRequest, "missing_file", `the form has no "file" field`)
			return nil, "", false
		case err != nil:
			WriteUploadError(w, err, maxBytes, kind)
			return nil, "", false
		case part.FormName() == "file":
			return &Upload{r: io.LimitReader(part, maxBytes+1), max: maxBytes}, part.FileName(), true
		case limits[part.FormName()] > 0:
			b, err := io.ReadAll(io.LimitReader(part, limits[part.FormName()]+1))
			if err != nil {
				WriteUploadError(w, err, maxBytes, kind)
				return nil, "", false
			}
			values[part.FormName()] = b
		}
	}
}

func WriteUploadError(w http.ResponseWriter, err error, maxBytes int64, kind UploadKind) {
	var maxBytesErr *http.MaxBytesError
	if errors.Is(err, errFileTooLarge) || errors.As(err, &maxBytesErr) {
		WriteErrorDetails(w, http.StatusRequestEntityTooLarge, "file_too_large",
			kind.TooLarge, map[string]int64{"maxBytes": maxBytes})
		return
	}
	WriteError(w, http.StatusBadRequest, "invalid_upload", "the uploaded form could not be read")
}

// Upload reads the file part, fails once it passes max bytes, and keeps the
// first read error so failures of the upload are told apart from the service's.
type Upload struct {
	r    io.Reader
	max  int64
	read int64
	err  error
}

func (u *Upload) Read(p []byte) (int, error) {
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

// Err is the first error reading the upload failed with, nil if none.
func (u *Upload) Err() error { return u.err }
