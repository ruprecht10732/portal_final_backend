// Package apperr provides standardized domain error types for the application.
// Domain services return these typed errors, and the HTTP layer middleware
// automatically maps them to appropriate HTTP status codes.
package apperr

import (
	"fmt"
	"net/http"
)

// Kind represents the category of error.
type Kind int

const (
	// KindUnknown is the default error kind when none is specified.
	KindUnknown Kind = iota
	// KindNotFound indicates a resource was not found.
	KindNotFound
	// KindValidation indicates invalid input data.
	KindValidation
	// KindConflict indicates a conflict with existing state (e.g., duplicate).
	KindConflict
	// KindForbidden indicates the action is not allowed for the user.
	KindForbidden
	// KindUnauthorized indicates authentication is required or failed.
	KindUnauthorized
	// KindBadRequest indicates a malformed or invalid request.
	KindBadRequest
	// KindInternal indicates an unexpected internal error.
	KindInternal
	// KindGone indicates a resource that existed but is no longer available.
	KindGone
)

// Error is a domain error with a typed Kind for HTTP mapping.
type Error struct {
	Kind    Kind
	Message string
	Op      string      // Operation that failed (optional)
	Err     error       // Underlying error (optional)
	Details interface{} // Additional details for response (optional)
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Op != "" {
		return fmt.Sprintf("%s: %s", e.Op, e.Message)
	}
	return e.Message
}

// Unwrap returns the underlying error for errors.Is/As support.
func (e *Error) Unwrap() error {
	return e.Err
}

// HTTPStatus returns the appropriate HTTP status code for this error kind.
func (e *Error) HTTPStatus() int {
	switch e.Kind {
	case KindNotFound:
		return http.StatusNotFound
	case KindValidation, KindBadRequest:
		return http.StatusBadRequest
	case KindConflict:
		return http.StatusConflict
	case KindForbidden:
		return http.StatusForbidden
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindInternal:
		return http.StatusInternalServerError
	case KindGone:
		return http.StatusGone
	default:
		return http.StatusBadRequest
	}
}

// New creates a new domain error with the given kind and message.
func New(kind Kind, message string) *Error {
	return &Error{Kind: kind, Message: message}
}

// Wrap creates a new domain error wrapping an existing error.
func Wrap(kind Kind, message string, err error) *Error {
	return &Error{Kind: kind, Message: message, Err: err}
}

// WithOp returns a copy of the error with the operation set.
func (e *Error) WithOp(op string) *Error {
	e.Op = op
	return e
}

// WithDetails returns a copy of the error with additional details.
func (e *Error) WithDetails(details interface{}) *Error {
	e.Details = details
	return e
}

// Convenience constructors for common error types.

// NotFound creates a not found error.
func NotFound(message string) *Error {
	return New(KindNotFound, message)
}

// Validation creates a validation error.
func Validation(message string) *Error {
	return New(KindValidation, message)
}

// Conflict creates a conflict error (e.g., duplicate resource).
func Conflict(message string) *Error {
	return New(KindConflict, message)
}

// Forbidden creates a forbidden error.
func Forbidden(message string) *Error {
	return New(KindForbidden, message)
}

// Unauthorized creates an unauthorized error.
func Unauthorized(message string) *Error {
	return New(KindUnauthorized, message)
}

// BadRequest creates a bad request error.
func BadRequest(message string) *Error {
	return New(KindBadRequest, message)
}

// Internal creates an internal server error.
func Internal(message string) *Error {
	return New(KindInternal, message)
}

// Gone creates a gone error (resource expired/removed).
func Gone(message string) *Error {
	return New(KindGone, message)
}

// GetKind extracts the error kind from an error.
// Returns KindUnknown if the error is not an *Error.
func GetKind(err error) Kind {
	if e, ok := err.(*Error); ok {
		return e.Kind
	}
	return KindUnknown
}

// Is checks if err is an *Error with the given kind.
func Is(err error, kind Kind) bool {
	return GetKind(err) == kind
}
