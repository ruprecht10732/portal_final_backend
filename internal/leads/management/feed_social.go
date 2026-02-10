// Package management — social interaction logic for feed reactions, comments, and @-mentions.
package management

import (
	"context"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"

	"github.com/google/uuid"
)

// SocialRepository defines the data access needed by feed social operations.
type SocialRepository interface {
	repository.FeedReactionStore
	repository.FeedCommentStore
	repository.OrgMemberReader
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
	_, err := s.repo.CreateComment(ctx, eventID, eventSource, userID, orgID, body, mentionIDs)
	if err != nil {
		return transport.CommentListResponse{}, err
	}

	return s.ListComments(ctx, eventID, eventSource, orgID)
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
