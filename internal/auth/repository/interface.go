package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// =====================================
// Segregated Interfaces (Interface Segregation Principle)
// =====================================

// UserReader provides read-only access to user data.
type UserReader interface {
	GetUserByEmail(ctx context.Context, email string) (User, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (User, error)
}

// UserWriter provides write operations for user management.
type UserWriter interface {
	CreateUser(ctx context.Context, email, passwordHash string) (User, error)
	MarkEmailVerified(ctx context.Context, userID uuid.UUID) error
	UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error
	UpdateUserEmail(ctx context.Context, userID uuid.UUID, email string) (User, error)
	ListUsers(ctx context.Context) ([]UserWithRoles, error)
}

// TokenStore manages one-time tokens (email verification, password reset).
type TokenStore interface {
	CreateUserToken(ctx context.Context, userID uuid.UUID, tokenHash string, tokenType string, expiresAt time.Time) error
	GetUserToken(ctx context.Context, tokenHash string, tokenType string) (uuid.UUID, time.Time, error)
	UseUserToken(ctx context.Context, tokenHash string, tokenType string) error
}

// RefreshTokenStore manages refresh tokens for session management.
type RefreshTokenStore interface {
	CreateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	GetRefreshToken(ctx context.Context, tokenHash string) (uuid.UUID, time.Time, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
	RevokeAllRefreshTokens(ctx context.Context, userID uuid.UUID) error
}

// RoleManager provides role-based access control operations.
type RoleManager interface {
	GetUserRoles(ctx context.Context, userID uuid.UUID) ([]string, error)
	SetUserRoles(ctx context.Context, userID uuid.UUID, RAC_roles []string) error
}

// =====================================
// Composite Interface (for backward compatibility)
// =====================================

// AuthRepository defines the complete interface for authentication data operations.
// Composed of smaller, focused interfaces for better testability and flexibility.
type AuthRepository interface {
	UserReader
	UserWriter
	TokenStore
	RefreshTokenStore
	RoleManager
}

// Ensure Repository implements AuthRepository
var _ AuthRepository = (*Repository)(nil)
