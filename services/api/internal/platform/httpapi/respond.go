package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"
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

// WriteJSON writes v as the JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes the error shape with a code and a message.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

// WriteErrorDetails is WriteError with details for the client.
func WriteErrorDetails(w http.ResponseWriter, status int, code, message string, details any) {
	WriteJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message, Details: details}})
}

// DecodeJSONBody decodes exactly one JSON object into v, which must point to
// a struct. It requires an application/json body, rejects bodies over
// maxJSONBodyBytes and rejects fields that are not v's JSON names spelled
// exactly (encoding/json alone would accept "Title" for "title"). On failure
// it writes the error response and returns false.
func DecodeJSONBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		WriteError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "send the body as application/json")
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxJSONBodyBytes))
	if err == nil {
		err = decodeStrict(body, v)
	}
	var maxBytesErr *http.MaxBytesError
	switch {
	case err == nil:
		return true
	case errors.As(err, &maxBytesErr):
		WriteErrorDetails(w, http.StatusRequestEntityTooLarge, "body_too_large", "the request body is too large",
			map[string]int{"maxBytes": maxJSONBodyBytes})
	default:
		WriteError(w, http.StatusBadRequest, "invalid_json", "the body must be one JSON object with only the documented fields")
	}
	return false
}

func decodeStrict(body []byte, v any) error {
	var fields map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&fields); err != nil {
		return err
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return errors.New("body must hold a single JSON object")
	}
	known := jsonFieldNames(reflect.TypeOf(v).Elem())
	for name := range fields {
		if !known[name] {
			return fmt.Errorf("unknown field %q", name)
		}
	}
	return json.Unmarshal(body, v)
}

// jsonFieldNames returns the JSON names of a struct type's exported fields.
func jsonFieldNames(t reflect.Type) map[string]bool {
	names := map[string]bool{}
	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		switch name {
		case "-":
		case "":
			names[f.Name] = true
		default:
			names[name] = true
		}
	}
	return names
}
