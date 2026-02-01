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

type Organization struct {
	ID        uuid.UUID
	Name      string
	CreatedBy uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
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
	err := q.QueryRow(ctx, `
    INSERT INTO organizations (name, created_by)
    VALUES ($1, $2)
    RETURNING id, name, created_by, created_at, updated_at
  `, name, createdBy).Scan(&org.ID, &org.Name, &org.CreatedBy, &org.CreatedAt, &org.UpdatedAt)
	return org, err
}

func (r *Repository) GetOrganization(ctx context.Context, organizationID uuid.UUID) (Organization, error) {
	var org Organization
	err := r.pool.QueryRow(ctx, `
    SELECT id, name, created_by, created_at, updated_at
    FROM organizations
    WHERE id = $1
  `, organizationID).Scan(&org.ID, &org.Name, &org.CreatedBy, &org.CreatedAt, &org.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	return org, err
}

func (r *Repository) UpdateOrganizationName(ctx context.Context, organizationID uuid.UUID, name string) (Organization, error) {
	var org Organization
	err := r.pool.QueryRow(ctx, `
    UPDATE organizations
    SET name = $2, updated_at = now()
    WHERE id = $1
    RETURNING id, name, created_by, created_at, updated_at
  `, organizationID, name).Scan(&org.ID, &org.Name, &org.CreatedBy, &org.CreatedAt, &org.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	return org, err
}

func (r *Repository) AddMember(ctx context.Context, q DBTX, organizationID, userID uuid.UUID) error {
	_, err := q.Exec(ctx, `
    INSERT INTO organization_members (organization_id, user_id)
    VALUES ($1, $2)
  `, organizationID, userID)
	return err
}

func (r *Repository) GetUserOrganizationID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	var orgID uuid.UUID
	err := r.pool.QueryRow(ctx, `
    SELECT organization_id
    FROM organization_members
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
    INSERT INTO organization_invites (organization_id, email, token_hash, expires_at, created_by)
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
    FROM organization_invites
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
	_, err := q.Exec(ctx, `
    UPDATE organization_invites
    SET used_at = now(), used_by = $2
    WHERE id = $1 AND used_at IS NULL
  `, inviteID, usedBy)
	return err
}
