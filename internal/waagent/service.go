package waagent

import (
	"context"
	"errors"
	"log"
	"strings"

	waagentdb "portal_final_backend/internal/waagent/db"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Hardcoded Dutch messages — zero LLM cost for rate limiting and unknown users.
const (
	msgRateLimited     = "Je stuurt te veel berichten. Probeer het over een paar minuten opnieuw."
	msgUnknownPhone    = "Je telefoonnummer is niet gekoppeld aan een organisatie. Neem contact op met je beheerder."
	recentMessageLimit = 20
)

// Service orchestrates the waagent flow: rate limit → phone→org → AI.
type Service struct {
	queries     waagentdb.Querier
	agent       *Agent
	sender      *Sender
	rateLimiter *RateLimiter
	leadHintStore *ConversationLeadHintStore
	log         *logger.Logger
}

// HandleIncomingMessage processes an incoming WhatsApp message from the global agent device.
// It resolves the organization from the sender's phone number.
// It is designed to be called in a goroutine with context.WithoutCancel.
func (s *Service) HandleIncomingMessage(ctx context.Context, externalMessageID, phone, displayName, text string) {
	replyTarget := strings.TrimSpace(phone)
	lookupPhone := normalizeAgentPhoneKey(phone)
	if replyTarget == "" {
		replyTarget = lookupPhone
	}
	if lookupPhone == "" {
		lookupPhone = replyTarget
	}

	claimed, err := s.rateLimiter.ClaimMessage(ctx, strings.TrimSpace(externalMessageID))
	if err != nil {
		log.Printf("waagent: message dedupe error id=%s: %v", externalMessageID, err)
	}
	if !claimed {
		log.Printf("waagent: duplicate inbound message ignored id=%s phone=%s", externalMessageID, lookupPhone)
		return
	}

	// Step 1: Rate limit check
	allowed, err := s.rateLimiter.Allow(ctx, lookupPhone)
	if err != nil {
		log.Printf("waagent: rate limiter error phone=%s: %v", lookupPhone, err)
	}
	if !allowed {
		s.sendHardcoded(ctx, uuid.Nil, lookupPhone, replyTarget, text, msgRateLimited)
		return
	}

	// Step 2: Phone → org lookup
	user, err := s.lookupAgentUser(ctx, phone)
	if err != nil {
		s.sendHardcoded(ctx, uuid.Nil, lookupPhone, replyTarget, text, msgUnknownPhone)
		return
	}

	orgID := uuidFromPgtype(user.OrganizationID)

	// Step 3: AI path
	s.handleAIMessage(ctx, orgID, lookupPhone, replyTarget, text)
}

func (s *Service) handleAIMessage(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget, text string) {
	// Persist incoming message
	if err := s.queries.InsertAgentMessage(ctx, waagentdb.InsertAgentMessageParams{
		OrganizationID: pgtypeUUID(orgID),
		PhoneNumber:    phoneKey,
		Role:           "user",
		Content:        text,
	}); err != nil {
		log.Printf("waagent: failed to persist user message phone=%s: %v", phoneKey, err)
	}

	// Load recent conversation history
	recent, err := s.queries.GetRecentAgentMessages(ctx, waagentdb.GetRecentAgentMessagesParams{
		PhoneNumber: phoneKey,
		Limit:       recentMessageLimit,
	})
	if err != nil {
		log.Printf("waagent: failed to load history phone=%s: %v", phoneKey, err)
		recent = nil
	}

	// Reverse to chronological order (DB returns DESC)
	messages := make([]ConversationMessage, len(recent))
	for i, msg := range recent {
		messages[len(recent)-1-i] = ConversationMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Run the AI agent
	var leadHint *ConversationLeadHint
	if s.leadHintStore != nil {
		leadHint, _ = s.leadHintStore.Get(orgID.String(), phoneKey)
	}

	reply, err := s.agent.Run(ctx, orgID, phoneKey, messages, leadHint)
	if err != nil {
		log.Printf("waagent: agent run error phone=%s org=%s: %v", phoneKey, orgID, err)
		return
	}

	reply = sanitizeWhatsAppReply(strings.TrimSpace(reply))
	if reply == "" {
		log.Printf("waagent: empty agent reply phone=%s org=%s", phoneKey, orgID)
		return
	}

	// Persist assistant reply
	if err := s.queries.InsertAgentMessage(ctx, waagentdb.InsertAgentMessageParams{
		OrganizationID: pgtypeUUID(orgID),
		PhoneNumber:    phoneKey,
		Role:           "assistant",
		Content:        reply,
	}); err != nil {
		log.Printf("waagent: failed to persist assistant message phone=%s: %v", phoneKey, err)
	}

	// Send via WhatsApp + inbox write
	if err := s.sender.SendReply(ctx, orgID, replyTarget, reply); err != nil {
		log.Printf("waagent: send reply error phone=%s org=%s: %v", replyTarget, orgID, err)
	}
}

// sendHardcoded persists messages and sends a hardcoded reply without invoking the LLM.
func (s *Service) sendHardcoded(ctx context.Context, orgID uuid.UUID, phoneKey, replyTarget, incomingText, reply string) {
	if orgID != uuid.Nil {
		_ = s.queries.InsertAgentMessage(ctx, waagentdb.InsertAgentMessageParams{
			OrganizationID: pgtypeUUID(orgID),
			PhoneNumber:    phoneKey,
			Role:           "user",
			Content:        incomingText,
		})
		_ = s.queries.InsertAgentMessage(ctx, waagentdb.InsertAgentMessageParams{
			OrganizationID: pgtypeUUID(orgID),
			PhoneNumber:    phoneKey,
			Role:           "assistant",
			Content:        reply,
		})
	}

	if err := s.sender.SendReply(ctx, orgID, replyTarget, reply); err != nil {
		log.Printf("waagent: hardcoded send error phone=%s: %v", replyTarget, err)
	}
}

func (s *Service) lookupAgentUser(ctx context.Context, phone string) (waagentdb.RacWhatsappAgentUser, error) {
	var lastErr error
	for _, candidate := range agentPhoneCandidates(phone) {
		user, err := s.queries.GetAgentUserByPhone(ctx, candidate)
		if err == nil {
			return user, nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			lastErr = err
			continue
		}
		return waagentdb.RacWhatsappAgentUser{}, err
	}
	if lastErr == nil {
		lastErr = pgx.ErrNoRows
	}
	return waagentdb.RacWhatsappAgentUser{}, lastErr
}

func pgtypeUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func uuidFromPgtype(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	return uuid.UUID(id.Bytes)
}
