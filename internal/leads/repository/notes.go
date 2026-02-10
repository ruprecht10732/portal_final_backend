package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
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

func (r *Repository) CreateLeadNote(ctx context.Context, params CreateLeadNoteParams) (LeadNote, error) {
	var note LeadNote
	query := `
		WITH inserted AS (
			INSERT INTO RAC_lead_notes (lead_id, organization_id, author_id, type, body, service_id)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, lead_id, organization_id, author_id, type, body, service_id, created_at, updated_at
		)
		SELECT inserted.id, inserted.lead_id, inserted.organization_id, inserted.author_id, u.email, inserted.type, inserted.body, inserted.service_id, inserted.created_at, inserted.updated_at
		FROM inserted
		JOIN RAC_users u ON u.id = inserted.author_id
	`

	err := r.pool.QueryRow(ctx, query, params.LeadID, params.OrganizationID, params.AuthorID, params.Type, params.Body, params.ServiceID).Scan(
		&note.ID,
		&note.LeadID,
		&note.OrganizationID,
		&note.AuthorID,
		&note.AuthorEmail,
		&note.Type,
		&note.Body,
		&note.ServiceID,
		&note.CreatedAt,
		&note.UpdatedAt,
	)
	return note, err
}

func (r *Repository) ListLeadNotes(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]LeadNote, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ln.id, ln.lead_id, ln.organization_id, ln.author_id, u.email, ln.type, ln.body, ln.service_id, ln.created_at, ln.updated_at
		FROM RAC_lead_notes ln
		JOIN RAC_users u ON u.id = ln.author_id
		WHERE ln.lead_id = $1 AND ln.organization_id = $2
		ORDER BY ln.created_at DESC
	`, leadID, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notes := make([]LeadNote, 0)
	for rows.Next() {
		var note LeadNote
		if err := rows.Scan(
			&note.ID,
			&note.LeadID,
			&note.OrganizationID,
			&note.AuthorID,
			&note.AuthorEmail,
			&note.Type,
			&note.Body,
			&note.ServiceID,
			&note.CreatedAt,
			&note.UpdatedAt,
		); err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return notes, nil
}

// ListNotesByService returns notes scoped to a specific service (including notes
// with no service_id for backward compatibility with pre-migration data).
func (r *Repository) ListNotesByService(ctx context.Context, leadID uuid.UUID, serviceID uuid.UUID, organizationID uuid.UUID) ([]LeadNote, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ln.id, ln.lead_id, ln.organization_id, ln.author_id, u.email, ln.type, ln.body, ln.service_id, ln.created_at, ln.updated_at
		FROM RAC_lead_notes ln
		JOIN RAC_users u ON u.id = ln.author_id
		WHERE ln.lead_id = $1 AND ln.organization_id = $2 AND (ln.service_id = $3 OR ln.service_id IS NULL)
		ORDER BY ln.created_at DESC
	`, leadID, organizationID, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notes := make([]LeadNote, 0)
	for rows.Next() {
		var note LeadNote
		if err := rows.Scan(
			&note.ID,
			&note.LeadID,
			&note.OrganizationID,
			&note.AuthorID,
			&note.AuthorEmail,
			&note.Type,
			&note.Body,
			&note.ServiceID,
			&note.CreatedAt,
			&note.UpdatedAt,
		); err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return notes, nil
}
