package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrAttachmentNotFound = errors.New("attachment not found")

// Attachment represents a file attachment for a lead service.
type Attachment struct {
	ID             uuid.UUID
	LeadServiceID  uuid.UUID
	OrganizationID uuid.UUID
	FileKey        string
	FileName       string
	ContentType    *string
	SizeBytes      *int64
	UploadedBy     *uuid.UUID
	CreatedAt      time.Time
}

// CreateAttachmentParams contains parameters for creating an attachment record.
type CreateAttachmentParams struct {
	LeadServiceID  uuid.UUID
	OrganizationID uuid.UUID
	FileKey        string
	FileName       string
	ContentType    string
	SizeBytes      int64
	UploadedBy     uuid.UUID
}

// CreateAttachment inserts a new attachment record.
func (r *Repository) CreateAttachment(ctx context.Context, params CreateAttachmentParams) (Attachment, error) {
	var att Attachment
	err := r.pool.QueryRow(ctx, `
		INSERT INTO RAC_lead_service_attachments (lead_service_id, organization_id, file_key, file_name, content_type, size_bytes, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, lead_service_id, organization_id, file_key, file_name, content_type, size_bytes, uploaded_by, created_at
	`, params.LeadServiceID, params.OrganizationID, params.FileKey, params.FileName, params.ContentType, params.SizeBytes, params.UploadedBy).Scan(
		&att.ID, &att.LeadServiceID, &att.OrganizationID, &att.FileKey, &att.FileName, &att.ContentType, &att.SizeBytes, &att.UploadedBy, &att.CreatedAt,
	)
	return att, err
}

// GetAttachmentByID retrieves an attachment by ID, scoped to organization.
func (r *Repository) GetAttachmentByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (Attachment, error) {
	var att Attachment
	err := r.pool.QueryRow(ctx, `
		SELECT id, lead_service_id, organization_id, file_key, file_name, content_type, size_bytes, uploaded_by, created_at
		FROM RAC_lead_service_attachments
		WHERE id = $1 AND organization_id = $2
	`, id, organizationID).Scan(
		&att.ID, &att.LeadServiceID, &att.OrganizationID, &att.FileKey, &att.FileName, &att.ContentType, &att.SizeBytes, &att.UploadedBy, &att.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, ErrAttachmentNotFound
	}
	return att, err
}

// ListAttachmentsByService retrieves all attachments for a lead service.
func (r *Repository) ListAttachmentsByService(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) ([]Attachment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, lead_service_id, organization_id, file_key, file_name, content_type, size_bytes, uploaded_by, created_at
		FROM RAC_lead_service_attachments
		WHERE lead_service_id = $1 AND organization_id = $2
		ORDER BY created_at DESC
	`, leadServiceID, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attachments := make([]Attachment, 0)
	for rows.Next() {
		var att Attachment
		if err := rows.Scan(
			&att.ID, &att.LeadServiceID, &att.OrganizationID, &att.FileKey, &att.FileName, &att.ContentType, &att.SizeBytes, &att.UploadedBy, &att.CreatedAt,
		); err != nil {
			return nil, err
		}
		attachments = append(attachments, att)
	}
	return attachments, rows.Err()
}

// DeleteAttachment removes an attachment record by ID.
func (r *Repository) DeleteAttachment(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `
		DELETE FROM RAC_lead_service_attachments
		WHERE id = $1 AND organization_id = $2
	`, id, organizationID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrAttachmentNotFound
	}
	return nil
}
