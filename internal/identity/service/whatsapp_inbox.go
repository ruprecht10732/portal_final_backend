package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"portal_final_backend/internal/identity/repository"
	leadsrepo "portal_final_backend/internal/leads/repository"
	leadstransport "portal_final_backend/internal/leads/transport"
	"portal_final_backend/internal/notification/sse"
	webhookinbox "portal_final_backend/internal/webhook"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/phone"

	"github.com/google/uuid"
)

const conversationNotFoundMsg = "conversation not found"
const whatsappDeviceNotLinkedMsg = "er is geen verbonden WhatsApp-apparaat voor deze organisatie"
const whatsappMessageNotFoundMsg = "message not found"

type SendWhatsAppConversationAttachmentInput struct {
	Filename   string
	Base64Data string
	RemoteURL  string
}

type SendWhatsAppConversationMessageInput struct {
	Type            string
	Body            string
	Scenario        string
	Caption         string
	ViewOnce        bool
	Compress        bool
	IsForwarded     bool
	PushToTalk      bool
	Attachment      *SendWhatsAppConversationAttachmentInput
	ContactName     string
	ContactPhone    string
	Link            string
	Latitude        string
	Longitude       string
	Question        string
	Options         []string
	MaxAnswer       int
	DurationSeconds *int
	AISuggestion    string
}

type WhatsAppConversationActionResult struct {
	Conversation *repository.WhatsAppConversation
	Message      *repository.WhatsAppMessage
}

type WhatsAppMediaDownloadResult struct {
	MessageID   string
	MediaType   string
	Filename    string
	FilePath    string
	FileSize    int64
	DownloadURL string
}

type WhatsAppReplySuggestionResult struct {
	Suggestion        string
	EffectiveScenario string
}

type AttachWhatsAppMessageToLeadResult struct {
	AttachmentID uuid.UUID
	LeadID       uuid.UUID
	ServiceID    uuid.UUID
}

type SaveWhatsAppMessagesToLeadResult struct {
	NoteID     uuid.UUID
	LeadID     uuid.UUID
	ServiceID  *uuid.UUID
	SavedCount int
}

type LeadInboxSummary struct {
	ID       uuid.UUID
	FullName string
	Phone    string
	Email    *string
	City     string
}

func (s *Service) SetSSE(sseService *sse.Service) {
	s.sse = sseService
}

func (s *Service) ListWhatsAppConversations(ctx context.Context, organizationID uuid.UUID, limit, offset int) ([]repository.WhatsAppConversation, error) {
	return s.repo.ListWhatsAppConversations(ctx, organizationID, limit, offset)
}

func (s *Service) GetWhatsAppConversationMessages(ctx context.Context, organizationID, conversationID uuid.UUID, limit int) (repository.WhatsAppConversation, []repository.WhatsAppMessage, error) {
	conversation, err := s.repo.GetWhatsAppConversation(ctx, organizationID, conversationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.WhatsAppConversation{}, nil, apperr.NotFound(conversationNotFoundMsg)
		}
		return repository.WhatsAppConversation{}, nil, err
	}

	messages, err := s.repo.ListWhatsAppMessages(ctx, organizationID, conversationID, limit)
	if err != nil {
		return repository.WhatsAppConversation{}, nil, err
	}

	return conversation, messages, nil
}

func (s *Service) GetWhatsAppConversationLeadState(ctx context.Context, organizationID uuid.UUID, conversation *repository.WhatsAppConversation) (*LeadInboxSummary, *LeadInboxSummary, error) {
	if conversation == nil {
		return nil, nil, nil
	}

	linkedLead, err := s.getLeadSummaryByID(ctx, organizationID, conversation.LeadID)
	if err != nil {
		if apperr.Is(err, apperr.KindNotFound) {
			clearStaleWhatsAppConversationLead(ctx, organizationID, conversation, s.repo.UpdateWhatsAppConversationLead)
		} else {
			return nil, nil, err
		}
	}
	if linkedLead != nil {
		return linkedLead, nil, nil
	}
	if s.leadsRepo == nil {
		return nil, nil, nil
	}
	summary, _, err := s.leadsRepo.GetByPhoneOrEmail(ctx, phone.NormalizeE164(conversation.PhoneNumber), "", organizationID)
	if err != nil || summary == nil {
		return nil, nil, err
	}
	return nil, &LeadInboxSummary{
		ID:       summary.ID,
		FullName: summary.ConsumerName,
		Phone:    summary.ConsumerPhone,
		Email:    summary.ConsumerEmail,
		City:     summary.AddressCity,
	}, nil
}

func (s *Service) LinkWhatsAppConversationLead(ctx context.Context, organizationID, conversationID, leadID uuid.UUID) (repository.WhatsAppConversation, error) {
	if _, err := s.getLeadSummaryByID(ctx, organizationID, &leadID); err != nil {
		return repository.WhatsAppConversation{}, err
	}

	conversation, err := s.repo.UpdateWhatsAppConversationLead(ctx, organizationID, conversationID, &leadID)
	if err == repository.ErrNotFound {
		return repository.WhatsAppConversation{}, apperr.NotFound(conversationNotFoundMsg)
	}
	if err != nil {
		return repository.WhatsAppConversation{}, err
	}
	s.recordWhatsAppLeadTimeline(ctx, organizationID, leadID, "WhatsApp-gesprek gekoppeld", whatsappLinkSummary("gekoppeld", conversation.DisplayName, conversation.PhoneNumber), map[string]any{
		"source":         "whatsapp_inbox",
		"action":         "linked",
		"conversationId": conversation.ID.String(),
		"phoneNumber":    conversation.PhoneNumber,
		"displayName":    strings.TrimSpace(conversation.DisplayName),
	})

	s.publishWhatsAppConversationUpdated(organizationID, conversation)
	return conversation, nil
}

func (s *Service) UnlinkWhatsAppConversationLead(ctx context.Context, organizationID, conversationID uuid.UUID) (repository.WhatsAppConversation, error) {
	existingConversation, err := s.repo.GetWhatsAppConversation(ctx, organizationID, conversationID)
	if err == repository.ErrNotFound {
		return repository.WhatsAppConversation{}, apperr.NotFound(conversationNotFoundMsg)
	}
	if err != nil {
		return repository.WhatsAppConversation{}, err
	}

	conversation, err := s.repo.UpdateWhatsAppConversationLead(ctx, organizationID, conversationID, nil)
	if err == repository.ErrNotFound {
		return repository.WhatsAppConversation{}, apperr.NotFound(conversationNotFoundMsg)
	}
	if err != nil {
		return repository.WhatsAppConversation{}, err
	}
	if existingConversation.LeadID != nil {
		s.recordWhatsAppLeadTimeline(ctx, organizationID, *existingConversation.LeadID, "WhatsApp-gesprek ontkoppeld", whatsappLinkSummary("ontkoppeld", existingConversation.DisplayName, existingConversation.PhoneNumber), map[string]any{
			"source":         "whatsapp_inbox",
			"action":         "unlinked",
			"conversationId": existingConversation.ID.String(),
			"phoneNumber":    existingConversation.PhoneNumber,
			"displayName":    strings.TrimSpace(existingConversation.DisplayName),
		})
	}

	s.publishWhatsAppConversationUpdated(organizationID, conversation)
	return conversation, nil
}

func (s *Service) CreateLeadFromWhatsAppConversation(ctx context.Context, organizationID, conversationID uuid.UUID, req leadstransport.CreateLeadRequest) (repository.WhatsAppConversation, *LeadInboxSummary, error) {
	if s.leadActions == nil {
		return repository.WhatsAppConversation{}, nil, apperr.Internal("lead actions are not configured")
	}

	conversation, err := s.repo.GetWhatsAppConversation(ctx, organizationID, conversationID)
	if err == repository.ErrNotFound {
		return repository.WhatsAppConversation{}, nil, apperr.NotFound(conversationNotFoundMsg)
	}
	if err != nil {
		return repository.WhatsAppConversation{}, nil, err
	}
	if conversation.LeadID != nil {
		return repository.WhatsAppConversation{}, nil, apperr.Validation("gesprek is al gekoppeld aan een lead")
	}

	req.Phone = conversation.PhoneNumber
	req.Source = normalizeInboxLeadSource(req.Source, "whatsapp_inbox")
	if req.WhatsAppOptedIn == nil {
		whatsAppOptedIn := true
		req.WhatsAppOptedIn = &whatsAppOptedIn
	}

	lead, err := s.leadActions.Create(ctx, req, organizationID)
	if err != nil {
		return repository.WhatsAppConversation{}, nil, err
	}

	conversation, err = s.repo.UpdateWhatsAppConversationLead(ctx, organizationID, conversationID, &lead.ID)
	if err != nil {
		return repository.WhatsAppConversation{}, nil, err
	}
	s.recordWhatsAppLeadTimeline(ctx, organizationID, lead.ID, "Lead aangemaakt vanuit WhatsApp inbox", whatsappLinkSummary("aangemaakt", conversation.DisplayName, conversation.PhoneNumber), map[string]any{
		"source":         "whatsapp_inbox",
		"action":         "created_and_linked",
		"conversationId": conversation.ID.String(),
		"phoneNumber":    conversation.PhoneNumber,
		"displayName":    strings.TrimSpace(conversation.DisplayName),
	})

	s.publishWhatsAppConversationUpdated(organizationID, conversation)
	linkedLead, err := s.getLeadSummaryByID(ctx, organizationID, &lead.ID)
	if err != nil {
		return repository.WhatsAppConversation{}, nil, err
	}
	return conversation, linkedLead, nil
}

