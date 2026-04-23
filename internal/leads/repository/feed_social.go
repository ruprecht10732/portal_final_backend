package repository

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"

	leadsdb "portal_final_backend/internal/leads/db"
)

// ──────────────────────────────────────────────────
// Reaction SQL
// ──────────────────────────────────────────────────

func (r *Repository) ToggleReaction(ctx context.Context, eventID, eventSource, reactionType string, userID, orgID uuid.UUID) (exists bool, err error) {
	// Try delete first – if a row was removed, the toggle means "remove"
	rowsAffected, err := r.queries.DeleteFeedReaction(ctx, leadsdb.DeleteFeedReactionParams{
		EventID:      eventID,
		EventSource:  eventSource,
		ReactionType: reactionType,
		UserID:       toPgUUID(userID),
		OrgID:        toPgUUID(orgID),
	})
	if err != nil {
		return false, fmt.Errorf("delete feed reaction: %w", err)
	}
	if rowsAffected > 0 {
		return false, nil // removed
	}

	// Not present → insert
	if err := r.queries.CreateFeedReaction(ctx, leadsdb.CreateFeedReactionParams{
		EventID:      eventID,
		EventSource:  eventSource,
		ReactionType: reactionType,
		UserID:       toPgUUID(userID),
		OrgID:        toPgUUID(orgID),
	}); err != nil {
		return false, fmt.Errorf("create feed reaction: %w", err)
	}
	return true, nil
}

func (r *Repository) ListReactionsByEvent(ctx context.Context, eventID, eventSource string, orgID uuid.UUID) ([]FeedReaction, error) {
	rows, err := r.queries.ListReactionsByEvent(ctx, leadsdb.ListReactionsByEventParams{
		EventID:     eventID,
		EventSource: eventSource,
		OrgID:       toPgUUID(orgID),
	})
	if err != nil {
		return nil, err
	}

	out := make([]FeedReaction, 0, len(rows))
	for _, row := range rows {
		out = append(out, feedReactionFromRow(row))
	}
	return out, nil
}

// ListReactionsByEvents returns reactions for a batch of event IDs (used by feed enrichment).
func (r *Repository) ListReactionsByEvents(ctx context.Context, eventIDs []string, orgID uuid.UUID) ([]FeedReaction, error) {
	rows, err := r.queries.ListReactionsByEvents(ctx, leadsdb.ListReactionsByEventsParams{
		Column1: eventIDs,
		OrgID:   toPgUUID(orgID),
	})
	if err != nil {
		return nil, err
	}

	out := make([]FeedReaction, 0, len(rows))
	for _, row := range rows {
		out = append(out, feedReactionFromBatchRow(row))
	}
	return out, nil
}

// ──────────────────────────────────────────────────
// Comment SQL
// ──────────────────────────────────────────────────

func (r *Repository) CreateComment(ctx context.Context, eventID, eventSource string, userID, orgID uuid.UUID, body string, mentionIDs []uuid.UUID) (FeedComment, error) {
	// Use a transaction so comment + mentions are atomic.
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return FeedComment{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)

	row, err := qtx.CreateFeedComment(ctx, leadsdb.CreateFeedCommentParams{
		EventID:     eventID,
		EventSource: eventSource,
		UserID:      toPgUUID(userID),
		OrgID:       toPgUUID(orgID),
		Body:        body,
	})
	if err != nil {
		return FeedComment{}, err
	}
	c := feedCommentFromRow(row)

	for _, mentionedID := range mentionIDs {
		if err := qtx.CreateFeedCommentMention(ctx, leadsdb.CreateFeedCommentMentionParams{
			CommentID:       toPgUUID(c.ID),
			MentionedUserID: toPgUUID(mentionedID),
		}); err != nil {
			return FeedComment{}, fmt.Errorf("insert mention: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return FeedComment{}, fmt.Errorf("commit tx: %w", err)
	}
	return c, nil
}

func (r *Repository) ListCommentsByEvent(ctx context.Context, eventID, eventSource string, orgID uuid.UUID) ([]FeedCommentWithAuthor, error) {
	rows, err := r.queries.ListCommentsByEvent(ctx, leadsdb.ListCommentsByEventParams{
		EventID:     eventID,
		EventSource: eventSource,
		OrgID:       toPgUUID(orgID),
	})
	if err != nil {
		return nil, err
	}

	out := make([]FeedCommentWithAuthor, 0, len(rows))
	for _, row := range rows {
		out = append(out, feedCommentWithAuthorFromRow(row))
	}

	// Batch-load mentions for all returned comments
	if len(out) > 0 {
		commentIDs := make([]uuid.UUID, len(out))
		for i, c := range out {
			commentIDs[i] = c.ID
		}
		mentions, err := r.listMentionsByComments(ctx, commentIDs)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].Mentions = mentions[out[i].ID]
		}
	}

	return out, nil
}

