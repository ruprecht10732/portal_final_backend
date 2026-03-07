package exports

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	exportsdb "portal_final_backend/internal/exports/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrCredentialNotFound = errors.New("google ads export credential not found")

const (
	credentialUsernamePrefix = "gadsu_"
	credentialPasswordPrefix = "gadsp_"
)

// ExportCredential represents Google Ads export credentials stored in the database.
type ExportCredential struct {
	ID                uuid.UUID
	OrganizationID    uuid.UUID
	Username          string
	PasswordHash      string
	PasswordEncrypted *string
	CreatedBy         *uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastUsedAt        *time.Time
}

// ConversionEvent represents a lead service event used for conversion exports.
type ConversionEvent struct {
	EventID             uuid.UUID
	OrganizationID      uuid.UUID
	LeadID              uuid.UUID
	LeadServiceID       uuid.UUID
	EventType           string
	Status              *string
	PipelineStage       *string
	OccurredAt          time.Time
	GCLID               string
	ConsumerEmail       *string
	ConsumerPhone       string
	ConsumerFirstName   string
	ConsumerLastName    string
	AddressStreet       string
	AddressHouseNumber  string
	AddressCity         string
	AddressZipCode      string
	ProjectedValueCents int64
}

// Repository provides data access for export operations.
type Repository struct {
	pool    *pgxpool.Pool
	queries *exportsdb.Queries
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: exportsdb.New(pool)}
}

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func toPgUUIDPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return toPgUUID(*id)
}

func toPgText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func optionalUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := uuid.UUID(value.Bytes)
	return &id
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time
	return &timestamp
}

func exportCredentialFromModel(model exportsdb.RacGoogleAdsExportCredential) ExportCredential {
	return ExportCredential{
		ID:                uuid.UUID(model.ID.Bytes),
		OrganizationID:    uuid.UUID(model.OrganizationID.Bytes),
		Username:          model.Username,
		PasswordHash:      model.PasswordHash,
		PasswordEncrypted: optionalString(model.PasswordEncrypted),
		CreatedBy:         optionalUUID(model.CreatedBy),
		CreatedAt:         model.CreatedAt.Time,
		UpdatedAt:         model.UpdatedAt.Time,
		LastUsedAt:        optionalTime(model.LastUsedAt),
	}
}

// GenerateCredential generates random username/password for Google Ads HTTPS Basic Auth.
func GenerateCredential() (username string, password string, err error) {
	usernameBytes := make([]byte, 12)
	if _, err := rand.Read(usernameBytes); err != nil {
		return "", "", err
	}
	passwordBytes := make([]byte, 24)
	if _, err := rand.Read(passwordBytes); err != nil {
		return "", "", err
	}

	username = credentialUsernamePrefix + hex.EncodeToString(usernameBytes)
	password = credentialPasswordPrefix + hex.EncodeToString(passwordBytes)
	return username, password, nil
}

// UpsertCredential creates or rotates an organization credential.
func (r *Repository) UpsertCredential(ctx context.Context, orgID uuid.UUID, username string, passwordHash string, passwordEncrypted *string, createdBy *uuid.UUID) (ExportCredential, error) {
	row, err := r.queries.UpsertGoogleAdsExportCredential(ctx, exportsdb.UpsertGoogleAdsExportCredentialParams{
		OrganizationID:    toPgUUID(orgID),
		Username:          username,
		PasswordHash:      passwordHash,
		PasswordEncrypted: toPgText(passwordEncrypted),
		CreatedBy:         toPgUUIDPtr(createdBy),
	})
	if err != nil {
		return ExportCredential{}, err
	}
	return exportCredentialFromModel(exportsdb.RacGoogleAdsExportCredential{
		ID:                row.ID,
		OrganizationID:    row.OrganizationID,
		Username:          row.Username,
		PasswordHash:      row.PasswordHash,
		CreatedBy:         row.CreatedBy,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
		LastUsedAt:        row.LastUsedAt,
		PasswordEncrypted: row.PasswordEncrypted,
	}), nil
}

