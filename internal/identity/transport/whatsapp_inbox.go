package transport

import (
	"encoding/json"
	"time"

	"portal_final_backend/internal/identity/repository"

	"github.com/google/uuid"
)

type WhatsAppConversationResponse struct {
	ID                   string                    `json:"id"`
	LeadID               *string                   `json:"leadId,omitempty"`
	LinkedLead           *LeadInboxSummaryResponse `json:"linkedLead,omitempty"`
	SuggestedLead        *LeadInboxSummaryResponse `json:"suggestedLead,omitempty"`
	PhoneNumber          string                    `json:"phoneNumber"`
	DisplayName          string                    `json:"displayName"`
	LastMessagePreview   string                    `json:"lastMessagePreview"`
	LastMessageAt        string                    `json:"lastMessageAt"`
	LastMessageDirection string                    `json:"lastMessageDirection"`
	LastMessageStatus    string                    `json:"lastMessageStatus"`
	UnreadCount          int                       `json:"unreadCount"`
	ArchivedAt           *string                   `json:"archivedAt,omitempty"`
	DeletedAt            *string                   `json:"deletedAt,omitempty"`
	CreatedAt            string                    `json:"createdAt"`
	UpdatedAt            string                    `json:"updatedAt"`
}

type WhatsAppMessageResponse struct {
	ID                string          `json:"id"`
	ConversationID    string          `json:"conversationId"`
	LeadID            *string         `json:"leadId,omitempty"`
	ExternalMessageID *string         `json:"externalMessageId,omitempty"`
	Direction         string          `json:"direction"`
	Status            string          `json:"status"`
	PhoneNumber       string          `json:"phoneNumber"`
	Body              string          `json:"body"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
	SentAt            *string         `json:"sentAt,omitempty"`
	ReadAt            *string         `json:"readAt,omitempty"`
	FailedAt          *string         `json:"failedAt,omitempty"`
	CreatedAt         string          `json:"createdAt"`
}

type LeadInboxSummaryResponse struct {
	ID       string  `json:"id"`
	FullName string  `json:"fullName"`
	Phone    string  `json:"phone"`
	Email    *string `json:"email,omitempty"`
	City     string  `json:"city,omitempty"`
}

type ListWhatsAppConversationsResponse struct {
	Conversations []WhatsAppConversationResponse `json:"conversations"`
}

type ListWhatsAppMessagesResponse struct {
	Conversation WhatsAppConversationResponse `json:"conversation"`
	Messages     []WhatsAppMessageResponse    `json:"messages"`
}

type SendWhatsAppConversationAttachmentRequest struct {
	Filename   string `json:"filename" validate:"max=255"`
	Base64Data string `json:"base64Data"`
	RemoteURL  string `json:"remoteUrl" validate:"max=2048"`
}

type SendWhatsAppConversationMessageRequest struct {
	Type            string                                     `json:"type" validate:"omitempty,oneof=text image video audio file sticker contact link location poll"`
	Body            string                                     `json:"body" validate:"max=4000"`
	AISuggestion    string                                     `json:"aiSuggestion" validate:"max=4000"`
	Caption         string                                     `json:"caption" validate:"max=4000"`
	ViewOnce        bool                                       `json:"viewOnce"`
	Compress        bool                                       `json:"compress"`
	IsForwarded     bool                                       `json:"isForwarded"`
	PushToTalk      bool                                       `json:"pushToTalk"`
	Attachment      *SendWhatsAppConversationAttachmentRequest `json:"attachment,omitempty"`
	ContactName     string                                     `json:"contactName" validate:"max=255"`
	ContactPhone    string                                     `json:"contactPhone" validate:"max=64"`
	Link            string                                     `json:"link" validate:"max=2048"`
	Latitude        string                                     `json:"latitude" validate:"max=64"`
	Longitude       string                                     `json:"longitude" validate:"max=64"`
	Question        string                                     `json:"question" validate:"max=1000"`
	Options         []string                                   `json:"options,omitempty"`
	MaxAnswer       int                                        `json:"maxAnswer"`
	DurationSeconds *int                                       `json:"durationSeconds,omitempty"`
}

type SendWhatsAppPresenceRequest struct {
	Type string `json:"type" validate:"required,oneof=available unavailable"`
}

type SendWhatsAppChatPresenceRequest struct {
	Action string `json:"action" validate:"required,oneof=start stop"`
}

type ReactWhatsAppMessageRequest struct {
	Emoji string `json:"emoji" validate:"required,max=16"`
}

type EditWhatsAppMessageRequest struct {
	Body string `json:"body" validate:"required,max=4000"`
}

type ToggleWhatsAppConversationStateRequest struct {
	Value bool `json:"value"`
}

type LinkWhatsAppConversationLeadRequest struct {
	LeadID string `json:"leadId" validate:"required,uuid4"`
}

type ToggleWhatsAppMessageStateRequest struct {
	Value bool `json:"value"`
}

type AttachWhatsAppMessageToLeadRequest struct {
	ServiceID string `json:"serviceId" validate:"omitempty,uuid4"`
}

type SaveWhatsAppMessagesToLeadRequest struct {
	MessageIDs []string `json:"messageIds" validate:"required,min=1,dive,required"`
	ServiceID  string   `json:"serviceId" validate:"omitempty,uuid4"`
}

type SetWhatsAppDisappearingTimerRequest struct {
	TimerSeconds int `json:"timerSeconds" validate:"min=0,max=31536000"`
}

type SendWhatsAppConversationMessageResponse struct {
	Status       string                       `json:"status"`
	Conversation WhatsAppConversationResponse `json:"conversation"`
	Message      WhatsAppMessageResponse      `json:"message"`
}

type SuggestWhatsAppReplyRequest struct {
	Scenario      string `json:"scenario" validate:"omitempty,oneof=generic follow_up appointment_reminder appointment_confirmation reschedule_request quote_reminder quote_expiry missing_information photos_or_documents post_visit_follow_up accepted_quote_next_steps delay_update complaint_recovery"`
	ScenarioNotes string `json:"scenarioNotes" validate:"omitempty,max=1000"`
}

type SuggestWhatsAppReplyResponse struct {
	Suggestion string `json:"suggestion"`
}

type WhatsAppConversationActionResponse struct {
	Status       string                        `json:"status"`
	Conversation *WhatsAppConversationResponse `json:"conversation,omitempty"`
	Message      *WhatsAppMessageResponse      `json:"message,omitempty"`
}

type MarkWhatsAppConversationReadResponse struct {
	Status         string `json:"status"`
	ProviderSynced bool   `json:"providerSynced"`
}

type SendWhatsAppPresenceResponse struct {
	Status string `json:"status"`
}

type SendWhatsAppChatPresenceResponse struct {
	Status string `json:"status"`
}

type WhatsAppMediaDownloadResponse struct {
	Status      string `json:"status"`
	MessageID   string `json:"messageId"`
	MediaType   string `json:"mediaType"`
	Filename    string `json:"filename"`
	FilePath    string `json:"filePath"`
	FileSize    int64  `json:"fileSize"`
	DownloadURL string `json:"downloadUrl,omitempty"`
}

type AttachWhatsAppMessageToLeadResponse struct {
	Status              string `json:"status"`
	AttachmentID        string `json:"attachmentId"`
	LeadID              string `json:"leadId"`
	ServiceID           string `json:"serviceId"`
	PhotoAnalysisQueued bool   `json:"photoAnalysisQueued"`
}

type SaveWhatsAppMessagesToLeadResponse struct {
	Status         string  `json:"status"`
	NoteID         string  `json:"noteId"`
	LeadID         string  `json:"leadId"`
	ServiceID      *string `json:"serviceId,omitempty"`
	SavedCount     int     `json:"savedCount"`
	ConversationID string  `json:"conversationId"`
}

type WhatsAppUnreadConversationCountResponse struct {
	Count int `json:"count"`
}

type WhatsAppConversationLeadResponse struct {
	Status       string                       `json:"status"`
	Conversation WhatsAppConversationResponse `json:"conversation"`
}

func ToWhatsAppConversationResponse(item repository.WhatsAppConversation) WhatsAppConversationResponse {
	return WhatsAppConversationResponse{
		ID:                   item.ID.String(),
		LeadID:               optionalUUID(item.LeadID),
		PhoneNumber:          item.PhoneNumber,
		DisplayName:          item.DisplayName,
		LastMessagePreview:   item.LastMessagePreview,
		LastMessageAt:        item.LastMessageAt.Format(time.RFC3339),
		LastMessageDirection: item.LastMessageDirection,
		LastMessageStatus:    item.LastMessageStatus,
		UnreadCount:          item.UnreadCount,
		ArchivedAt:           optionalTimeString(item.ArchivedAt),
		DeletedAt:            optionalTimeString(item.DeletedAt),
		CreatedAt:            item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            item.UpdatedAt.Format(time.RFC3339),
	}
}

func WithWhatsAppConversationLeadState(item WhatsAppConversationResponse, linkedLead, suggestedLead *LeadInboxSummaryResponse) WhatsAppConversationResponse {
	item.LinkedLead = linkedLead
	item.SuggestedLead = suggestedLead
	return item
}

func ToWhatsAppMessageResponse(item repository.WhatsAppMessage) WhatsAppMessageResponse {
	return WhatsAppMessageResponse{
		ID:                item.ID.String(),
		ConversationID:    item.ConversationID.String(),
		LeadID:            optionalUUID(item.LeadID),
		ExternalMessageID: item.ExternalMessageID,
		Direction:         item.Direction,
		Status:            item.Status,
		PhoneNumber:       item.PhoneNumber,
		Body:              item.Body,
		Metadata:          item.Metadata,
		SentAt:            optionalTimeString(item.SentAt),
		ReadAt:            optionalTimeString(item.ReadAt),
		FailedAt:          optionalTimeString(item.FailedAt),
		CreatedAt:         item.CreatedAt.Format(time.RFC3339),
	}
}

func optionalUUID(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	str := value.String()
	return &str
}

func optionalTimeString(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format(time.RFC3339)
	return &formatted
}
