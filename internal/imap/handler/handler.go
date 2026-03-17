package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"portal_final_backend/internal/imap/repository"
	"portal_final_backend/internal/imap/service"
	"portal_final_backend/internal/imap/transport"
	leadstransport "portal_final_backend/internal/leads/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
)

type Handler struct {
	svc *service.Service
	val *validator.Validator
}

func New(svc *service.Service, val *validator.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.ListAccounts)
	rg.GET("/unread-count", h.GetUnreadCount)
	rg.GET("/reply-scenario-analytics", h.ListReplyScenarioAnalytics)
	rg.POST("", h.CreateAccount)
	rg.POST("/detect", h.DetectAccount)
	rg.PATCH("/:id", h.UpdateAccount)
	rg.DELETE("/:id", h.DeleteAccount)
	rg.POST("/:id/test", h.TestAccount)
	rg.POST("/:id/sync", h.SyncAccount)
	rg.GET("/:id/messages", h.ListMessages)
	rg.GET("/:id/outbox", h.ListOutboundMessages)
	rg.POST("/:id/messages/send", h.SendMessage)
	rg.POST("/:id/messages/:uid/reply", h.ReplyMessage)
	rg.POST("/:id/messages/:uid/reply-all", h.ReplyAllMessage)
	rg.POST("/:id/messages/:uid/suggest-reply", h.SuggestReply)
	rg.POST("/:id/messages/:uid/seen", h.MarkMessageSeen)
	rg.POST("/:id/messages/:uid/unseen", h.MarkMessageUnseen)
	rg.GET("/:id/messages/:uid/content", h.GetMessageContent)
	rg.POST("/:id/messages/:uid/lead", h.LinkMessageLead)
	rg.POST("/:id/messages/:uid/create-lead", h.CreateLeadFromMessage)
	rg.DELETE("/:id/messages/:uid/lead", h.UnlinkMessageLead)
	rg.POST("/:id/messages/:uid/delete", h.DeleteMessage)
}

func (h *Handler) ListOutboundMessages(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	items, err := h.svc.ListOutboundMessages(c.Request.Context(), identity.UserID(), accountID)
	if httpkit.HandleError(c, err) {
		return
	}

	response := make([]transport.OutboundMessageResponse, 0, len(items))
	for _, item := range items {
		response = append(response, transport.OutboundMessageResponse{
			ID:           item.ID.String(),
			AccountID:    item.AccountID.String(),
			ToAddresses:  append([]string(nil), item.ToAddresses...),
			CcAddresses:  append([]string(nil), item.CcAddresses...),
			FromName:     item.FromName,
			FromAddress:  item.FromAddress,
			Subject:      item.Subject,
			Status:       string(item.Status),
			ErrorMessage: item.ErrorMessage,
			SentAt:       item.SentAt,
			CreatedAt:    item.CreatedAt,
			UpdatedAt:    item.UpdatedAt,
		})
	}

	httpkit.OK(c, transport.ListOutboundMessagesResponse{Items: response})
}

func (h *Handler) GetUnreadCount(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	count, err := h.svc.GetUnreadCount(c.Request.Context(), identity.UserID())
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.UnreadCountResponse{Count: count})
}

func (h *Handler) ListReplyScenarioAnalytics(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	items, err := h.svc.ListEmailReplyScenarioAnalytics(c.Request.Context(), identity.UserID())
	if httpkit.HandleError(c, err) {
		return
	}

	response := make([]transport.ReplyScenarioAnalyticsItemResponse, 0, len(items))
	for _, item := range items {
		var lastUsedAt *string
		if item.LastUsedAt != nil {
			value := item.LastUsedAt.UTC().Format(time.RFC3339)
			lastUsedAt = &value
		}
		editRate := 0.0
		if item.SentCount > 0 {
			editRate = float64(item.EditedCount) / float64(item.SentCount)
		}
		response = append(response, transport.ReplyScenarioAnalyticsItemResponse{
			Scenario:    item.Scenario,
			SentCount:   item.SentCount,
			EditedCount: item.EditedCount,
			EditRate:    editRate,
			LastUsedAt:  lastUsedAt,
		})
	}

	httpkit.OK(c, transport.ReplyScenarioAnalyticsResponse{Items: response})
}