// GetCredentialByUsername retrieves Google Ads export credential by username.
func (r *Repository) GetCredentialByUsername(ctx context.Context, username string) (ExportCredential, error) {
	row, err := r.queries.GetGoogleAdsExportCredentialByUsername(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		return ExportCredential{}, ErrCredentialNotFound
	}
	if err != nil {
		return ExportCredential{}, err
	}
	return exportCredentialFromModel(exportsdb.RacGoogleAdsExportCredential{
		ID:                row.ID,
		OrganizationID:    row.OrganizationID,
		Username:          row.Username,
		PasswordHash:      row.PasswordHash,
		CreatedBy:         row.CreatedBy,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
		LastUsedAt:        row.LastUsedAt,
		PasswordEncrypted: row.PasswordEncrypted,
	}), nil
}

// GetCredentialByOrganization retrieves credential metadata for an organization.
func (r *Repository) GetCredentialByOrganization(ctx context.Context, orgID uuid.UUID) (ExportCredential, error) {
	row, err := r.queries.GetGoogleAdsExportCredentialByOrganization(ctx, toPgUUID(orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ExportCredential{}, ErrCredentialNotFound
	}
	if err != nil {
		return ExportCredential{}, err
	}
	return exportCredentialFromModel(exportsdb.RacGoogleAdsExportCredential{
		ID:                row.ID,
		OrganizationID:    row.OrganizationID,
		Username:          row.Username,
		PasswordHash:      row.PasswordHash,
		CreatedBy:         row.CreatedBy,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
		LastUsedAt:        row.LastUsedAt,
		PasswordEncrypted: row.PasswordEncrypted,
	}), nil
}

// DeleteCredential removes an organization's credential.
func (r *Repository) DeleteCredential(ctx context.Context, orgID uuid.UUID) error {
	rowsAffected, err := r.queries.DeleteGoogleAdsExportCredential(ctx, toPgUUID(orgID))
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrCredentialNotFound
	}
	return nil
}

// TouchCredential updates the last_used_at timestamp for the credential.
func (r *Repository) TouchCredential(ctx context.Context, credentialID uuid.UUID) {
	_ = r.queries.TouchGoogleAdsExportCredential(ctx, toPgUUID(credentialID))
}

// ListConversionEvents returns conversion-relevant lead service events.
func (r *Repository) ListConversionEvents(ctx context.Context, orgID uuid.UUID, from time.Time, to time.Time, limit int) ([]ConversionEvent, error) {
	rows, err := r.queries.ListGoogleAdsConversionEvents(ctx, exportsdb.ListGoogleAdsConversionEventsParams{
		OrganizationID: toPgUUID(orgID),
		OccurredAt:     pgtype.Timestamptz{Time: from, Valid: true},
		OccurredAt_2:   pgtype.Timestamptz{Time: to, Valid: true},
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, err
	}

	items := make([]ConversionEvent, 0, len(rows))
	for _, row := range rows {
		items = append(items, ConversionEvent{
			EventID:             uuid.UUID(row.ID.Bytes),
			OrganizationID:      uuid.UUID(row.OrganizationID.Bytes),
			LeadID:              uuid.UUID(row.LeadID.Bytes),
			LeadServiceID:       uuid.UUID(row.LeadServiceID.Bytes),
			EventType:           row.EventType,
			Status:              optionalString(row.Status),
			PipelineStage:       optionalString(row.PipelineStage),
			OccurredAt:          row.OccurredAt.Time,
			GCLID:               row.Gclid,
			ConsumerEmail:       optionalString(row.ConsumerEmail),
			ConsumerPhone:       row.ConsumerPhone,
			ConsumerFirstName:   row.ConsumerFirstName,
			ConsumerLastName:    row.ConsumerLastName,
			AddressStreet:       row.AddressStreet,
			AddressHouseNumber:  row.AddressHouseNumber,
			AddressCity:         row.AddressCity,
			AddressZipCode:      row.AddressZipCode,
			ProjectedValueCents: row.ProjectedValueCents,
		})
	}
	return items, nil
}
