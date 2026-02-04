// Package adapter provides implementations of external interfaces that other domains need.
// This follows the Anti-Corruption Layer pattern - auth domain provides adapters
// that satisfy consumer-driven interfaces defined by other domains.
package adapter

import (
	"context"

	"portal_final_backend/internal/auth/repository"
	"portal_final_backend/internal/leads/ports"

	"github.com/google/uuid"
)

// UserProviderAdapter implements leads/ports.UserProvider using the auth repository.
// This allows the leads domain to get user information without depending on auth internals.
type UserProviderAdapter struct {
	repo repository.UserReader
}

// NewUserProviderAdapter creates a new adapter for providing user info to other domains.
func NewUserProviderAdapter(repo repository.UserReader) *UserProviderAdapter {
	return &UserProviderAdapter{repo: repo}
}

// GetUserByID implements ports.UserProvider.
func (a *UserProviderAdapter) GetUserByID(ctx context.Context, userID uuid.UUID) (ports.UserInfo, error) {
	user, err := a.repo.GetUserByID(ctx, userID)
	if err != nil {
		return ports.UserInfo{}, err
	}

	return ports.UserInfo{
		ID:    user.ID,
		Email: user.Email,
		// Roles would need to be fetched separately if needed
	}, nil
}

// Ensure UserProviderAdapter implements ports.UserProvider
var _ ports.UserProvider = (*UserProviderAdapter)(nil)

// UserExistenceAdapter implements RAC_leads/ports.UserExistenceChecker.
type UserExistenceAdapter struct {
	repo repository.UserReader
}

// NewUserExistenceAdapter creates a new adapter for checking user existence.
func NewUserExistenceAdapter(repo repository.UserReader) *UserExistenceAdapter {
	return &UserExistenceAdapter{repo: repo}
}

// UserExists implements ports.UserExistenceChecker.
func (a *UserExistenceAdapter) UserExists(ctx context.Context, userID uuid.UUID) (bool, error) {
	_, err := a.repo.GetUserByID(ctx, userID)
	if err != nil {
		if err == repository.ErrNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Ensure UserExistenceAdapter implements ports.UserExistenceChecker
var _ ports.UserExistenceChecker = (*UserExistenceAdapter)(nil)
