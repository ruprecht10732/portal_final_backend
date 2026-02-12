package exports

import (
	"net/http"
	"strings"

	"portal_final_backend/internal/auth/password"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
)

// BasicAuthMiddleware validates HTTP Basic Auth credentials for public export endpoints.
func BasicAuthMiddleware(repo *Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		username, plaintextPassword, ok := c.Request.BasicAuth()
		if !ok || strings.TrimSpace(username) == "" || strings.TrimSpace(plaintextPassword) == "" {
			httpkit.Error(c, http.StatusUnauthorized, "missing export basic auth credentials", nil)
			return
		}

		credential, err := repo.GetCredentialByUsername(c.Request.Context(), strings.TrimSpace(username))
		if err != nil {
			httpkit.Error(c, http.StatusUnauthorized, "invalid export credentials", nil)
			return
		}

		if err := password.Compare(credential.PasswordHash, plaintextPassword); err != nil {
			httpkit.Error(c, http.StatusUnauthorized, "invalid export credentials", nil)
			return
		}

		c.Set("exportOrgID", credential.OrganizationID)
		c.Set("exportCredentialID", credential.ID)
		c.Next()
	}
}