func clearStaleWhatsAppConversationLead(
	ctx context.Context,
	organizationID uuid.UUID,
	conversation *repository.WhatsAppConversation,
	clear func(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) (repository.WhatsAppConversation, error),
) {
	if conversation == nil || conversation.LeadID == nil {
		return
	}

	staleLeadID := conversation.LeadID.String()
	conversation.LeadID = nil
	if clear == nil {
		return
	}

	cleanedConversation, err := clear(ctx, organizationID, conversation.ID, nil)
	if err != nil {
		log.Printf("whatsapp inbox: failed to clear stale lead link conversation=%s organization=%s lead=%s err=%v", conversation.ID, organizationID, staleLeadID, err)
		return
	}
	conversation.LeadID = cleanedConversation.LeadID
}

func (s *Service) getLeadSummaryByID(ctx context.Context, organizationID uuid.UUID, leadID *uuid.UUID) (*LeadInboxSummary, error) {
	if leadID == nil || s.leadsRepo == nil {
		return nil, nil
	}
	lead, err := s.leadsRepo.GetByID(ctx, *leadID, organizationID)
	if err != nil {
		if errors.Is(err, leadsrepo.ErrNotFound) {
			return nil, apperr.NotFound("lead not found")
		}
		return nil, err
	}
	return &LeadInboxSummary{
		ID:       lead.ID,
		FullName: strings.TrimSpace(lead.ConsumerFirstName + " " + lead.ConsumerLastName),
		Phone:    lead.ConsumerPhone,
		Email:    lead.ConsumerEmail,
		City:     lead.AddressCity,
	}, nil
}

func (s *Service) recordWhatsAppLeadTimeline(ctx context.Context, organizationID, leadID uuid.UUID, title string, summary string, metadata map[string]any) {
	if s.leadActions == nil {
		return
	}
	_, _ = s.leadActions.CreateTimelineEvent(ctx, leadsrepo.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      s.currentLeadServiceID(ctx, leadID, organizationID),
		OrganizationID: organizationID,
		ActorType:      leadsrepo.ActorTypeSystem,
		ActorName:      "WhatsApp inbox",
		EventType:      leadsrepo.EventTypeLeadUpdate,
		Title:          title,
		Summary:        leadTimelineSummary(summary),
		Metadata:       metadata,
	})
}

func (s *Service) currentLeadServiceID(ctx context.Context, leadID, organizationID uuid.UUID) *uuid.UUID {
	if s.leadsRepo == nil {
		return nil
	}
	service, err := s.leadsRepo.GetCurrentLeadService(ctx, leadID, organizationID)
	if err != nil {
		return nil
	}
	serviceID := service.ID
	return &serviceID
}

func whatsappLinkSummary(action string, displayName string, phoneNumber string) string {
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = strings.TrimSpace(phoneNumber)
	}
	if name == "" {
		return "WhatsApp-inboxrelatie bijgewerkt"
	}
	return fmt.Sprintf("WhatsApp-gesprek met %s %s via de inbox", name, action)
}

func normalizeInboxLeadSource(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func leadTimelineSummary(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > leadsrepo.TimelineSummaryMaxLen {
		trimmed = strings.TrimSpace(trimmed[:leadsrepo.TimelineSummaryMaxLen])
	}
	return &trimmed
}

func (s *Service) SuggestWhatsAppReply(ctx context.Context, requesterUserID, organizationID, conversationID uuid.UUID, scenario, scenarioNotes string) (WhatsAppReplySuggestionResult, error) {
	if s.whatsappReplyer == nil {
		return WhatsAppReplySuggestionResult{}, apperr.Internal("WhatsApp reply agent is not configured")
	}

	conversation, messages, err := s.GetWhatsAppConversationMessages(ctx, organizationID, conversationID, 30)
	if err != nil {
		return WhatsAppReplySuggestionResult{}, err
	}

	input := SuggestWhatsAppReplyInput{
		OrganizationID:  organizationID,
		RequesterUserID: requesterUserID,
		LeadID:          conversation.LeadID,
		ConversationID:  conversation.ID,
		Scenario:        scenario,
		ScenarioNotes:   strings.TrimSpace(scenarioNotes),
		PhoneNumber:     conversation.PhoneNumber,
		DisplayName:     conversation.DisplayName,
		Messages:        make([]SuggestWhatsAppReplyMessage, 0, len(messages)),
	}
	if feedbackItems, feedbackErr := s.repo.ListRecentAppliedWhatsAppReplyFeedback(ctx, organizationID, conversation.LeadID, conversation.ID, 4); feedbackErr == nil {
		input.Feedback = make([]SuggestWhatsAppReplyFeedback, 0, len(feedbackItems))
		for _, item := range feedbackItems {
			input.Feedback = append(input.Feedback, SuggestWhatsAppReplyFeedback{
				AIReply:    item.AIReply,
				HumanReply: item.HumanReply,
				CreatedAt:  item.CreatedAt,
			})
		}
	}
	if examples, examplesErr := s.repo.ListRecentWhatsAppReplyExamples(ctx, organizationID, conversation.LeadID, conversation.ID, 4); examplesErr == nil {
		input.Examples = make([]SuggestWhatsAppReplyExample, 0, len(examples))
		for _, example := range examples {
			input.Examples = append(input.Examples, SuggestWhatsAppReplyExample{
				CustomerMessage: example.CustomerMessage,
				Reply:           example.Reply,
				CreatedAt:       example.CreatedAt,
			})
		}
	}
	for _, message := range messages {
		input.Messages = append(input.Messages, SuggestWhatsAppReplyMessage{
			Direction: message.Direction,
			Body:      message.Body,
			CreatedAt: message.CreatedAt,
		})
	}

	draft, err := s.whatsappReplyer.SuggestReply(ctx, input)
	if err != nil {
		return WhatsAppReplySuggestionResult{}, apperr.Internal("WhatsApp reply kon niet worden gegenereerd")
	}
	if strings.TrimSpace(draft.Text) == "" {
		return WhatsAppReplySuggestionResult{}, apperr.Internal("WhatsApp reply agent returned an empty suggestion")
	}

	return WhatsAppReplySuggestionResult{
		Suggestion:        strings.TrimSpace(draft.Text),
		EffectiveScenario: string(draft.EffectiveScenario),
	}, nil
}

func (s *Service) MarkWhatsAppConversationRead(ctx context.Context, organizationID, conversationID uuid.UUID) (bool, error) {
	providerSynced := false
	readTarget, err := s.repo.GetLatestUnreadWhatsAppReadSyncTarget(ctx, organizationID, conversationID)
	if err != nil {
		return false, err
	}
	if readTarget != nil {
		deviceID, deviceErr := s.getRequiredWhatsAppDeviceID(ctx, organizationID)
		if deviceErr == nil {
			if providerErr := s.whatsapp.MarkMessageRead(ctx, deviceID, readTarget.PhoneNumber, readTarget.ExternalMessageID); providerErr == nil {
				providerSynced = true
			}
		}
	}

	err = s.repo.MarkWhatsAppConversationRead(ctx, organizationID, conversationID)
	if err == repository.ErrNotFound {
		return false, apperr.NotFound(conversationNotFoundMsg)
	}
	if err != nil {
		return false, err
	}

	conversation, convErr := s.repo.GetWhatsAppConversation(ctx, organizationID, conversationID)
	if convErr == nil {
		s.publishWhatsAppConversationUpdated(organizationID, conversation)
	}

	return providerSynced, nil
}

func (s *Service) SendWhatsAppConversationMessage(ctx context.Context, organizationID, conversationID uuid.UUID, input SendWhatsAppConversationMessageInput) (repository.WhatsAppConversation, repository.WhatsAppMessage, error) {
	conversation, err := s.repo.GetWhatsAppConversation(ctx, organizationID, conversationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.NotFound(conversationNotFoundMsg)
		}
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}

	deviceID, err := s.getRequiredWhatsAppDeviceID(ctx, organizationID)
	if err != nil {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}

	outgoing, err := buildOutgoingWhatsAppMessage(input)
	if err != nil {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}

	result, err := s.sendWhatsAppConversationMessageByType(ctx, deviceID, conversation.PhoneNumber, outgoing)
	if err != nil {
		if errors.Is(err, whatsapp.ErrNoDevice) {
			return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.Validation(whatsappDeviceNotLinkedMsg)
		}
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.Internal("WhatsApp-bericht kon niet worden verstuurd")
	}

	updatedConversation, message, err := s.persistOutgoingWhatsAppMessage(ctx, organizationID, conversation.LeadID, conversation.PhoneNumber, outgoing.Preview, nilIfEmptyString(result.MessageID), outgoing.Metadata)
	if err != nil {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}

	s.captureWhatsAppReplyFeedback(ctx, conversation, input)

	return updatedConversation, message, nil
}

func (s *Service) StartWhatsAppConversationMessage(ctx context.Context, organizationID uuid.UUID, leadID *uuid.UUID, phoneNumber string, input SendWhatsAppConversationMessageInput) (repository.WhatsAppConversation, repository.WhatsAppMessage, error) {
	if leadID != nil {
		if _, err := s.getLeadSummaryByID(ctx, organizationID, leadID); err != nil {
			return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
		}
	}

	deviceID, err := s.getRequiredWhatsAppDeviceID(ctx, organizationID)
	if err != nil {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}

	outgoing, err := buildOutgoingWhatsAppMessage(input)
	if err != nil {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}

	result, err := s.sendWhatsAppConversationMessageByType(ctx, deviceID, phoneNumber, outgoing)
	if err != nil {
		if errors.Is(err, whatsapp.ErrNoDevice) {
			return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.Validation(whatsappDeviceNotLinkedMsg)
		}
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.Internal("WhatsApp-bericht kon niet worden verstuurd")
	}

	updatedConversation, message, err := s.persistOutgoingWhatsAppMessage(ctx, organizationID, leadID, phoneNumber, outgoing.Preview, nilIfEmptyString(result.MessageID), outgoing.Metadata)
	if err != nil {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}

	return updatedConversation, message, nil
}