func (r *Repository) DeleteComment(ctx context.Context, commentID uuid.UUID, userID, orgID uuid.UUID) error {
	rowsAffected, err := r.queries.DeleteFeedComment(ctx, leadsdb.DeleteFeedCommentParams{
		ID:     toPgUUID(commentID),
		UserID: toPgUUID(userID),
		OrgID:  toPgUUID(orgID),
	})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ListCommentCountsByEvents returns comment counts per event_id for a batch (feed enrichment).
func (r *Repository) ListCommentCountsByEvents(ctx context.Context, eventIDs []string, orgID uuid.UUID) (map[string]int, error) {
	rows, err := r.queries.ListCommentCountsByEvents(ctx, leadsdb.ListCommentCountsByEventsParams{
		Column1: eventIDs,
		OrgID:   toPgUUID(orgID),
	})
	if err != nil {
		return nil, err
	}

	out := make(map[string]int)
	for _, row := range rows {
		out[row.EventID] = int(row.CommentCount)
	}
	return out, nil
}

// ──────────────────────────────────────────────────
// Mentions helper
// ──────────────────────────────────────────────────

func (r *Repository) listMentionsByComments(ctx context.Context, commentIDs []uuid.UUID) (map[uuid.UUID][]CommentMention, error) {
	rows, err := r.queries.ListMentionsByComments(ctx, toPgUUIDSlice(commentIDs))
	if err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID][]CommentMention)
	for _, row := range rows {
		commentID := uuid.UUID(row.CommentID.Bytes)
		out[commentID] = append(out[commentID], CommentMention{
			UserID: uuid.UUID(row.MentionedUserID.Bytes),
			Email:  row.Email,
		})
	}
	return out, nil
}

// ──────────────────────────────────────────────────
// Org members (for @-mention autocomplete)
// ──────────────────────────────────────────────────

func (r *Repository) ListOrgMembers(ctx context.Context, orgID uuid.UUID) ([]OrgMember, error) {
	rows, err := r.queries.ListLeadOrgMembers(ctx, toPgUUID(orgID))
	if err != nil {
		return nil, err
	}

	out := make([]OrgMember, 0, len(rows))
	for _, row := range rows {
		roles, err := stringSliceFromAny(row.Roles)
		if err != nil {
			return nil, err
		}
		out = append(out, OrgMember{ID: uuid.UUID(row.ID.Bytes), Email: row.Email, Roles: roles})
	}
	return out, nil
}

func feedReactionFromRow(row leadsdb.ListReactionsByEventRow) FeedReaction {
	return FeedReaction{
		ID:           uuid.UUID(row.ID.Bytes),
		EventID:      row.EventID,
		EventSource:  row.EventSource,
		ReactionType: row.ReactionType,
		UserID:       uuid.UUID(row.UserID.Bytes),
		UserEmail:    row.Email,
		CreatedAt:    row.CreatedAt.Time,
	}
}

func feedReactionFromBatchRow(row leadsdb.ListReactionsByEventsRow) FeedReaction {
	return FeedReaction{
		ID:           uuid.UUID(row.ID.Bytes),
		EventID:      row.EventID,
		EventSource:  row.EventSource,
		ReactionType: row.ReactionType,
		UserID:       uuid.UUID(row.UserID.Bytes),
		UserEmail:    row.Email,
		CreatedAt:    row.CreatedAt.Time,
	}
}

func feedCommentFromRow(row leadsdb.RacFeedComment) FeedComment {
	return FeedComment{
		ID:          uuid.UUID(row.ID.Bytes),
		EventID:     row.EventID,
		EventSource: row.EventSource,
		UserID:      uuid.UUID(row.UserID.Bytes),
		OrgID:       uuid.UUID(row.OrgID.Bytes),
		Body:        row.Body,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}

func feedCommentWithAuthorFromRow(row leadsdb.ListCommentsByEventRow) FeedCommentWithAuthor {
	return FeedCommentWithAuthor{
		ID:          uuid.UUID(row.ID.Bytes),
		EventID:     row.EventID,
		EventSource: row.EventSource,
		UserID:      uuid.UUID(row.UserID.Bytes),
		UserEmail:   row.Email,
		Body:        row.Body,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}

func stringSliceFromAny(value any) ([]string, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case []string:
		return typed, nil
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, fmt.Sprint(item))
		}
		return out, nil
	default:
		reflected := reflect.ValueOf(value)
		if reflected.Kind() == reflect.Slice {
			out := make([]string, 0, reflected.Len())
			for index := 0; index < reflected.Len(); index++ {
				out = append(out, fmt.Sprint(reflected.Index(index).Interface()))
			}
			return out, nil
		}
		return nil, fmt.Errorf("unsupported roles type %T", value)
	}
}

// ──────────────────────────────────────────────────
// Structs
// ──────────────────────────────────────────────────

type FeedReaction struct {
	ID           uuid.UUID
	EventID      string
	EventSource  string
	ReactionType string
	UserID       uuid.UUID
	UserEmail    string
	CreatedAt    time.Time
}

type FeedComment struct {
	ID          uuid.UUID
	EventID     string
	EventSource string
	UserID      uuid.UUID
	OrgID       uuid.UUID
	Body        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type FeedCommentWithAuthor struct {
	ID          uuid.UUID
	EventID     string
	EventSource string
	UserID      uuid.UUID
	UserEmail   string
	Body        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Mentions    []CommentMention
}

type CommentMention struct {
	UserID uuid.UUID
	Email  string
}

type OrgMember struct {
	ID    uuid.UUID
	Email string
	Roles []string
}
