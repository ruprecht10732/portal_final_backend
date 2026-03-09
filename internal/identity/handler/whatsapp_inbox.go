package handler

import (
	"context"
	"net/http"
	"strconv"

	"portal_final_backend/internal/identity/service"
	"portal_final_backend/internal/identity/transport"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) RegisterProtectedRoutes(rg *gin.RouterGroup) {
	rg.GET("/whatsapp/conversations", h.ListWhatsAppConversations)
	rg.GET("/whatsapp/conversations/unread-count", h.GetWhatsAppUnreadConversationCount)
	rg.GET("/whatsapp/conversations/:conversationID/messages", h.ListWhatsAppMessages)
	rg.POST("/whatsapp/conversations/:conversationID/messages", h.SendWhatsAppConversationMessage)
	rg.POST("/whatsapp/conversations/:conversationID/messages/:messageID/reaction", h.ReactWhatsAppMessage)
	rg.POST("/whatsapp/conversations/:conversationID/messages/:messageID/edit", h.EditWhatsAppMessage)
	rg.POST("/whatsapp/conversations/:conversationID/messages/:messageID/delete", h.DeleteWhatsAppMessage)
	rg.POST("/whatsapp/conversations/:conversationID/messages/:messageID/revoke", h.RevokeWhatsAppMessage)
	rg.POST("/whatsapp/conversations/:conversationID/chat-presence", h.SendWhatsAppChatPresence)
	rg.POST("/whatsapp/conversations/:conversationID/archive", h.ArchiveWhatsAppConversation)
	rg.POST("/whatsapp/conversations/:conversationID/pin", h.PinWhatsAppConversation)
	rg.POST("/whatsapp/conversations/:conversationID/disappearing-timer", h.SetWhatsAppDisappearingTimer)
	rg.POST("/whatsapp/conversations/:conversationID/read", h.MarkWhatsAppConversationRead)
	rg.POST("/whatsapp/presence", h.SendWhatsAppPresence)
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

	messageInput := service.SendWhatsAppConversationMessageInput{
		Type:            req.Type,
		Body:            req.Body,
		Caption:         req.Caption,
		ViewOnce:        req.ViewOnce,
		Compress:        req.Compress,
		IsForwarded:     req.IsForwarded,
		PushToTalk:      req.PushToTalk,
		ContactName:     req.ContactName,
		ContactPhone:    req.ContactPhone,
		Link:            req.Link,
		Latitude:        req.Latitude,
		Longitude:       req.Longitude,
		Question:        req.Question,
		Options:         req.Options,
		MaxAnswer:       req.MaxAnswer,
		DurationSeconds: req.DurationSeconds,
	}
	if req.Attachment != nil {
		messageInput.Attachment = &service.SendWhatsAppConversationAttachmentInput{
			Filename:   req.Attachment.Filename,
			Base64Data: req.Attachment.Base64Data,
			RemoteURL:  req.Attachment.RemoteURL,
		}
	}

	conversation, message, err := h.svc.SendWhatsAppConversationMessage(c.Request.Context(), *tenantID, conversationID, messageInput)
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

	providerSynced, err := h.svc.MarkWhatsAppConversationRead(c.Request.Context(), *tenantID, conversationID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.MarkWhatsAppConversationReadResponse{Status: "ok", ProviderSynced: providerSynced})
}

func (h *Handler) SendWhatsAppPresence(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	var req transport.SendWhatsAppPresenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	err := h.svc.SendWhatsAppPresence(c.Request.Context(), *tenantID, req.Type)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.SendWhatsAppPresenceResponse{Status: "ok"})
}

func (h *Handler) SendWhatsAppChatPresence(c *gin.Context) {
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

	var req transport.SendWhatsAppChatPresenceRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err = h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	err = h.svc.SendWhatsAppChatPresence(c.Request.Context(), *tenantID, conversationID, req.Action)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.SendWhatsAppChatPresenceResponse{Status: "ok"})
}

func (h *Handler) ReactWhatsAppMessage(c *gin.Context) {
	h.handleWhatsAppMessageAction(c, func(ctx context.Context, tenantID, conversationID uuid.UUID, messageID string) (transport.WhatsAppConversationActionResponse, error) {
		var req transport.ReactWhatsAppMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			return transport.WhatsAppConversationActionResponse{}, apperr.Validation(msgInvalidRequest)
		}
		if err := h.val.Struct(req); err != nil {
			return transport.WhatsAppConversationActionResponse{}, apperr.Validation(err.Error())
		}
		result, err := h.svc.ReactWhatsAppMessage(ctx, tenantID, conversationID, messageID, req.Emoji)
		return toWhatsAppActionResponse("ok", result), err
	})
}

