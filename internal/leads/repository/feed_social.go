package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────
// Reaction SQL
// ──────────────────────────────────────────────────

func (r *Repository) ToggleReaction(ctx context.Context, eventID, eventSource, reactionType string, userID, orgID uuid.UUID) (exists bool, err error) {
	// Try delete first – if a row was removed, the toggle means "remove"
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM RAC_feed_reactions
		WHERE event_id      = $1
		  AND event_source  = $2
		  AND reaction_type = $3
		  AND user_id       = $4
		  AND org_id        = $5
	`, eventID, eventSource, reactionType, userID, orgID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		return false, nil // removed
	}

	// Not present → insert
	_, err = r.pool.Exec(ctx, `
		INSERT INTO RAC_feed_reactions (event_id, event_source, reaction_type, user_id, org_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT DO NOTHING
	`, eventID, eventSource, reactionType, userID, orgID)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) ListReactionsByEvent(ctx context.Context, eventID, eventSource string, orgID uuid.UUID) ([]FeedReaction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT fr.id, fr.event_id, fr.event_source, fr.reaction_type, fr.user_id, u.email, fr.created_at
		FROM RAC_feed_reactions fr
		JOIN RAC_users u ON u.id = fr.user_id
		WHERE fr.event_id     = $1
		  AND fr.event_source = $2
		  AND fr.org_id       = $3
		ORDER BY fr.created_at
	`, eventID, eventSource, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FeedReaction
	for rows.Next() {
		var r FeedReaction
		if err := rows.Scan(&r.ID, &r.EventID, &r.EventSource, &r.ReactionType, &r.UserID, &r.UserEmail, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListReactionsByEvents returns reactions for a batch of event IDs (used by feed enrichment).
func (r *Repository) ListReactionsByEvents(ctx context.Context, eventIDs []string, orgID uuid.UUID) ([]FeedReaction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT fr.id, fr.event_id, fr.event_source, fr.reaction_type, fr.user_id, u.email, fr.created_at
		FROM RAC_feed_reactions fr
		JOIN RAC_users u ON u.id = fr.user_id
		WHERE fr.event_id = ANY($1)
		  AND fr.org_id   = $2
		ORDER BY fr.event_id, fr.created_at
	`, eventIDs, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FeedReaction
	for rows.Next() {
		var r FeedReaction
		if err := rows.Scan(&r.ID, &r.EventID, &r.EventSource, &r.ReactionType, &r.UserID, &r.UserEmail, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ──────────────────────────────────────────────────
// Comment SQL
// ──────────────────────────────────────────────────

func (r *Repository) CreateComment(ctx context.Context, eventID, eventSource string, userID, orgID uuid.UUID, body string, mentionIDs []uuid.UUID) (FeedComment, error) {
	var c FeedComment
	err := r.pool.QueryRow(ctx, `
		INSERT INTO RAC_feed_comments (event_id, event_source, user_id, org_id, body)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, event_id, event_source, user_id, org_id, body, created_at, updated_at
	`, eventID, eventSource, userID, orgID, body).Scan(
		&c.ID, &c.EventID, &c.EventSource, &c.UserID, &c.OrgID, &c.Body, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return FeedComment{}, err
	}

	// Insert mentions
	for _, mentionedID := range mentionIDs {
		_, _ = r.pool.Exec(ctx, `
			INSERT INTO RAC_feed_comment_mentions (comment_id, mentioned_user_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, c.ID, mentionedID)
	}

	return c, nil
}

func (r *Repository) ListCommentsByEvent(ctx context.Context, eventID, eventSource string, orgID uuid.UUID) ([]FeedCommentWithAuthor, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.event_id, c.event_source, c.user_id, u.email, c.body, c.created_at, c.updated_at
		FROM RAC_feed_comments c
		JOIN RAC_users u ON u.id = c.user_id
		WHERE c.event_id     = $1
		  AND c.event_source = $2
		  AND c.org_id       = $3
		ORDER BY c.created_at ASC
	`, eventID, eventSource, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FeedCommentWithAuthor
	for rows.Next() {
		var c FeedCommentWithAuthor
		if err := rows.Scan(&c.ID, &c.EventID, &c.EventSource, &c.UserID, &c.UserEmail, &c.Body, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
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
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM RAC_feed_comments
		WHERE id      = $1
		  AND user_id = $2
		  AND org_id  = $3
	`, commentID, userID, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListCommentCountsByEvents returns comment counts per event_id for a batch (feed enrichment).
func (r *Repository) ListCommentCountsByEvents(ctx context.Context, eventIDs []string, orgID uuid.UUID) (map[string]int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT event_id, COUNT(*)::int
		FROM RAC_feed_comments
		WHERE event_id = ANY($1)
		  AND org_id   = $2
		GROUP BY event_id
	`, eventIDs, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]int)
	for rows.Next() {
		var id string
		var cnt int
		if err := rows.Scan(&id, &cnt); err != nil {
			return nil, err
		}
		out[id] = cnt
	}
	return out, rows.Err()
}

// ──────────────────────────────────────────────────
// Mentions helper
// ──────────────────────────────────────────────────

func (r *Repository) listMentionsByComments(ctx context.Context, commentIDs []uuid.UUID) (map[uuid.UUID][]CommentMention, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT m.comment_id, m.mentioned_user_id, u.email
		FROM RAC_feed_comment_mentions m
		JOIN RAC_users u ON u.id = m.mentioned_user_id
		WHERE m.comment_id = ANY($1)
		ORDER BY m.created_at
	`, commentIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[uuid.UUID][]CommentMention)
	for rows.Next() {
		var commentID uuid.UUID
		var m CommentMention
		if err := rows.Scan(&commentID, &m.UserID, &m.Email); err != nil {
			return nil, err
		}
		out[commentID] = append(out[commentID], m)
	}
	return out, rows.Err()
}

// ──────────────────────────────────────────────────
// Org members (for @-mention autocomplete)
// ──────────────────────────────────────────────────

func (r *Repository) ListOrgMembers(ctx context.Context, orgID uuid.UUID) ([]OrgMember, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.email
		FROM RAC_organization_members om
		JOIN RAC_users u ON u.id = om.user_id
		WHERE om.organization_id = $1
		ORDER BY u.email
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OrgMember
	for rows.Next() {
		var m OrgMember
		if err := rows.Scan(&m.ID, &m.Email); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
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
}