func (s *Service) captureWhatsAppReplyFeedback(ctx context.Context, conversation repository.WhatsAppConversation, input SendWhatsAppConversationMessageInput) {
	params, ok := buildWhatsAppReplyFeedbackParams(conversation, input)
	if !ok {
		return
	}
	_, _ = s.repo.CreateWhatsAppReplyFeedback(ctx, params)
}

func buildWhatsAppReplyFeedbackParams(conversation repository.WhatsAppConversation, input SendWhatsAppConversationMessageInput) (repository.CreateWhatsAppReplyFeedbackParams, bool) {
	if conversation.LeadID == nil {
		return repository.CreateWhatsAppReplyFeedbackParams{}, false
	}
	if input.Type != "" && input.Type != "text" {
		return repository.CreateWhatsAppReplyFeedbackParams{}, false
	}
	aiReply := strings.TrimSpace(input.AISuggestion)
	humanReply := strings.TrimSpace(input.Body)
	if aiReply == "" || humanReply == "" {
		return repository.CreateWhatsAppReplyFeedbackParams{}, false
	}

	return repository.CreateWhatsAppReplyFeedbackParams{
		OrganizationID: conversation.OrganizationID,
		ConversationID: conversation.ID,
		LeadID:         *conversation.LeadID,
		Scenario:       strings.TrimSpace(input.Scenario),
		AIReply:        aiReply,
		HumanReply:     humanReply,
		WasEdited:      aiReply != humanReply,
	}, true
}

func (s *Service) ReceiveIncomingWhatsAppMessage(ctx context.Context, input webhookinbox.IncomingWhatsAppMessage) (bool, error) {
	conversation, message, created, err := s.repo.RecordIncomingWhatsAppMessage(ctx, repository.WhatsAppIncomingMessageParams{
		OrganizationID:    input.OrganizationID,
		PhoneNumber:       input.PhoneNumber,
		DisplayName:       input.DisplayName,
		ExternalMessageID: input.ExternalMessageID,
		Body:              input.Body,
		Metadata:          input.Metadata,
		ReceivedAt:        input.ReceivedAt,
	})
	if err != nil {
		return false, err
	}
	if !created {
		return false, nil
	}

	s.publishWhatsAppMessageReceived(input.OrganizationID, conversation, message)
	s.publishWhatsAppConversationUpdated(input.OrganizationID, conversation)
	return true, nil
}

func (s *Service) PersistIncomingWhatsAppMessage(ctx context.Context, organizationID uuid.UUID, phoneNumber, displayName, body string, externalMessageID *string, metadata []byte) error {
	_, err := s.ReceiveIncomingWhatsAppMessage(ctx, webhookinbox.IncomingWhatsAppMessage{
		OrganizationID:    organizationID,
		PhoneNumber:       phoneNumber,
		DisplayName:       displayName,
		ExternalMessageID: externalMessageID,
		Body:              body,
		Metadata:          json.RawMessage(metadata),
	})
	return err
}

func (s *Service) UpdateIncomingWhatsAppMessage(ctx context.Context, organizationID uuid.UUID, externalMessageID, body string, metadata []byte) error {
	conversation, message, err := s.repo.UpdateWhatsAppMessageByExternalID(ctx, organizationID, externalMessageID, body, json.RawMessage(metadata))
	if err != nil {
		if err == repository.ErrNotFound {
			return nil
		}
		return err
	}
	s.publishWhatsAppMessageUpdated(organizationID, conversation, message)
	s.publishWhatsAppConversationUpdated(organizationID, conversation)
	return nil
}

func (s *Service) SyncOutgoingWhatsAppMessage(ctx context.Context, input webhookinbox.OutgoingWhatsAppMessage) (bool, error) {
	conversation, message, created, err := s.repo.SyncSentWhatsAppMessage(ctx, repository.WhatsAppOutgoingMessageParams{
		OrganizationID:    input.OrganizationID,
		PhoneNumber:       input.PhoneNumber,
		Body:              input.Body,
		ExternalMessageID: input.ExternalMessageID,
		Metadata:          input.Metadata,
		SentAt:            input.SentAt,
	})
	if err != nil {
		return false, err
	}
	if !created {
		return false, nil
	}

	s.publishWhatsAppMessageSent(input.OrganizationID, conversation, message)
	s.publishWhatsAppConversationUpdated(input.OrganizationID, conversation)
	return true, nil
}

func (s *Service) CountUnreadWhatsAppConversations(ctx context.Context, organizationID uuid.UUID) (int, error) {
	return s.repo.CountUnreadWhatsAppConversations(ctx, organizationID)
}

