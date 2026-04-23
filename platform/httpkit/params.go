package httpkit

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ParseUUIDParam extracts a UUID from a Gin path parameter.
// Returns the parsed UUID and true on success, or sends a 400 error and returns false.
func ParseUUIDParam(c *gin.Context, param string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(param))
	if err != nil {
		Error(c, http.StatusBadRequest, "invalid "+param, nil)
		return uuid.UUID{}, false
	}
	return id, true
}
