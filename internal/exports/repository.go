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

var ErrCredentialNotFound = errors.New("credential not found")

type Repository struct {
	pool    *pgxpool.Pool
	queries *exportsdb.Queries
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: exportsdb.New(pool)}
}

type ExportCredential struct {
	ID                uuid.UUID
	OrganizationID    uuid.UUID
	Username          string
	PasswordHash      string
	PasswordEncrypted *string
	CreatedAt         time.Time
	LastUsedAt        *time.Time
}

type ConversionEvent struct {
	OccurredAt          time.Time
	EventID             uuid.UUID
	LeadID              uuid.UUID
	LeadServiceID       uuid.UUID
	GCLID               string
	ConsumerEmail       *string
	ConsumerPhone       string
	ConsumerFirstName   string
	ConsumerLastName    string
	AddressStreet       string
	AddressHouseNumber  string
	AddressCity         string
	AddressZipCode      string
	Status              *string
	ProjectedValueCents int64
}

func (r *Repository) GetCredentialByUsername(ctx context.Context, user string) (ExportCredential, error) {
	row, err := r.queries.GetGoogleAdsExportCredentialByUsername(ctx, user)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ExportCredential{}, ErrCredentialNotFound
		}
		return ExportCredential{}, err
	}
	// Fixed: Mapping manually to resolve type mismatch with OrganizationRow
	return ExportCredential{
		ID:                uuid.UUID(row.ID.Bytes),
		OrganizationID:    uuid.UUID(row.OrganizationID.Bytes),
		Username:          row.Username,
		PasswordHash:      row.PasswordHash,
		PasswordEncrypted: ptr(row.PasswordEncrypted.String, row.PasswordEncrypted.Valid),
		CreatedAt:         row.CreatedAt.Time,
		LastUsedAt:        optionalTime(row.LastUsedAt),
	}, nil
}

func (r *Repository) GetCredentialByOrganization(ctx context.Context, tid uuid.UUID) (ExportCredential, error) {
	row, err := r.queries.GetGoogleAdsExportCredentialByOrganization(ctx, pgtype.UUID{Bytes: tid, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ExportCredential{}, ErrCredentialNotFound
		}
		return ExportCredential{}, err
	}
	return ExportCredential{
		ID:                uuid.UUID(row.ID.Bytes),
		OrganizationID:    uuid.UUID(row.OrganizationID.Bytes),
		Username:          row.Username,
		PasswordHash:      row.PasswordHash,
		PasswordEncrypted: ptr(row.PasswordEncrypted.String, row.PasswordEncrypted.Valid),
		CreatedAt:         row.CreatedAt.Time,
		LastUsedAt:        optionalTime(row.LastUsedAt),
	}, nil
}

func (r *Repository) UpsertCredential(ctx context.Context, tid uuid.UUID, user, hash string, enc *string, uid *uuid.UUID) (ExportCredential, error) {
	row, err := r.queries.UpsertGoogleAdsExportCredential(ctx, exportsdb.UpsertGoogleAdsExportCredentialParams{
		OrganizationID:    pgtype.UUID{Bytes: tid, Valid: true},
		Username:          user,
		PasswordHash:      hash,
		PasswordEncrypted: pgtype.Text{String: val(enc), Valid: enc != nil},
		CreatedBy:         pgtype.UUID{Bytes: valUUID(uid), Valid: uid != nil},
	})
	if err != nil {
		return ExportCredential{}, err
	}
	return ExportCredential{
		ID:        uuid.UUID(row.ID.Bytes),
		Username:  row.Username,
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

func (r *Repository) ListConversionEvents(ctx context.Context, tid uuid.UUID, from, to time.Time, limit int) ([]ConversionEvent, error) {
	rows, err := r.queries.ListGoogleAdsConversionEvents(ctx, exportsdb.ListGoogleAdsConversionEventsParams{
		OrganizationID: pgtype.UUID{Bytes: tid, Valid: true},
		OccurredAt:     pgtype.Timestamptz{Time: from, Valid: true},
		OccurredAt_2:   pgtype.Timestamptz{Time: to, Valid: true},
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, err
	}

	res := make([]ConversionEvent, 0, len(rows))
	for _, row := range rows {
		res = append(res, ConversionEvent{
			EventID:             uuid.UUID(row.ID.Bytes),
			LeadID:              uuid.UUID(row.LeadID.Bytes),
			LeadServiceID:       uuid.UUID(row.LeadServiceID.Bytes),
			GCLID:               row.Gclid,
			OccurredAt:          row.OccurredAt.Time,
			ProjectedValueCents: row.ProjectedValueCents,
			ConsumerEmail:       ptr(row.ConsumerEmail.String, row.ConsumerEmail.Valid),
			ConsumerPhone:       row.ConsumerPhone,
			ConsumerFirstName:   row.ConsumerFirstName,
			ConsumerLastName:    row.ConsumerLastName,
			AddressStreet:       row.AddressStreet,
			AddressHouseNumber:  row.AddressHouseNumber,
			AddressCity:         row.AddressCity,
			AddressZipCode:      row.AddressZipCode,
			Status:              ptr(row.Status.String, row.Status.Valid),
		})
	}
	return res, nil
}

func (r *Repository) DeleteCredential(ctx context.Context, tid uuid.UUID) error {
	ra, err := r.queries.DeleteGoogleAdsExportCredential(ctx, pgtype.UUID{Bytes: tid, Valid: true})
	if err == nil && ra == 0 {
		return ErrCredentialNotFound
	}
	return err
}

func (r *Repository) TouchCredential(ctx context.Context, id uuid.UUID) {
	_ = r.queries.TouchGoogleAdsExportCredential(ctx, pgtype.UUID{Bytes: id, Valid: true})
}

// ─── INTERNAL HELPERS ────────────────────────────────────────────────────────

func val(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func valUUID(u *uuid.UUID) [16]byte {
	if u == nil {
		return [16]byte{}
	}
	return *u
}

func ptr(s string, v bool) *string {
	if !v {
		return nil
	}
	return &s
}

func optionalTime(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func GenerateCredential() (string, string, error) {
	u := make([]byte, 8)
	p := make([]byte, 16)
	if _, err := rand.Read(u); err != nil {
		return "", "", err
	}
	if _, err := rand.Read(p); err != nil {
		return "", "", err
	}
	return "gadsu_" + hex.EncodeToString(u), "gadsp_" + hex.EncodeToString(p), nil
}
