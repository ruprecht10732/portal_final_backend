package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ProviderIntegration struct {
	OrganizationID   uuid.UUID
	Provider         string
	IsConnected      bool
	AccessToken      *string
	RefreshToken     *string
	TokenExpiresAt   *time.Time
	AdministrationID *string
	ConnectedBy      *uuid.UUID
	DisconnectedAt   *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type QuoteExport struct {
	ID             uuid.UUID
	QuoteID        uuid.UUID
	OrganizationID uuid.UUID
	Provider       string
	ExternalID     string
	ExternalURL    *string
	State          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (r *Repository) GetProviderIntegration(ctx context.Context, orgID uuid.UUID, provider string) (*ProviderIntegration, error) {
	query := `
		SELECT organization_id, provider, is_connected, access_token, refresh_token, token_expires_at,
			administration_id, connected_by, disconnected_at, created_at, updated_at
		FROM RAC_provider_integrations
		WHERE organization_id = $1 AND provider = $2`

	var integration ProviderIntegration
	err := r.pool.QueryRow(ctx, query, orgID, provider).Scan(
		&integration.OrganizationID,
		&integration.Provider,
		&integration.IsConnected,
		&integration.AccessToken,
		&integration.RefreshToken,
		&integration.TokenExpiresAt,
		&integration.AdministrationID,
		&integration.ConnectedBy,
		&integration.DisconnectedAt,
		&integration.CreatedAt,
		&integration.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get provider integration: %w", err)
	}

	return &integration, nil
}

func (r *Repository) UpsertProviderIntegration(ctx context.Context, integration ProviderIntegration) error {
	query := `
		INSERT INTO RAC_provider_integrations (
			organization_id, provider, is_connected, access_token, refresh_token, token_expires_at,
			administration_id, connected_by, disconnected_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now())
		ON CONFLICT (organization_id, provider)
		DO UPDATE SET
			is_connected = EXCLUDED.is_connected,
			access_token = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			token_expires_at = EXCLUDED.token_expires_at,
			administration_id = EXCLUDED.administration_id,
			connected_by = EXCLUDED.connected_by,
			disconnected_at = EXCLUDED.disconnected_at,
			updated_at = now()`

	_, err := r.pool.Exec(
		ctx,
		query,
		integration.OrganizationID,
		integration.Provider,
		integration.IsConnected,
		integration.AccessToken,
		integration.RefreshToken,
		integration.TokenExpiresAt,
		integration.AdministrationID,
		integration.ConnectedBy,
		integration.DisconnectedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert provider integration: %w", err)
	}

	return nil
}

func (r *Repository) DisconnectProviderIntegration(ctx context.Context, orgID uuid.UUID, provider string) error {
	query := `
		UPDATE RAC_provider_integrations
		SET is_connected = false,
			access_token = NULL,
			refresh_token = NULL,
			token_expires_at = NULL,
			administration_id = NULL,
			disconnected_at = now(),
			updated_at = now()
		WHERE organization_id = $1 AND provider = $2`

	result, err := r.pool.Exec(ctx, query, orgID, provider)
	if err != nil {
		return fmt.Errorf("disconnect provider integration: %w", err)
	}
	if result.RowsAffected() == 0 {
		return nil
	}

	return nil
}

func (r *Repository) GetQuoteExport(ctx context.Context, quoteID, orgID uuid.UUID, provider string) (*QuoteExport, error) {
	query := `
		SELECT id, quote_id, organization_id, provider, external_id, external_url, state, created_at, updated_at
		FROM RAC_quote_exports
		WHERE quote_id = $1 AND organization_id = $2 AND provider = $3`

	var export QuoteExport
	err := r.pool.QueryRow(ctx, query, quoteID, orgID, provider).Scan(
		&export.ID,
		&export.QuoteID,
		&export.OrganizationID,
		&export.Provider,
		&export.ExternalID,
		&export.ExternalURL,
		&export.State,
		&export.CreatedAt,
		&export.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get quote export: %w", err)
	}

	return &export, nil
}

func (r *Repository) CreateQuoteExport(ctx context.Context, export QuoteExport) error {
	query := `
		INSERT INTO RAC_quote_exports (
			id, quote_id, organization_id, provider, external_id, external_url, state, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := r.pool.Exec(ctx, query,
		export.ID,
		export.QuoteID,
		export.OrganizationID,
		export.Provider,
		export.ExternalID,
		export.ExternalURL,
		export.State,
		export.CreatedAt,
		export.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create quote export: %w", err)
	}

	return nil
}

func (r *Repository) MustBeAcceptedQuote(ctx context.Context, quoteID, orgID uuid.UUID) error {
	query := `
		SELECT status
		FROM RAC_quotes
		WHERE id = $1 AND organization_id = $2`

	var status string
	err := r.pool.QueryRow(ctx, query, quoteID, orgID).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound(quoteNotFoundMsg)
		}
		return fmt.Errorf("get quote status: %w", err)
	}

	if status != "Accepted" {
		return apperr.BadRequest("quote must be Accepted before export")
	}

	return nil
}
