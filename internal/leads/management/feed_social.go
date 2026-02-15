// Package management — social interaction logic for feed reactions, comments, and @-mentions.
package management

import (
	"context"
	"fmt"
	"strings"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/internal/notification/inapp"

	"github.com/google/uuid"
)

const (
	commentCategoryInfo      = "info"
	fallbackAuthorLabel      = "Een collega"
	mentionedTitle           = "Je bent vermeld in een reactie"
	participantReplyTitle    = "Nieuwe reactie in gesprek"
	mentionedContentTemplate = "%s vermeldde je: \"%s\""
	replyContentTemplate     = "%s plaatste een nieuwe reactie: \"%s\""
	commentExcerptMaxRunes   = 120
)

// SocialRepository defines the data access needed by feed social operations.
type SocialRepository interface {
	repository.FeedReactionStore
	repository.FeedCommentStore
	repository.OrgMemberReader
}

type commentNotificationContext struct {
	ctx          context.Context
	orgID        uuid.UUID
	authorID     uuid.UUID
	authorEmail  string
	excerpt      string
	resourcePtr  *uuid.UUID
	resourceType string
	notified     map[uuid.UUID]struct{}
}

// ──────────────────────────────────────────────────
// Reactions
// ──────────────────────────────────────────────────

// ToggleReaction toggles a reaction on/off and returns the updated summary.
func (s *Service) ToggleReaction(ctx context.Context, eventID, eventSource, reactionType string, userID, orgID uuid.UUID) (transport.ToggleReactionResponse, error) {
	active, err := s.repo.ToggleReaction(ctx, eventID, eventSource, reactionType, userID, orgID)
	if err != nil {
		return transport.ToggleReactionResponse{}, err
	}

	reactions, err := s.repo.ListReactionsByEvent(ctx, eventID, eventSource, orgID)
	if err != nil {
		return transport.ToggleReactionResponse{}, err
	}

	return transport.ToggleReactionResponse{
		Active:    active,
		Reactions: buildReactionSummary(reactions, userID),
	}, nil
}

// ListReactions returns the reaction summary for a single feed event.
func (s *Service) ListReactions(ctx context.Context, eventID, eventSource string, userID, orgID uuid.UUID) ([]transport.ReactionSummary, error) {
	reactions, err := s.repo.ListReactionsByEvent(ctx, eventID, eventSource, orgID)
	if err != nil {
		return nil, err
	}
	return buildReactionSummary(reactions, userID), nil
}

func buildReactionSummary(reactions []repository.FeedReaction, currentUserID uuid.UUID) []transport.ReactionSummary {
	type bucket struct {
		users []string
		me    bool
	}
	groups := map[string]*bucket{}
	order := []string{}

	for _, r := range reactions {
		b, ok := groups[r.ReactionType]
		if !ok {
			b = &bucket{}
			groups[r.ReactionType] = b
			order = append(order, r.ReactionType)
		}
		b.users = append(b.users, r.UserEmail)
		if r.UserID == currentUserID {
			b.me = true
		}
	}

	out := make([]transport.ReactionSummary, 0, len(order))
	for _, t := range order {
		b := groups[t]
		out = append(out, transport.ReactionSummary{
			Type:  t,
			Count: len(b.users),
			Users: b.users,
			Me:    b.me,
		})
	}
	return out
}

// ──────────────────────────────────────────────────
// Comments
// ──────────────────────────────────────────────────

// CreateComment creates a comment (with optional @-mentions) and returns the full thread.
func (s *Service) CreateComment(ctx context.Context, eventID, eventSource string, userID, orgID uuid.UUID, body string, mentionIDs []uuid.UUID) (transport.CommentListResponse, error) {
	created, err := s.repo.CreateComment(ctx, eventID, eventSource, userID, orgID, body, mentionIDs)
	if err != nil {
		return transport.CommentListResponse{}, err
	}

	s.sendCommentNotifications(ctx, created, userID, orgID, body, mentionIDs)

	return s.ListComments(ctx, eventID, eventSource, orgID)
}

func (s *Service) sendCommentNotifications(
	ctx context.Context,
	created repository.FeedComment,
	authorID uuid.UUID,
	orgID uuid.UUID,
	body string,
	mentionIDs []uuid.UUID,
) {
	if s.inAppService == nil {
		return
	}

	authorEmail, ok := s.resolveAuthorEmail(ctx, orgID, authorID)
	if !ok {
		return
	}
	excerpt := commentExcerpt(body)
	resourceID, hasResourceID := parseEventResourceID(created.EventID)
	resourceType := mapEventSourceToResourceType(created.EventSource)
	resourcePtr := resourcePointer(resourceID, hasResourceID)

	notifCtx := commentNotificationContext{
		ctx:          ctx,
		orgID:        orgID,
		authorID:     authorID,
		authorEmail:  authorEmail,
		excerpt:      excerpt,
		resourcePtr:  resourcePtr,
		resourceType: resourceType,
		notified:     make(map[uuid.UUID]struct{}),
	}
	s.notifyMentionedUsers(notifCtx, mentionIDs)

	comments, err := s.repo.ListCommentsByEvent(ctx, created.EventID, created.EventSource, orgID)
	if err != nil {
		return
	}
	s.notifyCommentParticipants(notifCtx, comments)
}

