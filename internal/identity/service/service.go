package service

import (
	"context"
	"regexp"
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

func (s *Service) UpdateOrganizationProfile(
	ctx context.Context,
	organizationID uuid.UUID,
	name *string,
	email *string,
	phone *string,
	vatNumber *string,
	kvkNumber *string,
	addressLine1 *string,
	addressLine2 *string,
	postalCode *string,
	city *string,
	country *string,
) (repository.Organization, error) {
	name = normalizeOptional(name)
	email = normalizeOptional(email)
	phone = normalizeOptional(phone)
	vatNumber = normalizeOptional(vatNumber)
	kvkNumber = normalizeOptional(kvkNumber)
	addressLine1 = normalizeOptional(addressLine1)
	addressLine2 = normalizeOptional(addressLine2)
	postalCode = normalizeOptional(postalCode)
	city = normalizeOptional(city)
	country = normalizeOptional(country)

	if name != nil && *name == "" {
		return repository.Organization{}, apperr.Validation("organization name is required")
	}

	if vatNumber != nil && !isValidNLVAT(*vatNumber) {
		return repository.Organization{}, apperr.Validation("invalid VAT number")
	}

	if kvkNumber != nil && !isValidKVK(*kvkNumber) {
		return repository.Organization{}, apperr.Validation("invalid KVK number")
	}

	org, err := s.repo.UpdateOrganizationProfile(
		ctx,
		organizationID,
		name,
		email,
		phone,
		vatNumber,
		kvkNumber,
		addressLine1,
		addressLine2,
		postalCode,
		city,
		country,
	)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.Organization{}, apperr.NotFound("organization not found")
		}
		return repository.Organization{}, err
	}

	return org, nil
}

func normalizeOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

var nlVATPattern = regexp.MustCompile(`^NL[0-9]{9}B[0-9]{2}$`)
var kvkPattern = regexp.MustCompile(`^[0-9]{8}$`)

func isValidNLVAT(value string) bool {
	return nlVATPattern.MatchString(strings.ToUpper(strings.TrimSpace(value)))
}

func isValidKVK(value string) bool {
	return kvkPattern.MatchString(strings.TrimSpace(value))
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
