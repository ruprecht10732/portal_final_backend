package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"portal_final_backend/internal/auth/password"
	"portal_final_backend/internal/auth/repository"
	"portal_final_backend/internal/auth/token"
	"portal_final_backend/internal/auth/transport"
	"portal_final_backend/internal/events"
	identityrepo "portal_final_backend/internal/identity/repository"
	identityservice "portal_final_backend/internal/identity/service"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	accessTokenType     = "access"
	refreshTokenType    = "refresh"
	defaultUserRole     = "user" // Default role for new RAC_users (not admin)
	defaultAdminRole    = "admin"
	tokenInvalidMessage = "token invalid"
	tokenExpiredMessage = "token expired"
)

type Service struct {
	repo     *repository.Repository
	identity *identityservice.Service
	cfg      config.AuthServiceConfig
	eventBus events.Bus
	log      *logger.Logger
}

type Profile struct {
	ID              uuid.UUID
	Email           string
	EmailVerified   bool
	FirstName       *string
	LastName        *string
	PreferredLang   string
	Roles           []string
	HasOrganization bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func New(repo *repository.Repository, identity *identityservice.Service, cfg config.AuthServiceConfig, eventBus events.Bus, log *logger.Logger) *Service {
	return &Service{repo: repo, identity: identity, cfg: cfg, eventBus: eventBus, log: log}
}

func (s *Service) SignUp(ctx context.Context, email, plainPassword string, organizationName *string, inviteToken *string) error {
	hash, err := password.Hash(plainPassword)
	if err != nil {
		s.log.Error("failed to hash password", "error", err)
		return err
	}

	trimmedInvite, usingInvite := normalizeInviteToken(inviteToken)

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	user, err := s.repo.CreateUserTx(ctx, tx, email, hash)
	if err != nil {
		s.log.Error("failed to create user", "email", email, "error", err)
		return err
	}

	roles := rolesForSignup(usingInvite)
	if err := s.repo.SetUserRolesTx(ctx, tx, user.ID, roles); err != nil {
		s.log.Error("failed to set user RAC_roles", "user_id", user.ID, "error", err)
		return err
	}

	if usingInvite {
		if err := s.applyInvite(ctx, tx, trimmedInvite, email, user.ID); err != nil {
			return err
		}
	}
	// Note: Organization is NOT created here for non-invite RAC_users.
	// It will be created during onboarding when the user provides an organization name.

	if err = tx.Commit(ctx); err != nil {
		return err
	}
	committed = true

	s.log.AuthEvent("signup", email, true, "")

	verifyToken, err := token.GenerateRandomToken(32)
	if err != nil {
		return err
	}

	verifyHash := token.HashSHA256(verifyToken)
	expiresAt := time.Now().Add(s.cfg.GetVerifyTokenTTL())
	if err := s.repo.CreateUserToken(ctx, user.ID, verifyHash, repository.TokenTypeEmailVerify, expiresAt); err != nil {
		return err
	}

	// Publish event - notification module handles email sending
	s.eventBus.Publish(ctx, events.UserSignedUp{
		BaseEvent:   events.NewBaseEvent(),
		UserID:      user.ID,
		Email:       user.Email,
		VerifyToken: verifyToken,
	})

	return nil
}

func normalizeInviteToken(inviteToken *string) (string, bool) {
	if inviteToken == nil {
		return "", false
	}
	trimmed := strings.TrimSpace(*inviteToken)
	return trimmed, trimmed != ""
}

func rolesForSignup(usingInvite bool) []string {
	if usingInvite {
		return []string{defaultUserRole}
	}
	return []string{defaultAdminRole}
}

func (s *Service) applyInvite(ctx context.Context, tx pgx.Tx, tokenValue, email string, userID uuid.UUID) error {
	invite, err := s.identity.ResolveInvite(ctx, tokenValue)
	if err != nil {
		return err
	}
	if !strings.EqualFold(invite.Email, email) {
		return apperr.Forbidden("invite does not match email")
	}
	if err := s.identity.AddMember(ctx, tx, invite.OrganizationID, userID); err != nil {
		return err
	}
	if err := s.identity.UseInvite(ctx, tx, invite.ID, userID); err != nil {
		return err
	}
	return nil
}

func (s *Service) SignIn(ctx context.Context, email, plainPassword string) (string, string, error) {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return "", "", apperr.Unauthorized("invalid credentials")
	}

	if err := password.Compare(user.PasswordHash, plainPassword); err != nil {
		return "", "", apperr.Unauthorized("invalid credentials")
	}

	if !user.EmailVerified {
		return "", "", apperr.Forbidden("email not verified")
	}

	return s.issueTokens(ctx, user.ID)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (string, string, error) {
	hash := token.HashSHA256(refreshToken)
	userID, expiresAt, err := s.repo.GetRefreshToken(ctx, hash)
	if err != nil {
		return "", "", apperr.Unauthorized(tokenInvalidMessage)
	}

	if time.Now().After(expiresAt) {
		_ = s.repo.RevokeRefreshToken(ctx, hash)
		return "", "", apperr.Unauthorized(tokenExpiredMessage)
	}

	_ = s.repo.RevokeRefreshToken(ctx, hash)
	return s.issueTokens(ctx, userID)
}

func (s *Service) SignOut(ctx context.Context, refreshToken string) error {
	hash := token.HashSHA256(refreshToken)
	return s.repo.RevokeRefreshToken(ctx, hash)
}

func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil
	}

	resetToken, err := token.GenerateRandomToken(32)
	if err != nil {
		return err
	}

	resetHash := token.HashSHA256(resetToken)
	expiresAt := time.Now().Add(s.cfg.GetResetTokenTTL())
	if err := s.repo.CreateUserToken(ctx, user.ID, resetHash, repository.TokenTypePasswordReset, expiresAt); err != nil {
		return err
	}

	// Publish event - notification module handles email sending
	s.eventBus.Publish(ctx, events.PasswordResetRequested{
		BaseEvent:  events.NewBaseEvent(),
		UserID:     user.ID,
		Email:      user.Email,
		ResetToken: resetToken,
	})

	return nil
}

