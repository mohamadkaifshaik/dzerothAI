// Package apierror provides the canonical error shape for all Dzeroth API responses.
//
// Error shape (from docs/API_CONTRACTS.md):
//
//	{
//	  "error": {
//	    "code":    "VALIDATION_ERROR",
//	    "message": "Human-readable description of the problem.",
//	    "details": [
//	      { "field": "handle", "message": "must be 3–50 characters" }
//	    ]
//	  }
//	}
//
// Stack traces, SQL errors, internal service names, and credentials are never serialized
// into an ErrorResponse.
package apierror

import (
	"encoding/json"
	"net/http"
)

// Standard machine-readable error codes. Flutter switches on Code values.
const (
	CodeValidation         = "VALIDATION_ERROR"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeForbidden          = "FORBIDDEN"
	CodeNotFound           = "NOT_FOUND"
	CodeConflict           = "CONFLICT"
	CodeRateLimit          = "RATE_LIMITED"
	CodeInternal           = "INTERNAL_ERROR"
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"
)

// Detail holds a single field-level validation message.
type Detail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ErrorBody is the inner "error" object.
type ErrorBody struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []Detail `json:"details,omitempty"`
}

// ErrorResponse is the top-level wrapper written to the HTTP response body.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// APIError is a typed error returned by service functions. It carries a
// machine-readable Code and a human-readable Message. Handlers convert it to
// an ErrorResponse via ToResponse. Using a named type (rather than *ErrorResponse
// directly) avoids the Go restriction that a struct field and method cannot share
// a name.
type APIError struct {
	Code    string
	Message string
}

// Error implements the error interface.
func (e *APIError) Error() string { return e.Code + ": " + e.Message }

// ToResponse converts an APIError to an ErrorResponse for rendering.
func (e *APIError) ToResponse() *ErrorResponse {
	return New(e.Code, e.Message)
}

// NewAPIError constructs an *APIError. Service layers return this as an error.
func NewAPIError(code, message string) *APIError {
	return &APIError{Code: code, Message: message}
}

// New constructs an ErrorResponse with the given code and message and no details.
func New(code, message string) *ErrorResponse {
	return &ErrorResponse{
		Error: ErrorBody{
			Code:    code,
			Message: message,
		},
	}
}

// WithDetails returns a copy of the ErrorResponse with the provided details attached.
// Intended for validation errors that reference specific request fields.
func WithDetails(base *ErrorResponse, details []Detail) *ErrorResponse {
	return &ErrorResponse{
		Error: ErrorBody{
			Code:    base.Error.Code,
			Message: base.Error.Message,
			Details: details,
		},
	}
}

// ValidationError constructs a VALIDATION_ERROR response with the provided field-level details.
func ValidationError(details []Detail) *ErrorResponse {
	return &ErrorResponse{
		Error: ErrorBody{
			Code:    CodeValidation,
			Message: "Request validation failed.",
			Details: details,
		},
	}
}

// Render writes the ErrorResponse as JSON to w with the given HTTP status code.
// It sets Content-Type to application/json. All API errors must go through Render.
func Render(w http.ResponseWriter, statusCode int, errResp *ErrorResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	// Encoding failures are intentionally ignored: if we cannot write the error body the
	// status code has already been sent, and there is nothing safe to do.
	_ = json.NewEncoder(w).Encode(errResp)
}
