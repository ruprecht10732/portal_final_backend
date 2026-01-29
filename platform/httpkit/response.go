// Package httpkit provides HTTP response utilities.
// This is part of the platform layer and contains no business logic.
package httpkit

import (
	"net/http"

	"portal_final_backend/platform/apperr"

	"github.com/gin-gonic/gin"
)

// ErrorResponse is the standard error response format.
type ErrorResponse struct {
	Error   string      `json:"error"`
	Details interface{} `json:"details,omitempty"`
}

// JSON sends a JSON response with the given status code.
func JSON(c *gin.Context, status int, payload interface{}) {
	c.JSON(status, payload)
}

// Error sends an error response with the given status code and message.
func Error(c *gin.Context, status int, message string, details interface{}) {
	c.JSON(status, ErrorResponse{Error: message, Details: details})
}

// OK sends a 200 OK response with the given payload.
func OK(c *gin.Context, payload interface{}) {
	c.JSON(http.StatusOK, payload)
}

// HandleError maps domain errors to HTTP responses.
// If the error is a typed *apperr.Error, it uses the error's Kind to determine
// the HTTP status code. Otherwise, it defaults to 400 Bad Request.
// Returns true if an error was handled, false otherwise.
func HandleError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}

	if domainErr, ok := err.(*apperr.Error); ok {
		c.JSON(domainErr.HTTPStatus(), ErrorResponse{
			Error:   domainErr.Message,
			Details: domainErr.Details,
		})
		return true
	}

	// Fallback for non-typed errors
	c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	return true
}
