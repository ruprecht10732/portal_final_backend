package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	leadsdb "portal_final_backend/internal/leads/db"
)

func (r *Repository) GetByPublicToken(ctx context.Context, token string) (Lead, error) {
	row, err := r.queries.GetLeadByPublicToken(ctx, toPgTextValue(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	if err != nil {
		return Lead{}, err
	}
	if row.DeletedAt.Valid {
		return Lead{}, ErrNotFound
	}
	return leadFromDB(row), nil
}

func (r *Repository) SetPublicToken(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, token string, expiresAt time.Time) error {
	return r.queries.SetLeadPublicToken(ctx, leadsdb.SetLeadPublicTokenParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID), PublicToken: toPgTextValue(token), PublicTokenExpiresAt: toPgTimestamp(expiresAt)})
}