func (s *Service) resolveAuthorEmail(ctx context.Context, orgID, authorID uuid.UUID) (string, bool) {
	members, err := s.repo.ListOrgMembers(ctx, orgID)
	if err != nil {
		return "", false
	}
	emailByID := make(map[uuid.UUID]string, len(members))
	for _, member := range members {
		emailByID[member.ID] = member.Email
	}
	authorEmail := strings.TrimSpace(emailByID[authorID])
	if authorEmail == "" {
		authorEmail = fallbackAuthorLabel
	}
	return authorEmail, true
}

func commentExcerpt(body string) string {
	excerpt := strings.TrimSpace(body)
	runes := []rune(excerpt)
	if len(runes) <= commentExcerptMaxRunes {
		return excerpt
	}
	return string(runes[:commentExcerptMaxRunes]) + "..."
}

func resourcePointer(resourceID uuid.UUID, hasResourceID bool) *uuid.UUID {
	if !hasResourceID {
		return nil
	}
	return &resourceID
}

func shouldNotifyRecipient(recipientID, authorID uuid.UUID, notified map[uuid.UUID]struct{}) bool {
	if recipientID == authorID {
		return false
	}
	if _, exists := notified[recipientID]; exists {
		return false
	}
	notified[recipientID] = struct{}{}
	return true
}

func (s *Service) notifyMentionedUsers(notifCtx commentNotificationContext, mentionIDs []uuid.UUID) {
	for _, mentionedID := range mentionIDs {
		if !shouldNotifyRecipient(mentionedID, notifCtx.authorID, notifCtx.notified) {
			continue
		}
		s.sendInAppCommentNotification(
			notifCtx.ctx,
			notifCtx.orgID,
			mentionedID,
			mentionedTitle,
			fmt.Sprintf(mentionedContentTemplate, notifCtx.authorEmail, notifCtx.excerpt),
			notifCtx.resourcePtr,
			notifCtx.resourceType,
		)
	}
}

func (s *Service) notifyCommentParticipants(notifCtx commentNotificationContext, comments []repository.FeedCommentWithAuthor) {
	for _, comment := range comments {
		recipientID := comment.UserID
		if !shouldNotifyRecipient(recipientID, notifCtx.authorID, notifCtx.notified) {
			continue
		}
		s.sendInAppCommentNotification(
			notifCtx.ctx,
			notifCtx.orgID,
			recipientID,
			participantReplyTitle,
			fmt.Sprintf(replyContentTemplate, notifCtx.authorEmail, notifCtx.excerpt),
			notifCtx.resourcePtr,
			notifCtx.resourceType,
		)
	}
}

func (s *Service) sendInAppCommentNotification(
	ctx context.Context,
	orgID uuid.UUID,
	userID uuid.UUID,
	title string,
	content string,
	resourceID *uuid.UUID,
	resourceType string,
) {
	_ = s.inAppService.Send(ctx, inapp.SendParams{
		OrgID:        orgID,
		UserID:       userID,
		Title:        title,
		Content:      content,
		ResourceID:   resourceID,
		ResourceType: resourceType,
		Category:     commentCategoryInfo,
	})
}

func parseEventResourceID(eventID string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(eventID))
	if err != nil {
		return uuid.UUID{}, false
	}
	return id, true
}

func mapEventSourceToResourceType(eventSource string) string {
	switch strings.ToLower(strings.TrimSpace(eventSource)) {
	case "quotes", "quote":
		return "quote"
	case "appointments", "appointment":
		return "appointment"
	case "leads", "lead":
		fallthrough
	default:
		return "lead"
	}
}

// ListComments returns the full comment thread for a feed event.
func (s *Service) ListComments(ctx context.Context, eventID, eventSource string, orgID uuid.UUID) (transport.CommentListResponse, error) {
	rows, err := s.repo.ListCommentsByEvent(ctx, eventID, eventSource, orgID)
	if err != nil {
		return transport.CommentListResponse{}, err
	}

	items := make([]transport.CommentItem, 0, len(rows))
	for _, c := range rows {
		mentions := make([]transport.MentionItem, 0, len(c.Mentions))
		for _, m := range c.Mentions {
			mentions = append(mentions, transport.MentionItem{UserID: m.UserID.String(), Email: m.Email})
		}
		items = append(items, transport.CommentItem{
			ID:        c.ID.String(),
			UserEmail: c.UserEmail,
			Body:      c.Body,
			Mentions:  mentions,
			CreatedAt: c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return transport.CommentListResponse{Comments: items}, nil
}

// DeleteComment removes a comment (only the author can delete).
func (s *Service) DeleteComment(ctx context.Context, commentID, userID, orgID uuid.UUID) error {
	return s.repo.DeleteComment(ctx, commentID, userID, orgID)
}

// ──────────────────────────────────────────────────
// Org Members (for @-mention autocomplete)
// ──────────────────────────────────────────────────

// ListOrgMembers returns all users in the organisation, for @-mention autocomplete.
func (s *Service) ListOrgMembers(ctx context.Context, orgID uuid.UUID) (transport.OrgMembersResponse, error) {
	members, err := s.repo.ListOrgMembers(ctx, orgID)
	if err != nil {
		return transport.OrgMembersResponse{}, err
	}

	items := make([]transport.OrgMemberItem, 0, len(members))
	for _, m := range members {
		items = append(items, transport.OrgMemberItem{ID: m.ID.String(), Email: m.Email})
	}

	return transport.OrgMembersResponse{Members: items}, nil
}
