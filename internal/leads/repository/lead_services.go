package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	leadsdb "portal_final_backend/internal/leads/db"
)

var ErrServiceNotFound = errors.New("lead service not found")
var ErrServiceTypeNotFound = errors.New("service type not found")

type LeadService struct {
	ID                                 uuid.UUID
	LeadID                             uuid.UUID
	OrganizationID                     uuid.UUID
	ServiceType                        string
	Status                             string
	PipelineStage                      string
	ConsumerNote                       *string
	Source                             *string
	CustomerPreferences                json.RawMessage
	GatekeeperNurturingLoopCount       int
	GatekeeperNurturingLoopFingerprint *string
	ExtraWorkAmountCents               *int64
	ExtraWorkNotes                     *string
	CreatedAt                          time.Time
	UpdatedAt                          time.Time
}

type CreateLeadServiceParams struct {
	LeadID         uuid.UUID
	OrganizationID uuid.UUID
	ServiceType    string
	ConsumerNote   *string
	Source         *string
}

type leadServiceFields struct {
	ID                                 pgtype.UUID
	LeadID                             pgtype.UUID
	OrganizationID                     pgtype.UUID
	ServiceType                        string
	Status                             string
	PipelineStage                      string
	ConsumerNote                       pgtype.Text
	Source                             pgtype.Text
	CustomerPreferences                []byte
	GatekeeperNurturingLoopCount       int32
	GatekeeperNurturingLoopFingerprint pgtype.Text
	ExtraWorkAmountCents               pgtype.Int8
	ExtraWorkNotes                     pgtype.Text
	CreatedAt                          pgtype.Timestamptz
	UpdatedAt                          pgtype.Timestamptz
}

