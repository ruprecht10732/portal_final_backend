package inapp

import (
	"context"
	"strings"

	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

type Service struct {
	repo *Repository
	sse  *sse.Service
	log  *logger.Logger
}

func NewService(repo *Repository, log *logger.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// SetSSE injects the SSE service (circular dependency avoidance).
func (s *Service) SetSSE(sseSvc *sse.Service) {
	s.sse = sseSvc
}

type SendParams struct {
	OrgID        uuid.UUID
	UserID       uuid.UUID
	Title        string
	Content      string
	ResourceID   *uuid.UUID
	ResourceType string
	Category     string // "info", "success", "warning", "error"
}

// Send persists the notification and pushes it via SSE if the user is online.
func (s *Service) Send(ctx context.Context, p SendParams) error {
	if s == nil || s.repo == nil {
		return apperr.Internal("in-app notification service not configured")
	}

	if p.Category == "" {
		p.Category = "info"
	}

	var resourceType *string
	if p.ResourceType != "" {
		resourceType = &p.ResourceType
	}

	notif, err := s.repo.Create(ctx, CreateParams{
		OrganizationID: p.OrgID,
		UserID:         p.UserID,
		Title:          p.Title,
		Content:        p.Content,
		ResourceID:     p.ResourceID,
		ResourceType:   resourceType,
		Category:       p.Category,
	})
	if err != nil {
		if s.log != nil {
			s.log.Error("failed to persist in-app notification", "error", err, "userId", p.UserID)
		}
		return err
	}

	if s.sse != nil {
		s.sse.Publish(p.UserID, sse.Event{
			Type:    "in_app_notification",
			Message: "New Notification",
			Data:    notif,
		})
	}

	return nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]Notification, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize
	return s.repo.List(ctx, userID, pageSize, offset)
}

func (s *Service) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	return s.repo.CountUnread(ctx, userID)
}

func (s *Service) CountUnreadByResourceTypes(ctx context.Context, userID uuid.UUID, resourceTypes []string) (int, error) {
	normalized := make([]string, 0, len(resourceTypes))
	for _, item := range resourceTypes {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return s.repo.CountUnreadByResourceTypes(ctx, userID, normalized)
}

func (s *Service) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	return s.repo.MarkRead(ctx, userID, id)
}

func (s *Service) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	return s.repo.MarkAllRead(ctx, userID)
}

func (s *Service) Delete(ctx context.Context, userID, id uuid.UUID) error {
	return s.repo.Delete(ctx, userID, id)
}
