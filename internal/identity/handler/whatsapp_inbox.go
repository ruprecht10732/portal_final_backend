package handler

import (
	"context"
	"net/http"
	"strconv"

	"portal_final_backend/internal/identity/service"
	"portal_final_backend/internal/identity/transport"
	leadstransport "portal_final_backend/internal/leads/transport"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) RegisterProtectedRoutes(rg *gin.RouterGroup) {
	rg.GET("/whatsapp/conversations", h.ListWhatsAppConversations)
	rg.GET("/whatsapp/conversations/unread-count", h.GetWhatsAppUnreadConversationCount)
	rg.GET("/whatsapp/conversations/:conversationID/messages", h.ListWhatsAppMessages)
	rg.POST("/whatsapp/conversations/:conversationID/lead", h.LinkWhatsAppConversationLead)
	rg.POST("/whatsapp/conversations/:conversationID/create-lead", h.CreateLeadFromWhatsAppConversation)
	rg.DELETE("/whatsapp/conversations/:conversationID/lead", h.UnlinkWhatsAppConversationLead)
	rg.POST("/whatsapp/conversations/:conversationID/messages", h.SendWhatsAppConversationMessage)
	rg.POST("/whatsapp/conversations/:conversationID/suggest-reply", h.SuggestWhatsAppReply)
	rg.POST("/whatsapp/conversations/:conversationID/messages/:messageID/reaction", h.ReactWhatsAppMessage)
	rg.POST("/whatsapp/conversations/:conversationID/messages/:messageID/edit", h.EditWhatsAppMessage)
	rg.POST("/whatsapp/conversations/:conversationID/messages/:messageID/delete", h.DeleteWhatsAppMessage)
	rg.POST("/whatsapp/conversations/:conversationID/messages/:messageID/revoke", h.RevokeWhatsAppMessage)
	rg.POST("/whatsapp/conversations/:conversationID/messages/:messageID/star", h.StarWhatsAppMessage)
	rg.POST("/whatsapp/conversations/:conversationID/messages/:messageID/attach-to-lead", h.AttachWhatsAppMessageToLead)
	rg.POST("/whatsapp/conversations/:conversationID/messages/save-to-lead", h.SaveWhatsAppMessagesToLead)
	rg.GET("/whatsapp/conversations/:conversationID/messages/:messageID/download", h.DownloadWhatsAppMessageMedia)
	rg.POST("/whatsapp/conversations/:conversationID/chat-presence", h.SendWhatsAppChatPresence)
	rg.POST("/whatsapp/conversations/:conversationID/archive", h.ArchiveWhatsAppConversation)
	rg.POST("/whatsapp/conversations/:conversationID/delete", h.DeleteWhatsAppConversation)
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
		linkedLead, suggestedLead, err := h.svc.GetWhatsAppConversationLeadState(c.Request.Context(), *tenantID, item)
		if httpkit.HandleError(c, err) {
			return
		}
		response = append(response, transport.WithWhatsAppConversationLeadState(
			transport.ToWhatsAppConversationResponse(item),
			toLeadInboxSummaryResponse(linkedLead),
			toLeadInboxSummaryResponse(suggestedLead),
		))
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
	linkedLead, suggestedLead, err := h.svc.GetWhatsAppConversationLeadState(c.Request.Context(), *tenantID, conversation)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.ListWhatsAppMessagesResponse{
		Conversation: transport.WithWhatsAppConversationLeadState(
			transport.ToWhatsAppConversationResponse(conversation),
			toLeadInboxSummaryResponse(linkedLead),
			toLeadInboxSummaryResponse(suggestedLead),
		),
		Messages: response,
	})
}

func (h *Handler) LinkWhatsAppConversationLead(c *gin.Context) {
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

	var req transport.LinkWhatsAppConversationLeadRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err = h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	leadID, err := uuid.Parse(req.LeadID)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	conversation, err := h.svc.LinkWhatsAppConversationLead(c.Request.Context(), *tenantID, conversationID, leadID)
	if httpkit.HandleError(c, err) {
		return
	}
	linkedLead, suggestedLead, err := h.svc.GetWhatsAppConversationLeadState(c.Request.Context(), *tenantID, conversation)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.WhatsAppConversationLeadResponse{
		Status: "ok",
		Conversation: transport.WithWhatsAppConversationLeadState(
			transport.ToWhatsAppConversationResponse(conversation),
			toLeadInboxSummaryResponse(linkedLead),
			toLeadInboxSummaryResponse(suggestedLead),
		),
	})
}

func (h *Handler) UnlinkWhatsAppConversationLead(c *gin.Context) {
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

	conversation, err := h.svc.UnlinkWhatsAppConversationLead(c.Request.Context(), *tenantID, conversationID)
	if httpkit.HandleError(c, err) {
		return
	}
	linkedLead, suggestedLead, err := h.svc.GetWhatsAppConversationLeadState(c.Request.Context(), *tenantID, conversation)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.WhatsAppConversationLeadResponse{
		Status: "ok",
		Conversation: transport.WithWhatsAppConversationLeadState(
			transport.ToWhatsAppConversationResponse(conversation),
			toLeadInboxSummaryResponse(linkedLead),
			toLeadInboxSummaryResponse(suggestedLead),
		),
	})
}

