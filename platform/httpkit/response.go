// Package httpkit provides HTTP response utilities.
// This is part of the platform layer and contains no business logic.
package httpkit

import (
	"net/http"

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
