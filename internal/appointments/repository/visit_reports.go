package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	appointmentsdb "portal_final_backend/internal/appointments/db"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type VisitReport struct {
	AppointmentID       uuid.UUID
	OrganizationID      uuid.UUID
	Measurements        *string
	MeasurementProducts json.RawMessage
	AccessDifficulty    *string
	Notes               *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type AppointmentAttachment struct {
	ID             uuid.UUID
	AppointmentID  uuid.UUID
	OrganizationID uuid.UUID
	FileKey        string
	FileName       string
	ContentType    *string
	SizeBytes      *int64
	CreatedAt      time.Time
}

// --- Type Helpers ---

func optionalInt64(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func toPgInt64(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

// --- Visit Reports ---

func (r *Repository) GetVisitReport(ctx context.Context, apptID, orgID uuid.UUID) (*VisitReport, error) {
	row, err := r.queries.GetAppointmentVisitReport(ctx, appointmentsdb.GetAppointmentVisitReportParams{
		AppointmentID:  toPgUUID(apptID),
		OrganizationID: toPgUUID(orgID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("visit report not found")
		}
		return nil, err
	}

	return &VisitReport{
		AppointmentID:       uuid.UUID(row.AppointmentID.Bytes),
		OrganizationID:      uuid.UUID(row.OrganizationID.Bytes),
		Measurements:        optionalString(row.Measurements),
		MeasurementProducts: row.MeasurementProducts,
		AccessDifficulty:    optionalString(row.AccessDifficulty),
		Notes:               optionalString(row.Notes),
		CreatedAt:           row.CreatedAt.Time,
		UpdatedAt:           row.UpdatedAt.Time,
	}, nil
}

func (r *Repository) UpsertVisitReport(ctx context.Context, report VisitReport) (*VisitReport, error) {
	row, err := r.queries.UpsertAppointmentVisitReport(ctx, appointmentsdb.UpsertAppointmentVisitReportParams{
		AppointmentID:       toPgUUID(report.AppointmentID),
		OrganizationID:      toPgUUID(report.OrganizationID),
		Measurements:        toPgText(report.Measurements),
		MeasurementProducts: report.MeasurementProducts,
		AccessDifficulty:    toPgText(report.AccessDifficulty),
		Notes:               toPgText(report.Notes),
	})
	if err != nil {
		return nil, err
	}

	return &VisitReport{
		AppointmentID:       uuid.UUID(row.AppointmentID.Bytes),
		OrganizationID:      uuid.UUID(row.OrganizationID.Bytes),
		Measurements:        optionalString(row.Measurements),
		MeasurementProducts: row.MeasurementProducts,
		AccessDifficulty:    optionalString(row.AccessDifficulty),
		Notes:               optionalString(row.Notes),
		CreatedAt:           row.CreatedAt.Time,
		UpdatedAt:           row.UpdatedAt.Time,
	}, nil
}

// --- Attachments ---

func (r *Repository) CreateAttachment(ctx context.Context, att AppointmentAttachment) (*AppointmentAttachment, error) {
	row, err := r.queries.CreateAppointmentAttachment(ctx, appointmentsdb.CreateAppointmentAttachmentParams{
		ID:             toPgUUID(att.ID),
		AppointmentID:  toPgUUID(att.AppointmentID),
		OrganizationID: toPgUUID(att.OrganizationID),
		FileKey:        att.FileKey,
		FileName:       att.FileName,
		ContentType:    toPgText(att.ContentType),
		SizeBytes:      toPgInt64(att.SizeBytes),
	})
	if err != nil {
		return nil, err
	}

	return &AppointmentAttachment{
		ID:             uuid.UUID(row.ID.Bytes),
		AppointmentID:  uuid.UUID(row.AppointmentID.Bytes),
		OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
		FileKey:        row.FileKey,
		FileName:       row.FileName,
		ContentType:    optionalString(row.ContentType),
		SizeBytes:      optionalInt64(row.SizeBytes),
		CreatedAt:      row.CreatedAt.Time,
	}, nil
}

func (r *Repository) ListAttachments(ctx context.Context, apptID, orgID uuid.UUID) ([]AppointmentAttachment, error) {
	rows, err := r.queries.ListAppointmentAttachments(ctx, appointmentsdb.ListAppointmentAttachmentsParams{
		AppointmentID:  toPgUUID(apptID),
		OrganizationID: toPgUUID(orgID),
	})
	if err != nil {
		return nil, err
	}

	// Optimized O(N) allocation with direct index assignment
	items := make([]AppointmentAttachment, len(rows))
	for i, row := range rows {
		items[i] = AppointmentAttachment{
			ID:             uuid.UUID(row.ID.Bytes),
			AppointmentID:  uuid.UUID(row.AppointmentID.Bytes),
			OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
			FileKey:        row.FileKey,
			FileName:       row.FileName,
			ContentType:    optionalString(row.ContentType),
			SizeBytes:      optionalInt64(row.SizeBytes),
			CreatedAt:      row.CreatedAt.Time,
		}
	}

	return items, nil
}
