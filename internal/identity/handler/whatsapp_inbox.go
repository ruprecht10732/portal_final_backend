package handler

import (
	"net/http"
	"strconv"

	"portal_final_backend/internal/identity/transport"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) RegisterProtectedRoutes(rg *gin.RouterGroup) {
	rg.GET("/whatsapp/conversations", h.ListWhatsAppConversations)
	rg.GET("/whatsapp/conversations/unread-count", h.GetWhatsAppUnreadConversationCount)
	rg.GET("/whatsapp/conversations/:conversationID/messages", h.ListWhatsAppMessages)
	rg.POST("/whatsapp/conversations/:conversationID/messages", h.SendWhatsAppConversationMessage)
	rg.POST("/whatsapp/conversations/:conversationID/read", h.MarkWhatsAppConversationRead)
}

func (h *Handler) GetWhatsAppUnreadConversationCount(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	count, err := h.svc.CountUnreadWhatsAppConversations(c.Request.Context(), *tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.WhatsAppUnreadConversationCountResponse{Count: count})
}

func (h *Handler) ListWhatsAppConversations(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	limit := 50
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 200 {
			httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, "invalid limit")
			return
		}
		limit = parsed
	}

	items, err := h.svc.ListWhatsAppConversations(c.Request.Context(), *tenantID, limit, 0)
	if httpkit.HandleError(c, err) {
		return
	}

	response := make([]transport.WhatsAppConversationResponse, 0, len(items))
	for _, item := range items {
		response = append(response, transport.ToWhatsAppConversationResponse(item))
	}

	httpkit.OK(c, transport.ListWhatsAppConversationsResponse{Conversations: response})
}

func (h *Handler) ListWhatsAppMessages(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	conversationID, err := uuid.Parse(c.Param("conversationID"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	limit := 200
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil || parsed < 1 || parsed > 500 {
			httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, "invalid limit")
			return
		}
		limit = parsed
	}

	conversation, messages, err := h.svc.GetWhatsAppConversationMessages(c.Request.Context(), *tenantID, conversationID, limit)
	if httpkit.HandleError(c, err) {
		return
	}

	response := make([]transport.WhatsAppMessageResponse, 0, len(messages))
	for _, item := range messages {
		response = append(response, transport.ToWhatsAppMessageResponse(item))
	}

	httpkit.OK(c, transport.ListWhatsAppMessagesResponse{
		Conversation: transport.ToWhatsAppConversationResponse(conversation),
		Messages:     response,
	})
}

func (h *Handler) SendWhatsAppConversationMessage(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	conversationID, err := uuid.Parse(c.Param("conversationID"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.SendWhatsAppConversationMessageRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err = h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	conversation, message, err := h.svc.SendWhatsAppConversationMessage(c.Request.Context(), *tenantID, conversationID, req.Body)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.SendWhatsAppConversationMessageResponse{
		Status:       "sent",
		Conversation: transport.ToWhatsAppConversationResponse(conversation),
		Message:      transport.ToWhatsAppMessageResponse(message),
	})
}

func (h *Handler) MarkWhatsAppConversationRead(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	conversationID, err := uuid.Parse(c.Param("conversationID"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	err = h.svc.MarkWhatsAppConversationRead(c.Request.Context(), *tenantID, conversationID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.MarkWhatsAppConversationReadResponse{Status: "ok"})
}
