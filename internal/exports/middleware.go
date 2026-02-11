package exports

import (
	"net/http"

	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
)

// APIKeyAuthMiddleware validates export API keys for public export endpoints.
func APIKeyAuthMiddleware(repo *Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		plaintext := c.GetHeader("X-Export-API-Key")
		if plaintext == "" {
			httpkit.Error(c, http.StatusUnauthorized, "missing export API key", nil)
			return
		}

		hash := HashKey(plaintext)
		key, err := repo.GetAPIKeyByHash(c.Request.Context(), hash)
		if err != nil {
			httpkit.Error(c, http.StatusUnauthorized, "invalid export API key", nil)
			return
		}

		c.Set("exportOrgID", key.OrganizationID)
		c.Set("exportKeyID", key.ID)
		c.Next()
	}
}
