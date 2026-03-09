package transport

import (
	"encoding/json"
	"time"

	"portal_final_backend/internal/identity/repository"
)

type WhatsAppConversationResponse struct {
	ID                   string  `json:"id"`
	LeadID               *string `json:"leadId,omitempty"`
	PhoneNumber          string  `json:"phoneNumber"`
	DisplayName          string  `json:"displayName"`
	LastMessagePreview   string  `json:"lastMessagePreview"`
	LastMessageAt        string  `json:"lastMessageAt"`
	LastMessageDirection string  `json:"lastMessageDirection"`
	LastMessageStatus    string  `json:"lastMessageStatus"`
	UnreadCount          int     `json:"unreadCount"`
	CreatedAt            string  `json:"createdAt"`
	UpdatedAt            string  `json:"updatedAt"`
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

type ListWhatsAppConversationsResponse struct {
	Conversations []WhatsAppConversationResponse `json:"conversations"`
}

type ListWhatsAppMessagesResponse struct {
	Conversation WhatsAppConversationResponse `json:"conversation"`
	Messages     []WhatsAppMessageResponse    `json:"messages"`
}

type SendWhatsAppConversationMessageRequest struct {
	Body string `json:"body" validate:"required,max=4000"`
}

type SendWhatsAppPresenceRequest struct {
	Type string `json:"type" validate:"required,oneof=available unavailable"`
}

type SendWhatsAppChatPresenceRequest struct {
	Action string `json:"action" validate:"required,oneof=start stop"`
}

type SendWhatsAppConversationMessageResponse struct {
	Status       string                       `json:"status"`
	Conversation WhatsAppConversationResponse `json:"conversation"`
	Message      WhatsAppMessageResponse      `json:"message"`
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

type WhatsAppUnreadConversationCountResponse struct {
	Count int `json:"count"`
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
		CreatedAt:            item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            item.UpdatedAt.Format(time.RFC3339),
	}
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

func optionalUUID(value interface{ String() string }) *string {
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