func (r *Repository) CreateLeadService(ctx context.Context, params CreateLeadServiceParams) (LeadService, error) {
	row, err := r.queries.CreateLeadService(ctx, leadsdb.CreateLeadServiceParams{
		LeadID:         toPgUUID(params.LeadID),
		OrganizationID: toPgUUID(params.OrganizationID),
		Name:           params.ServiceType,
		ConsumerNote:   toPgText(params.ConsumerNote),
		Source:         toPgText(params.Source),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceTypeNotFound
	}
	if err != nil {
		return LeadService{}, err
	}
	return leadServiceFromRow(leadServiceFields{ID: row.ID, LeadID: row.LeadID, OrganizationID: row.OrganizationID, ServiceType: row.ServiceType, Status: row.Status, PipelineStage: string(row.PipelineStage), ConsumerNote: row.ConsumerNote, Source: row.Source, CustomerPreferences: row.CustomerPreferences, GatekeeperNurturingLoopCount: row.GatekeeperNurturingLoopCount, GatekeeperNurturingLoopFingerprint: row.GatekeeperNurturingLoopFingerprint, ExtraWorkAmountCents: row.ExtraWorkAmountCents, ExtraWorkNotes: row.ExtraWorkNotes, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}), nil
}

func (r *Repository) GetLeadServiceByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (LeadService, error) {
	row, err := r.queries.GetLeadServiceByID(ctx, leadsdb.GetLeadServiceByIDParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	if err != nil {
		return LeadService{}, err
	}
	return leadServiceFromRow(leadServiceFields{ID: row.ID, LeadID: row.LeadID, OrganizationID: row.OrganizationID, ServiceType: row.ServiceType, Status: row.Status, PipelineStage: string(row.PipelineStage), ConsumerNote: row.ConsumerNote, Source: row.Source, CustomerPreferences: row.CustomerPreferences, GatekeeperNurturingLoopCount: row.GatekeeperNurturingLoopCount, GatekeeperNurturingLoopFingerprint: row.GatekeeperNurturingLoopFingerprint, ExtraWorkAmountCents: row.ExtraWorkAmountCents, ExtraWorkNotes: row.ExtraWorkNotes, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}), nil
}

func (r *Repository) ListLeadServices(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]LeadService, error) {
	rows, err := r.queries.ListLeadServices(ctx, leadsdb.ListLeadServicesParams{LeadID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		return nil, err
	}

	services := make([]LeadService, 0, len(rows))
	for _, row := range rows {
		services = append(services, leadServiceFromRow(leadServiceFields{ID: row.ID, LeadID: row.LeadID, OrganizationID: row.OrganizationID, ServiceType: row.ServiceType, Status: row.Status, PipelineStage: string(row.PipelineStage), ConsumerNote: row.ConsumerNote, Source: row.Source, CustomerPreferences: row.CustomerPreferences, GatekeeperNurturingLoopCount: row.GatekeeperNurturingLoopCount, GatekeeperNurturingLoopFingerprint: row.GatekeeperNurturingLoopFingerprint, ExtraWorkAmountCents: row.ExtraWorkAmountCents, ExtraWorkNotes: row.ExtraWorkNotes, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}))
	}
	return services, nil
}

// GetCurrentLeadService returns the most recent non-terminal service,
// or falls back to the most recent service if all are terminal.
func (r *Repository) GetCurrentLeadService(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (LeadService, error) {
	row, err := r.queries.GetCurrentActiveLeadService(ctx, leadsdb.GetCurrentActiveLeadServiceParams{LeadID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		fallback, fallbackErr := r.queries.GetLatestLeadService(ctx, leadsdb.GetLatestLeadServiceParams{LeadID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID)})
		if errors.Is(fallbackErr, pgx.ErrNoRows) {
			return LeadService{}, ErrServiceNotFound
		}
		if fallbackErr != nil {
			return LeadService{}, fallbackErr
		}
		return leadServiceFromRow(leadServiceFields{ID: fallback.ID, LeadID: fallback.LeadID, OrganizationID: fallback.OrganizationID, ServiceType: fallback.ServiceType, Status: fallback.Status, PipelineStage: string(fallback.PipelineStage), ConsumerNote: fallback.ConsumerNote, Source: fallback.Source, CustomerPreferences: fallback.CustomerPreferences, GatekeeperNurturingLoopCount: fallback.GatekeeperNurturingLoopCount, GatekeeperNurturingLoopFingerprint: fallback.GatekeeperNurturingLoopFingerprint, ExtraWorkAmountCents: fallback.ExtraWorkAmountCents, ExtraWorkNotes: fallback.ExtraWorkNotes, CreatedAt: fallback.CreatedAt, UpdatedAt: fallback.UpdatedAt}), nil
	}
	if err != nil {
		return LeadService{}, err
	}
	return leadServiceFromRow(leadServiceFields{ID: row.ID, LeadID: row.LeadID, OrganizationID: row.OrganizationID, ServiceType: row.ServiceType, Status: row.Status, PipelineStage: string(row.PipelineStage), ConsumerNote: row.ConsumerNote, Source: row.Source, CustomerPreferences: row.CustomerPreferences, GatekeeperNurturingLoopCount: row.GatekeeperNurturingLoopCount, GatekeeperNurturingLoopFingerprint: row.GatekeeperNurturingLoopFingerprint, ExtraWorkAmountCents: row.ExtraWorkAmountCents, ExtraWorkNotes: row.ExtraWorkNotes, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}), nil
}

func (r *Repository) UpdateServiceStatusAndPipelineStage(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, status string, stage string) (LeadService, error) {
	row, err := r.queries.UpdateServiceStatusAndPipelineStage(ctx, leadsdb.UpdateServiceStatusAndPipelineStageParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID), Status: status, PipelineStage: leadsdb.PipelineStage(stage)})
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	if err != nil {
		return LeadService{}, err
	}
	return leadServiceFromRow(leadServiceFields{ID: row.ID, LeadID: row.LeadID, OrganizationID: row.OrganizationID, ServiceType: row.ServiceType, Status: row.Status, PipelineStage: string(row.PipelineStage), ConsumerNote: row.ConsumerNote, Source: row.Source, CustomerPreferences: row.CustomerPreferences, GatekeeperNurturingLoopCount: row.GatekeeperNurturingLoopCount, GatekeeperNurturingLoopFingerprint: row.GatekeeperNurturingLoopFingerprint, ExtraWorkAmountCents: row.ExtraWorkAmountCents, ExtraWorkNotes: row.ExtraWorkNotes, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}), nil
}

// UpdateLeadServiceType updates the service type for a lead service using an active service type name/slug.
func (r *Repository) UpdateLeadServiceType(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, serviceType string) (LeadService, error) {
	row, err := r.queries.UpdateLeadServiceType(ctx, leadsdb.UpdateLeadServiceTypeParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID), Name: serviceType})
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceTypeNotFound
	}
	if err != nil {
		return LeadService{}, err
	}
	return leadServiceFromRow(leadServiceFields{ID: row.ID, LeadID: row.LeadID, OrganizationID: row.OrganizationID, ServiceType: row.ServiceType, Status: row.Status, PipelineStage: string(row.PipelineStage), ConsumerNote: row.ConsumerNote, Source: row.Source, CustomerPreferences: row.CustomerPreferences, GatekeeperNurturingLoopCount: row.GatekeeperNurturingLoopCount, GatekeeperNurturingLoopFingerprint: row.GatekeeperNurturingLoopFingerprint, ExtraWorkAmountCents: row.ExtraWorkAmountCents, ExtraWorkNotes: row.ExtraWorkNotes, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}), nil
}

