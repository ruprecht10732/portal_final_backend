package httpkit

import (
	"net/http"

	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
)

// BindJSON binds a JSON request body to a new value of type T and validates it.
// Returns the bound value and true on success, or sends a 400 error and returns false.
func BindJSON[T any](c *gin.Context, val *validator.Validator) (T, bool) {
	var req T
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, "invalid request", nil)
		return req, false
	}
	if err := val.Struct(req); err != nil {
		Error(c, http.StatusBadRequest, "validation failed", err.Error())
		return req, false
	}
	return req, true
}

// BindQuery binds query parameters to a new value of type T and validates it.
// Returns the bound value and true on success, or sends a 400 error and returns false.
func BindQuery[T any](c *gin.Context, val *validator.Validator) (T, bool) {
	var req T
	if err := c.ShouldBindQuery(&req); err != nil {
		Error(c, http.StatusBadRequest, "invalid request", nil)
		return req, false
	}
	if err := val.Struct(req); err != nil {
		Error(c, http.StatusBadRequest, "validation failed", err.Error())
		return req, false
	}
	return req, true
}
