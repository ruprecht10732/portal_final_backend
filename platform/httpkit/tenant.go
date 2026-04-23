package httpkit

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequireTenant extracts the tenant ID from the authenticated identity in the context.
// If the identity has no tenant, it sends a 403 error and returns false.
func RequireTenant(c *gin.Context) (uuid.UUID, bool) {
	id := GetIdentity(c)
	if !id.IsAuthenticated() {
		Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return uuid.UUID{}, false
	}
	tenantID := id.TenantID()
	if tenantID == nil {
		Error(c, http.StatusForbidden, "organization required", nil)
		return uuid.UUID{}, false
	}
	return *tenantID, true
}