func (s *Service) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	hash := token.HashSHA256(rawToken)
	userID, expiresAt, err := s.repo.GetUserToken(ctx, hash, repository.TokenTypePasswordReset)
	if err != nil {
		return apperr.Unauthorized(tokenInvalidMessage)
	}

	if time.Now().After(expiresAt) {
		return apperr.Unauthorized(tokenExpiredMessage)
	}

	passwordHash, err := password.Hash(newPassword)
	if err != nil {
		return err
	}

	if err := s.repo.UpdatePassword(ctx, userID, passwordHash); err != nil {
		return err
	}

	_ = s.repo.UseUserToken(ctx, hash, repository.TokenTypePasswordReset)
	_ = s.repo.RevokeAllRefreshTokens(ctx, userID)

	return nil
}

func (s *Service) VerifyEmail(ctx context.Context, rawToken string) error {
	hash := token.HashSHA256(rawToken)
	userID, expiresAt, err := s.repo.GetUserToken(ctx, hash, repository.TokenTypeEmailVerify)
	if err != nil {
		return apperr.Unauthorized(tokenInvalidMessage)
	}

	if time.Now().After(expiresAt) {
		return apperr.Unauthorized(tokenExpiredMessage)
	}

	if err := s.repo.MarkEmailVerified(ctx, userID); err != nil {
		return err
	}

	_ = s.repo.UseUserToken(ctx, hash, repository.TokenTypeEmailVerify)
	return nil
}

