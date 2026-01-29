package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AuthRepository defines the interface for authentication data operations.
// This allows services to depend on an abstraction rather than concrete implementation,
// improving testability and modularity.
type AuthRepository interface {
	// User operations
	CreateUser(ctx context.Context, email, passwordHash string) (User, error)
	GetUserByEmail(ctx context.Context, email string) (User, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (User, error)
	MarkEmailVerified(ctx context.Context, userID uuid.UUID) error
	UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error
	UpdateUserEmail(ctx context.Context, userID uuid.UUID, email string) (User, error)
	ListUsers(ctx context.Context) ([]UserWithRoles, error)

	// Token operations
	CreateUserToken(ctx context.Context, userID uuid.UUID, tokenHash string, tokenType string, expiresAt time.Time) error
	GetUserToken(ctx context.Context, tokenHash string, tokenType string) (uuid.UUID, time.Time, error)
	UseUserToken(ctx context.Context, tokenHash string, tokenType string) error

	// Refresh token operations
	CreateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	GetRefreshToken(ctx context.Context, tokenHash string) (uuid.UUID, time.Time, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
	RevokeAllRefreshTokens(ctx context.Context, userID uuid.UUID) error

	// Role operations
	GetUserRoles(ctx context.Context, userID uuid.UUID) ([]string, error)
	SetUserRoles(ctx context.Context, userID uuid.UUID, roles []string) error
}

// Ensure Repository implements AuthRepository
var _ AuthRepository = (*Repository)(nil)