func (h *Handler) EditWhatsAppMessage(c *gin.Context) {
	h.handleWhatsAppMessageAction(c, func(ctx context.Context, tenantID, conversationID uuid.UUID, messageID string) (transport.WhatsAppConversationActionResponse, error) {
		var req transport.EditWhatsAppMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			return transport.WhatsAppConversationActionResponse{}, apperr.Validation(msgInvalidRequest)
		}
		if err := h.val.Struct(req); err != nil {
			return transport.WhatsAppConversationActionResponse{}, apperr.Validation(err.Error())
		}
		result, err := h.svc.EditWhatsAppMessage(ctx, tenantID, conversationID, messageID, req.Body)
		return toWhatsAppActionResponse("ok", result), err
	})
}

func (h *Handler) DeleteWhatsAppMessage(c *gin.Context) {
	h.handleWhatsAppMessageAction(c, func(ctx context.Context, tenantID, conversationID uuid.UUID, messageID string) (transport.WhatsAppConversationActionResponse, error) {
		result, err := h.svc.DeleteWhatsAppMessage(ctx, tenantID, conversationID, messageID)
		return toWhatsAppActionResponse("ok", result), err
	})
}

func (h *Handler) RevokeWhatsAppMessage(c *gin.Context) {
	h.handleWhatsAppMessageAction(c, func(ctx context.Context, tenantID, conversationID uuid.UUID, messageID string) (transport.WhatsAppConversationActionResponse, error) {
		result, err := h.svc.RevokeWhatsAppMessage(ctx, tenantID, conversationID, messageID)
		return toWhatsAppActionResponse("ok", result), err
	})
}

func (h *Handler) ArchiveWhatsAppConversation(c *gin.Context) {
	h.handleWhatsAppConversationToggleAction(c, func(ctx context.Context, tenantID, conversationID uuid.UUID, value bool) (transport.WhatsAppConversationActionResponse, error) {
		result, err := h.svc.ArchiveWhatsAppConversation(ctx, tenantID, conversationID, value)
		return toWhatsAppActionResponse("ok", result), err
	})
}

func (h *Handler) PinWhatsAppConversation(c *gin.Context) {
	h.handleWhatsAppConversationToggleAction(c, func(ctx context.Context, tenantID, conversationID uuid.UUID, value bool) (transport.WhatsAppConversationActionResponse, error) {
		result, err := h.svc.PinWhatsAppConversation(ctx, tenantID, conversationID, value)
		return toWhatsAppActionResponse("ok", result), err
	})
}

func (h *Handler) SetWhatsAppDisappearingTimer(c *gin.Context) {
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

	var req transport.SetWhatsAppDisappearingTimerRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err = h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	result, err := h.svc.SetWhatsAppDisappearingTimer(c.Request.Context(), *tenantID, conversationID, req.TimerSeconds)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, toWhatsAppActionResponse("ok", result))
}

func (h *Handler) handleWhatsAppMessageAction(c *gin.Context, action func(ctx context.Context, tenantID, conversationID uuid.UUID, messageID string) (transport.WhatsAppConversationActionResponse, error)) {
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
	messageID := c.Param("messageID")
	if messageID == "" {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	response, err := action(c.Request.Context(), *tenantID, conversationID, messageID)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, response)
}

func (h *Handler) handleWhatsAppConversationToggleAction(c *gin.Context, action func(ctx context.Context, tenantID, conversationID uuid.UUID, value bool) (transport.WhatsAppConversationActionResponse, error)) {
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

	var req transport.ToggleWhatsAppConversationStateRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	response, err := action(c.Request.Context(), *tenantID, conversationID, req.Value)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, response)
}

func toWhatsAppActionResponse(status string, result service.WhatsAppConversationActionResult) transport.WhatsAppConversationActionResponse {
	response := transport.WhatsAppConversationActionResponse{Status: status}
	if result.Conversation != nil {
		conversation := transport.ToWhatsAppConversationResponse(*result.Conversation)
		response.Conversation = &conversation
	}
	if result.Message != nil {
		message := transport.ToWhatsAppMessageResponse(*result.Message)
		response.Message = &message
	}
	return response
}