func (s *Service) issueTokens(ctx context.Context, userID uuid.UUID) (string, string, error) {
	roles, err := s.repo.GetUserRoles(ctx, userID)
	if err != nil {
		return "", "", err
	}

	orgID, err := s.identity.GetUserOrganizationID(ctx, userID)
	if err != nil {
		if errors.Is(err, identityrepo.ErrNotFound) {
			return "", "", apperr.Forbidden("organization not found")
		}
		return "", "", err
	}

	accessToken, err := s.signJWT(userID, &orgID, roles, s.cfg.GetAccessTokenTTL(), accessTokenType, s.cfg.GetJWTAccessSecret())
	if err != nil {
		return "", "", err
	}

	refreshToken, err := token.GenerateRandomToken(48)
	if err != nil {
		return "", "", err
	}

	hash := token.HashSHA256(refreshToken)
	expiresAt := time.Now().Add(s.cfg.GetRefreshTokenTTL())
	if err := s.repo.CreateRefreshToken(ctx, userID, hash, expiresAt); err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

func (s *Service) signJWT(userID uuid.UUID, tenantID *uuid.UUID, roles []string, ttl time.Duration, tokenType, secret string) (string, error) {
	claims := jwt.MapClaims{
		"sub":   userID.String(),
		"type":  tokenType,
		"roles": roles,
		"exp":   time.Now().Add(ttl).Unix(),
		"iat":   time.Now().Unix(),
	}
	if tenantID != nil {
		claims["tenant_id"] = tenantID.String()
	}

	tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tokenObj.SignedString([]byte(secret))
}

func (s *Service) SetUserRoles(ctx context.Context, userID uuid.UUID, roles []string) error {
	return s.repo.SetUserRoles(ctx, userID, roles)
}

func (s *Service) ListUsers(ctx context.Context) ([]transport.UserSummary, error) {
	users, err := s.repo.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]transport.UserSummary, 0, len(users))
	for _, user := range users {
		result = append(result, transport.UserSummary{
			ID:        user.ID.String(),
			Email:     user.Email,
			FirstName: user.FirstName,
			LastName:  user.LastName,
			Roles:     user.Roles,
		})
	}

	return result, nil
}

func (s *Service) GetMe(ctx context.Context, userID uuid.UUID) (Profile, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return Profile{}, err
	}

	roles, err := s.repo.GetUserRoles(ctx, userID)
	if err != nil {
		return Profile{}, err
	}

	if err := s.repo.EnsureUserSettings(ctx, userID); err != nil {
		return Profile{}, err
	}

	preferredLang, err := s.repo.GetUserSettings(ctx, userID)
	if err != nil {
		return Profile{}, err
	}

	// Check if user belongs to an organization
	_, orgErr := s.identity.GetUserOrganizationID(ctx, userID)
	hasOrganization := orgErr == nil

	return Profile{
		ID:              user.ID,
		Email:           user.Email,
		EmailVerified:   user.EmailVerified,
		FirstName:       user.FirstName,
		LastName:        user.LastName,
		PreferredLang:   preferredLang,
		Roles:           roles,
		HasOrganization: hasOrganization,
		CreatedAt:       user.CreatedAt,
		UpdatedAt:       user.UpdatedAt,
	}, nil
}

func (s *Service) UpdateMe(ctx context.Context, userID uuid.UUID, req transport.UpdateProfileRequest) (Profile, error) {
	current, roles, preferredLang, err := s.loadProfileContext(ctx, userID)
	if err != nil {
		return Profile{}, err
	}

	updatedUser, err := s.applyNameUpdates(ctx, userID, current, req)
	if err != nil {
		return Profile{}, err
	}

	preferredLang, err = s.applyPreferredLanguage(ctx, userID, preferredLang, req)
	if err != nil {
		return Profile{}, err
	}

	updatedUser, err = s.applyEmailUpdate(ctx, userID, current.Email, updatedUser, req)
	if err != nil {
		return Profile{}, err
	}

	return s.buildProfile(updatedUser, roles, preferredLang), nil
}

func (s *Service) loadProfileContext(ctx context.Context, userID uuid.UUID) (repository.User, []string, string, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return repository.User{}, nil, "", err
	}

	roles, err := s.repo.GetUserRoles(ctx, userID)
	if err != nil {
		return repository.User{}, nil, "", err
	}

	if err := s.repo.EnsureUserSettings(ctx, userID); err != nil {
		return repository.User{}, nil, "", err
	}

	preferredLang, err := s.repo.GetUserSettings(ctx, userID)
	if err != nil {
		return repository.User{}, nil, "", err
	}

	return user, roles, preferredLang, nil
}

func (s *Service) applyNameUpdates(ctx context.Context, userID uuid.UUID, current repository.User, req transport.UpdateProfileRequest) (repository.User, error) {
	if req.FirstName == nil && req.LastName == nil {
		return current, nil
	}

	return s.repo.UpdateUserNames(ctx, userID, req.FirstName, req.LastName)
}

func (s *Service) applyPreferredLanguage(ctx context.Context, userID uuid.UUID, currentLang string, req transport.UpdateProfileRequest) (string, error) {
	if req.PreferredLanguage == nil {
		return currentLang, nil
	}

	preferredLang := strings.TrimSpace(*req.PreferredLanguage)
	if err := s.repo.UpdateUserSettings(ctx, userID, preferredLang); err != nil {
		return currentLang, err
	}

	return preferredLang, nil
}

