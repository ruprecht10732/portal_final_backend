package exports

import (
	"net/http"
	"strings"

	"portal_final_backend/internal/auth/password"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
)

const exportAuthChallenge = `Basic realm="google-ads-export", charset="UTF-8"`

func respondUnauthorized(c *gin.Context, message string) {
	c.Header("WWW-Authenticate", exportAuthChallenge)
	httpkit.Error(c, http.StatusUnauthorized, message, nil)
}

// BasicAuthMiddleware validates HTTP Basic Auth credentials for public export endpoints.
func BasicAuthMiddleware(repo *Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		username, plaintextPassword, ok := c.Request.BasicAuth()
		if !ok || strings.TrimSpace(username) == "" || strings.TrimSpace(plaintextPassword) == "" {
			respondUnauthorized(c, "missing export basic auth credentials")
			return
		}

		credential, err := repo.GetCredentialByUsername(c.Request.Context(), strings.TrimSpace(username))
		if err != nil {
			respondUnauthorized(c, "invalid export credentials")
			return
		}

		if err := password.Compare(credential.PasswordHash, plaintextPassword); err != nil {
			respondUnauthorized(c, "invalid export credentials")
			return
		}

		c.Set("exportOrgID", credential.OrganizationID)
		c.Set("exportCredentialID", credential.ID)
		c.Next()
	}
}