func (s *Service) SendWhatsAppPresence(ctx context.Context, organizationID uuid.UUID, presenceType string) error {
	deviceID, err := s.getRequiredWhatsAppDeviceID(ctx, organizationID)
	if err != nil {
		return err
	}

	trimmedType := strings.ToLower(strings.TrimSpace(presenceType))
	if trimmedType != "available" && trimmedType != "unavailable" {
		return apperr.Validation("ongeldig presence-type")
	}

	if err := s.whatsapp.SendPresence(ctx, deviceID, trimmedType); err != nil {
		return apperr.Internal("WhatsApp presence kon niet worden verstuurd")
	}
	if _, err := s.repo.UpsertOrganizationSettings(ctx, organizationID, repository.OrganizationSettingsUpdate{
		WhatsAppPresence: &trimmedType,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) SendWhatsAppChatPresence(ctx context.Context, organizationID, conversationID uuid.UUID, action string) error {
	conversation, err := s.repo.GetWhatsAppConversation(ctx, organizationID, conversationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return apperr.NotFound(conversationNotFoundMsg)
		}
		return err
	}

	deviceID, err := s.getRequiredWhatsAppDeviceID(ctx, organizationID)
	if err != nil {
		return err
	}

	trimmedAction := strings.ToLower(strings.TrimSpace(action))
	if trimmedAction != "start" && trimmedAction != "stop" {
		return apperr.Validation("ongeldige chat presence-actie")
	}

	if err := s.whatsapp.SendChatPresence(ctx, deviceID, conversation.PhoneNumber, trimmedAction); err != nil {
		return apperr.Internal("WhatsApp typing-indicator kon niet worden verstuurd")
	}
	return nil
}

func (s *Service) ReactWhatsAppMessage(ctx context.Context, organizationID, conversationID uuid.UUID, externalMessageID string, emoji string) (WhatsAppConversationActionResult, error) {
	message, conversation, deviceID, err := s.getWhatsAppMessageActionContext(ctx, organizationID, conversationID, externalMessageID)
	if err != nil {
		return WhatsAppConversationActionResult{}, err
	}

	reaction := strings.TrimSpace(emoji)
	if reaction == "" {
		return WhatsAppConversationActionResult{}, apperr.Validation("emoji is verplicht")
	}
	if err := s.whatsapp.ReactMessage(ctx, deviceID, externalMessageID, whatsapp.ReactMessageInput{PhoneNumber: conversation.PhoneNumber, Emoji: reaction}); err != nil {
		return WhatsAppConversationActionResult{}, apperr.Internal("WhatsApp-reactie kon niet worden verstuurd")
	}

	updatedConversation, updatedMessage, err := s.applyLocalWhatsAppMutation(ctx, organizationID, message, conversation, "message.reaction", nil, &reaction)
	if err != nil {
		return WhatsAppConversationActionResult{}, err
	}
	return WhatsAppConversationActionResult{Conversation: &updatedConversation, Message: &updatedMessage}, nil
}

func (s *Service) EditWhatsAppMessage(ctx context.Context, organizationID, conversationID uuid.UUID, externalMessageID string, body string) (WhatsAppConversationActionResult, error) {
	message, conversation, deviceID, err := s.getWhatsAppMessageActionContext(ctx, organizationID, conversationID, externalMessageID)
	if err != nil {
		return WhatsAppConversationActionResult{}, err
	}
	if err := requireOutboundWhatsAppMessage(message, "bewerken"); err != nil {
		return WhatsAppConversationActionResult{}, err
	}

	updatedBody := strings.TrimSpace(body)
	if updatedBody == "" {
		return WhatsAppConversationActionResult{}, apperr.Validation("WhatsApp-bericht is leeg")
	}
	if err := s.whatsapp.UpdateMessage(ctx, deviceID, externalMessageID, whatsapp.UpdateMessageInput{PhoneNumber: conversation.PhoneNumber, Message: updatedBody}); err != nil {
		return WhatsAppConversationActionResult{}, apperr.Internal("WhatsApp-bericht kon niet worden bijgewerkt")
	}

	updatedConversation, updatedMessage, err := s.applyLocalWhatsAppMutation(ctx, organizationID, message, conversation, "message.edited", &updatedBody, nil)
	if err != nil {
		return WhatsAppConversationActionResult{}, err
	}
	return WhatsAppConversationActionResult{Conversation: &updatedConversation, Message: &updatedMessage}, nil
}

func (s *Service) DeleteWhatsAppMessage(ctx context.Context, organizationID, conversationID uuid.UUID, externalMessageID string) (WhatsAppConversationActionResult, error) {
	message, conversation, deviceID, err := s.getWhatsAppMessageActionContext(ctx, organizationID, conversationID, externalMessageID)
	if err != nil {
		return WhatsAppConversationActionResult{}, err
	}
	if err := requireOutboundWhatsAppMessage(message, "verwijderen"); err != nil {
		return WhatsAppConversationActionResult{}, err
	}

	if err := s.whatsapp.DeleteMessage(ctx, deviceID, externalMessageID, whatsapp.MessageTargetInput{PhoneNumber: conversation.PhoneNumber}); err != nil {
		return WhatsAppConversationActionResult{}, apperr.Internal("WhatsApp-bericht kon niet worden verwijderd")
	}

	updatedConversation, updatedMessage, err := s.applyLocalWhatsAppMutation(ctx, organizationID, message, conversation, "message.deleted", nil, nil)
	if err != nil {
		return WhatsAppConversationActionResult{}, err
	}
	return WhatsAppConversationActionResult{Conversation: &updatedConversation, Message: &updatedMessage}, nil
}

func (s *Service) RevokeWhatsAppMessage(ctx context.Context, organizationID, conversationID uuid.UUID, externalMessageID string) (WhatsAppConversationActionResult, error) {
	message, conversation, deviceID, err := s.getWhatsAppMessageActionContext(ctx, organizationID, conversationID, externalMessageID)
	if err != nil {
		return WhatsAppConversationActionResult{}, err
	}
	if err := requireOutboundWhatsAppMessage(message, "intrekken"); err != nil {
		return WhatsAppConversationActionResult{}, err
	}

	if err := s.whatsapp.RevokeMessage(ctx, deviceID, externalMessageID, whatsapp.MessageTargetInput{PhoneNumber: conversation.PhoneNumber}); err != nil {
		return WhatsAppConversationActionResult{}, apperr.Internal("WhatsApp-bericht kon niet worden ingetrokken")
	}

	updatedConversation, updatedMessage, err := s.applyLocalWhatsAppMutation(ctx, organizationID, message, conversation, "message.revoked", nil, nil)
	if err != nil {
		return WhatsAppConversationActionResult{}, err
	}
	return WhatsAppConversationActionResult{Conversation: &updatedConversation, Message: &updatedMessage}, nil
}

func (s *Service) StarWhatsAppMessage(ctx context.Context, organizationID, conversationID uuid.UUID, externalMessageID string, value bool) (WhatsAppConversationActionResult, error) {
	message, conversation, deviceID, err := s.getWhatsAppMessageActionContext(ctx, organizationID, conversationID, externalMessageID)
	if err != nil {
		return WhatsAppConversationActionResult{}, err
	}

	if err := s.whatsapp.StarMessage(ctx, deviceID, externalMessageID, whatsapp.MessageStarInput{PhoneNumber: conversation.PhoneNumber, Value: value}); err != nil {
		if value {
			return WhatsAppConversationActionResult{}, apperr.Internal("WhatsApp-bericht kon niet worden gemarkeerd")
		}
		return WhatsAppConversationActionResult{}, apperr.Internal("WhatsApp-bericht kon niet worden gedemarkeerd")
	}

	return WhatsAppConversationActionResult{Conversation: &conversation, Message: &message}, nil
}

func (s *Service) DownloadWhatsAppMessageMedia(ctx context.Context, organizationID, conversationID uuid.UUID, externalMessageID string) (WhatsAppMediaDownloadResult, error) {
	message, conversation, deviceID, err := s.getWhatsAppMessageActionContext(ctx, organizationID, conversationID, externalMessageID)
	if err != nil {
		return WhatsAppMediaDownloadResult{}, err
	}
	if override := whatsAppMessageDeviceOverride(message.Metadata); override != "" {
		deviceID = override
	}

	result, err := s.whatsapp.DownloadMedia(ctx, deviceID, externalMessageID, conversation.PhoneNumber)
	if err != nil {
		return WhatsAppMediaDownloadResult{}, apperr.Internal("WhatsApp-media kon niet worden gedownload")
	}

	return WhatsAppMediaDownloadResult{
		MessageID:   result.MessageID,
		MediaType:   result.MediaType,
		Filename:    result.Filename,
		FilePath:    result.FilePath,
		FileSize:    result.FileSize,
		DownloadURL: result.DownloadURL,
	}, nil
}

func (s *Service) AttachWhatsAppMessageToLead(ctx context.Context, organizationID, authorID, conversationID uuid.UUID, externalMessageID string, requestedServiceID *uuid.UUID) (AttachWhatsAppMessageToLeadResult, error) {
	if s.leadActions == nil || s.storage == nil || s.whatsapp == nil || strings.TrimSpace(s.attachmentsBucket) == "" {
		return AttachWhatsAppMessageToLeadResult{}, apperr.Internal("WhatsApp lead import is not configured")
	}

	message, conversation, err := s.getAttachableWhatsAppImage(ctx, organizationID, conversationID, externalMessageID)
	if err != nil {
		return AttachWhatsAppMessageToLeadResult{}, err
	}

	serviceID, err := s.leadActions.ResolveServiceID(ctx, *conversation.LeadID, organizationID, requestedServiceID)
	if err != nil {
		return AttachWhatsAppMessageToLeadResult{}, err
	}

	attachment, err := s.importWhatsAppImageAttachment(ctx, organizationID, authorID, serviceID, externalMessageID, message, conversation)
	if err != nil {
		return AttachWhatsAppMessageToLeadResult{}, err
	}

	return AttachWhatsAppMessageToLeadResult{AttachmentID: attachment.AttachmentID, LeadID: *conversation.LeadID, ServiceID: serviceID}, nil
}

func (s *Service) SaveWhatsAppMessagesToLead(ctx context.Context, organizationID, authorID, conversationID uuid.UUID, externalMessageIDs []string, requestedServiceID *uuid.UUID) (SaveWhatsAppMessagesToLeadResult, error) {
	if s.leadActions == nil {
		return SaveWhatsAppMessagesToLeadResult{}, apperr.Internal("WhatsApp lead capture is not configured")
	}

	conversation, err := s.repo.GetWhatsAppConversation(ctx, organizationID, conversationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return SaveWhatsAppMessagesToLeadResult{}, apperr.NotFound(conversationNotFoundMsg)
		}
		return SaveWhatsAppMessagesToLeadResult{}, err
	}
	if conversation.LeadID == nil {
		return SaveWhatsAppMessagesToLeadResult{}, apperr.Validation("gesprek is nog niet gekoppeld aan een lead")
	}

	trimmedIDs := uniqueTrimmedStrings(externalMessageIDs)
	if len(trimmedIDs) == 0 {
		return SaveWhatsAppMessagesToLeadResult{}, apperr.Validation("selecteer minimaal één WhatsApp-bericht")
	}

	messages, err := s.loadWhatsAppMessagesForConversation(ctx, organizationID, conversationID, trimmedIDs)
	if err != nil {
		return SaveWhatsAppMessagesToLeadResult{}, err
	}

	noteBody := buildImportantWhatsAppMessagesNote(conversation, messages)
	metadata := map[string]any{
		"source":               "whatsapp",
		"conversationId":       conversationID.String(),
		"messageIds":           trimmedIDs,
		"capturedMessageCount": len(messages),
		"phoneNumber":          conversation.PhoneNumber,
	}
	if displayName := strings.TrimSpace(conversation.DisplayName); displayName != "" {
		metadata["displayName"] = displayName
	}

	result, err := s.leadActions.CreateImportantNote(ctx, CreateImportantLeadNoteParams{
		LeadID:         *conversation.LeadID,
		ServiceID:      requestedServiceID,
		OrganizationID: organizationID,
		AuthorID:       authorID,
		Body:           noteBody,
		Metadata:       metadata,
	})
	if err != nil {
		return SaveWhatsAppMessagesToLeadResult{}, err
	}

	return SaveWhatsAppMessagesToLeadResult{NoteID: result.NoteID, LeadID: *conversation.LeadID, ServiceID: result.ServiceID, SavedCount: len(messages)}, nil
}

func (s *Service) ArchiveWhatsAppConversation(ctx context.Context, organizationID, conversationID uuid.UUID, value bool) (WhatsAppConversationActionResult, error) {
	conversation, deviceID, err := s.getWhatsAppConversationActionContext(ctx, organizationID, conversationID)
	if err != nil {
		return WhatsAppConversationActionResult{}, err
	}
	if err := s.whatsapp.ArchiveChat(ctx, deviceID, conversation.PhoneNumber, value); err != nil {
		return WhatsAppConversationActionResult{}, apperr.Internal("WhatsApp-gesprek kon niet worden gearchiveerd")
	}
	updatedConversation, err := s.repo.SetWhatsAppConversationArchived(ctx, organizationID, conversationID, value)
	if err != nil {
		if err == repository.ErrNotFound {
			return WhatsAppConversationActionResult{}, apperr.NotFound(conversationNotFoundMsg)
		}
		return WhatsAppConversationActionResult{}, err
	}
	s.publishWhatsAppConversationUpdated(organizationID, updatedConversation)
	return WhatsAppConversationActionResult{Conversation: &updatedConversation}, nil
}

func (s *Service) DeleteWhatsAppConversation(ctx context.Context, organizationID, conversationID uuid.UUID) (WhatsAppConversationActionResult, error) {
	conversation, err := s.repo.DeleteWhatsAppConversation(ctx, organizationID, conversationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return WhatsAppConversationActionResult{}, apperr.NotFound(conversationNotFoundMsg)
		}
		return WhatsAppConversationActionResult{}, err
	}
	s.publishWhatsAppConversationUpdated(organizationID, conversation)
	return WhatsAppConversationActionResult{Conversation: &conversation}, nil
}

