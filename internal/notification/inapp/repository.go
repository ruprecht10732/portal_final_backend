package inapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	notificationdb "portal_final_backend/internal/notification/db"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
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
	pool    *pgxpool.Pool
	queries *notificationdb.Queries
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	if pool == nil {
		return &Repository{}
	}
	return &Repository{pool: pool, queries: notificationdb.New(pool)}
}

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func toPgUUIDPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return toPgUUID(*id)
}

func toPgText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func notificationFromModel(model notificationdb.RacInAppNotification) Notification {
	return Notification{
		ID:           uuid.UUID(model.ID.Bytes),
		UserID:       uuid.UUID(model.UserID.Bytes),
		Title:        model.Title,
		Content:      model.Content,
		ResourceID:   optionalUUID(model.ResourceID),
		ResourceType: optionalString(model.ResourceType),
		Category:     model.Category,
		IsRead:       model.IsRead,
		CreatedAt:    model.CreatedAt.Time,
	}
}

func optionalUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := uuid.UUID(value.Bytes)
	return &id
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

	model, err := r.queries.CreateInAppNotification(ctx, notificationdb.CreateInAppNotificationParams{
		OrganizationID: toPgUUID(p.OrganizationID),
		UserID:         toPgUUID(p.UserID),
		Title:          p.Title,
		Content:        p.Content,
		ResourceID:     toPgUUIDPtr(p.ResourceID),
		ResourceType:   toPgText(p.ResourceType),
		Category:       category,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return Notification{}, apperr.Validation("invalid organizationId or userId").WithOp(opCreate)
		}
		return Notification{}, apperr.Internal(fmt.Sprintf("create in-app notification failed: %v", err)).WithOp(opCreate)
	}

	return notificationFromModel(model), nil
}

func (r *Repository) List(ctx context.Context, userID uuid.UUID, limit, offset int) ([]Notification, int, error) {
	if r == nil || r.pool == nil {
		return nil, 0, apperr.Internal(errRepoNotConfigured).WithOp(opList)
	}
	if userID == uuid.Nil {
		return nil, 0, apperr.Validation(errUserIDRequired).WithOp(opList)
	}

	total, err := r.queries.CountInAppNotifications(ctx, toPgUUID(userID))
	if err != nil {
		return nil, 0, apperr.Internal(fmt.Sprintf("count notifications failed: %v", err)).WithOp(opList)
	}

	rows, err := r.queries.ListInAppNotifications(ctx, notificationdb.ListInAppNotificationsParams{
		UserID:      toPgUUID(userID),
		OffsetCount: int32(offset),
		LimitCount:  int32(limit),
	})
	if err != nil {
		return nil, 0, apperr.Internal(fmt.Sprintf("list notifications query failed: %v", err)).WithOp(opList)
	}

	items := make([]Notification, 0, limit)
	for _, row := range rows {
		items = append(items, notificationFromModel(row))
	}

	return items, int(total), nil
}

func (r *Repository) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	if r == nil || r.pool == nil {
		return 0, apperr.Internal(errRepoNotConfigured).WithOp(opCountUnread)
	}
	if userID == uuid.Nil {
		return 0, apperr.Validation(errUserIDRequired).WithOp(opCountUnread)
	}

	count, err := r.queries.CountUnreadInAppNotifications(ctx, toPgUUID(userID))
	if err != nil {
		return 0, apperr.Internal(fmt.Sprintf("count unread notifications failed: %v", err)).WithOp(opCountUnread)
	}

	return int(count), nil
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

	count, err := r.queries.CountUnreadInAppNotificationsByResourceTypes(ctx, notificationdb.CountUnreadInAppNotificationsByResourceTypesParams{
		UserID:        toPgUUID(userID),
		ResourceTypes: resourceTypes,
	})
	if err != nil {
		return 0, apperr.Internal(fmt.Sprintf("count unread notifications by resource failed: %v", err)).WithOp(opCountUnreadByResource)
	}

	return int(count), nil
}

func (r *Repository) MarkRead(ctx context.Context, userID, notificationID uuid.UUID) error {
	if r == nil || r.pool == nil {
		return apperr.Internal(errRepoNotConfigured).WithOp(opMarkRead)
	}
	if userID == uuid.Nil || notificationID == uuid.Nil {
		return apperr.Validation("userId and notificationId are required").WithOp(opMarkRead)
	}

	err := r.queries.MarkInAppNotificationRead(ctx, notificationdb.MarkInAppNotificationReadParams{
		NotificationID: toPgUUID(notificationID),
		UserID:         toPgUUID(userID),
	})
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

	err := r.queries.MarkAllInAppNotificationsRead(ctx, toPgUUID(userID))
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

	err := r.queries.DeleteInAppNotification(ctx, notificationdb.DeleteInAppNotificationParams{
		NotificationID: toPgUUID(notificationID),
		UserID:         toPgUUID(userID),
	})
	if err != nil {
		return apperr.Internal(fmt.Sprintf("delete notification failed: %v", err)).WithOp(opDelete)
	}

	return nil
}