func (h *Handler) DetectAccount(c *gin.Context) {
	var req transport.DetectAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}
	result := h.svc.DetectAccountSettings(c.Request.Context(), req.Email)
	httpkit.OK(c, result)
}

func (h *Handler) CreateAccount(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	var req transport.CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	account, err := h.svc.CreateAccount(c.Request.Context(), identity.UserID(), req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, toAccountResponse(account))
}

func (h *Handler) ListAccounts(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	accounts, err := h.svc.ListAccounts(c.Request.Context(), identity.UserID())
	if httpkit.HandleError(c, err) {
		return
	}

	items := make([]transport.AccountResponse, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, toAccountResponse(account))
	}
	httpkit.OK(c, items)
}

func (h *Handler) UpdateAccount(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	account, err := h.svc.UpdateAccount(c.Request.Context(), identity.UserID(), accountID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, toAccountResponse(account))
}

func (h *Handler) DeleteAccount(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	if httpkit.HandleError(c, h.svc.DeleteAccount(c.Request.Context(), identity.UserID(), accountID)) {
		return
	}

	httpkit.OK(c, gin.H{"message": "imap account deleted"})
}

func (h *Handler) TestAccount(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	if httpkit.HandleError(c, h.svc.TestAccountConnection(c.Request.Context(), identity.UserID(), accountID)) {
		return
	}

	httpkit.OK(c, gin.H{"message": "imap connection successful"})
}

func (h *Handler) SyncAccount(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	if httpkit.HandleError(c, h.svc.SyncAccount(c.Request.Context(), identity.UserID(), accountID)) {
		return
	}

	httpkit.OK(c, gin.H{"message": "imap sync started"})
}

func (h *Handler) ListMessages(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var query transport.ListMessagesQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, err.Error())
		return
	}
	if err := h.val.Struct(query); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	result, err := h.svc.ListMessages(c.Request.Context(), identity.UserID(), accountID, query)
	if httpkit.HandleError(c, err) {
		return
	}

	items := make([]transport.MessageResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, transport.MessageResponse{
			ID:             item.ID.String(),
			AccountID:      item.AccountID.String(),
			FolderName:     item.FolderName,
			UID:            item.UID,
			MessageID:      item.MessageID,
			FromName:       item.FromName,
			FromAddress:    item.FromAddress,
			Subject:        item.Subject,
			SentAt:         item.SentAt,
			ReceivedAt:     item.ReceivedAt,
			Snippet:        item.Snippet,
			SizeBytes:      item.SizeBytes,
			Seen:           item.Seen,
			Flagged:        item.Flagged,
			Answered:       item.Answered,
			Deleted:        item.Deleted,
			HasAttachments: item.HasAttachments,
			SyncedAt:       item.SyncedAt,
			CreatedAt:      item.CreatedAt,
			UpdatedAt:      item.UpdatedAt,
		})
	}

	httpkit.OK(c, transport.ListMessagesResponse{
		Items:      items,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	})
}

func (h *Handler) DeleteMessage(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	if httpkit.HandleError(c, h.svc.DeleteMessage(c.Request.Context(), identity.UserID(), accountID, uid)) {
		return
	}
	httpkit.OK(c, gin.H{"message": "message deleted"})
}

func (h *Handler) SendMessage(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	var req transport.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}
	if httpkit.HandleError(c, h.svc.SendMessage(c.Request.Context(), identity.UserID(), accountID, req)) {
		return
	}
	httpkit.OK(c, gin.H{"message": "message sent"})
}

func (h *Handler) ReplyMessage(c *gin.Context) {
	h.handleReply(c, false)
}