func (s *Service) PinWhatsAppConversation(ctx context.Context, organizationID, conversationID uuid.UUID, value bool) (WhatsAppConversationActionResult, error) {
	conversation, deviceID, err := s.getWhatsAppConversationActionContext(ctx, organizationID, conversationID)
	if err != nil {
		return WhatsAppConversationActionResult{}, err
	}
	if err := s.whatsapp.PinChat(ctx, deviceID, conversation.PhoneNumber, value); err != nil {
		return WhatsAppConversationActionResult{}, apperr.Internal("WhatsApp-gesprek kon niet worden vastgezet")
	}
	return WhatsAppConversationActionResult{Conversation: &conversation}, nil
}

func (s *Service) SetWhatsAppDisappearingTimer(ctx context.Context, organizationID, conversationID uuid.UUID, timerSeconds int) (WhatsAppConversationActionResult, error) {
	conversation, deviceID, err := s.getWhatsAppConversationActionContext(ctx, organizationID, conversationID)
	if err != nil {
		return WhatsAppConversationActionResult{}, err
	}
	if timerSeconds < 0 {
		return WhatsAppConversationActionResult{}, apperr.Validation("timerSeconds moet 0 of hoger zijn")
	}
	if err := s.whatsapp.SetDisappearingTimer(ctx, deviceID, conversation.PhoneNumber, timerSeconds); err != nil {
		return WhatsAppConversationActionResult{}, apperr.Internal("WhatsApp disappearing timer kon niet worden ingesteld")
	}
	return WhatsAppConversationActionResult{Conversation: &conversation}, nil
}

func (s *Service) PersistOutgoingWhatsAppMessage(ctx context.Context, organizationID uuid.UUID, leadID *uuid.UUID, phoneNumber string, body string, externalMessageID *string) error {
	_, _, err := s.persistOutgoingWhatsAppMessage(ctx, organizationID, leadID, phoneNumber, body, externalMessageID, nil)
	return err
}

func (s *Service) persistOutgoingWhatsAppMessage(ctx context.Context, organizationID uuid.UUID, leadID *uuid.UUID, phoneNumber string, body string, externalMessageID *string, metadata json.RawMessage) (repository.WhatsAppConversation, repository.WhatsAppMessage, error) {
	conversation, message, err := s.repo.RecordSentWhatsAppMessage(ctx, repository.WhatsAppOutgoingMessageParams{
		OrganizationID:    organizationID,
		LeadID:            leadID,
		PhoneNumber:       phoneNumber,
		Body:              body,
		ExternalMessageID: externalMessageID,
		Metadata:          metadata,
	})
	if err != nil {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}

	s.publishWhatsAppMessageSent(organizationID, conversation, message)
	s.publishWhatsAppConversationUpdated(organizationID, conversation)

	return conversation, message, nil
}

func (s *Service) ApplyWhatsAppMessageReceipt(ctx context.Context, organizationID uuid.UUID, externalMessageIDs []string, receiptType string, receiptAt *time.Time) (bool, error) {
	conversations, messages, err := s.repo.ApplyWhatsAppMessageReceipt(ctx, organizationID, externalMessageIDs, receiptType, receiptAt)
	if err != nil {
		return false, err
	}
	if len(messages) == 0 {
		return false, nil
	}

	conversationsByID := make(map[uuid.UUID]repository.WhatsAppConversation, len(conversations))
	for _, conversation := range conversations {
		conversationsByID[conversation.ID] = conversation
	}
	for _, message := range messages {
		conversation, ok := conversationsByID[message.ConversationID]
		if !ok {
			conversation, err = s.repo.GetWhatsAppConversation(ctx, organizationID, message.ConversationID)
			if err != nil {
				continue
			}
		}
		s.publishWhatsAppMessageSent(organizationID, conversation, message)
	}
	for _, conversation := range conversations {
		s.publishWhatsAppConversationUpdated(organizationID, conversation)
	}

	return true, nil
}

func (s *Service) ApplyWhatsAppMessageMutation(ctx context.Context, input webhookinbox.WhatsAppMessageMutation) (bool, error) {
	conversation, message, applied, err := s.repo.ApplyWhatsAppMessageMutation(ctx, repository.WhatsAppMessageMutationParams{
		OrganizationID:          input.OrganizationID,
		EventType:               input.EventType,
		TargetExternalMessageID: input.TargetExternalMessageID,
		PhoneNumber:             input.PhoneNumber,
		ActorJID:                input.ActorJID,
		ActorName:               input.ActorName,
		EventMessageID:          input.EventMessageID,
		Body:                    input.Body,
		Reaction:                input.Reaction,
		Metadata:                input.Metadata,
		OccurredAt:              input.OccurredAt,
		IsFromMe:                input.IsFromMe,
	})
	if err != nil {
		return false, err
	}
	if !applied {
		return false, nil
	}

	s.publishWhatsAppMessageUpdated(input.OrganizationID, conversation, message)
	s.publishWhatsAppConversationUpdated(input.OrganizationID, conversation)
	return true, nil
}

func (s *Service) publishWhatsAppMessageSent(organizationID uuid.UUID, conversation repository.WhatsAppConversation, message repository.WhatsAppMessage) {
	if s.sse == nil {
		return
	}

	event := sse.Event{
		Type:    sse.EventWhatsAppMessageSent,
		Message: "WhatsApp message sent",
		Data: map[string]any{
			"conversation": map[string]any{
				"id":                   conversation.ID.String(),
				"leadId":               optionalUUIDString(conversation.LeadID),
				"phoneNumber":          conversation.PhoneNumber,
				"displayName":          conversation.DisplayName,
				"lastMessagePreview":   conversation.LastMessagePreview,
				"lastMessageAt":        conversation.LastMessageAt,
				"lastMessageDirection": conversation.LastMessageDirection,
				"lastMessageStatus":    conversation.LastMessageStatus,
				"unreadCount":          conversation.UnreadCount,
				"archivedAt":           optionalTimeValue(conversation.ArchivedAt),
				"deletedAt":            optionalTimeValue(conversation.DeletedAt),
			},
			"message": map[string]any{
				"id":                message.ID.String(),
				"conversationId":    message.ConversationID.String(),
				"leadId":            optionalUUIDString(message.LeadID),
				"externalMessageId": message.ExternalMessageID,
				"direction":         message.Direction,
				"status":            message.Status,
				"phoneNumber":       message.PhoneNumber,
				"body":              message.Body,
				"metadata":          message.Metadata,
				"createdAt":         message.CreatedAt,
				"sentAt":            message.SentAt,
			},
		},
	}
	if conversation.LeadID != nil {
		event.LeadID = *conversation.LeadID
	}

	s.sse.PublishToOrganization(organizationID, event)
}

func (s *Service) publishWhatsAppMessageReceived(organizationID uuid.UUID, conversation repository.WhatsAppConversation, message repository.WhatsAppMessage) {
	if s.sse == nil {
		return
	}

	event := sse.Event{
		Type:    sse.EventWhatsAppMessageReceived,
		Message: "WhatsApp message received",
		Data: map[string]any{
			"conversation": map[string]any{
				"id":                   conversation.ID.String(),
				"leadId":               optionalUUIDString(conversation.LeadID),
				"phoneNumber":          conversation.PhoneNumber,
				"displayName":          conversation.DisplayName,
				"lastMessagePreview":   conversation.LastMessagePreview,
				"lastMessageAt":        conversation.LastMessageAt,
				"lastMessageDirection": conversation.LastMessageDirection,
				"lastMessageStatus":    conversation.LastMessageStatus,
				"unreadCount":          conversation.UnreadCount,
				"archivedAt":           optionalTimeValue(conversation.ArchivedAt),
				"deletedAt":            optionalTimeValue(conversation.DeletedAt),
			},
			"message": map[string]any{
				"id":                message.ID.String(),
				"conversationId":    message.ConversationID.String(),
				"leadId":            optionalUUIDString(message.LeadID),
				"externalMessageId": message.ExternalMessageID,
				"direction":         message.Direction,
				"status":            message.Status,
				"phoneNumber":       message.PhoneNumber,
				"body":              message.Body,
				"metadata":          message.Metadata,
				"createdAt":         message.CreatedAt,
				"sentAt":            message.SentAt,
				"readAt":            message.ReadAt,
			},
		},
	}
	if conversation.LeadID != nil {
		event.LeadID = *conversation.LeadID
	}

	s.sse.PublishToOrganization(organizationID, event)
}

func (s *Service) publishWhatsAppConversationUpdated(organizationID uuid.UUID, conversation repository.WhatsAppConversation) {
	if s.sse == nil {
		return
	}

	event := sse.Event{
		Type:    sse.EventWhatsAppConversationUpdated,
		Message: "WhatsApp conversation updated",
		Data: map[string]any{
			"conversation": map[string]any{
				"id":                   conversation.ID.String(),
				"leadId":               optionalUUIDString(conversation.LeadID),
				"phoneNumber":          conversation.PhoneNumber,
				"displayName":          conversation.DisplayName,
				"lastMessagePreview":   conversation.LastMessagePreview,
				"lastMessageAt":        conversation.LastMessageAt,
				"lastMessageDirection": conversation.LastMessageDirection,
				"lastMessageStatus":    conversation.LastMessageStatus,
				"unreadCount":          conversation.UnreadCount,
				"archivedAt":           optionalTimeValue(conversation.ArchivedAt),
				"deletedAt":            optionalTimeValue(conversation.DeletedAt),
			},
		},
	}
	if conversation.LeadID != nil {
		event.LeadID = *conversation.LeadID
	}

	s.sse.PublishToOrganization(organizationID, event)
}

