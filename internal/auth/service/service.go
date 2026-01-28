package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"portal_final_backend/internal/auth/password"
	"portal_final_backend/internal/auth/repository"
	"portal_final_backend/internal/auth/token"
	"portal_final_backend/internal/config"
	"portal_final_backend/internal/email"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrTokenExpired = errors.New("token expired")
var ErrTokenInvalid = errors.New("token invalid")
var ErrEmailNotVerified = errors.New("email not verified")

const (
	accessTokenType  = "access"
	refreshTokenType = "refresh"
)

type Service struct {
	repo *repository.Repository
	cfg  *config.Config
	mail email.Sender
}

func New(repo *repository.Repository, cfg *config.Config, mailer email.Sender) *Service {
	return &Service{repo: repo, cfg: cfg, mail: mailer}
}

func (s *Service) SignUp(ctx context.Context, email, plainPassword string) error {
	hash, err := password.Hash(plainPassword)
	if err != nil {
		return err
	}

	user, err := s.repo.CreateUser(ctx, email, hash)
	if err != nil {
		return err
	}

	if err := s.repo.SetUserRoles(ctx, user.ID, []string{"admin"}); err != nil {
		return err
	}

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

func (s *Service) buildURL(path string, tokenValue string) string {
	base := strings.TrimRight(s.cfg.AppBaseURL, "/")
	return base + path + "?token=" + tokenValue
}
