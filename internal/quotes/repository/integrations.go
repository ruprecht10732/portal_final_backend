package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	quotesdb "portal_final_backend/internal/quotes/db"
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
	row, err := r.queries.GetProviderIntegration(ctx, quotesdb.GetProviderIntegrationParams{
		OrganizationID: toPgUUID(orgID),
		Provider:       provider,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get provider integration: %w", err)
	}

	integration := &ProviderIntegration{
		OrganizationID:   orgID,
		Provider:         row.Provider,
		IsConnected:      row.IsConnected,
		AccessToken:      optionalString(row.AccessToken),
		RefreshToken:     optionalString(row.RefreshToken),
		TokenExpiresAt:   optionalTime(row.TokenExpiresAt),
		AdministrationID: optionalString(row.AdministrationID),
		ConnectedBy:      optionalUUID(row.ConnectedBy),
		DisconnectedAt:   optionalTime(row.DisconnectedAt),
		CreatedAt:        timeFromPg(row.CreatedAt),
		UpdatedAt:        timeFromPg(row.UpdatedAt),
	}
	return integration, nil
}

func (r *Repository) UpsertProviderIntegration(ctx context.Context, integration ProviderIntegration) error {
	err := r.queries.UpsertProviderIntegration(ctx, quotesdb.UpsertProviderIntegrationParams{
		OrganizationID:   toPgUUID(integration.OrganizationID),
		Provider:         integration.Provider,
		IsConnected:      integration.IsConnected,
		AccessToken:      toPgTextPtr(integration.AccessToken),
		RefreshToken:     toPgTextPtr(integration.RefreshToken),
		TokenExpiresAt:   toPgTimestampPtr(integration.TokenExpiresAt),
		AdministrationID: toPgTextPtr(integration.AdministrationID),
		ConnectedBy:      toPgUUIDPtr(integration.ConnectedBy),
		DisconnectedAt:   toPgTimestampPtr(integration.DisconnectedAt),
	})
	if err != nil {
		return fmt.Errorf("upsert provider integration: %w", err)
	}
	return nil
}

func (r *Repository) DisconnectProviderIntegration(ctx context.Context, orgID uuid.UUID, provider string) error {
	_, err := r.queries.DisconnectProviderIntegration(ctx, quotesdb.DisconnectProviderIntegrationParams{
		OrganizationID: toPgUUID(orgID),
		Provider:       provider,
	})
	if err != nil {
		return fmt.Errorf("disconnect provider integration: %w", err)
	}
	return nil
}

func (r *Repository) GetQuoteExport(ctx context.Context, quoteID, orgID uuid.UUID, provider string) (*QuoteExport, error) {
	row, err := r.queries.GetQuoteExport(ctx, quotesdb.GetQuoteExportParams{
		QuoteID:        toPgUUID(quoteID),
		OrganizationID: toPgUUID(orgID),
		Provider:       provider,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get quote export: %w", err)
	}

	export := &QuoteExport{
		ID:             uuid.UUID(row.ID.Bytes),
		QuoteID:        uuid.UUID(row.QuoteID.Bytes),
		OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
		Provider:       row.Provider,
		ExternalID:     row.ExternalID,
		ExternalURL:    optionalString(row.ExternalUrl),
		State:          row.State,
		CreatedAt:      timeFromPg(row.CreatedAt),
		UpdatedAt:      timeFromPg(row.UpdatedAt),
	}
	return export, nil
}

func (r *Repository) CreateQuoteExport(ctx context.Context, export QuoteExport) error {
	err := r.queries.CreateQuoteExport(ctx, quotesdb.CreateQuoteExportParams{
		ID:             toPgUUID(export.ID),
		QuoteID:        toPgUUID(export.QuoteID),
		OrganizationID: toPgUUID(export.OrganizationID),
		Provider:       export.Provider,
		ExternalID:     export.ExternalID,
		ExternalUrl:    toPgTextPtr(export.ExternalURL),
		State:          export.State,
		CreatedAt:      toPgTimestamp(export.CreatedAt),
		UpdatedAt:      toPgTimestamp(export.UpdatedAt),
	})
	if err != nil {
		return fmt.Errorf("create quote export: %w", err)
	}
	return nil
}

func (r *Repository) MustBeAcceptedQuote(ctx context.Context, quoteID, orgID uuid.UUID) error {
	status, err := r.queries.GetQuoteStatus(ctx, quotesdb.GetQuoteStatusParams{
		ID:             toPgUUID(quoteID),
		OrganizationID: toPgUUID(orgID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound(quoteNotFoundMsg)
		}
		return fmt.Errorf("get quote status: %w", err)
	}
	if status != quotesdb.QuoteStatusAccepted {
		return apperr.BadRequest("quote must be Accepted before export")
	}
	return nil
}
