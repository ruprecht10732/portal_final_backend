package maps

import (
	"net/http"

	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
)

// Handler exposes the maps search endpoint.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// LookupAddress handles GET /api/v1/maps/address-lookup?q=...
func (h *Handler) LookupAddress(c *gin.Context) {
	var req LookupRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "query 'q' is required (min 3 chars)", nil)
		return
	}

	results, err := h.svc.SearchAddress(c.Request.Context(), req.Query)
	if err != nil {
		httpkit.Error(c, http.StatusBadGateway, "address lookup service unavailable", nil)
		return
	}

	httpkit.OK(c, results)
}
