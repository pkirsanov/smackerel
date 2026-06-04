package graphapi

import (
	"encoding/json"
	"net/http"
)

// Error codes — closed set used across all 8 spec 080 endpoints.
// Mirrors the design.md §8 error-shape allowlist.
const (
	CodeInvalidCursor   = "invalid_cursor"
	CodeInvalidWindow   = "invalid_window"
	CodeInvalidKind     = "invalid_kind"
	CodeMissingParam    = "missing_param"
	CodeLimitExceeded   = "limit_exceeded"
	CodeUnauthenticated = "unauthenticated"
	CodeForbidden       = "forbidden"
)

// ErrorEnvelope is the uniform JSON shape every spec 080 endpoint
// returns on error: {"error": {"code","message","field"}}. The field
// element is omitted when empty.
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody carries the code/message/field triple.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

// APIError is the typed representation of a graphapi error condition
// used internally by handlers and middleware. Each named error in this
// file (ErrUnauthenticated, ErrMissingScope, etc.) is an APIError
// instance; handlers translate them to HTTP via WriteError or
// WriteAPIError. Implements the error interface so it can flow
// through normal Go error plumbing.
type APIError struct {
	Status  int
	Code    string
	Message string
	Field   string
}

// Error satisfies the error interface. The format is stable for log
// inspection but is not the wire payload — that is the JSON envelope
// produced by WriteAPIError.
func (e *APIError) Error() string {
	return "graphapi: " + e.Code + ": " + e.Message
}

// WithField returns a copy of e with Field set. Lets handlers reuse a
// canonical APIError instance (e.g. ErrMalformedCursor) and stamp the
// field name without mutating the package-level singleton.
func (e *APIError) WithField(field string) *APIError {
	if e == nil {
		return nil
	}
	out := *e
	out.Field = field
	return &out
}

// Canonical APIError instances. Handlers SHOULD reuse these and clone
// via WithField when a more specific field name is needed.
var (
	ErrUnauthenticated = &APIError{
		Status:  http.StatusUnauthorized,
		Code:    CodeUnauthenticated,
		Message: "request is missing a bearer token",
	}
	ErrMissingScope = &APIError{
		Status:  http.StatusForbidden,
		Code:    CodeForbidden,
		Message: "bearer token does not carry the knowledge-graph:read scope",
	}
	ErrMalformedCursor = &APIError{
		Status:  http.StatusBadRequest,
		Code:    CodeInvalidCursor,
		Message: "cursor is malformed or signature does not verify",
		Field:   "cursor",
	}
	ErrLimitExceeded = &APIError{
		Status:  http.StatusBadRequest,
		Code:    CodeLimitExceeded,
		Message: "limit exceeds the configured maximum",
		Field:   "limit",
	}
	ErrTimeRangeTooLarge = &APIError{
		Status:  http.StatusBadRequest,
		Code:    CodeInvalidWindow,
		Message: "time window exceeds the configured maximum",
		Field:   "window",
	}
	ErrMissingParam = &APIError{
		Status:  http.StatusBadRequest,
		Code:    CodeMissingParam,
		Message: "required query parameter is missing",
	}
	ErrUnknownSourceKind = &APIError{
		Status:  http.StatusBadRequest,
		Code:    CodeInvalidKind,
		Message: "source kind is not recognized",
		Field:   "kind",
	}
)

// WriteError emits the uniform JSON error envelope from raw arguments.
// Handlers that already hold an APIError SHOULD prefer WriteAPIError.
func WriteError(w http.ResponseWriter, status int, code, field, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Error: ErrorBody{Code: code, Message: message, Field: field},
	})
}

// WriteAPIError emits an APIError instance as the uniform JSON
// envelope. Always Content-Type application/json.
func WriteAPIError(w http.ResponseWriter, e *APIError) {
	if e == nil {
		// Defensive: never silently 200 OK on a nil APIError; route
		// the bug into the error path so it is observable.
		WriteError(w, http.StatusInternalServerError, "internal_error", "", "nil APIError")
		return
	}
	WriteError(w, e.Status, e.Code, e.Field, e.Message)
}