func (h *Handler) CreateLeadFromWhatsAppConversation(c *gin.Context) {
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

	var req leadstransport.CreateLeadRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err = h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	conversation, linkedLead, err := h.svc.CreateLeadFromWhatsAppConversation(c.Request.Context(), *tenantID, conversationID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.WhatsAppConversationLeadResponse{
		Status: "ok",
		Conversation: transport.WithWhatsAppConversationLeadState(
			transport.ToWhatsAppConversationResponse(conversation),
			toLeadInboxSummaryResponse(linkedLead),
			nil,
		),
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
		AISuggestion:    req.AISuggestion,
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

func (h *Handler) SuggestWhatsAppReply(c *gin.Context) {
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

	result, err := h.svc.SuggestWhatsAppReply(c.Request.Context(), identity.UserID(), *tenantID, conversationID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.SuggestWhatsAppReplyResponse{Suggestion: result.Suggestion})
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

func (h *Handler) StarWhatsAppMessage(c *gin.Context) {
	h.handleWhatsAppMessageToggleAction(c, func(ctx context.Context, tenantID, conversationID uuid.UUID, messageID string, value bool) (transport.WhatsAppConversationActionResponse, error) {
		result, err := h.svc.StarWhatsAppMessage(ctx, tenantID, conversationID, messageID, value)
		return toWhatsAppActionResponse("ok", result), err
	})
}

func (h *Handler) DownloadWhatsAppMessageMedia(c *gin.Context) {
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

	result, err := h.svc.DownloadWhatsAppMessageMedia(c.Request.Context(), *tenantID, conversationID, messageID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.WhatsAppMediaDownloadResponse{
		Status:      "ok",
		MessageID:   result.MessageID,
		MediaType:   result.MediaType,
		Filename:    result.Filename,
		FilePath:    result.FilePath,
		FileSize:    result.FileSize,
		DownloadURL: result.DownloadURL,
	})
}

func (h *Handler) AttachWhatsAppMessageToLead(c *gin.Context) {
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

	var req transport.AttachWhatsAppMessageToLeadRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err = h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	var serviceID *uuid.UUID
	if req.ServiceID != "" {
		parsed, parseErr := uuid.Parse(req.ServiceID)
		if parseErr != nil {
			httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
			return
		}
		serviceID = &parsed
	}

	result, err := h.svc.AttachWhatsAppMessageToLead(c.Request.Context(), *tenantID, identity.UserID(), conversationID, messageID, serviceID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.AttachWhatsAppMessageToLeadResponse{
		Status:              "ok",
		AttachmentID:        result.AttachmentID.String(),
		LeadID:              result.LeadID.String(),
		ServiceID:           result.ServiceID.String(),
		PhotoAnalysisQueued: true,
	})
}

func (h *Handler) SaveWhatsAppMessagesToLead(c *gin.Context) {
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

	var req transport.SaveWhatsAppMessagesToLeadRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err = h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	var serviceID *uuid.UUID
	if req.ServiceID != "" {
		parsed, parseErr := uuid.Parse(req.ServiceID)
		if parseErr != nil {
			httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
			return
		}
		serviceID = &parsed
	}

	result, err := h.svc.SaveWhatsAppMessagesToLead(c.Request.Context(), *tenantID, identity.UserID(), conversationID, req.MessageIDs, serviceID)
	if httpkit.HandleError(c, err) {
		return
	}

	var responseServiceID *string
	if result.ServiceID != nil {
		value := result.ServiceID.String()
		responseServiceID = &value
	}

	httpkit.OK(c, transport.SaveWhatsAppMessagesToLeadResponse{
		Status:         "ok",
		NoteID:         result.NoteID.String(),
		LeadID:         result.LeadID.String(),
		ServiceID:      responseServiceID,
		SavedCount:     result.SavedCount,
		ConversationID: conversationID.String(),
	})
}

func (h *Handler) ArchiveWhatsAppConversation(c *gin.Context) {
	h.handleWhatsAppConversationToggleAction(c, func(ctx context.Context, tenantID, conversationID uuid.UUID, value bool) (transport.WhatsAppConversationActionResponse, error) {
		result, err := h.svc.ArchiveWhatsAppConversation(ctx, tenantID, conversationID, value)
		return toWhatsAppActionResponse("ok", result), err
	})
}

func (h *Handler) DeleteWhatsAppConversation(c *gin.Context) {
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

	result, err := h.svc.DeleteWhatsAppConversation(c.Request.Context(), *tenantID, conversationID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, toWhatsAppActionResponse("ok", result))
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

func (h *Handler) handleWhatsAppMessageToggleAction(c *gin.Context, action func(ctx context.Context, tenantID, conversationID uuid.UUID, messageID string, value bool) (transport.WhatsAppConversationActionResponse, error)) {
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

	var req transport.ToggleWhatsAppMessageStateRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	response, err := action(c.Request.Context(), *tenantID, conversationID, messageID, req.Value)
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

func toLeadInboxSummaryResponse(summary *service.LeadInboxSummary) *transport.LeadInboxSummaryResponse {
	if summary == nil {
		return nil
	}
	return &transport.LeadInboxSummaryResponse{
		ID:       summary.ID.String(),
		FullName: summary.FullName,
		Phone:    summary.Phone,
		Email:    summary.Email,
		City:     summary.City,
	}
}
