package exports

import (
	"net/http"
	"strings"

	"portal_final_backend/internal/auth/password"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
)

const exportAuthChallenge = `Basic realm="google-ads-export", charset="UTF-8"`

func BasicAuthMiddleware(repo *Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, plain, ok := c.Request.BasicAuth()
		if !ok || strings.TrimSpace(user) == "" {
			c.Header("WWW-Authenticate", exportAuthChallenge)
			httpkit.Error(c, http.StatusUnauthorized, "missing credentials", nil)
			return
		}

		cred, err := repo.GetCredentialByUsername(c.Request.Context(), strings.TrimSpace(user))
		if err != nil || password.Compare(cred.PasswordHash, plain) != nil {
			c.Header("WWW-Authenticate", exportAuthChallenge)
			httpkit.Error(c, http.StatusUnauthorized, "invalid credentials", nil)
			return
		}

		c.Set("exportOrgID", cred.OrganizationID)
		c.Set("exportCredentialID", cred.ID)
		c.Next()
	}
}
