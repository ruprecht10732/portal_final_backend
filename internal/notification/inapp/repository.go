package inapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	opCreate                = "notification.inapp.repository.create"
	opList                  = "notification.inapp.repository.list"
	opCountUnread           = "notification.inapp.repository.count_unread"
	opCountUnreadByResource = "notification.inapp.repository.count_unread_by_resource"
	opMarkRead              = "notification.inapp.repository.mark_read"
	opMarkAllRead           = "notification.inapp.repository.mark_all_read"
	opDelete                = "notification.inapp.repository.delete"

	errRepoNotConfigured = "in-app notification repository not configured"
	errUserIDRequired    = "userId is required"
)

type Notification struct {
	ID           uuid.UUID  `json:"id"`
	UserID       uuid.UUID  `json:"userId"`
	Title        string     `json:"title"`
	Content      string     `json:"content"`
	ResourceID   *uuid.UUID `json:"resourceId,omitempty"`
	ResourceType *string    `json:"resourceType,omitempty"`
	Category     string     `json:"category"`
	IsRead       bool       `json:"isRead"`
	CreatedAt    time.Time  `json:"createdAt"`
}

type CreateParams struct {
	OrganizationID uuid.UUID
	UserID         uuid.UUID
	Title          string
	Content        string
	ResourceID     *uuid.UUID
	ResourceType   *string
	Category       string
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, p CreateParams) (Notification, error) {
	if r == nil || r.pool == nil {
		return Notification{}, apperr.Internal(errRepoNotConfigured).WithOp(opCreate)
	}
	if p.OrganizationID == uuid.Nil || p.UserID == uuid.Nil {
		return Notification{}, apperr.Validation("organizationId and userId are required").WithOp(opCreate)
	}
	if p.Title == "" || p.Content == "" {
		return Notification{}, apperr.Validation("title and content are required").WithOp(opCreate)
	}

	category := p.Category
	if category == "" {
		category = "info"
	}

	var n Notification
	err := r.pool.QueryRow(ctx, `
		INSERT INTO RAC_in_app_notifications 
		(organization_id, user_id, title, content, resource_id, resource_type, category)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, title, content, resource_id, resource_type, category, is_read, created_at
	`, p.OrganizationID, p.UserID, p.Title, p.Content, p.ResourceID, p.ResourceType, category).Scan(
		&n.ID, &n.UserID, &n.Title, &n.Content, &n.ResourceID, &n.ResourceType, &n.Category, &n.IsRead, &n.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return Notification{}, apperr.Validation("invalid organizationId or userId").WithOp(opCreate)
		}
		return Notification{}, apperr.Internal(fmt.Sprintf("create in-app notification failed: %v", err)).WithOp(opCreate)
	}

	return n, nil
}

func (r *Repository) List(ctx context.Context, userID uuid.UUID, limit, offset int) ([]Notification, int, error) {
	if r == nil || r.pool == nil {
		return nil, 0, apperr.Internal(errRepoNotConfigured).WithOp(opList)
	}
	if userID == uuid.Nil {
		return nil, 0, apperr.Validation(errUserIDRequired).WithOp(opList)
	}

	var total int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM RAC_in_app_notifications WHERE user_id = $1`, userID).Scan(&total)
	if err != nil {
		return nil, 0, apperr.Internal(fmt.Sprintf("count notifications failed: %v", err)).WithOp(opList)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, title, content, resource_id, resource_type, category, is_read, created_at
		FROM RAC_in_app_notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, 0, apperr.Internal(fmt.Sprintf("list notifications query failed: %v", err)).WithOp(opList)
	}
	defer rows.Close()

	items := make([]Notification, 0, limit)
	for rows.Next() {
		var n Notification
		if scanErr := rows.Scan(&n.ID, &n.UserID, &n.Title, &n.Content, &n.ResourceID, &n.ResourceType, &n.Category, &n.IsRead, &n.CreatedAt); scanErr != nil {
			return nil, 0, apperr.Internal(fmt.Sprintf("scan notifications failed: %v", scanErr)).WithOp(opList)
		}
		items = append(items, n)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, 0, apperr.Internal(fmt.Sprintf("iterate notifications failed: %v", rowsErr)).WithOp(opList)
	}

	return items, total, nil
}

func (r *Repository) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	if r == nil || r.pool == nil {
		return 0, apperr.Internal(errRepoNotConfigured).WithOp(opCountUnread)
	}
	if userID == uuid.Nil {
		return 0, apperr.Validation(errUserIDRequired).WithOp(opCountUnread)
	}

	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM RAC_in_app_notifications 
		WHERE user_id = $1 AND is_read = FALSE
	`, userID).Scan(&count)
	if err != nil {
		return 0, apperr.Internal(fmt.Sprintf("count unread notifications failed: %v", err)).WithOp(opCountUnread)
	}

	return count, nil
}

