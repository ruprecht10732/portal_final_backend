package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) getDB(q DBTX) DBTX {
	if q != nil {
		return q
	}
	return r.pool
}

type Organization struct {
	ID           uuid.UUID
	Name         string
	Email        *string
	Phone        *string
	VatNumber    *string
	KvkNumber    *string
	AddressLine1 *string
	AddressLine2 *string
	PostalCode   *string
	City         *string
	Country      *string
	CreatedBy    uuid.UUID
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type OrganizationProfileUpdate struct {
	Name         *string
	Email        *string
	Phone        *string
	VatNumber    *string
	KvkNumber    *string
	AddressLine1 *string
	AddressLine2 *string
	PostalCode   *string
	City         *string
	Country      *string
}

type Invite struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Email          string
	TokenHash      string
	ExpiresAt      time.Time
	CreatedBy      uuid.UUID
	CreatedAt      time.Time
	UsedAt         *time.Time
	UsedBy         *uuid.UUID
}

func (r *Repository) CreateOrganization(ctx context.Context, q DBTX, name string, createdBy uuid.UUID) (Organization, error) {
	var org Organization
	err := r.getDB(q).QueryRow(ctx, `
    INSERT INTO RAC_organizations (name, created_by)
    VALUES ($1, $2)
    RETURNING id, name, created_by, created_at, updated_at
  `, name, createdBy).Scan(&org.ID, &org.Name, &org.CreatedBy, &org.CreatedAt, &org.UpdatedAt)
	return org, err
}

func (r *Repository) GetOrganization(ctx context.Context, organizationID uuid.UUID) (Organization, error) {
	var org Organization
	err := r.pool.QueryRow(ctx, `
    SELECT id, name, email, phone, vat_number, kvk_number, address_line1, address_line2, postal_code, city, country,
      created_by, created_at, updated_at
    FROM RAC_organizations
    WHERE id = $1
  `, organizationID).Scan(
		&org.ID,
		&org.Name,
		&org.Email,
		&org.Phone,
		&org.VatNumber,
		&org.KvkNumber,
		&org.AddressLine1,
		&org.AddressLine2,
		&org.PostalCode,
		&org.City,
		&org.Country,
		&org.CreatedBy,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	return org, err
}

func (r *Repository) UpdateOrganizationProfile(
	ctx context.Context,
	organizationID uuid.UUID,
	update OrganizationProfileUpdate,
) (Organization, error) {
	var org Organization
	err := r.pool.QueryRow(ctx, `
    UPDATE RAC_organizations
    SET
      name = COALESCE($2, name),
      email = COALESCE($3, email),
      phone = COALESCE($4, phone),
      vat_number = COALESCE($5, vat_number),
      kvk_number = COALESCE($6, kvk_number),
      address_line1 = COALESCE($7, address_line1),
      address_line2 = COALESCE($8, address_line2),
      postal_code = COALESCE($9, postal_code),
      city = COALESCE($10, city),
      country = COALESCE($11, country),
      updated_at = now()
    WHERE id = $1
    RETURNING id, name, email, phone, vat_number, kvk_number, address_line1, address_line2, postal_code, city, country,
      created_by, created_at, updated_at
	`, organizationID, update.Name, update.Email, update.Phone, update.VatNumber, update.KvkNumber, update.AddressLine1, update.AddressLine2, update.PostalCode, update.City, update.Country).Scan(
		&org.ID,
		&org.Name,
		&org.Email,
		&org.Phone,
		&org.VatNumber,
		&org.KvkNumber,
		&org.AddressLine1,
		&org.AddressLine2,
		&org.PostalCode,
		&org.City,
		&org.Country,
		&org.CreatedBy,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	return org, err
}

func (r *Repository) AddMember(ctx context.Context, q DBTX, organizationID, userID uuid.UUID) error {
	_, err := r.getDB(q).Exec(ctx, `
    INSERT INTO RAC_organization_members (organization_id, user_id)
    VALUES ($1, $2)
  `, organizationID, userID)
	return err
}

func (r *Repository) GetUserOrganizationID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	var orgID uuid.UUID
	err := r.pool.QueryRow(ctx, `
    SELECT organization_id
    FROM RAC_organization_members
    WHERE user_id = $1
  `, userID).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, ErrNotFound
	}
	return orgID, err
}

