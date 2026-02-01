package service

import (
	"context"
	"strings"
	"time"

	"portal_final_backend/internal/auth/token"
	"portal_final_backend/internal/identity/repository"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

const (
	inviteTokenBytes = 32
	inviteTTL        = 72 * time.Hour
)

type Service struct {
	repo *repository.Repository
}

func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetUserOrganizationID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return s.repo.GetUserOrganizationID(ctx, userID)
}

func (s *Service) CreateOrganizationForUser(ctx context.Context, q repository.DBTX, name string, userID uuid.UUID) (uuid.UUID, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return uuid.UUID{}, apperr.Validation("organization name is required")
	}

	org, err := s.repo.CreateOrganization(ctx, q, trimmed, userID)
	if err != nil {
		return uuid.UUID{}, err
	}

	return org.ID, nil
}

func (s *Service) AddMember(ctx context.Context, q repository.DBTX, organizationID, userID uuid.UUID) error {
	return s.repo.AddMember(ctx, q, organizationID, userID)
}

func (s *Service) CreateInvite(ctx context.Context, organizationID uuid.UUID, email string, createdBy uuid.UUID) (string, time.Time, error) {
	rawToken, err := token.GenerateRandomToken(inviteTokenBytes)
	if err != nil {
		return "", time.Time{}, err
	}

	tokenHash := token.HashSHA256(rawToken)
	expiresAt := time.Now().Add(inviteTTL)

	if _, err := s.repo.CreateInvite(ctx, organizationID, email, tokenHash, expiresAt, createdBy); err != nil {
		return "", time.Time{}, err
	}

	return rawToken, expiresAt, nil
}

func (s *Service) GetOrganization(ctx context.Context, organizationID uuid.UUID) (repository.Organization, error) {
	org, err := s.repo.GetOrganization(ctx, organizationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.Organization{}, apperr.NotFound("organization not found")
		}
		return repository.Organization{}, err
	}
	return org, nil
}

func (s *Service) UpdateOrganizationName(ctx context.Context, organizationID uuid.UUID, name string) (repository.Organization, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return repository.Organization{}, apperr.Validation("organization name is required")
	}

	org, err := s.repo.UpdateOrganizationName(ctx, organizationID, trimmed)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.Organization{}, apperr.NotFound("organization not found")
		}
		return repository.Organization{}, err
	}

	return org, nil
}

func (s *Service) ResolveInvite(ctx context.Context, rawToken string) (repository.Invite, error) {
	tokenHash := token.HashSHA256(rawToken)
	invite, err := s.repo.GetInviteByToken(ctx, tokenHash)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.Invite{}, apperr.NotFound("invite not found")
		}
		return repository.Invite{}, err
	}

	if invite.UsedAt != nil {
		return repository.Invite{}, apperr.Conflict("invite already used")
	}

	if time.Now().After(invite.ExpiresAt) {
		return repository.Invite{}, apperr.Forbidden("invite expired")
	}

	return invite, nil
}

func (s *Service) UseInvite(ctx context.Context, q repository.DBTX, inviteID, userID uuid.UUID) error {
	return s.repo.UseInvite(ctx, q, inviteID, userID)
}
