package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	leadsdb "portal_final_backend/internal/leads/db"
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
	UploadedBy     *uuid.UUID
}

// CreateAttachment inserts a new attachment record.
func (r *Repository) CreateAttachment(ctx context.Context, params CreateAttachmentParams) (Attachment, error) {
	row, err := r.queries.CreateAttachment(ctx, leadsdb.CreateAttachmentParams{
		LeadServiceID:  toPgUUID(params.LeadServiceID),
		OrganizationID: toPgUUID(params.OrganizationID),
		FileKey:        params.FileKey,
		FileName:       params.FileName,
		ContentType:    toPgTextValue(params.ContentType),
		SizeBytes:      toPgInt8Value(params.SizeBytes),
		UploadedBy:     toPgUUIDPtr(params.UploadedBy),
	})
	if err != nil {
		return Attachment{}, err
	}
	return attachmentFromRow(row), nil
}

// GetAttachmentByID retrieves an attachment by ID, scoped to organization.
func (r *Repository) GetAttachmentByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (Attachment, error) {
	row, err := r.queries.GetAttachmentByID(ctx, leadsdb.GetAttachmentByIDParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, ErrAttachmentNotFound
	}
	if err != nil {
		return Attachment{}, err
	}
	return attachmentFromRow(row), nil
}

// ListAttachmentsByService retrieves all attachments for a lead service.
func (r *Repository) ListAttachmentsByService(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) ([]Attachment, error) {
	rows, err := r.queries.ListAttachmentsByService(ctx, leadsdb.ListAttachmentsByServiceParams{LeadServiceID: toPgUUID(leadServiceID), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		return nil, err
	}

	attachments := make([]Attachment, 0, len(rows))
	attachmentIndexByFileKey := make(map[string]int, len(rows))
	for _, row := range rows {
		attachment := attachmentFromRow(row)
		if existingIndex, exists := attachmentIndexByFileKey[attachment.FileKey]; exists {
			if preferAttachmentRecord(attachment, attachments[existingIndex]) {
				attachments[existingIndex] = attachment
			}
			continue
		}

		attachmentIndexByFileKey[attachment.FileKey] = len(attachments)
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

// DeleteAttachment removes an attachment record by ID.
func (r *Repository) DeleteAttachment(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error {
	rowsAffected, err := r.queries.DeleteAttachment(ctx, leadsdb.DeleteAttachmentParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrAttachmentNotFound
	}
	return nil
}

func attachmentFromRow(row leadsdb.RacLeadServiceAttachment) Attachment {
	return Attachment{
		ID:             row.ID.Bytes,
		LeadServiceID:  row.LeadServiceID.Bytes,
		OrganizationID: row.OrganizationID.Bytes,
		FileKey:        row.FileKey,
		FileName:       row.FileName,
		ContentType:    optionalString(row.ContentType),
		SizeBytes:      optionalInt64(row.SizeBytes),
		UploadedBy:     optionalUUID(row.UploadedBy),
		CreatedAt:      row.CreatedAt.Time,
	}
}

func preferAttachmentRecord(candidate Attachment, existing Attachment) bool {
	candidateHasSize := attachmentHasSize(candidate)
	existingHasSize := attachmentHasSize(existing)
	if candidateHasSize != existingHasSize {
		return candidateHasSize
	}
	return candidate.CreatedAt.After(existing.CreatedAt)
}

func attachmentHasSize(att Attachment) bool {
	return att.SizeBytes != nil && *att.SizeBytes > 0
}