func (r *Repository) CreateInvite(ctx context.Context, organizationID uuid.UUID, email, tokenHash string, expiresAt time.Time, createdBy uuid.UUID) (Invite, error) {
	var invite Invite
	err := r.pool.QueryRow(ctx, `
    INSERT INTO RAC_organization_invites (organization_id, email, token_hash, expires_at, created_by)
    VALUES ($1, $2, $3, $4, $5)
    RETURNING id, organization_id, email, token_hash, expires_at, created_by, created_at, used_at, used_by
  `, organizationID, email, tokenHash, expiresAt, createdBy).Scan(
		&invite.ID,
		&invite.OrganizationID,
		&invite.Email,
		&invite.TokenHash,
		&invite.ExpiresAt,
		&invite.CreatedBy,
		&invite.CreatedAt,
		&invite.UsedAt,
		&invite.UsedBy,
	)
	return invite, err
}

func (r *Repository) GetInviteByToken(ctx context.Context, tokenHash string) (Invite, error) {
	var invite Invite
	err := r.pool.QueryRow(ctx, `
    SELECT id, organization_id, email, token_hash, expires_at, created_by, created_at, used_at, used_by
    FROM RAC_organization_invites
    WHERE token_hash = $1
  `, tokenHash).Scan(
		&invite.ID,
		&invite.OrganizationID,
		&invite.Email,
		&invite.TokenHash,
		&invite.ExpiresAt,
		&invite.CreatedBy,
		&invite.CreatedAt,
		&invite.UsedAt,
		&invite.UsedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	return invite, err
}

func (r *Repository) UseInvite(ctx context.Context, q DBTX, inviteID, usedBy uuid.UUID) error {
	_, err := r.getDB(q).Exec(ctx, `
    UPDATE RAC_organization_invites
    SET used_at = now(), used_by = $2
    WHERE id = $1 AND used_at IS NULL
  `, inviteID, usedBy)
	return err
}

func (r *Repository) ListInvites(ctx context.Context, organizationID uuid.UUID) ([]Invite, error) {
	rows, err := r.pool.Query(ctx, `
    SELECT id, organization_id, email, token_hash, expires_at, created_by, created_at, used_at, used_by
    FROM RAC_organization_invites
    WHERE organization_id = $1
    ORDER BY created_at DESC
  `, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []Invite
	for rows.Next() {
		var invite Invite
		if err := rows.Scan(
			&invite.ID,
			&invite.OrganizationID,
			&invite.Email,
			&invite.TokenHash,
			&invite.ExpiresAt,
			&invite.CreatedBy,
			&invite.CreatedAt,
			&invite.UsedAt,
			&invite.UsedBy,
		); err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return invites, nil
}

func (r *Repository) UpdateInvite(
	ctx context.Context,
	organizationID uuid.UUID,
	inviteID uuid.UUID,
	email *string,
	tokenHash *string,
	expiresAt *time.Time,
) (Invite, error) {
	var invite Invite
	err := r.pool.QueryRow(ctx, `
    UPDATE RAC_organization_invites
    SET
      email = COALESCE($3, email),
      token_hash = COALESCE($4, token_hash),
      expires_at = COALESCE($5, expires_at)
    WHERE id = $1 AND organization_id = $2 AND used_at IS NULL
    RETURNING id, organization_id, email, token_hash, expires_at, created_by, created_at, used_at, used_by
  `, inviteID, organizationID, email, tokenHash, expiresAt).Scan(
		&invite.ID,
		&invite.OrganizationID,
		&invite.Email,
		&invite.TokenHash,
		&invite.ExpiresAt,
		&invite.CreatedBy,
		&invite.CreatedAt,
		&invite.UsedAt,
		&invite.UsedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	return invite, err
}

func (r *Repository) RevokeInvite(ctx context.Context, organizationID, inviteID uuid.UUID) (Invite, error) {
	var invite Invite
	err := r.pool.QueryRow(ctx, `
    UPDATE RAC_organization_invites
    SET expires_at = now()
    WHERE id = $1 AND organization_id = $2 AND used_at IS NULL
    RETURNING id, organization_id, email, token_hash, expires_at, created_by, created_at, used_at, used_by
  `, inviteID, organizationID).Scan(
		&invite.ID,
		&invite.OrganizationID,
		&invite.Email,
		&invite.TokenHash,
		&invite.ExpiresAt,
		&invite.CreatedBy,
		&invite.CreatedAt,
		&invite.UsedAt,
		&invite.UsedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	return invite, err
}
