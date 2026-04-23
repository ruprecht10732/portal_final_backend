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
	"portal_final_backend/platform/phone"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

const (
	accessTokenType           = "access"
	refreshTokenType          = "refresh"
	defaultUserRole           = "user" // Default role for new RAC_users (not admin)
	defaultAdminRole          = "admin"
	superAdminRole            = "superadmin"
	tokenInvalidMessage       = "token invalid"
	tokenExpiredMessage       = "token expired"
	invalidCredentialsMessage = "invalid credentials"
)

type Service struct {
	repo     *repository.Repository
	identity *identityservice.Service
	cfg      config.AuthServiceConfig
	eventBus events.Bus
	log      *logger.Logger
	redis    *redis.Client
	webauthn *webauthn.WebAuthn
}

type Profile struct {
	ID                  uuid.UUID
	Email               string
	EmailVerified       bool
	FirstName           *string
	LastName            *string
	Phone               *string
	PreferredLang       string
	Roles               []string
	HasOrganization     bool
	OnboardingCompleted bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func New(repo *repository.Repository, identity *identityservice.Service, cfg config.AuthServiceConfig, eventBus events.Bus, log *logger.Logger) *Service {
	return &Service{repo: repo, identity: identity, cfg: cfg, eventBus: eventBus, log: log}
}

func (s *Service) SetAccessTokenBlocklistRedis(client *redis.Client) {
	s.redis = client
}

// =============================================================================
// Authentication & Registration
// =============================================================================

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
	// Safe defer: Rollback is a no-op if tx.Commit() succeeds. Avoids boolean flags.
	defer func() { _ = tx.Rollback(ctx) }()

	user, err := s.repo.CreateUserTx(ctx, tx, email, hash)
	if err != nil {
		s.log.Error("failed to create user", "email", email, "error", err)
		return err
	}

	roles := rolesForSignup(usingInvite, email, s.cfg.GetBootstrapSuperAdminEmail())
	if err := s.repo.SetUserRolesTx(ctx, tx, user.ID, roles); err != nil {
		if errors.Is(err, repository.ErrSuperAdminAlreadyAssigned) {
			return apperr.Conflict("only one superadmin user is allowed")
		}
		s.log.Error("failed to set user RAC_roles", "user_id", user.ID, "error", err)
		return err
	}

	if usingInvite {
		if err := s.applyInvite(ctx, tx, trimmedInvite, email, user.ID); err != nil {
			return err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return err
	}

	s.log.AuthEvent("signup", email, true, "")

	return s.enqueueEmailVerification(ctx, user.ID, user.Email)
}

func (s *Service) SignIn(ctx context.Context, email, plainPassword string) (string, string, error) {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return "", "", apperr.Unauthorized(invalidCredentialsMessage)
	}

	if err := password.Compare(user.PasswordHash, plainPassword); err != nil {
		return "", "", apperr.Unauthorized(invalidCredentialsMessage)
	}

	if !user.EmailVerified {
		return "", "", apperr.Forbidden("email not verified")
	}

	if err := s.ensureBootstrapSuperAdmin(ctx, user.ID, user.Email); err != nil {
		return "", "", err
	}

	return s.issueTokens(ctx, user.ID, user.Email)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (string, string, error) {
	hash := token.HashSHA256(refreshToken)
	userID, expiresAt, err := s.repo.GetRefreshToken(ctx, hash)
	if err != nil {
		return "", "", apperr.Unauthorized(tokenInvalidMessage)
	}

	// Always revoke the used token (consumed or expired) to prevent reuse.
	_ = s.repo.RevokeRefreshToken(ctx, hash)

	if time.Now().After(expiresAt) {
		return "", "", apperr.Unauthorized(tokenExpiredMessage)
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", "", apperr.Unauthorized(invalidCredentialsMessage)
		}
		return "", "", err
	}

	return s.issueTokens(ctx, userID, user.Email)
}

func (s *Service) SignOut(ctx context.Context, refreshToken string, accessToken string) error {
	hash := token.HashSHA256(refreshToken)
	if err := s.repo.RevokeRefreshToken(ctx, hash); err != nil {
		return err
	}

	if err := s.blocklistAccessToken(ctx, accessToken); err != nil {
		s.log.Warn("failed to blocklist access token on sign out", "error", err)
	}

	return nil
}