func (r *Repository) UpdateServiceStatus(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, status string) (LeadService, error) {
	row, err := r.queries.UpdateServiceStatus(ctx, leadsdb.UpdateServiceStatusParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID), Status: status})
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	if err != nil {
		return LeadService{}, err
	}
	return leadServiceFromRow(leadServiceFields{ID: row.ID, LeadID: row.LeadID, OrganizationID: row.OrganizationID, ServiceType: row.ServiceType, Status: row.Status, PipelineStage: string(row.PipelineStage), ConsumerNote: row.ConsumerNote, Source: row.Source, CustomerPreferences: row.CustomerPreferences, GatekeeperNurturingLoopCount: row.GatekeeperNurturingLoopCount, GatekeeperNurturingLoopFingerprint: row.GatekeeperNurturingLoopFingerprint, ExtraWorkAmountCents: row.ExtraWorkAmountCents, ExtraWorkNotes: row.ExtraWorkNotes, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}), nil
}

// CompleteLeadService moves a Fulfillment-stage service to Completed and optionally records extra work.
// Returns ErrServiceNotFound if the service does not exist or is not in the Fulfillment stage.
func (r *Repository) CompleteLeadService(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, extraWorkAmountCents *int64, extraWorkNotes *string) (LeadService, error) {
	row, err := r.queries.CompleteLeadService(ctx, leadsdb.CompleteLeadServiceParams{
		ID:                   toPgUUID(id),
		OrganizationID:       toPgUUID(organizationID),
		ExtraWorkAmountCents: toPgInt8Ptr(extraWorkAmountCents),
		ExtraWorkNotes:       toPgText(extraWorkNotes),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	if err != nil {
		return LeadService{}, err
	}
	return leadServiceFromRow(leadServiceFields{ID: row.ID, LeadID: row.LeadID, OrganizationID: row.OrganizationID, ServiceType: row.ServiceType, Status: row.Status, PipelineStage: string(row.PipelineStage), ConsumerNote: row.ConsumerNote, Source: row.Source, CustomerPreferences: row.CustomerPreferences, GatekeeperNurturingLoopCount: row.GatekeeperNurturingLoopCount, GatekeeperNurturingLoopFingerprint: row.GatekeeperNurturingLoopFingerprint, ExtraWorkAmountCents: row.ExtraWorkAmountCents, ExtraWorkNotes: row.ExtraWorkNotes, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}), nil
}

func (r *Repository) SetGatekeeperNurturingLoopState(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, count int, fingerprint string) error {
	return r.queries.SetGatekeeperNurturingLoopState(ctx, leadsdb.SetGatekeeperNurturingLoopStateParams{
		ID:                                 toPgUUID(id),
		OrganizationID:                     toPgUUID(organizationID),
		GatekeeperNurturingLoopCount:       int32(count),
		GatekeeperNurturingLoopFingerprint: toPgTextValue(fingerprint),
	})
}

func (r *Repository) ResetGatekeeperNurturingLoopState(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error {
	return r.queries.ResetGatekeeperNurturingLoopState(ctx, leadsdb.ResetGatekeeperNurturingLoopStateParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
	})
}

func (r *Repository) UpdatePipelineStage(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, stage string) (LeadService, error) {
	row, err := r.queries.UpdatePipelineStage(ctx, leadsdb.UpdatePipelineStageParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID), PipelineStage: leadsdb.PipelineStage(stage)})
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	if err != nil {
		return LeadService{}, err
	}
	return leadServiceFromRow(leadServiceFields{ID: row.ID, LeadID: row.LeadID, OrganizationID: row.OrganizationID, ServiceType: row.ServiceType, Status: row.Status, PipelineStage: string(row.PipelineStage), ConsumerNote: row.ConsumerNote, Source: row.Source, CustomerPreferences: row.CustomerPreferences, GatekeeperNurturingLoopCount: row.GatekeeperNurturingLoopCount, GatekeeperNurturingLoopFingerprint: row.GatekeeperNurturingLoopFingerprint, ExtraWorkAmountCents: row.ExtraWorkAmountCents, ExtraWorkNotes: row.ExtraWorkNotes, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}), nil
}