func (s *Service) publishWhatsAppMessageUpdated(organizationID uuid.UUID, conversation repository.WhatsAppConversation, message repository.WhatsAppMessage) {
	if s.sse == nil {
		return
	}

	event := sse.Event{
		Type:    sse.EventWhatsAppMessageUpdated,
		Message: "WhatsApp message updated",
		Data: map[string]any{
			"conversation": map[string]any{
				"id":                   conversation.ID.String(),
				"leadId":               optionalUUIDString(conversation.LeadID),
				"phoneNumber":          conversation.PhoneNumber,
				"displayName":          conversation.DisplayName,
				"lastMessagePreview":   conversation.LastMessagePreview,
				"lastMessageAt":        conversation.LastMessageAt,
				"lastMessageDirection": conversation.LastMessageDirection,
				"lastMessageStatus":    conversation.LastMessageStatus,
				"unreadCount":          conversation.UnreadCount,
				"archivedAt":           optionalTimeValue(conversation.ArchivedAt),
				"deletedAt":            optionalTimeValue(conversation.DeletedAt),
			},
			"message": map[string]any{
				"id":                message.ID.String(),
				"conversationId":    message.ConversationID.String(),
				"leadId":            optionalUUIDString(message.LeadID),
				"externalMessageId": message.ExternalMessageID,
				"direction":         message.Direction,
				"status":            message.Status,
				"phoneNumber":       message.PhoneNumber,
				"body":              message.Body,
				"metadata":          message.Metadata,
				"createdAt":         message.CreatedAt,
				"sentAt":            message.SentAt,
				"readAt":            message.ReadAt,
				"failedAt":          message.FailedAt,
			},
		},
	}
	if conversation.LeadID != nil {
		event.LeadID = *conversation.LeadID
	}

	s.sse.PublishToOrganization(organizationID, event)
}

func optionalUUIDString(value *uuid.UUID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func optionalTimeValue(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.Format(time.RFC3339)
}

func nilIfEmptyString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

type outgoingWhatsAppMessage struct {
	Type         string
	Preview      string
	Metadata     json.RawMessage
	Attachment   *whatsapp.MediaAttachment
	RemoteURL    string
	Caption      string
	Body         string
	ViewOnce     bool
	Compress     bool
	IsForwarded  bool
	PushToTalk   bool
	ContactName  string
	ContactPhone string
	Link         string
	Latitude     string
	Longitude    string
	Question     string
	Options      []string
	MaxAnswer    int
	Duration     *int
}

func buildOutgoingWhatsAppMessage(input SendWhatsAppConversationMessageInput) (outgoingWhatsAppMessage, error) {
	messageType := strings.ToLower(strings.TrimSpace(input.Type))
	if messageType == "" {
		messageType = "text"
	}

	message := normalizeOutgoingWhatsAppMessage(input, messageType)

	attachment, remoteURL, err := buildWhatsAppAttachment(input.Attachment)
	if err != nil {
		return outgoingWhatsAppMessage{}, err
	}
	message.Attachment = attachment
	message.RemoteURL = remoteURL

	if err := applyOutgoingWhatsAppMessageTypeRules(&message); err != nil {
		return outgoingWhatsAppMessage{}, err
	}

	metadata, err := marshalOutgoingWhatsAppMetadata(message, input.Attachment)
	if err != nil {
		return outgoingWhatsAppMessage{}, apperr.Internal("WhatsApp-berichtmetadata kon niet worden opgebouwd")
	}
	message.Metadata = metadata
	return message, nil
}

func normalizeOutgoingWhatsAppMessage(input SendWhatsAppConversationMessageInput, messageType string) outgoingWhatsAppMessage {
	return outgoingWhatsAppMessage{
		Type:         messageType,
		Caption:      strings.TrimSpace(input.Caption),
		Body:         strings.TrimSpace(input.Body),
		ViewOnce:     input.ViewOnce,
		Compress:     input.Compress,
		IsForwarded:  input.IsForwarded,
		PushToTalk:   input.PushToTalk,
		ContactName:  strings.TrimSpace(input.ContactName),
		ContactPhone: strings.TrimSpace(input.ContactPhone),
		Link:         strings.TrimSpace(input.Link),
		Latitude:     strings.TrimSpace(input.Latitude),
		Longitude:    strings.TrimSpace(input.Longitude),
		Question:     strings.TrimSpace(input.Question),
		Options:      trimWhatsAppOptions(input.Options),
		MaxAnswer:    input.MaxAnswer,
		Duration:     input.DurationSeconds,
	}
}

func trimWhatsAppOptions(options []string) []string {
	trimmedOptions := make([]string, 0, len(options))
	for _, option := range options {
		trimmed := strings.TrimSpace(option)
		if trimmed != "" {
			trimmedOptions = append(trimmedOptions, trimmed)
		}
	}
	return trimmedOptions
}

func applyOutgoingWhatsAppMessageTypeRules(message *outgoingWhatsAppMessage) error {
	switch message.Type {
	case "text":
		return applyTextMessageRules(message)
	case "image":
		return applyMediaMessageRules(message, "[Afbeelding]")
	case "video":
		return applyMediaMessageRules(message, "[Video]")
	case "audio":
		return applyAudioMessageRules(message)
	case "file":
		return applyMediaMessageRules(message, "[Bestand]")
	case "sticker":
		return applyStickerMessageRules(message)
	case "contact":
		return applyContactMessageRules(message)
	case "link":
		return applyLinkMessageRules(message)
	case "location":
		return applyLocationMessageRules(message)
	case "poll":
		return applyPollMessageRules(message)
	default:
		return apperr.Validation("ongeldig WhatsApp-berichttype")
	}
}

func applyTextMessageRules(message *outgoingWhatsAppMessage) error {
	if message.Body == "" {
		return apperr.Validation("WhatsApp-bericht is leeg")
	}
	message.Preview = message.Body
	return nil
}

func applyMediaMessageRules(message *outgoingWhatsAppMessage, label string) error {
	if err := requireAttachmentOrRemoteURL(message.Type, message.Attachment, message.RemoteURL); err != nil {
		return err
	}
	message.Preview = previewWithCaption(label, message.Caption)
	return nil
}

func applyAudioMessageRules(message *outgoingWhatsAppMessage) error {
	if err := requireAttachmentOrRemoteURL(message.Type, message.Attachment, message.RemoteURL); err != nil {
		return err
	}
	message.Preview = "[Audio]"
	if message.PushToTalk {
		message.Preview = "[Spraakbericht]"
	}
	return nil
}

func applyStickerMessageRules(message *outgoingWhatsAppMessage) error {
	if err := requireAttachmentOrRemoteURL(message.Type, message.Attachment, message.RemoteURL); err != nil {
		return err
	}
	message.Preview = "[Sticker]"
	return nil
}

func applyContactMessageRules(message *outgoingWhatsAppMessage) error {
	if message.ContactName == "" || message.ContactPhone == "" {
		return apperr.Validation("contactnaam en contacttelefoon zijn verplicht")
	}
	message.Preview = fmt.Sprintf("[Contact] %s", message.ContactName)
	return nil
}

func applyLinkMessageRules(message *outgoingWhatsAppMessage) error {
	if message.Link == "" {
		return apperr.Validation("link is verplicht")
	}
	message.Preview = message.Link
	if message.Caption != "" {
		message.Preview = previewWithCaption("[Link]", message.Caption)
	}
	return nil
}

func applyLocationMessageRules(message *outgoingWhatsAppMessage) error {
	if message.Latitude == "" || message.Longitude == "" {
		return apperr.Validation("latitude en longitude zijn verplicht")
	}
	message.Preview = "[Locatie]"
	return nil
}

func applyPollMessageRules(message *outgoingWhatsAppMessage) error {
	if message.Question == "" {
		return apperr.Validation("poll-vraag is verplicht")
	}
	if len(message.Options) < 2 {
		return apperr.Validation("een poll heeft minimaal twee opties nodig")
	}
	if message.MaxAnswer < 1 || message.MaxAnswer > len(message.Options) {
		return apperr.Validation("ongeldig maximaal aantal poll-antwoorden")
	}
	message.Preview = fmt.Sprintf("[Poll] %s", message.Question)
	return nil
}

func buildWhatsAppAttachment(input *SendWhatsAppConversationAttachmentInput) (*whatsapp.MediaAttachment, string, error) {
	if input == nil {
		return nil, "", nil
	}

	remoteURL := strings.TrimSpace(input.RemoteURL)
	encoded := strings.TrimSpace(input.Base64Data)
	if encoded == "" {
		return nil, remoteURL, nil
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, "", apperr.Validation("attachment base64Data is ongeldig")
	}

	return &whatsapp.MediaAttachment{
		Filename: strings.TrimSpace(input.Filename),
		Data:     data,
	}, remoteURL, nil
}

func requireAttachmentOrRemoteURL(messageType string, attachment *whatsapp.MediaAttachment, remoteURL string) error {
	if attachment == nil && strings.TrimSpace(remoteURL) == "" {
		return apperr.Validation(fmt.Sprintf("%s vereist een attachment of remoteUrl", messageType))
	}
	return nil
}

func previewWithCaption(label string, caption string) string {
	if strings.TrimSpace(caption) == "" {
		return label
	}
	return label + " " + strings.TrimSpace(caption)
}

type whatsappPortalMetadata struct {
	MessageType string `json:"messageType"`
	Caption     string `json:"caption,omitempty"`
	Text        string `json:"text,omitempty"`
	Attachment  *struct {
		MediaType string `json:"mediaType,omitempty"`
		Filename  string `json:"filename,omitempty"`
		Path      string `json:"path,omitempty"`
		RemoteURL string `json:"remoteUrl,omitempty"`
	} `json:"attachment,omitempty"`
}

func whatsAppMessageDeviceOverride(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var envelope struct {
		DeviceID string `json:"device_id"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ""
	}
	return strings.TrimSpace(envelope.DeviceID)
}

func parseWhatsAppPortalMetadata(raw json.RawMessage) whatsappPortalMetadata {
	if len(raw) == 0 {
		return whatsappPortalMetadata{}
	}
	var envelope struct {
		Portal whatsappPortalMetadata `json:"portal"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil {
		if strings.TrimSpace(envelope.Portal.MessageType) != "" || envelope.Portal.Attachment != nil || strings.TrimSpace(envelope.Portal.Caption) != "" || strings.TrimSpace(envelope.Portal.Text) != "" {
			return envelope.Portal
		}
	}
	var metadata whatsappPortalMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return whatsappPortalMetadata{}
	}
	return metadata
}

func isWhatsAppImageMessage(message repository.WhatsAppMessage) bool {
	metadata := parseWhatsAppPortalMetadata(message.Metadata)
	if strings.EqualFold(strings.TrimSpace(metadata.MessageType), "image") {
		return true
	}
	if metadata.Attachment != nil && strings.EqualFold(strings.TrimSpace(metadata.Attachment.MediaType), "image") {
		return true
	}
	return false
}

func normalizeWhatsAppImportContentType(contentType string, mediaType string) string {
	trimmed := strings.TrimSpace(strings.Split(contentType, ";")[0])
	if trimmed != "" {
		return trimmed
	}
	if strings.EqualFold(strings.TrimSpace(mediaType), "image") {
		return "image/jpeg"
	}
	return ""
}

func chooseWhatsAppImportFilename(fileResult whatsapp.DownloadMediaFileResult, message repository.WhatsAppMessage) string {
	if trimmed := strings.TrimSpace(fileResult.Filename); trimmed != "" {
		return trimmed
	}
	metadata := parseWhatsAppPortalMetadata(message.Metadata)
	if metadata.Attachment != nil {
		if trimmed := strings.TrimSpace(metadata.Attachment.Filename); trimmed != "" {
			return trimmed
		}
	}
	return fmt.Sprintf("whatsapp-image-%s.jpg", message.CreatedAt.UTC().Format("20060102-150405"))
}

func buildImportantWhatsAppMessagesNote(conversation repository.WhatsAppConversation, messages []repository.WhatsAppMessage) string {
	lines := []string{"Belangrijke WhatsApp-berichten"}
	if displayName := strings.TrimSpace(conversation.DisplayName); displayName != "" {
		lines = append(lines, fmt.Sprintf("Contact: %s", displayName))
	} else {
		lines = append(lines, fmt.Sprintf("Telefoon: %s", conversation.PhoneNumber))
	}
	lines = append(lines, "")

	for _, message := range messages {
		body := strings.TrimSpace(message.Body)
		if body == "" {
			metadata := parseWhatsAppPortalMetadata(message.Metadata)
			body = firstNonEmptyTrimmed(metadata.Caption, metadata.Text, fmt.Sprintf("[%s]", firstNonEmptyTrimmed(metadata.MessageType, "bericht")))
		}
		body = strings.ReplaceAll(body, "\n", " ")
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", message.CreatedAt.Local().Format("2006-01-02 15:04"), whatsappMessageActorLabel(message.Direction), body))
	}

	note := strings.TrimSpace(strings.Join(lines, "\n"))
	if len(note) <= 2000 {
		return note
	}
	return strings.TrimSpace(note[:1997]) + "..."
}

func whatsappMessageActorLabel(direction string) string {
	if strings.EqualFold(strings.TrimSpace(direction), "outbound") {
		return "Team"
	}
	return "Klant"
}

func uniqueTrimmedStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (s *Service) getAttachableWhatsAppImage(ctx context.Context, organizationID, conversationID uuid.UUID, externalMessageID string) (repository.WhatsAppMessage, repository.WhatsAppConversation, error) {
	message, conversation, err := s.repo.GetWhatsAppMessageByExternalID(ctx, organizationID, externalMessageID)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.WhatsAppMessage{}, repository.WhatsAppConversation{}, apperr.NotFound(whatsappMessageNotFoundMsg)
		}
		return repository.WhatsAppMessage{}, repository.WhatsAppConversation{}, err
	}
	if conversation.ID != conversationID {
		return repository.WhatsAppMessage{}, repository.WhatsAppConversation{}, apperr.NotFound(whatsappMessageNotFoundMsg)
	}
	if conversation.LeadID == nil {
		return repository.WhatsAppMessage{}, repository.WhatsAppConversation{}, apperr.Validation("gesprek is nog niet gekoppeld aan een lead")
	}
	if message.Direction != "inbound" {
		return repository.WhatsAppMessage{}, repository.WhatsAppConversation{}, apperr.Validation("alleen ontvangen WhatsApp-afbeeldingen kunnen aan een lead worden gekoppeld")
	}
	if !isWhatsAppImageMessage(message) {
		return repository.WhatsAppMessage{}, repository.WhatsAppConversation{}, apperr.Validation("geselecteerd WhatsApp-bericht bevat geen afbeelding")
	}
	return message, conversation, nil
}

