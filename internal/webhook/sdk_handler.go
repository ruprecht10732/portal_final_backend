package webhook

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed sdk.js
var sdkJS []byte

// HandleServeSDK serves the form capture JavaScript SDK.
// GET /api/v1/webhook/sdk.js
func (h *Handler) HandleServeSDK(c *gin.Context) {
	c.Header("Content-Type", "application/javascript; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Data(http.StatusOK, "application/javascript; charset=utf-8", sdkJS)
}
