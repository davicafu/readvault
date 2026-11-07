package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// maxJSONBodyBytes bounds every JSON request body.
const maxJSONBodyBytes = 64 << 10

// errorBody is the single error shape returned by every endpoint:
//
//	{"error": {"code": "not_found", "message": "route not found"}}
//
// Messages never carry internal details such as SQL errors or stack traces.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

func writeErrorDetails(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message, Details: details}})
}

// decodeJSONBody decodes exactly one JSON object into v, rejecting unknown
// fields and bodies over maxJSONBodyBytes. On failure it writes the error
// response and returns false.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBodyBytes))
	dec.DisallowUnknownFields()
	err := dec.Decode(v)
	if err == nil && dec.Decode(&struct{}{}) != io.EOF {
		err = errors.New("body must hold a single JSON object")
	}
	var maxBytesErr *http.MaxBytesError
	switch {
	case err == nil:
		return true
	case errors.As(err, &maxBytesErr):
		writeErrorDetails(w, http.StatusRequestEntityTooLarge, "body_too_large", "the request body is too large",
			map[string]int{"maxBytes": maxJSONBodyBytes})
	default:
		writeError(w, http.StatusBadRequest, "invalid_json", "the body must be one JSON object with only the documented fields")
	}
	return false
}