func (s *Service) importWhatsAppImageAttachment(ctx context.Context, organizationID, authorID, serviceID uuid.UUID, externalMessageID string, message repository.WhatsAppMessage, conversation repository.WhatsAppConversation) (CreateLeadAttachmentResult, error) {
	deviceID, err := s.getRequiredWhatsAppDeviceID(ctx, organizationID)
	if err != nil {
		return CreateLeadAttachmentResult{}, err
	}

	fileResult, err := s.whatsapp.DownloadMediaFile(ctx, deviceID, externalMessageID, conversation.PhoneNumber)
	if err != nil {
		return CreateLeadAttachmentResult{}, apperr.Internal("WhatsApp-afbeelding kon niet worden opgehaald")
	}
	contentType := normalizeWhatsAppImportContentType(fileResult.ContentType, fileResult.MediaType)
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return CreateLeadAttachmentResult{}, apperr.Validation("geselecteerd WhatsApp-bericht bevat geen ondersteunde afbeelding")
	}
	if err := s.storage.ValidateContentType(contentType); err != nil {
		return CreateLeadAttachmentResult{}, apperr.Validation("afbeeldingstype wordt niet ondersteund")
	}
	sizeBytes := int64(len(fileResult.Data))
	if err := s.storage.ValidateFileSize(sizeBytes); err != nil {
		return CreateLeadAttachmentResult{}, apperr.Validation(err.Error())
	}
	fileName := chooseWhatsAppImportFilename(fileResult, message)
	folder := fmt.Sprintf("%s/%s/%s", organizationID.String(), conversation.LeadID.String(), serviceID.String())
	fileKey, err := s.storage.UploadFile(ctx, s.attachmentsBucket, folder, fileName, contentType, bytes.NewReader(fileResult.Data), sizeBytes)
	if err != nil {
		return CreateLeadAttachmentResult{}, apperr.Internal("WhatsApp-afbeelding kon niet worden opgeslagen")
	}

	return s.leadActions.CreateAttachment(ctx, CreateLeadAttachmentParams{
		LeadID:         *conversation.LeadID,
		ServiceID:      serviceID,
		OrganizationID: organizationID,
		AuthorID:       authorID,
		FileKey:        fileKey,
		FileName:       fileName,
		ContentType:    contentType,
		SizeBytes:      sizeBytes,
	})
}