// =============================================================================
// Password & Email Management
// =============================================================================

func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil // $O(1)$ Time mitigation: silently succeed to prevent user enumeration attacks
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

// =============================================================================
// Profile & User Queries
// =============================================================================

func (s *Service) GetMe(ctx context.Context, userID uuid.UUID) (Profile, error) {
	user, roles, preferredLang, err := s.loadProfileContext(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return Profile{}, apperr.Unauthorized(invalidCredentialsMessage)
		}
		return Profile{}, err
	}

	_, orgErr := s.identity.GetUserOrganizationID(ctx, userID)

	return s.buildProfile(user, roles, preferredLang, orgErr == nil), nil
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

	updatedUser, err = s.applyPhoneUpdate(ctx, userID, current.Phone, updatedUser, req)
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

	_, orgErr := s.identity.GetUserOrganizationID(ctx, userID)
	return s.buildProfile(updatedUser, roles, preferredLang, orgErr == nil), nil
}

func (s *Service) ListUsers(ctx context.Context) ([]transport.UserSummary, error) {
	users, err := s.repo.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	return mapUsersToSummary(users), nil
}

func (s *Service) ListUsersForRequester(ctx context.Context, requesterID uuid.UUID) ([]transport.UserSummary, error) {
	organizationID, err := s.identity.GetUserOrganizationID(ctx, requesterID)
	if err != nil {
		if errors.Is(err, identityrepo.ErrNotFound) {
			user, getErr := s.repo.GetUserByID(ctx, requesterID)
			if getErr != nil {
				return nil, getErr
			}
			roles, rolesErr := s.repo.GetUserRoles(ctx, requesterID)
			if rolesErr != nil {
				return nil, rolesErr
			}

			return []transport.UserSummary{{
				ID:        user.ID.String(),
				Email:     user.Email,
				FirstName: user.FirstName,
				LastName:  user.LastName,
				Roles:     roles,
			}}, nil
		}
		return nil, err
	}

	users, err := s.repo.ListUsersByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	return mapUsersToSummary(users), nil
}

// =============================================================================
// Onboarding & Invites
// =============================================================================

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

func (s *Service) CompleteOnboarding(ctx context.Context, userID uuid.UUID, req transport.CompleteOnboardingRequest) error {
	if _, err := s.repo.UpdateUserNames(ctx, userID, &req.FirstName, &req.LastName); err != nil {
		return err
	}

	if _, err := s.identity.GetUserOrganizationID(ctx, userID); err == nil {
		return nil // Organization already exists
	}

	return s.createOrganizationFromOnboarding(ctx, userID, req)
}

func (s *Service) MarkOnboardingComplete(ctx context.Context, userID uuid.UUID) error {
	return s.repo.MarkOnboardingComplete(ctx, userID)
}

// =============================================================================
// RBAC
// =============================================================================

func (s *Service) SetUserRoles(ctx context.Context, actorID uuid.UUID, actorRoles []string, userID uuid.UUID, roles []string) error {
	if containsString(roles, superAdminRole) && !containsString(actorRoles, superAdminRole) {
		return apperr.Forbidden("only superadmin can assign the superadmin role")
	}

	if err := s.repo.SetUserRoles(ctx, userID, roles); err != nil {
		if errors.Is(err, repository.ErrSuperAdminAlreadyAssigned) {
			return apperr.Conflict("only one superadmin user is allowed")
		}
		return err
	}
	return nil
}

func (s *Service) ensureBootstrapSuperAdmin(ctx context.Context, userID uuid.UUID, email string) error {
	bootstrapEmail := strings.TrimSpace(s.cfg.GetBootstrapSuperAdminEmail())
	// $O(1)$ fast-path exit for 99.9% of user logins
	if bootstrapEmail == "" || !strings.EqualFold(strings.TrimSpace(email), bootstrapEmail) {
		return nil
	}

	currentRoles, err := s.repo.GetUserRoles(ctx, userID)
	if err != nil {
		return err
	}
	if containsString(currentRoles, superAdminRole) {
		return nil
	}

	hasSuperAdmin, err := s.repo.HasAnyUserWithRole(ctx, superAdminRole)
	if err != nil {
		return err
	}
	if hasSuperAdmin {
		return nil
	}

	updatedRoles := uniqueRoleList(append(currentRoles, defaultAdminRole, superAdminRole))
	if err := s.repo.SetUserRoles(ctx, userID, updatedRoles); err != nil {
		if errors.Is(err, repository.ErrSuperAdminAlreadyAssigned) {
			return nil
		}
		return err
	}

	return nil
}