// CloseAllActiveServices marks all non-terminal services for a lead as Completed (pipeline stage).
func (r *Repository) CloseAllActiveServices(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) error {
	return r.queries.CloseAllActiveServices(ctx, leadsdb.CloseAllActiveServicesParams{LeadID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID)})
}

func (r *Repository) UpdateServicePreferences(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID, prefs []byte) error {
	return r.queries.UpdateServicePreferences(ctx, leadsdb.UpdateServicePreferencesParams{ID: toPgUUID(serviceID), OrganizationID: toPgUUID(organizationID), CustomerPreferences: prefs})
}

// InsertServiceEventParams groups the fields for inserting a service event.
type InsertServiceEventParams struct {
	OrganizationID uuid.UUID
	LeadID         uuid.UUID
	LeadServiceID  uuid.UUID
	EventType      string
	Status         string
	PipelineStage  string
	OccurredAt     time.Time
}

// InsertLeadServiceEvent inserts an exportable service event without mutating the service state.
// This is used for milestone events that are not represented as a generic status.
// The insert is guarded by a NOT EXISTS check to prevent duplicates for the same
// (lead_service_id, event_type) combination.
func (r *Repository) InsertLeadServiceEvent(ctx context.Context, params InsertServiceEventParams) error {
	return r.queries.InsertLeadServiceEvent(ctx, leadsdb.InsertLeadServiceEventParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		LeadID:         toPgUUID(params.LeadID),
		LeadServiceID:  toPgUUID(params.LeadServiceID),
		EventType:      params.EventType,
		Status:         toPgTextValue(params.Status),
		PipelineStage:  toPgTextValue(params.PipelineStage),
		OccurredAt:     toPgTimestamp(params.OccurredAt),
	})
}

func leadServiceFromRow(fields leadServiceFields) LeadService {
	return LeadService{
		ID:                                 fields.ID.Bytes,
		LeadID:                             fields.LeadID.Bytes,
		OrganizationID:                     fields.OrganizationID.Bytes,
		ServiceType:                        fields.ServiceType,
		Status:                             fields.Status,
		PipelineStage:                      fields.PipelineStage,
		ConsumerNote:                       optionalString(fields.ConsumerNote),
		Source:                             optionalString(fields.Source),
		CustomerPreferences:                fields.CustomerPreferences,
		GatekeeperNurturingLoopCount:       int(fields.GatekeeperNurturingLoopCount),
		GatekeeperNurturingLoopFingerprint: optionalString(fields.GatekeeperNurturingLoopFingerprint),
		ExtraWorkAmountCents:               optionalInt64(fields.ExtraWorkAmountCents),
		ExtraWorkNotes:                     optionalString(fields.ExtraWorkNotes),
		CreatedAt:                          fields.CreatedAt.Time,
		UpdatedAt:                          fields.UpdatedAt.Time,
	}
}
