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
	ID              uuid.UUID
	Name            string
	Email           *string
	Phone           *string
	VatNumber       *string
	KvkNumber       *string
	AddressLine1    *string
	AddressLine2    *string
	PostalCode      *string
	City            *string
	Country         *string
	LogoFileKey     *string
	LogoFileName    *string
	LogoContentType *string
	LogoSizeBytes   *int64
	CreatedBy       uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type OrganizationLogo struct {
	FileKey     string
	FileName    string
	ContentType string
	SizeBytes   int64
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

type OrganizationSettings struct {
	OrganizationID              uuid.UUID
	QuotePaymentDays            int
	QuoteValidDays              int
	AIAutoDisqualifyJunk        bool
	AIAutoDispatch              bool
	AIAutoEstimate              bool
	CatalogGapThreshold         int
	CatalogGapLookbackDays      int
	NotificationEmail           *string
	WhatsAppDeviceID            *string
	WhatsAppWelcomeDelayMinutes int
	SMTPHost                    *string
	SMTPPort                    *int
	SMTPUsername                *string
	SMTPPassword                *string // AES-256-GCM encrypted
	SMTPFromEmail               *string
	SMTPFromName                *string
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
}

type OrganizationSettingsUpdate struct {
	QuotePaymentDays            *int
	QuoteValidDays              *int
	AIAutoDisqualifyJunk        *bool
	AIAutoDispatch              *bool
	AIAutoEstimate              *bool
	CatalogGapThreshold         *int
	CatalogGapLookbackDays      *int
	NotificationEmail           *string
	WhatsAppDeviceID            *string
	WhatsAppWelcomeDelayMinutes *int
}

// OrganizationSMTPUpdate holds encrypted SMTP configuration fields.
type OrganizationSMTPUpdate struct {
	SMTPHost      string
	SMTPPort      int
	SMTPUsername  string
	SMTPPassword  string // already encrypted
	SMTPFromEmail string
	SMTPFromName  string
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
      logo_file_key, logo_file_name, logo_content_type, logo_size_bytes,
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
		&org.LogoFileKey,
		&org.LogoFileName,
		&org.LogoContentType,
		&org.LogoSizeBytes,
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
      logo_file_key, logo_file_name, logo_content_type, logo_size_bytes,
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
		&org.LogoFileKey,
		&org.LogoFileName,
		&org.LogoContentType,
		&org.LogoSizeBytes,
		&org.CreatedBy,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	return org, err
}

func (r *Repository) UpdateOrganizationLogo(
	ctx context.Context,
	organizationID uuid.UUID,
	logo OrganizationLogo,
) (Organization, error) {
	var org Organization
	err := r.pool.QueryRow(ctx, `
    UPDATE RAC_organizations
    SET
      logo_file_key = $2,
      logo_file_name = $3,
      logo_content_type = $4,
      logo_size_bytes = $5,
      updated_at = now()
    WHERE id = $1
    RETURNING id, name, email, phone, vat_number, kvk_number, address_line1, address_line2, postal_code, city, country,
      logo_file_key, logo_file_name, logo_content_type, logo_size_bytes,
      created_by, created_at, updated_at
	`, organizationID, logo.FileKey, logo.FileName, logo.ContentType, logo.SizeBytes).Scan(
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
		&org.LogoFileKey,
		&org.LogoFileName,
		&org.LogoContentType,
		&org.LogoSizeBytes,
		&org.CreatedBy,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	return org, err
}

func (r *Repository) ClearOrganizationLogo(
	ctx context.Context,
	organizationID uuid.UUID,
) (Organization, error) {
	var org Organization
	err := r.pool.QueryRow(ctx, `
    UPDATE RAC_organizations
    SET
      logo_file_key = NULL,
      logo_file_name = NULL,
      logo_content_type = NULL,
      logo_size_bytes = NULL,
      updated_at = now()
    WHERE id = $1
    RETURNING id, name, email, phone, vat_number, kvk_number, address_line1, address_line2, postal_code, city, country,
      logo_file_key, logo_file_name, logo_content_type, logo_size_bytes,
      created_by, created_at, updated_at
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
		&org.LogoFileKey,
		&org.LogoFileName,
		&org.LogoContentType,
		&org.LogoSizeBytes,
		&org.CreatedBy,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	return org, err
}

func (r *Repository) GetOrganizationSettings(ctx context.Context, organizationID uuid.UUID) (OrganizationSettings, error) {
	var s OrganizationSettings
	err := r.pool.QueryRow(ctx, `
	SELECT organization_id, quote_payment_days, quote_valid_days,
	       ai_auto_disqualify_junk, ai_auto_dispatch, ai_auto_estimate,
	       catalog_gap_threshold, catalog_gap_lookback_days,
	       notification_email, whatsapp_device_id, whatsapp_welcome_delay_minutes,
           smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_email, smtp_from_name,
           created_at, updated_at
    FROM RAC_organization_settings
    WHERE organization_id = $1
  `, organizationID).Scan(
		&s.OrganizationID,
		&s.QuotePaymentDays,
		&s.QuoteValidDays,
		&s.AIAutoDisqualifyJunk,
		&s.AIAutoDispatch,
		&s.AIAutoEstimate,
		&s.CatalogGapThreshold,
		&s.CatalogGapLookbackDays,
		&s.NotificationEmail,
		&s.WhatsAppDeviceID,
		&s.WhatsAppWelcomeDelayMinutes,
		&s.SMTPHost,
		&s.SMTPPort,
		&s.SMTPUsername,
		&s.SMTPPassword,
		&s.SMTPFromEmail,
		&s.SMTPFromName,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		// Return defaults if no row exists yet
		return OrganizationSettings{
			OrganizationID:              organizationID,
			QuotePaymentDays:            7,
			QuoteValidDays:              14,
			AIAutoDisqualifyJunk:        true,
			AIAutoDispatch:              false,
			AIAutoEstimate:              true,
			CatalogGapThreshold:         3,
			CatalogGapLookbackDays:      30,
			WhatsAppDeviceID:            nil,
			WhatsAppWelcomeDelayMinutes: 2,
		}, nil
	}
	return s, err
}

func (r *Repository) UpsertOrganizationSettings(
	ctx context.Context,
	organizationID uuid.UUID,
	update OrganizationSettingsUpdate,
) (OrganizationSettings, error) {
	var s OrganizationSettings
	err := r.pool.QueryRow(ctx, `
		INSERT INTO RAC_organization_settings (
			organization_id,
			quote_payment_days,
			quote_valid_days,
			ai_auto_disqualify_junk,
			ai_auto_dispatch,
			ai_auto_estimate,
			catalog_gap_threshold,
			catalog_gap_lookback_days,
			notification_email,
			whatsapp_device_id,
			whatsapp_welcome_delay_minutes
		)
		VALUES (
			$1,
			COALESCE($2, 7),
			COALESCE($3, 14),
			COALESCE($4, true),
			COALESCE($5, false),
			COALESCE($6, true),
			COALESCE($7, 3),
			COALESCE($8, 30),
			NULLIF($9, ''),
			NULLIF($10, ''),
			COALESCE($11, 2)
		)
		ON CONFLICT (organization_id) DO UPDATE SET
			quote_payment_days = COALESCE($2, RAC_organization_settings.quote_payment_days),
			quote_valid_days   = COALESCE($3, RAC_organization_settings.quote_valid_days),
			ai_auto_disqualify_junk = COALESCE($4, RAC_organization_settings.ai_auto_disqualify_junk),
			ai_auto_dispatch        = COALESCE($5, RAC_organization_settings.ai_auto_dispatch),
			ai_auto_estimate        = COALESCE($6, RAC_organization_settings.ai_auto_estimate),
			catalog_gap_threshold   = COALESCE($7, RAC_organization_settings.catalog_gap_threshold),
			catalog_gap_lookback_days = COALESCE($8, RAC_organization_settings.catalog_gap_lookback_days),
			notification_email = CASE WHEN $9 IS NULL THEN RAC_organization_settings.notification_email ELSE NULLIF($9, '') END,
			whatsapp_device_id = CASE WHEN $10 IS NULL THEN RAC_organization_settings.whatsapp_device_id ELSE NULLIF($10, '') END,
			whatsapp_welcome_delay_minutes = COALESCE($11, RAC_organization_settings.whatsapp_welcome_delay_minutes),
			updated_at         = now()
		RETURNING organization_id, quote_payment_days, quote_valid_days,
		          ai_auto_disqualify_junk, ai_auto_dispatch, ai_auto_estimate,
		          catalog_gap_threshold, catalog_gap_lookback_days,
		          notification_email, whatsapp_device_id, whatsapp_welcome_delay_minutes,
		          smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_email, smtp_from_name,
		          created_at, updated_at
	`, organizationID,
		update.QuotePaymentDays,
		update.QuoteValidDays,
		update.AIAutoDisqualifyJunk,
		update.AIAutoDispatch,
		update.AIAutoEstimate,
		update.CatalogGapThreshold,
		update.CatalogGapLookbackDays,
		update.NotificationEmail,
		update.WhatsAppDeviceID,
		update.WhatsAppWelcomeDelayMinutes,
	).Scan(
		&s.OrganizationID,
		&s.QuotePaymentDays,
		&s.QuoteValidDays,
		&s.AIAutoDisqualifyJunk,
		&s.AIAutoDispatch,
		&s.AIAutoEstimate,
		&s.CatalogGapThreshold,
		&s.CatalogGapLookbackDays,
		&s.NotificationEmail,
		&s.WhatsAppDeviceID,
		&s.WhatsAppWelcomeDelayMinutes,
		&s.SMTPHost,
		&s.SMTPPort,
		&s.SMTPUsername,
		&s.SMTPPassword,
		&s.SMTPFromEmail,
		&s.SMTPFromName,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrganizationSettings{}, ErrNotFound
	}
	return s, err
}

// UpsertOrganizationSMTP stores encrypted SMTP settings for an organization.
func (r *Repository) UpsertOrganizationSMTP(
	ctx context.Context,
	organizationID uuid.UUID,
	update OrganizationSMTPUpdate,
) (OrganizationSettings, error) {
	var s OrganizationSettings
	err := r.pool.QueryRow(ctx, `
		INSERT INTO RAC_organization_settings (organization_id, smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_email, smtp_from_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (organization_id) DO UPDATE SET
			smtp_host       = $2,
			smtp_port       = $3,
			smtp_username   = $4,
			smtp_password   = $5,
			smtp_from_email = $6,
			smtp_from_name  = $7,
			updated_at      = now()
		RETURNING organization_id, quote_payment_days, quote_valid_days,
		          ai_auto_disqualify_junk, ai_auto_dispatch, ai_auto_estimate,
		          catalog_gap_threshold, catalog_gap_lookback_days,
		          notification_email, whatsapp_device_id, whatsapp_welcome_delay_minutes,
		          smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_email, smtp_from_name,
		          created_at, updated_at
	`, organizationID, update.SMTPHost, update.SMTPPort, update.SMTPUsername, update.SMTPPassword, update.SMTPFromEmail, update.SMTPFromName).Scan(
		&s.OrganizationID,
		&s.QuotePaymentDays,
		&s.QuoteValidDays,
		&s.AIAutoDisqualifyJunk,
		&s.AIAutoDispatch,
		&s.AIAutoEstimate,
		&s.CatalogGapThreshold,
		&s.CatalogGapLookbackDays,
		&s.NotificationEmail,
		&s.WhatsAppDeviceID,
		&s.WhatsAppWelcomeDelayMinutes,
		&s.SMTPHost,
		&s.SMTPPort,
		&s.SMTPUsername,
		&s.SMTPPassword,
		&s.SMTPFromEmail,
		&s.SMTPFromName,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrganizationSettings{}, ErrNotFound
	}
	return s, err
}

// ClearOrganizationSMTP removes SMTP configuration for an organization.
func (r *Repository) ClearOrganizationSMTP(ctx context.Context, organizationID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE RAC_organization_settings
		SET smtp_host = NULL, smtp_port = NULL, smtp_username = NULL, smtp_password = NULL,
		    smtp_from_email = NULL, smtp_from_name = NULL, updated_at = now()
		WHERE organization_id = $1
	`, organizationID)
	return err
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
