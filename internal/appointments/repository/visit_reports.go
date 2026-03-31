package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func optionalInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	intValue := value.Int64
	return &intValue
}

func toPgInt64(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func visitReportFromModel(model appointmentsdb.RacAppointmentVisitReport) VisitReport {
	return VisitReport{
		AppointmentID:       uuid.UUID(model.AppointmentID.Bytes),
		OrganizationID:      uuid.UUID(model.OrganizationID.Bytes),
		Measurements:        optionalString(model.Measurements),
		MeasurementProducts: model.MeasurementProducts,
		AccessDifficulty:    optionalString(model.AccessDifficulty),
		Notes:               optionalString(model.Notes),
		CreatedAt:           model.CreatedAt.Time,
		UpdatedAt:           model.UpdatedAt.Time,
	}
}

func appointmentAttachmentFromModel(model appointmentsdb.RacAppointmentAttachment) AppointmentAttachment {
	return AppointmentAttachment{
		ID:             uuid.UUID(model.ID.Bytes),
		AppointmentID:  uuid.UUID(model.AppointmentID.Bytes),
		OrganizationID: uuid.UUID(model.OrganizationID.Bytes),
		FileKey:        model.FileKey,
		FileName:       model.FileName,
		ContentType:    optionalString(model.ContentType),
		SizeBytes:      optionalInt64(model.SizeBytes),
		CreatedAt:      model.CreatedAt.Time,
	}
}

func (r *Repository) GetVisitReport(ctx context.Context, appointmentID uuid.UUID, organizationID uuid.UUID) (*VisitReport, error) {
	row, err := r.queries.GetAppointmentVisitReport(ctx, appointmentsdb.GetAppointmentVisitReportParams{
		AppointmentID:  toPgUUID(appointmentID),
		OrganizationID: toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("visit report not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get visit report: %w", err)
	}

	report := visitReportFromModel(appointmentsdb.RacAppointmentVisitReport{AppointmentID: row.AppointmentID, Measurements: row.Measurements, MeasurementProducts: row.MeasurementProducts, AccessDifficulty: row.AccessDifficulty, Notes: row.Notes, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID})
	return &report, nil
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
		return nil, fmt.Errorf("failed to upsert visit report: %w", err)
	}

	saved := visitReportFromModel(appointmentsdb.RacAppointmentVisitReport{AppointmentID: row.AppointmentID, Measurements: row.Measurements, MeasurementProducts: row.MeasurementProducts, AccessDifficulty: row.AccessDifficulty, Notes: row.Notes, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID})
	return &saved, nil
}

func (r *Repository) CreateAttachment(ctx context.Context, attachment AppointmentAttachment) (*AppointmentAttachment, error) {
	row, err := r.queries.CreateAppointmentAttachment(ctx, appointmentsdb.CreateAppointmentAttachmentParams{
		ID:             toPgUUID(attachment.ID),
		AppointmentID:  toPgUUID(attachment.AppointmentID),
		OrganizationID: toPgUUID(attachment.OrganizationID),
		FileKey:        attachment.FileKey,
		FileName:       attachment.FileName,
		ContentType:    toPgText(attachment.ContentType),
		SizeBytes:      toPgInt64(attachment.SizeBytes),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create appointment attachment: %w", err)
	}

	saved := appointmentAttachmentFromModel(appointmentsdb.RacAppointmentAttachment{ID: row.ID, AppointmentID: row.AppointmentID, FileKey: row.FileKey, FileName: row.FileName, ContentType: row.ContentType, SizeBytes: row.SizeBytes, CreatedAt: row.CreatedAt, OrganizationID: row.OrganizationID})
	return &saved, nil
}

func (r *Repository) ListAttachments(ctx context.Context, appointmentID uuid.UUID, organizationID uuid.UUID) ([]AppointmentAttachment, error) {
	rows, err := r.queries.ListAppointmentAttachments(ctx, appointmentsdb.ListAppointmentAttachmentsParams{
		AppointmentID:  toPgUUID(appointmentID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list appointment attachments: %w", err)
	}

	items := make([]AppointmentAttachment, 0, len(rows))
	for _, row := range rows {
		items = append(items, appointmentAttachmentFromModel(appointmentsdb.RacAppointmentAttachment{ID: row.ID, AppointmentID: row.AppointmentID, FileKey: row.FileKey, FileName: row.FileName, ContentType: row.ContentType, SizeBytes: row.SizeBytes, CreatedAt: row.CreatedAt, OrganizationID: row.OrganizationID}))
	}

	return items, nil
}
