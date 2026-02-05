package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrServiceNotFound = errors.New("lead service not found")
var ErrServiceTypeNotFound = errors.New("service type not found")

type LeadService struct {
	ID             uuid.UUID
	LeadID         uuid.UUID
	OrganizationID uuid.UUID
	ServiceType    string
	Status         string
	PipelineStage  string
	ConsumerNote   *string
	Source         *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateLeadServiceParams struct {
	LeadID         uuid.UUID
	OrganizationID uuid.UUID
	ServiceType    string
	ConsumerNote   *string
	Source         *string
}

func (r *Repository) CreateLeadService(ctx context.Context, params CreateLeadServiceParams) (LeadService, error) {
	var svc LeadService
	err := r.pool.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO RAC_lead_services (lead_id, organization_id, service_type_id, status, consumer_note, source)
			VALUES (
				$1,
				$2,
				(SELECT id FROM RAC_service_types WHERE (name = $3 OR slug = $3) AND organization_id = $2 LIMIT 1),
				'New',
				$4,
				$5
			)
			RETURNING *
		)
		SELECT i.id, i.lead_id, i.organization_id, st.name AS service_type, i.status, i.pipeline_stage, i.consumer_note, i.source,
			i.created_at, i.updated_at
		FROM inserted i
		JOIN RAC_service_types st ON st.id = i.service_type_id AND st.organization_id = i.organization_id
	`, params.LeadID, params.OrganizationID, params.ServiceType, params.ConsumerNote, params.Source).Scan(
		&svc.ID, &svc.LeadID, &svc.OrganizationID, &svc.ServiceType, &svc.Status, &svc.PipelineStage, &svc.ConsumerNote, &svc.Source,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	return svc, err
}

func (r *Repository) GetLeadServiceByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (LeadService, error) {
	var svc LeadService
	err := r.pool.QueryRow(ctx, `
		SELECT ls.id, ls.lead_id, ls.organization_id, st.name AS service_type, ls.status, ls.pipeline_stage, ls.consumer_note, ls.source,
			ls.created_at, ls.updated_at
		FROM RAC_lead_services ls
		JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
		WHERE ls.id = $1 AND ls.organization_id = $2
	`, id, organizationID).Scan(
		&svc.ID, &svc.LeadID, &svc.OrganizationID, &svc.ServiceType, &svc.Status, &svc.PipelineStage, &svc.ConsumerNote, &svc.Source,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	return svc, err
}

func (r *Repository) ListLeadServices(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]LeadService, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ls.id, ls.lead_id, ls.organization_id, st.name AS service_type, ls.status, ls.pipeline_stage, ls.consumer_note, ls.source,
			ls.created_at, ls.updated_at
		FROM RAC_lead_services ls
		JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
		WHERE ls.lead_id = $1 AND ls.organization_id = $2
		ORDER BY ls.created_at DESC
	`, leadID, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	services := make([]LeadService, 0)
	for rows.Next() {
		var svc LeadService
		if err := rows.Scan(
			&svc.ID, &svc.LeadID, &svc.OrganizationID, &svc.ServiceType, &svc.Status, &svc.PipelineStage, &svc.ConsumerNote, &svc.Source,
			&svc.CreatedAt, &svc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		services = append(services, svc)
	}
	return services, rows.Err()
}

// GetCurrentLeadService returns the most recent non-terminal (not Closed, not Bad_Lead, not Surveyed) service,
// or falls back to the most recent service if all are terminal.
func (r *Repository) GetCurrentLeadService(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (LeadService, error) {
	var svc LeadService
	// Try to find an active (non-terminal) service first
	err := r.pool.QueryRow(ctx, `
		SELECT ls.id, ls.lead_id, ls.organization_id, st.name AS service_type, ls.status, ls.pipeline_stage, ls.consumer_note, ls.source,
			ls.created_at, ls.updated_at
		FROM RAC_lead_services ls
		JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
		WHERE ls.lead_id = $1 AND ls.organization_id = $2 AND ls.status NOT IN ('Closed', 'Bad_Lead', 'Surveyed')
		ORDER BY ls.created_at DESC
		LIMIT 1
	`, leadID, organizationID).Scan(
		&svc.ID, &svc.LeadID, &svc.OrganizationID, &svc.ServiceType, &svc.Status, &svc.PipelineStage, &svc.ConsumerNote, &svc.Source,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		// Fallback to most recent service of any status
		err = r.pool.QueryRow(ctx, `
			SELECT ls.id, ls.lead_id, ls.organization_id, st.name AS service_type, ls.status, ls.pipeline_stage, ls.consumer_note, ls.source,
				ls.created_at, ls.updated_at
			FROM RAC_lead_services ls
			JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
			WHERE ls.lead_id = $1 AND ls.organization_id = $2
			ORDER BY ls.created_at DESC
			LIMIT 1
		`, leadID, organizationID).Scan(
			&svc.ID, &svc.LeadID, &svc.OrganizationID, &svc.ServiceType, &svc.Status, &svc.PipelineStage, &svc.ConsumerNote, &svc.Source,
			&svc.CreatedAt, &svc.UpdatedAt,
		)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	return svc, err
}

type UpdateLeadServiceParams struct {
	Status *string
}

func (r *Repository) UpdateLeadService(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params UpdateLeadServiceParams) (LeadService, error) {
	if params.Status == nil {
		return r.GetLeadServiceByID(ctx, id, organizationID)
	}

	query := `
		WITH updated AS (
			UPDATE RAC_lead_services SET status = $3, updated_at = now()
			WHERE id = $1 AND organization_id = $2
			RETURNING *
		)
		SELECT u.id, u.lead_id, u.organization_id, st.name AS service_type, u.status, u.pipeline_stage, u.consumer_note, u.source,
			u.created_at, u.updated_at
		FROM updated u
		JOIN RAC_service_types st ON st.id = u.service_type_id AND st.organization_id = u.organization_id
	`

	var svc LeadService
	err := r.pool.QueryRow(ctx, query, id, organizationID, *params.Status).Scan(
		&svc.ID, &svc.LeadID, &svc.OrganizationID, &svc.ServiceType, &svc.Status, &svc.PipelineStage, &svc.ConsumerNote, &svc.Source,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	return svc, err
}

// UpdateLeadServiceType updates the service type for a lead service using an active service type name/slug.
func (r *Repository) UpdateLeadServiceType(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, serviceType string) (LeadService, error) {
	var svc LeadService
	err := r.pool.QueryRow(ctx, `
		WITH target AS (
			SELECT id FROM RAC_service_types
			WHERE (name = $3 OR slug = $3)
				AND organization_id = $2
				AND is_active = true
			LIMIT 1
		), updated AS (
			UPDATE RAC_lead_services
			SET service_type_id = (SELECT id FROM target), updated_at = now()
			WHERE id = $1 AND organization_id = $2 AND EXISTS (SELECT 1 FROM target)
			RETURNING *
		)
		SELECT u.id, u.lead_id, u.organization_id, st.name AS service_type, u.status, u.pipeline_stage, u.consumer_note, u.source,
			u.created_at, u.updated_at
		FROM updated u
		JOIN RAC_service_types st ON st.id = u.service_type_id AND st.organization_id = u.organization_id
	`, id, organizationID, serviceType).Scan(
		&svc.ID, &svc.LeadID, &svc.OrganizationID, &svc.ServiceType, &svc.Status, &svc.PipelineStage, &svc.ConsumerNote, &svc.Source,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceTypeNotFound
	}
	return svc, err
}

func (r *Repository) UpdateServiceStatus(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, status string) (LeadService, error) {
	var svc LeadService
	err := r.pool.QueryRow(ctx, `
		WITH updated AS (
			UPDATE RAC_lead_services SET status = $3, updated_at = now()
			WHERE id = $1 AND organization_id = $2
			RETURNING *
		)
		SELECT u.id, u.lead_id, u.organization_id, st.name AS service_type, u.status, u.pipeline_stage, u.consumer_note, u.source,
			u.created_at, u.updated_at
		FROM updated u
		JOIN RAC_service_types st ON st.id = u.service_type_id AND st.organization_id = u.organization_id
	`, id, organizationID, status).Scan(
		&svc.ID, &svc.LeadID, &svc.OrganizationID, &svc.ServiceType, &svc.Status, &svc.PipelineStage, &svc.ConsumerNote, &svc.Source,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	return svc, err
}

func (r *Repository) UpdatePipelineStage(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, stage string) (LeadService, error) {
	var svc LeadService
	err := r.pool.QueryRow(ctx, `
		WITH updated AS (
			UPDATE RAC_lead_services SET pipeline_stage = $3, updated_at = now()
			WHERE id = $1 AND organization_id = $2
			RETURNING *
		)
		SELECT u.id, u.lead_id, u.organization_id, st.name AS service_type, u.status, u.pipeline_stage, u.consumer_note, u.source,
			u.created_at, u.updated_at
		FROM updated u
		JOIN RAC_service_types st ON st.id = u.service_type_id AND st.organization_id = u.organization_id
	`, id, organizationID, stage).Scan(
		&svc.ID, &svc.LeadID, &svc.OrganizationID, &svc.ServiceType, &svc.Status, &svc.PipelineStage, &svc.ConsumerNote, &svc.Source,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	return svc, err
}

// CloseAllActiveServices marks all non-terminal services for a lead as Closed
func (r *Repository) CloseAllActiveServices(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE RAC_lead_services 
		SET status = 'Closed', updated_at = now()
		WHERE lead_id = $1 AND organization_id = $2 AND status NOT IN ('Closed', 'Bad_Lead', 'Surveyed')
	`, leadID, organizationID)
	return err
}
