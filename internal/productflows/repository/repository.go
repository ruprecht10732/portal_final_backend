package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"portal_final_backend/platform/apperr"
)

// ProductFlow is the domain model stored in rac_product_flows.
type ProductFlow struct {
	ID             uuid.UUID
	OrganizationID *uuid.UUID
	ProductGroupID string
	Version        int
	IsActive       bool
	Definition     json.RawMessage
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Repository provides data access for product flows.
type Repository interface {
	// GetActiveFlow returns the active flow for a product group.
	// It prioritises a tenant-specific override, falling back to the global default.
	GetActiveFlow(ctx context.Context, organizationID uuid.UUID, productGroupID string) (ProductFlow, error)
	// ListAll returns every active flow visible to the tenant (admin).
	ListAll(ctx context.Context, organizationID uuid.UUID) ([]ProductFlow, error)
	// Create inserts a new flow definition.
	Create(ctx context.Context, organizationID uuid.UUID, productGroupID string, definition json.RawMessage) (ProductFlow, error)
	// Update replaces the definition of an existing flow.
	Update(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, definition json.RawMessage) (ProductFlow, error)
	// Delete soft-deletes a flow by setting is_active = false.
	Delete(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error
	// Duplicate copies an existing flow into a new row for the given tenant.
	Duplicate(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (ProductFlow, error)
}

// Repo is the PostgreSQL implementation of Repository.
type Repo struct {
	pool *pgxpool.Pool
}

// New creates a new product-flows repository.
func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

var _ Repository = (*Repo)(nil)

// GetActiveFlow resolves the active flow for a product group.
// Priority: tenant override → global default (organization_id IS NULL).
func (r *Repo) GetActiveFlow(ctx context.Context, organizationID uuid.UUID, productGroupID string) (ProductFlow, error) {
	const query = `
	SELECT id, organization_id, product_group_id, version, is_active, definition, created_at, updated_at
		FROM rac_product_flows
		WHERE (organization_id = $1 OR organization_id IS NULL)
		  AND product_group_id = $2
		  AND is_active = true
		ORDER BY organization_id NULLS LAST
		LIMIT 1`
	row := r.pool.QueryRow(ctx, query, organizationID, productGroupID)
	return scanFlow(row)
}

// ListAll returns all active flows visible to the given tenant.
func (r *Repo) ListAll(ctx context.Context, organizationID uuid.UUID) ([]ProductFlow, error) {
	const query = `
	SELECT id, organization_id, product_group_id, version, is_active, definition, created_at, updated_at
		FROM rac_product_flows
		WHERE (organization_id = $1 OR organization_id IS NULL)
		  AND is_active = true
		ORDER BY product_group_id, organization_id NULLS LAST`
	rows, err := r.pool.Query(ctx, query, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list product flows: %w", err)
	}
	defer rows.Close()
	var flows []ProductFlow
	for rows.Next() {
		f, err := scanFlowFromRows(rows)
		if err != nil {
			return nil, err
		}
		flows = append(flows, f)
	}
	return flows, rows.Err()
}

// Create inserts a new global flow definition (organization_id = NULL).
func (r *Repo) Create(ctx context.Context, organizationID uuid.UUID, productGroupID string, definition json.RawMessage) (ProductFlow, error) {
	const query = `
	INSERT INTO rac_product_flows (organization_id, product_group_id, definition)
		VALUES ($1, $2, $3)
		RETURNING id, organization_id, product_group_id, version, is_active, definition, created_at, updated_at`
	row := r.pool.QueryRow(ctx, query, organizationID, productGroupID, definition)
	return scanFlow(row)
}

// Update replaces the definition JSONB for a specific flow.
func (r *Repo) Update(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, definition json.RawMessage) (ProductFlow, error) {
	const query = `
	UPDATE rac_product_flows
		SET definition = $1, version = version + 1, updated_at = NOW()
		WHERE id = $2 AND (organization_id = $3 OR organization_id IS NULL)
		RETURNING id, organization_id, product_group_id, version, is_active, definition, created_at, updated_at`
	row := r.pool.QueryRow(ctx, query, definition, id, organizationID)
	return scanFlow(row)
}

// Delete soft-deletes a flow by setting is_active = false.
func (r *Repo) Delete(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error {
	const query = `
	UPDATE rac_product_flows
		SET is_active = false, updated_at = NOW()
		WHERE id = $1 AND organization_id = $2 AND is_active = true`
	tag, err := r.pool.Exec(ctx, query, id, organizationID)
	if err != nil {
		return fmt.Errorf("delete product flow: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("product flow not found")
	}
	return nil
}

// Duplicate copies an existing flow into a new row for the given tenant.
func (r *Repo) Duplicate(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (ProductFlow, error) {
	const query = `
	INSERT INTO rac_product_flows (organization_id, product_group_id, definition)
		SELECT $2, product_group_id || '-copy', definition
		FROM rac_product_flows
		WHERE id = $1 AND (organization_id = $2 OR organization_id IS NULL) AND is_active = true
		RETURNING id, organization_id, product_group_id, version, is_active, definition, created_at, updated_at`
	row := r.pool.QueryRow(ctx, query, id, organizationID)
	return scanFlow(row)
}

func scanFlow(row pgx.Row) (ProductFlow, error) {
	var f ProductFlow
	err := row.Scan(&f.ID, &f.OrganizationID, &f.ProductGroupID, &f.Version, &f.IsActive, &f.Definition, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProductFlow{}, apperr.NotFound("product flow not found")
		}
		return ProductFlow{}, fmt.Errorf("scan product flow: %w", err)
	}
	return f, nil
}

func scanFlowFromRows(rows pgx.Rows) (ProductFlow, error) {
	var f ProductFlow
	err := rows.Scan(&f.ID, &f.OrganizationID, &f.ProductGroupID, &f.Version, &f.IsActive, &f.Definition, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return ProductFlow{}, fmt.Errorf("scan product flow row: %w", err)
	}
	return f, nil
}
