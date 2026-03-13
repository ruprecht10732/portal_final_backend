package waagent

import (
	"context"
	"log"
	"strings"

	waagentdb "portal_final_backend/internal/waagent/db"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
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
	log         *logger.Logger
}

// HandleIncomingMessage processes an incoming WhatsApp message from the global agent device.
// It resolves the organization from the sender's phone number.
// It is designed to be called in a goroutine with context.WithoutCancel.
func (s *Service) HandleIncomingMessage(ctx context.Context, phone, displayName, text string) {
	// Step 1: Rate limit check
	allowed, err := s.rateLimiter.Allow(ctx, phone)
	if err != nil {
		log.Printf("waagent: rate limiter error phone=%s: %v", phone, err)
	}
	if !allowed {
		s.sendHardcoded(ctx, uuid.Nil, phone, text, msgRateLimited)
		return
	}

	// Step 2: Phone → org lookup
	user, err := s.queries.GetAgentUserByPhone(ctx, phone)
	if err != nil {
		s.sendHardcoded(ctx, uuid.Nil, phone, text, msgUnknownPhone)
		return
	}

	orgID := uuidFromPgtype(user.OrganizationID)

	// Step 3: AI path
	s.handleAIMessage(ctx, orgID, phone, text)
}

func (s *Service) handleAIMessage(ctx context.Context, orgID uuid.UUID, phone, text string) {
	// Persist incoming message
	if err := s.queries.InsertAgentMessage(ctx, waagentdb.InsertAgentMessageParams{
		OrganizationID: pgtypeUUID(orgID),
		PhoneNumber:    phone,
		Role:           "user",
		Content:        text,
	}); err != nil {
		log.Printf("waagent: failed to persist user message phone=%s: %v", phone, err)
	}

	// Load recent conversation history
	recent, err := s.queries.GetRecentAgentMessages(ctx, waagentdb.GetRecentAgentMessagesParams{
		PhoneNumber: phone,
		Limit:       recentMessageLimit,
	})
	if err != nil {
		log.Printf("waagent: failed to load history phone=%s: %v", phone, err)
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
	reply, err := s.agent.Run(ctx, orgID, messages)
	if err != nil {
		log.Printf("waagent: agent run error phone=%s org=%s: %v", phone, orgID, err)
		return
	}

	reply = strings.TrimSpace(reply)
	if reply == "" {
		log.Printf("waagent: empty agent reply phone=%s org=%s", phone, orgID)
		return
	}

	// Persist assistant reply
	if err := s.queries.InsertAgentMessage(ctx, waagentdb.InsertAgentMessageParams{
		OrganizationID: pgtypeUUID(orgID),
		PhoneNumber:    phone,
		Role:           "assistant",
		Content:        reply,
	}); err != nil {
		log.Printf("waagent: failed to persist assistant message phone=%s: %v", phone, err)
	}

	// Send via WhatsApp + inbox write
	if err := s.sender.SendReply(ctx, orgID, phone, reply); err != nil {
		log.Printf("waagent: send reply error phone=%s org=%s: %v", phone, orgID, err)
	}
}

// sendHardcoded persists messages and sends a hardcoded reply without invoking the LLM.
func (s *Service) sendHardcoded(ctx context.Context, orgID uuid.UUID, phone, incomingText, reply string) {
	if orgID != uuid.Nil {
		_ = s.queries.InsertAgentMessage(ctx, waagentdb.InsertAgentMessageParams{
			OrganizationID: pgtypeUUID(orgID),
			PhoneNumber:    phone,
			Role:           "user",
			Content:        incomingText,
		})
		_ = s.queries.InsertAgentMessage(ctx, waagentdb.InsertAgentMessageParams{
			OrganizationID: pgtypeUUID(orgID),
			PhoneNumber:    phone,
			Role:           "assistant",
			Content:        reply,
		})
	}

	if err := s.sender.SendReply(ctx, orgID, phone, reply); err != nil {
		log.Printf("waagent: hardcoded send error phone=%s: %v", phone, err)
	}
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
