package service

import (
	"context"
	"strings"
	"time"

	"portal_final_backend/internal/imap/client"
	"portal_final_backend/internal/imap/repository"
	"portal_final_backend/internal/imap/transport"
	leadports "portal_final_backend/internal/leads/ports"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

const errEmailReplyNotConfigured = "email reply agent is not configured"

type SuggestEmailReplyFeedback struct {
	AIReply    string
	HumanReply string
	CreatedAt  time.Time
}

type SuggestEmailReplyExample struct {
	CustomerMessage string
	Reply           string
	CreatedAt       time.Time
}

type SuggestEmailReplyInput struct {
	OrganizationID  uuid.UUID
	RequesterUserID uuid.UUID
	AccountID       uuid.UUID
	MessageUID      int64
	LeadID          *uuid.UUID
	LeadServiceID   *uuid.UUID
	Scenario        string
	ScenarioNotes   string
	CustomerEmail   string
	CustomerName    string
	Subject         string
	MessageBody     string
	Feedback        []SuggestEmailReplyFeedback
	Examples        []SuggestEmailReplyExample
}

type EmailReplySuggester interface {
	SuggestReply(ctx context.Context, input SuggestEmailReplyInput) (leadports.ReplySuggestionDraft, error)
}

type EmailReplySuggestionResult struct {
	Suggestion        string
	EffectiveScenario string
}

func (s *Service) SetEmailReplySuggester(replyer EmailReplySuggester) {
	s.emailReplyer = replyer
}

func (s *Service) SuggestEmailReply(ctx context.Context, userID, accountID uuid.UUID, uid int64, scenario, scenarioNotes string) (EmailReplySuggestionResult, error) {
	if s.emailReplyer == nil {
		return EmailReplySuggestionResult{}, apperr.Internal(errEmailReplyNotConfigured)
	}
	if s.identityRepo == nil {
		return EmailReplySuggestionResult{}, apperr.Internal("identity repository is not configured")
	}

	organizationID, err := s.identityRepo.GetUserOrganizationID(ctx, userID)
	if err != nil {
		return EmailReplySuggestionResult{}, err
	}

	content, account, err := s.loadMessageForReply(ctx, userID, accountID, uid)
	if err != nil {
		return EmailReplySuggestionResult{}, err
	}

	customerEmail, customerName := resolveEmailReplyCustomer(content)
	if customerEmail == "" {
		return EmailReplySuggestionResult{}, apperr.Validation("suggest reply is alleen beschikbaar voor e-mails met een geldig afzenderadres")
	}

	input := s.buildSuggestEmailReplyInput(organizationID, account.ID, uid, content, customerEmail, customerName)
	input.RequesterUserID = userID
	input.Scenario = scenario
	input.ScenarioNotes = strings.TrimSpace(scenarioNotes)
	input.LeadID, input.LeadServiceID = s.resolveEmailReplyReferenceContext(ctx, userID, account.ID, uid, customerEmail)
	s.appendEmailReplyFeedback(ctx, &input)
	s.appendEmailReplyExamples(ctx, &input)

	draft, err := s.emailReplyer.SuggestReply(ctx, input)
	if err != nil {
		return EmailReplySuggestionResult{}, mapSuggestEmailReplyError(err)
	}

	trimmed := strings.TrimSpace(draft.Text)
	if trimmed == "" {
		return EmailReplySuggestionResult{}, apperr.Internal("email reply agent returned an empty suggestion")
	}

	return EmailReplySuggestionResult{Suggestion: trimmed, EffectiveScenario: string(draft.EffectiveScenario)}, nil
}

func (s *Service) buildSuggestEmailReplyInput(organizationID, accountID uuid.UUID, uid int64, content client.MessageContent, customerEmail, customerName string) SuggestEmailReplyInput {
	return SuggestEmailReplyInput{
		OrganizationID: organizationID,
		AccountID:      accountID,
		MessageUID:     uid,
		LeadID:         nil,
		LeadServiceID:  nil,
		CustomerEmail:  customerEmail,
		CustomerName:   customerName,
		Subject:        strings.TrimSpace(content.Subject),
		MessageBody:    emailReplyBody(content.Text, content.HTML),
		Feedback:       make([]SuggestEmailReplyFeedback, 0, 4),
		Examples:       make([]SuggestEmailReplyExample, 0, 4),
	}
}

func (s *Service) appendEmailReplyFeedback(ctx context.Context, input *SuggestEmailReplyInput) {
	feedbackItems, feedbackErr := s.repo.ListRecentAppliedEmailReplyFeedback(ctx, input.OrganizationID, repository.EmailReplyReference{
		LeadID:        input.LeadID,
		LeadServiceID: input.LeadServiceID,
	}, input.CustomerEmail, input.AccountID, input.MessageUID, 4)
	if feedbackErr == nil {
		for _, item := range feedbackItems {
			input.Feedback = append(input.Feedback, SuggestEmailReplyFeedback{
				AIReply:    item.AIReply,
				HumanReply: item.HumanReply,
				CreatedAt:  item.CreatedAt,
			})
		}
	}
}

func (s *Service) appendEmailReplyExamples(ctx context.Context, input *SuggestEmailReplyInput) {
	exampleItems, exampleErr := s.repo.ListRecentEmailReplyExamples(ctx, input.OrganizationID, repository.EmailReplyReference{
		LeadID:        input.LeadID,
		LeadServiceID: input.LeadServiceID,
	}, input.CustomerEmail, input.AccountID, input.MessageUID, 4)
	if exampleErr == nil {
		for _, item := range exampleItems {
			input.Examples = append(input.Examples, SuggestEmailReplyExample{
				CustomerMessage: item.CustomerMessage,
				Reply:           item.Reply,
				CreatedAt:       item.CreatedAt,
			})
		}
	}
}

func (s *Service) resolveEmailReplyReferenceContext(ctx context.Context, userID, accountID uuid.UUID, uid int64, customerEmail string) (*uuid.UUID, *uuid.UUID) {
	leadID := s.resolveEmailReplyLeadID(ctx, userID, accountID, uid, customerEmail)
	leadServiceID := s.resolveEmailReplyLeadServiceID(ctx, userID, leadID)
	return leadID, leadServiceID
}

func (s *Service) resolveEmailReplyLeadID(ctx context.Context, userID, accountID uuid.UUID, uid int64, customerEmail string) *uuid.UUID {
	if linked, err := s.repo.GetMessageLeadLinkByUser(ctx, userID, accountID, uid); err == nil && linked != nil && linked.LeadID != uuid.Nil {
		return &linked.LeadID
	}
	if s.leadsRepo == nil || s.identityRepo == nil || customerEmail == "" {
		return nil
	}
	organizationID, err := s.identityRepo.GetUserOrganizationID(ctx, userID)
	if err != nil {
		return nil
	}
	summary, _, lookupErr := s.leadsRepo.GetByPhoneOrEmail(ctx, "", customerEmail, organizationID)
	if lookupErr != nil || summary == nil || summary.ID == uuid.Nil {
		return nil
	}
	return &summary.ID
}

func (s *Service) resolveEmailReplyLeadServiceID(ctx context.Context, userID uuid.UUID, leadID *uuid.UUID) *uuid.UUID {
	if leadID == nil || s.leadsRepo == nil || s.identityRepo == nil {
		return nil
	}
	organizationID, err := s.identityRepo.GetUserOrganizationID(ctx, userID)
	if err != nil {
		return nil
	}
	service, serviceErr := s.leadsRepo.GetCurrentLeadService(ctx, *leadID, organizationID)
	if serviceErr != nil || service.ID == uuid.Nil {
		return nil
	}
	return &service.ID
}

func mapSuggestEmailReplyError(err error) error {
	if apperr.Is(err, apperr.KindValidation) || apperr.Is(err, apperr.KindBadRequest) || apperr.Is(err, apperr.KindNotFound) {
		return err
	}
	return apperr.Internal("email reply kon niet worden gegenereerd")
}

func (s *Service) captureEmailReplyFeedback(ctx context.Context, userID uuid.UUID, account repository.Account, uid int64, includeAll bool, content client.MessageContent, req transport.ReplyRequest) {
	if s.identityRepo == nil {
		return
	}
	organizationID, err := s.identityRepo.GetUserOrganizationID(ctx, userID)
	if err != nil {
		return
	}
	params, ok := buildEmailReplyFeedbackParams(organizationID, account, uid, includeAll, content, req)
	if !ok {
		return
	}
	_, _ = s.repo.CreateEmailReplyFeedback(ctx, params)
}

func buildEmailReplyFeedbackParams(
	organizationID uuid.UUID,
	account repository.Account,
	uid int64,
	replyAll bool,
	content client.MessageContent,
	req transport.ReplyRequest,
) (repository.CreateEmailReplyFeedbackParams, bool) {
	if organizationID == uuid.Nil || account.ID == uuid.Nil || uid <= 0 {
		return repository.CreateEmailReplyFeedbackParams{}, false
	}

	customerEmail, customerName := resolveEmailReplyCustomer(content)
	customerMessage := emailReplyBody(content.Text, content.HTML)
	humanReply := strings.TrimSpace(req.Body)
	if customerEmail == "" || customerMessage == "" || humanReply == "" {
		return repository.CreateEmailReplyFeedbackParams{}, false
	}

	var aiReply *string
	wasEdited := false
	if req.AISuggestion != nil {
		trimmed := strings.TrimSpace(*req.AISuggestion)
		if trimmed != "" {
			aiReply = &trimmed
			wasEdited = trimmed != humanReply
		}
	}

	scenario := ""
	if req.Scenario != nil {
		scenario = strings.TrimSpace(*req.Scenario)
	}

	return repository.CreateEmailReplyFeedbackParams{
		OrganizationID:  organizationID,
		AccountID:       account.ID,
		SourceUID:       uid,
		CustomerEmail:   customerEmail,
		CustomerName:    emptyStringPtr(customerName),
		Subject:         emptyStringPtr(strings.TrimSpace(content.Subject)),
		CustomerMessage: customerMessage,
		Scenario:        scenario,
		AIReply:         aiReply,
		HumanReply:      humanReply,
		WasEdited:       wasEdited,
		ReplyAll:        replyAll,
	}, true
}

func resolveEmailReplyCustomer(content client.MessageContent) (string, string) {
	if len(content.ReplyTo) > 0 {
		address := strings.ToLower(strings.TrimSpace(content.ReplyTo[0].Address))
		name := strings.TrimSpace(content.ReplyTo[0].Name)
		if address != "" {
			return address, name
		}
	}
	name := ""
	if content.FromName != nil {
		name = strings.TrimSpace(*content.FromName)
	}
	if content.FromAddress == nil {
		return "", name
	}
	return strings.ToLower(strings.TrimSpace(*content.FromAddress)), name
}

func emailReplyBody(text, html *string) string {
	if text != nil {
		trimmed := strings.TrimSpace(*text)
		if trimmed != "" {
			return trimmed
		}
	}
	if html != nil {
		return strings.TrimSpace(*html)
	}
	return ""
}

func emptyStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
