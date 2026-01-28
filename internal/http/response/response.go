package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ErrorResponse struct {
	Error   string      `json:"error"`
	Details interface{} `json:"details,omitempty"`
}

func JSON(c *gin.Context, status int, payload interface{}) {
	c.JSON(status, payload)
}

func Error(c *gin.Context, status int, message string, details interface{}) {
	c.JSON(status, ErrorResponse{Error: message, Details: details})
}

func OK(c *gin.Context, payload interface{}) {
	c.JSON(http.StatusOK, payload)
}
