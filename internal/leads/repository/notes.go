package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	leadsdb "portal_final_backend/internal/leads/db"
)

type LeadNote struct {
	ID             uuid.UUID
	LeadID         uuid.UUID
	OrganizationID uuid.UUID
	AuthorID       uuid.UUID
	AuthorEmail    string
	Type           string
	Body           string
	ServiceID      *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateLeadNoteParams struct {
	LeadID         uuid.UUID
	OrganizationID uuid.UUID
	AuthorID       uuid.UUID
	Type           string
	Body           string
	ServiceID      *uuid.UUID
}

type leadNoteFields struct {
	ID             pgtype.UUID
	LeadID         pgtype.UUID
	OrganizationID pgtype.UUID
	AuthorID       pgtype.UUID
	Email          string
	Type           string
	Body           string
	ServiceID      pgtype.UUID
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
}

func (r *Repository) CreateLeadNote(ctx context.Context, params CreateLeadNoteParams) (LeadNote, error) {
	row, err := r.queries.CreateLeadNote(ctx, leadsdb.CreateLeadNoteParams{
		LeadID:         toPgUUID(params.LeadID),
		OrganizationID: toPgUUID(params.OrganizationID),
		AuthorID:       toPgUUID(params.AuthorID),
		Type:           params.Type,
		Body:           params.Body,
		ServiceID:      toPgUUIDPtr(params.ServiceID),
	})
	if err != nil {
		return LeadNote{}, err
	}
	return leadNoteFromRow(leadNoteFields{ID: row.ID, LeadID: row.LeadID, OrganizationID: row.OrganizationID, AuthorID: row.AuthorID, Email: row.Email, Type: row.Type, Body: row.Body, ServiceID: row.ServiceID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}), nil
}

func (r *Repository) ListLeadNotes(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]LeadNote, error) {
	rows, err := r.queries.ListLeadNotes(ctx, leadsdb.ListLeadNotesParams{LeadID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		return nil, err
	}

	notes := make([]LeadNote, 0, len(rows))
	for _, row := range rows {
		notes = append(notes, leadNoteFromRow(leadNoteFields{ID: row.ID, LeadID: row.LeadID, OrganizationID: row.OrganizationID, AuthorID: row.AuthorID, Email: row.Email, Type: row.Type, Body: row.Body, ServiceID: row.ServiceID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}))
	}
	return notes, nil
}

// ListNotesByService returns notes scoped to a specific service (including notes
// with no service_id for backward compatibility with pre-migration data).
func (r *Repository) ListNotesByService(ctx context.Context, leadID uuid.UUID, serviceID uuid.UUID, organizationID uuid.UUID) ([]LeadNote, error) {
	rows, err := r.queries.ListNotesByService(ctx, leadsdb.ListNotesByServiceParams{LeadID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID), ServiceID: toPgUUID(serviceID)})
	if err != nil {
		return nil, err
	}

	notes := make([]LeadNote, 0, len(rows))
	for _, row := range rows {
		notes = append(notes, leadNoteFromRow(leadNoteFields{ID: row.ID, LeadID: row.LeadID, OrganizationID: row.OrganizationID, AuthorID: row.AuthorID, Email: row.Email, Type: row.Type, Body: row.Body, ServiceID: row.ServiceID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}))
	}
	return notes, nil
}

func leadNoteFromRow(fields leadNoteFields) LeadNote {
	return LeadNote{
		ID:             fields.ID.Bytes,
		LeadID:         fields.LeadID.Bytes,
		OrganizationID: fields.OrganizationID.Bytes,
		AuthorID:       fields.AuthorID.Bytes,
		AuthorEmail:    fields.Email,
		Type:           fields.Type,
		Body:           fields.Body,
		ServiceID:      optionalUUID(fields.ServiceID),
		CreatedAt:      fields.CreatedAt.Time,
		UpdatedAt:      fields.UpdatedAt.Time,
	}
}