func (s *Service) loadWhatsAppMessagesForConversation(ctx context.Context, organizationID, conversationID uuid.UUID, externalMessageIDs []string) ([]repository.WhatsAppMessage, error) {
	messages := make([]repository.WhatsAppMessage, 0, len(externalMessageIDs))
	for _, messageID := range externalMessageIDs {
		message, owningConversation, err := s.repo.GetWhatsAppMessageByExternalID(ctx, organizationID, messageID)
		if err != nil {
			if err == repository.ErrNotFound {
				return nil, apperr.NotFound(whatsappMessageNotFoundMsg)
			}
			return nil, err
		}
		if owningConversation.ID != conversationID {
			return nil, apperr.Validation("geselecteerde WhatsApp-berichten horen niet bij hetzelfde gesprek")
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func marshalOutgoingWhatsAppMetadata(message outgoingWhatsAppMessage, attachmentInput *SendWhatsAppConversationAttachmentInput) (json.RawMessage, error) {
	portal := map[string]any{
		"messageType": message.Type,
	}
	addOutgoingTextMetadata(portal, message)
	addOutgoingFlagMetadata(portal, message)
	addOutgoingStructuredMetadata(portal, message)
	addOutgoingAttachmentMetadata(portal, attachmentInput)

	metadata := map[string]any{"portal": portal}

	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func addOutgoingTextMetadata(portal map[string]any, message outgoingWhatsAppMessage) {
	if message.Caption != "" {
		portal["caption"] = message.Caption
	}
	if message.Body != "" && message.Type == "text" {
		portal["text"] = message.Body
	}
}

func addOutgoingFlagMetadata(portal map[string]any, message outgoingWhatsAppMessage) {
	if message.ViewOnce {
		portal["viewOnce"] = true
	}
	if message.Compress {
		portal["compress"] = true
	}
	if message.IsForwarded {
		portal["isForwarded"] = true
	}
	if message.PushToTalk {
		portal["pushToTalk"] = true
	}
	if message.Duration != nil {
		portal["durationSeconds"] = *message.Duration
	}
}

func addOutgoingStructuredMetadata(portal map[string]any, message outgoingWhatsAppMessage) {
	if message.ContactName != "" || message.ContactPhone != "" {
		portal["contact"] = map[string]any{
			"name":  message.ContactName,
			"phone": message.ContactPhone,
		}
	}
	if message.Link != "" {
		portal["link"] = message.Link
	}
	if message.Latitude != "" || message.Longitude != "" {
		portal["location"] = map[string]any{
			"latitude":  message.Latitude,
			"longitude": message.Longitude,
		}
	}
	if message.Question != "" {
		portal["poll"] = map[string]any{
			"question":  message.Question,
			"options":   message.Options,
			"maxAnswer": message.MaxAnswer,
		}
	}
}

func addOutgoingAttachmentMetadata(portal map[string]any, attachmentInput *SendWhatsAppConversationAttachmentInput) {
	if attachmentInput == nil {
		return
	}

	attachment := map[string]any{}
	if strings.TrimSpace(attachmentInput.Filename) != "" {
		attachment["filename"] = strings.TrimSpace(attachmentInput.Filename)
	}
	if strings.TrimSpace(attachmentInput.RemoteURL) != "" {
		attachment["remoteUrl"] = strings.TrimSpace(attachmentInput.RemoteURL)
	}
	if strings.TrimSpace(attachmentInput.Base64Data) != "" {
		attachment["hasInlineData"] = true
	}
	if len(attachment) > 0 {
		portal["attachment"] = attachment
	}
}

func (s *Service) getWhatsAppConversationActionContext(ctx context.Context, organizationID, conversationID uuid.UUID) (repository.WhatsAppConversation, string, error) {
	conversation, err := s.repo.GetWhatsAppConversation(ctx, organizationID, conversationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.WhatsAppConversation{}, "", apperr.NotFound(conversationNotFoundMsg)
		}
		return repository.WhatsAppConversation{}, "", err
	}

	deviceID, err := s.getRequiredWhatsAppDeviceID(ctx, organizationID)
	if err != nil {
		return repository.WhatsAppConversation{}, "", err
	}
	return conversation, deviceID, nil
}

func (s *Service) getWhatsAppMessageActionContext(ctx context.Context, organizationID, conversationID uuid.UUID, externalMessageID string) (repository.WhatsAppMessage, repository.WhatsAppConversation, string, error) {
	conversation, deviceID, err := s.getWhatsAppConversationActionContext(ctx, organizationID, conversationID)
	if err != nil {
		return repository.WhatsAppMessage{}, repository.WhatsAppConversation{}, "", err
	}

	message, messageConversation, err := s.repo.GetWhatsAppMessageByExternalID(ctx, organizationID, externalMessageID)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.WhatsAppMessage{}, repository.WhatsAppConversation{}, "", apperr.NotFound(whatsappMessageNotFoundMsg)
		}
		return repository.WhatsAppMessage{}, repository.WhatsAppConversation{}, "", err
	}
	if messageConversation.ID != conversation.ID {
		return repository.WhatsAppMessage{}, repository.WhatsAppConversation{}, "", apperr.NotFound(whatsappMessageNotFoundMsg)
	}
	return message, conversation, deviceID, nil
}

func requireOutboundWhatsAppMessage(message repository.WhatsAppMessage, action string) error {
	if message.Direction != "outbound" {
		return apperr.Validation(fmt.Sprintf("alleen uitgaande berichten kunnen worden %s", action))
	}
	return nil
}

func (s *Service) applyLocalWhatsAppMutation(ctx context.Context, organizationID uuid.UUID, message repository.WhatsAppMessage, conversation repository.WhatsAppConversation, eventType string, body *string, reaction *string) (repository.WhatsAppConversation, repository.WhatsAppMessage, error) {
	now := time.Now().UTC()
	isFromMe := true
	applied, err := s.ApplyWhatsAppMessageMutation(ctx, webhookinbox.WhatsAppMessageMutation{
		OrganizationID:          organizationID,
		EventType:               eventType,
		TargetExternalMessageID: strings.TrimSpace(valueOrEmpty(message.ExternalMessageID)),
		PhoneNumber:             conversation.PhoneNumber,
		Body:                    body,
		Reaction:                reaction,
		OccurredAt:              &now,
		IsFromMe:                &isFromMe,
	})
	if err != nil {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}
	if !applied {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.NotFound(whatsappMessageNotFoundMsg)
	}

	updatedMessage, updatedConversation, err := s.repo.GetWhatsAppMessageByExternalID(ctx, organizationID, strings.TrimSpace(valueOrEmpty(message.ExternalMessageID)))
	if err != nil {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}
	if updatedConversation.ID != conversation.ID {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.NotFound(whatsappMessageNotFoundMsg)
	}
	return updatedConversation, updatedMessage, nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Service) sendWhatsAppConversationMessageByType(ctx context.Context, deviceID string, phoneNumber string, message outgoingWhatsAppMessage) (whatsapp.SendResult, error) {
	switch message.Type {
	case "text":
		return s.whatsapp.SendMessage(ctx, deviceID, phoneNumber, message.Body)
	case "image":
		return s.whatsapp.SendImage(ctx, deviceID, whatsapp.SendImageInput{
			PhoneNumber:     phoneNumber,
			Caption:         message.Caption,
			ViewOnce:        message.ViewOnce,
			Compress:        message.Compress,
			IsForwarded:     message.IsForwarded,
			DurationSeconds: message.Duration,
			Attachment:      message.Attachment,
			RemoteURL:       message.RemoteURL,
		})
	case "video":
		return s.whatsapp.SendVideo(ctx, deviceID, whatsapp.SendVideoInput{
			PhoneNumber:     phoneNumber,
			Caption:         message.Caption,
			ViewOnce:        message.ViewOnce,
			Compress:        message.Compress,
			IsForwarded:     message.IsForwarded,
			DurationSeconds: message.Duration,
			Attachment:      message.Attachment,
			RemoteURL:       message.RemoteURL,
		})
	case "audio":
		return s.whatsapp.SendAudio(ctx, deviceID, whatsapp.SendAudioInput{
			PhoneNumber:     phoneNumber,
			IsForwarded:     message.IsForwarded,
			PTT:             message.PushToTalk,
			DurationSeconds: message.Duration,
			Attachment:      message.Attachment,
			RemoteURL:       message.RemoteURL,
		})
	case "file":
		return s.whatsapp.SendFile(ctx, deviceID, whatsapp.SendFileInput{
			PhoneNumber:     phoneNumber,
			Caption:         message.Caption,
			IsForwarded:     message.IsForwarded,
			DurationSeconds: message.Duration,
			Attachment:      message.Attachment,
			RemoteURL:       message.RemoteURL,
		})
	case "sticker":
		return s.whatsapp.SendSticker(ctx, deviceID, whatsapp.SendStickerInput{
			PhoneNumber:     phoneNumber,
			IsForwarded:     message.IsForwarded,
			DurationSeconds: message.Duration,
			Attachment:      message.Attachment,
			RemoteURL:       message.RemoteURL,
		})
	case "contact":
		return s.whatsapp.SendContact(ctx, deviceID, whatsapp.SendContactInput{
			PhoneNumber:     phoneNumber,
			ContactName:     message.ContactName,
			ContactPhone:    message.ContactPhone,
			IsForwarded:     message.IsForwarded,
			DurationSeconds: message.Duration,
		})
	case "link":
		return s.whatsapp.SendLink(ctx, deviceID, whatsapp.SendLinkInput{
			PhoneNumber:     phoneNumber,
			Link:            message.Link,
			Caption:         message.Caption,
			IsForwarded:     message.IsForwarded,
			DurationSeconds: message.Duration,
		})
	case "location":
		return s.whatsapp.SendLocation(ctx, deviceID, whatsapp.SendLocationInput{
			PhoneNumber:     phoneNumber,
			Latitude:        message.Latitude,
			Longitude:       message.Longitude,
			IsForwarded:     message.IsForwarded,
			DurationSeconds: message.Duration,
		})
	case "poll":
		return s.whatsapp.SendPoll(ctx, deviceID, whatsapp.SendPollInput{
			PhoneNumber:     phoneNumber,
			Question:        message.Question,
			Options:         message.Options,
			MaxAnswer:       message.MaxAnswer,
			DurationSeconds: message.Duration,
		})
	default:
		return whatsapp.SendResult{}, apperr.Validation("ongeldig WhatsApp-berichttype")
	}
}

func (s *Service) getRequiredWhatsAppDeviceID(ctx context.Context, organizationID uuid.UUID) (string, error) {
	if s.whatsapp == nil {
		return "", apperr.Internal(whatsappNotConfiguredMsg)
	}

	settings, err := s.repo.GetOrganizationSettings(ctx, organizationID)
	if err != nil {
		return "", err
	}
	if settings.WhatsAppDeviceID == nil || strings.TrimSpace(*settings.WhatsAppDeviceID) == "" {
		return "", apperr.Validation(whatsappDeviceNotLinkedMsg)
	}

	return strings.TrimSpace(*settings.WhatsAppDeviceID), nil
}
