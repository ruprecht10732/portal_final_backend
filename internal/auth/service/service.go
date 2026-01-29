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
	"portal_final_backend/internal/config"
	"portal_final_backend/internal/email"
	"portal_final_backend/internal/logger"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrTokenExpired = errors.New("token expired")
var ErrTokenInvalid = errors.New("token invalid")
var ErrEmailNotVerified = errors.New("email not verified")
var ErrEmailTaken = errors.New("email already in use")
var ErrInvalidCurrentPassword = errors.New("current password is incorrect")

const (
	accessTokenType  = "access"
	refreshTokenType = "refresh"
	defaultUserRole  = "user" // Default role for new users (not admin)
)

type Service struct {
	repo *repository.Repository
	cfg  *config.Config
	mail email.Sender
	log  *logger.Logger
}

type Profile struct {
	ID            uuid.UUID
	Email         string
	EmailVerified bool
	Roles         []string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func New(repo *repository.Repository, cfg *config.Config, mailer email.Sender, log *logger.Logger) *Service {
	return &Service{repo: repo, cfg: cfg, mail: mailer, log: log}
}

func (s *Service) SignUp(ctx context.Context, email, plainPassword string) error {
	hash, err := password.Hash(plainPassword)
	if err != nil {
		s.log.Error("failed to hash password", "error", err)
		return err
	}

	user, err := s.repo.CreateUser(ctx, email, hash)
	if err != nil {
		s.log.Error("failed to create user", "email", email, "error", err)
		return err
	}

	// Assign default 'user' role - not admin
	if err := s.repo.SetUserRoles(ctx, user.ID, []string{defaultUserRole}); err != nil {
		s.log.Error("failed to set user roles", "user_id", user.ID, "error", err)
		return err
	}

	s.log.AuthEvent("signup", email, true, "")

	verifyToken, err := token.GenerateRandomToken(32)
	if err != nil {
		return err
	}

	verifyHash := token.HashSHA256(verifyToken)
	expiresAt := time.Now().Add(s.cfg.VerifyTokenTTL)
	if err := s.repo.CreateUserToken(ctx, user.ID, verifyHash, repository.TokenTypeEmailVerify, expiresAt); err != nil {
		return err
	}

	verifyURL := s.buildURL("/verify-email", verifyToken)
	if err := s.mail.SendVerificationEmail(ctx, user.Email, verifyURL); err != nil {
		return err
	}

	return nil
}

func (s *Service) SignIn(ctx context.Context, email, plainPassword string) (string, string, error) {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return "", "", ErrInvalidCredentials
	}

	if err := password.Compare(user.PasswordHash, plainPassword); err != nil {
		return "", "", ErrInvalidCredentials
	}

	if !user.EmailVerified {
		return "", "", ErrEmailNotVerified
	}

	return s.issueTokens(ctx, user.ID)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (string, string, error) {
	hash := token.HashSHA256(refreshToken)
	userID, expiresAt, err := s.repo.GetRefreshToken(ctx, hash)
	if err != nil {
		return "", "", ErrTokenInvalid
	}

	if time.Now().After(expiresAt) {
		_ = s.repo.RevokeRefreshToken(ctx, hash)
		return "", "", ErrTokenExpired
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
	expiresAt := time.Now().Add(s.cfg.ResetTokenTTL)
	if err := s.repo.CreateUserToken(ctx, user.ID, resetHash, repository.TokenTypePasswordReset, expiresAt); err != nil {
		return err
	}

	resetURL := s.buildURL("/reset-password", resetToken)
	if err := s.mail.SendPasswordResetEmail(ctx, user.Email, resetURL); err != nil {
		return err
	}

	return nil
}

func (s *Service) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	hash := token.HashSHA256(rawToken)
	userID, expiresAt, err := s.repo.GetUserToken(ctx, hash, repository.TokenTypePasswordReset)
	if err != nil {
		return ErrTokenInvalid
	}

	if time.Now().After(expiresAt) {
		return ErrTokenExpired
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
		return ErrTokenInvalid
	}

	if time.Now().After(expiresAt) {
		return ErrTokenExpired
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

	accessToken, err := s.signJWT(userID, roles, s.cfg.AccessTokenTTL, accessTokenType, s.cfg.JWTAccessSecret)
	if err != nil {
		return "", "", err
	}

	refreshToken, err := token.GenerateRandomToken(48)
	if err != nil {
		return "", "", err
	}

	hash := token.HashSHA256(refreshToken)
	expiresAt := time.Now().Add(s.cfg.RefreshTokenTTL)
	if err := s.repo.CreateRefreshToken(ctx, userID, hash, expiresAt); err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

func (s *Service) signJWT(userID uuid.UUID, roles []string, ttl time.Duration, tokenType, secret string) (string, error) {
	claims := jwt.MapClaims{
		"sub":   userID.String(),
		"type":  tokenType,
		"roles": roles,
		"exp":   time.Now().Add(ttl).Unix(),
		"iat":   time.Now().Unix(),
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
			ID:    user.ID.String(),
			Email: user.Email,
			Roles: user.Roles,
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

	return Profile{
		ID:            user.ID,
		Email:         user.Email,
		EmailVerified: user.EmailVerified,
		Roles:         roles,
		CreatedAt:     user.CreatedAt,
		UpdatedAt:     user.UpdatedAt,
	}, nil
}

func (s *Service) UpdateMe(ctx context.Context, userID uuid.UUID, email string) (Profile, error) {
	current, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return Profile{}, err
	}

	roles, err := s.repo.GetUserRoles(ctx, userID)
	if err != nil {
		return Profile{}, err
	}

	if strings.EqualFold(strings.TrimSpace(email), current.Email) {
		return Profile{
			ID:            current.ID,
			Email:         current.Email,
			EmailVerified: current.EmailVerified,
			Roles:         roles,
			CreatedAt:     current.CreatedAt,
			UpdatedAt:     current.UpdatedAt,
		}, nil
	}

	if existing, err := s.repo.GetUserByEmail(ctx, email); err == nil && existing.ID != userID {
		return Profile{}, ErrEmailTaken
	} else if err != nil && err != repository.ErrNotFound {
		return Profile{}, err
	}

	updated, err := s.repo.UpdateUserEmail(ctx, userID, email)
	if err != nil {
		return Profile{}, err
	}

	verifyToken, err := token.GenerateRandomToken(32)
	if err != nil {
		return Profile{}, err
	}

	verifyHash := token.HashSHA256(verifyToken)
	expiresAt := time.Now().Add(s.cfg.VerifyTokenTTL)
	if err := s.repo.CreateUserToken(ctx, userID, verifyHash, repository.TokenTypeEmailVerify, expiresAt); err != nil {
		return Profile{}, err
	}

	verifyURL := s.buildURL("/verify-email", verifyToken)
	if err := s.mail.SendVerificationEmail(ctx, updated.Email, verifyURL); err != nil {
		return Profile{}, err
	}

	return Profile{
		ID:            updated.ID,
		Email:         updated.Email,
		EmailVerified: updated.EmailVerified,
		Roles:         roles,
		CreatedAt:     updated.CreatedAt,
		UpdatedAt:     updated.UpdatedAt,
	}, nil
}

func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	if err := password.Compare(user.PasswordHash, currentPassword); err != nil {
		return ErrInvalidCurrentPassword
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

func (s *Service) buildURL(path string, tokenValue string) string {
	base := strings.TrimRight(s.cfg.AppBaseURL, "/")
	return base + path + "?token=" + tokenValue
}