func (r *Repository) CountUnreadByResourceTypes(ctx context.Context, userID uuid.UUID, resourceTypes []string) (int, error) {
	if r == nil || r.pool == nil {
		return 0, apperr.Internal(errRepoNotConfigured).WithOp(opCountUnreadByResource)
	}
	if userID == uuid.Nil {
		return 0, apperr.Validation(errUserIDRequired).WithOp(opCountUnreadByResource)
	}
	if len(resourceTypes) == 0 {
		return r.CountUnread(ctx, userID)
	}

	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM RAC_in_app_notifications
		WHERE user_id = $1 AND is_read = FALSE AND resource_type = ANY($2)
	`, userID, resourceTypes).Scan(&count)
	if err != nil {
		return 0, apperr.Internal(fmt.Sprintf("count unread notifications by resource failed: %v", err)).WithOp(opCountUnreadByResource)
	}

	return count, nil
}

func (r *Repository) MarkRead(ctx context.Context, userID, notificationID uuid.UUID) error {
	if r == nil || r.pool == nil {
		return apperr.Internal(errRepoNotConfigured).WithOp(opMarkRead)
	}
	if userID == uuid.Nil || notificationID == uuid.Nil {
		return apperr.Validation("userId and notificationId are required").WithOp(opMarkRead)
	}

	_, err := r.pool.Exec(ctx, `
		UPDATE RAC_in_app_notifications 
		SET is_read = TRUE, read_at = now() 
		WHERE id = $1 AND user_id = $2
	`, notificationID, userID)
	if err != nil {
		return apperr.Internal(fmt.Sprintf("mark notification read failed: %v", err)).WithOp(opMarkRead)
	}

	return nil
}

func (r *Repository) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	if r == nil || r.pool == nil {
		return apperr.Internal(errRepoNotConfigured).WithOp(opMarkAllRead)
	}
	if userID == uuid.Nil {
		return apperr.Validation(errUserIDRequired).WithOp(opMarkAllRead)
	}

	_, err := r.pool.Exec(ctx, `
		UPDATE RAC_in_app_notifications 
		SET is_read = TRUE, read_at = now() 
		WHERE user_id = $1 AND is_read = FALSE
	`, userID)
	if err != nil {
		return apperr.Internal(fmt.Sprintf("mark all notifications read failed: %v", err)).WithOp(opMarkAllRead)
	}

	return nil
}

func (r *Repository) Delete(ctx context.Context, userID, notificationID uuid.UUID) error {
	if r == nil || r.pool == nil {
		return apperr.Internal(errRepoNotConfigured).WithOp(opDelete)
	}
	if userID == uuid.Nil || notificationID == uuid.Nil {
		return apperr.Validation("userId and notificationId are required").WithOp(opDelete)
	}

	_, err := r.pool.Exec(ctx, `
		DELETE FROM RAC_in_app_notifications
		WHERE id = $1 AND user_id = $2
	`, notificationID, userID)
	if err != nil {
		return apperr.Internal(fmt.Sprintf("delete notification failed: %v", err)).WithOp(opDelete)
	}

	return nil
}