func (h *Handler) ReplyAllMessage(c *gin.Context) {
	h.handleReply(c, true)
}

func (h *Handler) SuggestReply(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.SuggestReplyRequest
	if err = c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err = h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	result, err := h.svc.SuggestEmailReply(c.Request.Context(), identity.UserID(), accountID, uid, req.Scenario, req.ScenarioNotes)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.SuggestReplyResponse{Suggestion: result.Suggestion, EffectiveScenario: result.EffectiveScenario})
}

func (h *Handler) MarkMessageSeen(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if httpkit.HandleError(c, h.svc.SetMessageSeen(c.Request.Context(), identity.UserID(), accountID, uid, true)) {
		return
	}
	httpkit.OK(c, gin.H{"message": "message marked as seen"})
}

func (h *Handler) MarkMessageUnseen(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if httpkit.HandleError(c, h.svc.SetMessageSeen(c.Request.Context(), identity.UserID(), accountID, uid, false)) {
		return
	}
	httpkit.OK(c, gin.H{"message": "message marked as unseen"})
}

func (h *Handler) handleReply(c *gin.Context, includeAll bool) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	var req transport.ReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}
	if httpkit.HandleError(c, h.svc.ReplyMessage(c.Request.Context(), identity.UserID(), accountID, uid, req, includeAll)) {
		return
	}
	httpkit.OK(c, gin.H{"message": "reply sent"})
}

func (h *Handler) GetMessageContent(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	content, err := h.svc.GetMessageContent(c.Request.Context(), identity.UserID(), accountID, uid)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, content)
}

func (h *Handler) LinkMessageLead(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	var req transport.LinkMessageLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}
	leadID, err := uuid.Parse(req.LeadID)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	linkedLead, err := h.svc.LinkMessageLead(c.Request.Context(), identity.UserID(), accountID, uid, leadID)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, transport.MessageLeadLinkResponse{Status: "ok", LinkedLead: toLeadInboxSummary(linkedLead)})
}

func (h *Handler) UnlinkMessageLead(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if httpkit.HandleError(c, h.svc.UnlinkMessageLead(c.Request.Context(), identity.UserID(), accountID, uid)) {
		return
	}
	httpkit.OK(c, transport.MessageLeadLinkResponse{Status: "ok"})
}

func (h *Handler) CreateLeadFromMessage(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	var req leadstransport.CreateLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}
	linkedLead, err := h.svc.CreateLeadFromMessage(c.Request.Context(), identity.UserID(), accountID, uid, req)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, transport.MessageLeadLinkResponse{Status: "ok", LinkedLead: toLeadInboxSummary(linkedLead)})
}

func toAccountResponse(account repository.Account) transport.AccountResponse {
	smtpConfigured := account.SMTPHost != nil &&
		account.SMTPPort != nil &&
		account.SMTPUsername != nil &&
		account.SMTPPasswordEncrypted != nil &&
		account.SMTPFromEmail != nil
	return transport.AccountResponse{
		ID:             account.ID.String(),
		UserID:         account.UserID.String(),
		EmailAddress:   account.EmailAddress,
		IMAPHost:       account.IMAPHost,
		IMAPPort:       account.IMAPPort,
		IMAPUsername:   account.IMAPUsername,
		SMTPHost:       account.SMTPHost,
		SMTPPort:       account.SMTPPort,
		SMTPUsername:   account.SMTPUsername,
		SMTPFromEmail:  account.SMTPFromEmail,
		SMTPFromName:   account.SMTPFromName,
		SMTPConfigured: smtpConfigured,
		FolderName:     account.FolderName,
		Enabled:        account.Enabled,
		LastSyncAt:     account.LastSyncAt,
		LastError:      account.LastError,
		LastErrorAt:    account.LastErrorAt,
		CreatedAt:      account.CreatedAt,
		UpdatedAt:      account.UpdatedAt,
	}
}

func toLeadInboxSummary(summary *service.LeadInboxSummary) *transport.LeadInboxSummaryResponse {
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