func (s *Service) applyEmailUpdate(ctx context.Context, userID uuid.UUID, currentEmail string, updatedUser repository.User, req transport.UpdateProfileRequest) (repository.User, error) {
	if req.Email == nil || strings.EqualFold(strings.TrimSpace(*req.Email), currentEmail) {
		return updatedUser, nil
	}

	newEmail := strings.TrimSpace(*req.Email)
	if existing, err := s.repo.GetUserByEmail(ctx, newEmail); err == nil && existing.ID != userID {
		return repository.User{}, apperr.Conflict("email already in use")
	} else if err != nil && err != repository.ErrNotFound {
		return repository.User{}, err
	}

	updated, err := s.repo.UpdateUserEmail(ctx, userID, newEmail)
	if err != nil {
		return repository.User{}, err
	}

	if err := s.enqueueEmailVerification(ctx, userID, updated.Email); err != nil {
		return repository.User{}, err
	}

	return updated, nil
}

func (s *Service) enqueueEmailVerification(ctx context.Context, userID uuid.UUID, email string) error {
	verifyToken, err := token.GenerateRandomToken(32)
	if err != nil {
		return err
	}

	verifyHash := token.HashSHA256(verifyToken)
	expiresAt := time.Now().Add(s.cfg.GetVerifyTokenTTL())
	if err := s.repo.CreateUserToken(ctx, userID, verifyHash, repository.TokenTypeEmailVerify, expiresAt); err != nil {
		return err
	}

	// Publish event - notification module handles email sending
	s.eventBus.Publish(ctx, events.EmailVerificationRequested{
		BaseEvent:   events.NewBaseEvent(),
		UserID:      userID,
		Email:       email,
		VerifyToken: verifyToken,
	})

	return nil
}

func (s *Service) buildProfile(user repository.User, roles []string, preferredLang string) Profile {
	return Profile{
		ID:            user.ID,
		Email:         user.Email,
		EmailVerified: user.EmailVerified,
		FirstName:     user.FirstName,
		LastName:      user.LastName,
		PreferredLang: preferredLang,
		Roles:         roles,
		CreatedAt:     user.CreatedAt,
		UpdatedAt:     user.UpdatedAt,
	}
}

func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	if err := password.Compare(user.PasswordHash, currentPassword); err != nil {
		return apperr.Validation("current password is incorrect")
	}

	passwordHash, err := password.Hash(newPassword)
	if err != nil {
		return err
	}

	if err := s.repo.UpdatePassword(ctx, userID, passwordHash); err != nil {
		return err
	}

	_ = s.repo.RevokeAllRefreshTokens(ctx, userID)
	return nil
}

func (s *Service) ResolveInvite(ctx context.Context, rawToken string) (transport.ResolveInviteResponse, error) {
	invite, err := s.identity.ResolveInvite(ctx, rawToken)
	if err != nil {
		return transport.ResolveInviteResponse{}, err
	}

	org, err := s.identity.GetOrganization(ctx, invite.OrganizationID)
	if err != nil {
		return transport.ResolveInviteResponse{}, err
	}

	return transport.ResolveInviteResponse{
		Email:            invite.Email,
		OrganizationName: org.Name,
	}, nil
}

func (s *Service) CompleteOnboarding(ctx context.Context, userID uuid.UUID, firstName, lastName string, organizationName *string) error {
	// Update user profile
	_, err := s.repo.UpdateUserNames(ctx, userID, &firstName, &lastName)
	if err != nil {
		return err
	}

	// Check if user already has an organization
	_, orgErr := s.identity.GetUserOrganizationID(ctx, userID)
	hasOrganization := orgErr == nil

	// If user doesn't have an organization, create one
	if !hasOrganization {
		if organizationName == nil || strings.TrimSpace(*organizationName) == "" {
			return apperr.Validation("organization name is required")
		}

		orgID, err := s.identity.CreateOrganizationForUser(ctx, nil, strings.TrimSpace(*organizationName), userID)
		if err != nil {
			return err
		}

		if err := s.identity.AddMember(ctx, nil, orgID, userID); err != nil {
			return err
		}
	}

	return nil
}
