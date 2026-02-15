package handler

import (
	"strconv"
	"strings"

	"portal_final_backend/internal/notification/inapp"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type HTTPHandler struct {
	svc *inapp.Service
}

func NewHTTPHandler(svc *inapp.Service) *HTTPHandler {
	return &HTTPHandler{svc: svc}
}

func (h *HTTPHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.List)
	rg.GET("/unread", h.CountUnread)
	rg.GET("/unread-by-resource", h.CountUnreadByResource)
	rg.PATCH("/:id/read", h.MarkRead)
	rg.PATCH("/read-all", h.MarkAllRead)
	rg.DELETE("/:id", h.Delete)
}

func (h *HTTPHandler) List(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	items, total, err := h.svc.List(c.Request.Context(), identity.UserID(), page, limit)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{
		"items": items,
		"total": total,
		"page":  page,
	})
}

func (h *HTTPHandler) CountUnread(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	count, err := h.svc.CountUnread(c.Request.Context(), identity.UserID())
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"count": count})
}

func (h *HTTPHandler) CountUnreadByResource(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	typesParam := strings.TrimSpace(c.Query("types"))
	resourceTypes := make([]string, 0)
	if typesParam != "" {
		for _, item := range strings.Split(typesParam, ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed == "" {
				continue
			}
			resourceTypes = append(resourceTypes, trimmed)
		}
	}

	count, err := h.svc.CountUnreadByResourceTypes(c.Request.Context(), identity.UserID(), resourceTypes)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"count": count})
}

func (h *HTTPHandler) MarkRead(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, 400, "invalid id", nil)
		return
	}

	if err := h.svc.MarkRead(c.Request.Context(), identity.UserID(), id); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"status": "ok"})
}

func (h *HTTPHandler) MarkAllRead(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	if err := h.svc.MarkAllRead(c.Request.Context(), identity.UserID()); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"status": "ok"})
}

func (h *HTTPHandler) Delete(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, 400, "invalid id", nil)
		return
	}

	if err := h.svc.Delete(c.Request.Context(), identity.UserID(), id); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"status": "ok"})
}