// =============================================================================
// Internal Helpers
// =============================================================================

func (s *Service) blocklistAccessToken(ctx context.Context, accessToken string) error {
	// Defensive programming: prevent parsing DoS attacks via massively bloated headers
	if s.redis == nil || len(accessToken) == 0 || len(accessToken) > 8192 {
		return nil
	}

	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser(jwt.WithoutClaimsValidation()).ParseUnverified(accessToken, claims); err != nil {
		return err
	}

	jti, _ := claims["jti"].(string)
	if jti = strings.TrimSpace(jti); jti == "" {
		return nil
	}

	expFloat, ok := claims["exp"].(float64)
	if !ok {
		return nil
	}

	ttl := time.Until(time.Unix(int64(expFloat), 0))
	if ttl <= 0 {
		return nil
	}

	return s.redis.Set(ctx, "auth:blocklist:jti:"+jti, "1", ttl).Err()
}

func (s *Service) issueTokens(ctx context.Context, userID uuid.UUID, email string) (string, string, error) {
	roles, err := s.repo.GetUserRoles(ctx, userID)
	if err != nil {
		return "", "", err
	}

	var tenantID *uuid.UUID
	if orgID, err := s.identity.GetUserOrganizationID(ctx, userID); err == nil {
		tenantID = &orgID
	} else if !errors.Is(err, identityrepo.ErrNotFound) {
		return "", "", err
	}

	accessToken, err := s.signJWT(userID, email, tenantID, roles, s.cfg.GetAccessTokenTTL(), accessTokenType, s.cfg.GetJWTAccessSecret())
	if err != nil {
		return "", "", err
	}

	refreshToken, err := token.GenerateRandomToken(48)
	if err != nil {
		return "", "", err
	}

	hash := token.HashSHA256(refreshToken)
	if err := s.repo.CreateRefreshToken(ctx, userID, hash, time.Now().Add(s.cfg.GetRefreshTokenTTL())); err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

func (s *Service) signJWT(userID uuid.UUID, email string, tenantID *uuid.UUID, roles []string, ttl time.Duration, tokenType, secret string) (string, error) {
	claims := jwt.MapClaims{
		"sub":   userID.String(),
		"email": email,
		"type":  tokenType,
		"roles": roles,
		"jti":   uuid.NewString(),
		"exp":   time.Now().Add(ttl).Unix(),
		"iat":   time.Now().Unix(),
	}
	if tenantID != nil {
		claims["tenant_id"] = tenantID.String()
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func normalizeInviteToken(inviteToken *string) (string, bool) {
	if inviteToken == nil {
		return "", false
	}
	trimmed := strings.TrimSpace(*inviteToken)
	return trimmed, trimmed != ""
}

func rolesForSignup(usingInvite bool, email, bootstrapSuperAdminEmail string) []string {
	if usingInvite {
		return []string{defaultUserRole}
	}
	if strings.TrimSpace(bootstrapSuperAdminEmail) != "" && strings.EqualFold(strings.TrimSpace(email), strings.TrimSpace(bootstrapSuperAdminEmail)) {
		return []string{defaultAdminRole, superAdminRole}
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
	return s.identity.UseInvite(ctx, tx, invite.ID, userID)
}

func (s *Service) enqueueEmailVerification(ctx context.Context, userID uuid.UUID, email string) error {
	verifyToken, err := token.GenerateRandomToken(32)
	if err != nil {
		return err
	}

	verifyHash := token.HashSHA256(verifyToken)
	if err := s.repo.CreateUserToken(ctx, userID, verifyHash, repository.TokenTypeEmailVerify, time.Now().Add(s.cfg.GetVerifyTokenTTL())); err != nil {
		return err
	}

	s.eventBus.Publish(ctx, events.EmailVerificationRequested{
		BaseEvent:   events.NewBaseEvent(),
		UserID:      userID,
		Email:       email,
		VerifyToken: verifyToken,
	})

	return nil
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
	return user, roles, preferredLang, err
}

func (s *Service) buildProfile(user repository.User, roles []string, preferredLang string, hasOrg bool) Profile {
	return Profile{
		ID:                  user.ID,
		Email:               user.Email,
		EmailVerified:       user.EmailVerified,
		FirstName:           user.FirstName,
		LastName:            user.LastName,
		Phone:               user.Phone,
		PreferredLang:       preferredLang,
		Roles:               roles,
		HasOrganization:     hasOrg,
		OnboardingCompleted: user.OnboardingCompletedAt != nil,
		CreatedAt:           user.CreatedAt,
		UpdatedAt:           user.UpdatedAt,
	}
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
	lang := strings.TrimSpace(*req.PreferredLanguage)
	if err := s.repo.UpdateUserSettings(ctx, userID, lang); err != nil {
		return currentLang, err
	}
	return lang, nil
}

func (s *Service) applyPhoneUpdate(ctx context.Context, userID uuid.UUID, currentPhone *string, updatedUser repository.User, req transport.UpdateProfileRequest) (repository.User, error) {
	if req.Phone == nil {
		return updatedUser, nil
	}

	normalizedPhone := strings.TrimSpace(phone.NormalizeE164(*req.Phone))
	current := ""
	if currentPhone != nil {
		current = strings.TrimSpace(*currentPhone)
	}

	if normalizedPhone == current {
		return updatedUser, nil
	}

	var phoneValue *string
	if normalizedPhone != "" {
		phoneValue = &normalizedPhone
	}

	return s.repo.UpdateUserPhone(ctx, userID, phoneValue)
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

func (s *Service) createOrganizationFromOnboarding(ctx context.Context, userID uuid.UUID, req transport.CompleteOnboardingRequest) error {
	orgName, err := requireOrganizationField(req.OrganizationName, "organization name")
	if err != nil {
		return err
	}
	if _, err := requireOrganizationField(req.OrganizationEmail, "organization email"); err != nil {
		return err
	}
	if _, err := requireOrganizationField(req.OrganizationPhone, "organization phone"); err != nil {
		return err
	}

	orgID, err := s.identity.CreateOrganizationForUser(ctx, nil, orgName, userID)
	if err != nil {
		return err
	}

	if err := s.identity.AddMember(ctx, nil, orgID, userID); err != nil {
		return err
	}

	if !hasOrganizationProfileUpdate(req) {
		return nil
	}

	_, err = s.identity.UpdateOrganizationProfile(ctx, orgID, identityservice.OrganizationProfileUpdate{
		Email:        req.OrganizationEmail,
		Phone:        req.OrganizationPhone,
		VATNumber:    req.VatNumber,
		KVKNumber:    req.KvkNumber,
		AddressLine1: req.AddressLine1,
		AddressLine2: req.AddressLine2,
		PostalCode:   req.PostalCode,
		City:         req.City,
		Country:      req.Country,
	})
	return err
}

func mapUsersToSummary(users []repository.UserWithRoles) []transport.UserSummary {
	result := make([]transport.UserSummary, len(users))
	for i, user := range users {
		result[i] = transport.UserSummary{
			ID:        user.ID.String(),
			Email:     user.Email,
			FirstName: user.FirstName,
			LastName:  user.LastName,
			Roles:     user.Roles,
		}
	}
	return result
}

func requireOrganizationField(value *string, fieldName string) (string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "", apperr.Validation(fieldName + " is required")
	}
	return strings.TrimSpace(*value), nil
}

func hasOrganizationProfileUpdate(req transport.CompleteOnboardingRequest) bool {
	return hasOptionalValue(req.OrganizationEmail) ||
		hasOptionalValue(req.OrganizationPhone) ||
		hasOptionalValue(req.VatNumber) ||
		hasOptionalValue(req.KvkNumber) ||
		hasOptionalValue(req.AddressLine1) ||
		hasOptionalValue(req.AddressLine2) ||
		hasOptionalValue(req.PostalCode) ||
		hasOptionalValue(req.City) ||
		hasOptionalValue(req.Country)
}

func hasOptionalValue(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
}

func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func uniqueRoleList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, exists := seen[v]; !exists {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}
