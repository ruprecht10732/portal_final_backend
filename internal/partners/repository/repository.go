package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const partnerNotFoundMsg = "partner not found"
const partnerInviteNotFoundMsg = "partner invite not found"

// Repository provides database operations for partners.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a new partners repository.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

type Partner struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	BusinessName    string
	KVKNumber       string
	VATNumber       string
	AddressLine1    string
	AddressLine2    *string
	PostalCode      string
	City            string
	Country         string
	ContactName     string
	ContactEmail    string
	ContactPhone    string
	LogoFileKey     *string
	LogoFileName    *string
	LogoContentType *string
	LogoSizeBytes   *int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type PartnerUpdate struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	BusinessName   *string
	KVKNumber      *string
	VATNumber      *string
	AddressLine1   *string
	AddressLine2   *string
	PostalCode     *string
	City           *string
	Country        *string
	ContactName    *string
	ContactEmail   *string
	ContactPhone   *string
}

type PartnerLogo struct {
	FileKey     string
	FileName    string
	ContentType string
	SizeBytes   int64
}

type ListParams struct {
	OrganizationID uuid.UUID
	Search         string
	SortBy         string
	SortOrder      string
	Page           int
	PageSize       int
}

type ListResult struct {
	Items      []Partner
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}

type PartnerLead struct {
	ID          uuid.UUID
	FirstName   string
	LastName    string
	Phone       string
	Street      string
	HouseNumber string
	City        string
}

type PartnerInvite struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	PartnerID      uuid.UUID
	Email          string
	TokenHash      string
	ExpiresAt      time.Time
	CreatedBy      uuid.UUID
	CreatedAt      time.Time
	UsedAt         *time.Time
	UsedBy         *uuid.UUID
	LeadID         *uuid.UUID
	LeadServiceID  *uuid.UUID
}

func (r *Repository) Create(ctx context.Context, partner Partner) (Partner, error) {
	query := `
		INSERT INTO RAC_partners (
			id, organization_id, business_name, kvk_number, vat_number,
			address_line1, address_line2, postal_code, city, country,
			contact_name, contact_email, contact_phone, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15
		)
	`

	_, err := r.pool.Exec(ctx, query,
		partner.ID,
		partner.OrganizationID,
		partner.BusinessName,
		partner.KVKNumber,
		partner.VATNumber,
		partner.AddressLine1,
		partner.AddressLine2,
		partner.PostalCode,
		partner.City,
		partner.Country,
		partner.ContactName,
		partner.ContactEmail,
		partner.ContactPhone,
		partner.CreatedAt,
		partner.UpdatedAt,
	)
	if err != nil {
		return Partner{}, fmt.Errorf("create partner: %w", err)
	}

	return partner, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (Partner, error) {
	query := `
		SELECT id, organization_id, business_name, kvk_number, vat_number,
			address_line1, address_line2, postal_code, city, country,
			contact_name, contact_email, contact_phone,
			logo_file_key, logo_file_name, logo_content_type, logo_size_bytes,
			created_at, updated_at
		FROM RAC_partners
		WHERE id = $1 AND organization_id = $2
	`

	var partner Partner
	err := r.pool.QueryRow(ctx, query, id, organizationID).Scan(
		&partner.ID,
		&partner.OrganizationID,
		&partner.BusinessName,
		&partner.KVKNumber,
		&partner.VATNumber,
		&partner.AddressLine1,
		&partner.AddressLine2,
		&partner.PostalCode,
		&partner.City,
		&partner.Country,
		&partner.ContactName,
		&partner.ContactEmail,
		&partner.ContactPhone,
		&partner.LogoFileKey,
		&partner.LogoFileName,
		&partner.LogoContentType,
		&partner.LogoSizeBytes,
		&partner.CreatedAt,
		&partner.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Partner{}, apperr.NotFound(partnerNotFoundMsg)
		}
		return Partner{}, fmt.Errorf("get partner: %w", err)
	}

	return partner, nil
}

func (r *Repository) Update(ctx context.Context, update PartnerUpdate) (Partner, error) {
	query := `
		UPDATE RAC_partners
		SET
			business_name = COALESCE($3, business_name),
			kvk_number = COALESCE($4, kvk_number),
			vat_number = COALESCE($5, vat_number),
			address_line1 = COALESCE($6, address_line1),
			address_line2 = COALESCE($7, address_line2),
			postal_code = COALESCE($8, postal_code),
			city = COALESCE($9, city),
			country = COALESCE($10, country),
			contact_name = COALESCE($11, contact_name),
			contact_email = COALESCE($12, contact_email),
			contact_phone = COALESCE($13, contact_phone),
			updated_at = now()
		WHERE id = $1 AND organization_id = $2
		RETURNING id, organization_id, business_name, kvk_number, vat_number,
			address_line1, address_line2, postal_code, city, country,
			contact_name, contact_email, contact_phone,
			logo_file_key, logo_file_name, logo_content_type, logo_size_bytes,
			created_at, updated_at
	`

	var partner Partner
	err := r.pool.QueryRow(ctx, query,
		update.ID,
		update.OrganizationID,
		update.BusinessName,
		update.KVKNumber,
		update.VATNumber,
		update.AddressLine1,
		update.AddressLine2,
		update.PostalCode,
		update.City,
		update.Country,
		update.ContactName,
		update.ContactEmail,
		update.ContactPhone,
	).Scan(
		&partner.ID,
		&partner.OrganizationID,
		&partner.BusinessName,
		&partner.KVKNumber,
		&partner.VATNumber,
		&partner.AddressLine1,
		&partner.AddressLine2,
		&partner.PostalCode,
		&partner.City,
		&partner.Country,
		&partner.ContactName,
		&partner.ContactEmail,
		&partner.ContactPhone,
		&partner.LogoFileKey,
		&partner.LogoFileName,
		&partner.LogoContentType,
		&partner.LogoSizeBytes,
		&partner.CreatedAt,
		&partner.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Partner{}, apperr.NotFound(partnerNotFoundMsg)
		}
		return Partner{}, fmt.Errorf("update partner: %w", err)
	}

	return partner, nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error {
	query := `DELETE FROM RAC_partners WHERE id = $1 AND organization_id = $2`

	result, err := r.pool.Exec(ctx, query, id, organizationID)
	if err != nil {
		return fmt.Errorf("delete partner: %w", err)
	}
	if result.RowsAffected() == 0 {
		return apperr.NotFound(partnerNotFoundMsg)
	}
	return nil
}

func (r *Repository) List(ctx context.Context, params ListParams) (ListResult, error) {
	searchParam := optionalSearch(params.Search)

	sortBy, err := resolveSortBy(params.SortBy)
	if err != nil {
		return ListResult{}, err
	}
	orderBy, err := resolveSortOrder(params.SortOrder)
	if err != nil {
		return ListResult{}, err
	}

	baseQuery := `
		FROM RAC_partners
		WHERE organization_id = $1
			AND ($2::text IS NULL OR business_name ILIKE $2 OR contact_name ILIKE $2 OR contact_email ILIKE $2 OR kvk_number ILIKE $2 OR vat_number ILIKE $2)
	`
	args := []interface{}{params.OrganizationID, searchParam}

	var total int
	countQuery := "SELECT COUNT(*) " + baseQuery
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return ListResult{}, fmt.Errorf("count partners: %w", err)
	}

	page := params.Page
	pageSize := params.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	pageTotal := 0
	if pageSize > 0 {
		pageTotal = (total + pageSize - 1) / pageSize
	}

	selectQuery := `
		SELECT id, organization_id, business_name, kvk_number, vat_number,
			address_line1, address_line2, postal_code, city, country,
			contact_name, contact_email, contact_phone,
			logo_file_key, logo_file_name, logo_content_type, logo_size_bytes,
			created_at, updated_at
		` + baseQuery + `
		ORDER BY
			CASE WHEN $3 = 'businessName' AND $4 = 'asc' THEN business_name END ASC,
			CASE WHEN $3 = 'businessName' AND $4 = 'desc' THEN business_name END DESC,
			CASE WHEN $3 = 'createdAt' AND $4 = 'asc' THEN created_at END ASC,
			CASE WHEN $3 = 'createdAt' AND $4 = 'desc' THEN created_at END DESC,
			CASE WHEN $3 = 'updatedAt' AND $4 = 'asc' THEN updated_at END ASC,
			CASE WHEN $3 = 'updatedAt' AND $4 = 'desc' THEN updated_at END DESC,
			business_name ASC
		LIMIT $5 OFFSET $6
	`

	args = append(args, sortBy, orderBy, pageSize, offset)
	rows, err := r.pool.Query(ctx, selectQuery, args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list partners: %w", err)
	}
	defer rows.Close()

	items := make([]Partner, 0)
	for rows.Next() {
		var partner Partner
		if err := rows.Scan(
			&partner.ID,
			&partner.OrganizationID,
			&partner.BusinessName,
			&partner.KVKNumber,
			&partner.VATNumber,
			&partner.AddressLine1,
			&partner.AddressLine2,
			&partner.PostalCode,
			&partner.City,
			&partner.Country,
			&partner.ContactName,
			&partner.ContactEmail,
			&partner.ContactPhone,
			&partner.LogoFileKey,
			&partner.LogoFileName,
			&partner.LogoContentType,
			&partner.LogoSizeBytes,
			&partner.CreatedAt,
			&partner.UpdatedAt,
		); err != nil {
			return ListResult{}, fmt.Errorf("scan partner: %w", err)
		}
		items = append(items, partner)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterate partners: %w", err)
	}

	return ListResult{Items: items, Total: total, Page: page, PageSize: pageSize, TotalPages: pageTotal}, nil
}

func (r *Repository) Exists(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM RAC_partners WHERE id = $1 AND organization_id = $2)`
	if err := r.pool.QueryRow(ctx, query, id, organizationID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check partner exists: %w", err)
	}
	return exists, nil
}

func (r *Repository) LeadExists(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM RAC_leads WHERE id = $1 AND organization_id = $2)`
	if err := r.pool.QueryRow(ctx, query, id, organizationID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check lead exists: %w", err)
	}
	return exists, nil
}

func (r *Repository) LeadServiceExists(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM RAC_lead_services WHERE id = $1 AND organization_id = $2)`
	if err := r.pool.QueryRow(ctx, query, id, organizationID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check lead service exists: %w", err)
	}
	return exists, nil
}

func (r *Repository) LinkLead(ctx context.Context, organizationID, partnerID, leadID uuid.UUID) error {
	query := `
		INSERT INTO RAC_partner_leads (organization_id, partner_id, lead_id)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`
	result, err := r.pool.Exec(ctx, query, organizationID, partnerID, leadID)
	if err != nil {
		return fmt.Errorf("link partner lead: %w", err)
	}
	if result.RowsAffected() == 0 {
		return apperr.Conflict("lead already linked to partner")
	}
	return nil
}

func (r *Repository) UnlinkLead(ctx context.Context, organizationID, partnerID, leadID uuid.UUID) error {
	query := `DELETE FROM RAC_partner_leads WHERE organization_id = $1 AND partner_id = $2 AND lead_id = $3`
	result, err := r.pool.Exec(ctx, query, organizationID, partnerID, leadID)
	if err != nil {
		return fmt.Errorf("unlink partner lead: %w", err)
	}
	if result.RowsAffected() == 0 {
		return apperr.NotFound("link not found")
	}
	return nil
}

func (r *Repository) ListLeads(ctx context.Context, organizationID, partnerID uuid.UUID) ([]PartnerLead, error) {
	query := `
		SELECT l.id, l.consumer_first_name, l.consumer_last_name, l.consumer_phone,
			l.address_street, l.address_house_number, l.address_city
		FROM RAC_partner_leads pl
		JOIN RAC_leads l ON l.id = pl.lead_id
		WHERE pl.organization_id = $1 AND pl.partner_id = $2
		ORDER BY l.created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, organizationID, partnerID)
	if err != nil {
		return nil, fmt.Errorf("list partner leads: %w", err)
	}
	defer rows.Close()

	leads := make([]PartnerLead, 0)
	for rows.Next() {
		var lead PartnerLead
		if err := rows.Scan(
			&lead.ID,
			&lead.FirstName,
			&lead.LastName,
			&lead.Phone,
			&lead.Street,
			&lead.HouseNumber,
			&lead.City,
		); err != nil {
			return nil, fmt.Errorf("scan partner lead: %w", err)
		}
		leads = append(leads, lead)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate partner leads: %w", err)
	}

	return leads, nil
}

func (r *Repository) CreateInvite(ctx context.Context, invite PartnerInvite) (PartnerInvite, error) {
	query := `
		INSERT INTO RAC_partner_invites (
			id, organization_id, partner_id, email, token_hash, expires_at, created_by,
			created_at, used_at, used_by, lead_id, lead_service_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12
		)
	`

	_, err := r.pool.Exec(ctx, query,
		invite.ID,
		invite.OrganizationID,
		invite.PartnerID,
		invite.Email,
		invite.TokenHash,
		invite.ExpiresAt,
		invite.CreatedBy,
		invite.CreatedAt,
		invite.UsedAt,
		invite.UsedBy,
		invite.LeadID,
		invite.LeadServiceID,
	)
	if err != nil {
		return PartnerInvite{}, fmt.Errorf("create partner invite: %w", err)
	}

	return invite, nil
}

func (r *Repository) ListInvites(ctx context.Context, organizationID, partnerID uuid.UUID) ([]PartnerInvite, error) {
	query := `
		SELECT id, organization_id, partner_id, email, token_hash, expires_at, created_by,
			created_at, used_at, used_by, lead_id, lead_service_id
		FROM RAC_partner_invites
		WHERE organization_id = $1 AND partner_id = $2
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, organizationID, partnerID)
	if err != nil {
		return nil, fmt.Errorf("list partner invites: %w", err)
	}
	defer rows.Close()

	invites := make([]PartnerInvite, 0)
	for rows.Next() {
		var invite PartnerInvite
		if err := rows.Scan(
			&invite.ID,
			&invite.OrganizationID,
			&invite.PartnerID,
			&invite.Email,
			&invite.TokenHash,
			&invite.ExpiresAt,
			&invite.CreatedBy,
			&invite.CreatedAt,
			&invite.UsedAt,
			&invite.UsedBy,
			&invite.LeadID,
			&invite.LeadServiceID,
		); err != nil {
			return nil, fmt.Errorf("scan partner invite: %w", err)
		}
		invites = append(invites, invite)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate partner invites: %w", err)
	}

	return invites, nil
}

func (r *Repository) RevokeInvite(ctx context.Context, organizationID, inviteID uuid.UUID) (PartnerInvite, error) {
	query := `
		UPDATE RAC_partner_invites
		SET expires_at = now()
		WHERE id = $1 AND organization_id = $2 AND used_at IS NULL
		RETURNING id, organization_id, partner_id, email, token_hash, expires_at, created_by,
			created_at, used_at, used_by, lead_id, lead_service_id
	`

	var invite PartnerInvite
	err := r.pool.QueryRow(ctx, query, inviteID, organizationID).Scan(
		&invite.ID,
		&invite.OrganizationID,
		&invite.PartnerID,
		&invite.Email,
		&invite.TokenHash,
		&invite.ExpiresAt,
		&invite.CreatedBy,
		&invite.CreatedAt,
		&invite.UsedAt,
		&invite.UsedBy,
		&invite.LeadID,
		&invite.LeadServiceID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PartnerInvite{}, apperr.NotFound(partnerInviteNotFoundMsg)
		}
		return PartnerInvite{}, fmt.Errorf("revoke partner invite: %w", err)
	}

	return invite, nil
}

func (r *Repository) UpdateLogo(ctx context.Context, organizationID, partnerID uuid.UUID, logo PartnerLogo) (Partner, error) {
	query := `
		UPDATE RAC_partners
		SET logo_file_key = $3,
			logo_file_name = $4,
			logo_content_type = $5,
			logo_size_bytes = $6,
			updated_at = now()
		WHERE id = $1 AND organization_id = $2
		RETURNING id, organization_id, business_name, kvk_number, vat_number,
			address_line1, address_line2, postal_code, city, country,
			contact_name, contact_email, contact_phone,
			logo_file_key, logo_file_name, logo_content_type, logo_size_bytes,
			created_at, updated_at
	`

	var partner Partner
	err := r.pool.QueryRow(ctx, query,
		partnerID,
		organizationID,
		logo.FileKey,
		logo.FileName,
		logo.ContentType,
		logo.SizeBytes,
	).Scan(
		&partner.ID,
		&partner.OrganizationID,
		&partner.BusinessName,
		&partner.KVKNumber,
		&partner.VATNumber,
		&partner.AddressLine1,
		&partner.AddressLine2,
		&partner.PostalCode,
		&partner.City,
		&partner.Country,
		&partner.ContactName,
		&partner.ContactEmail,
		&partner.ContactPhone,
		&partner.LogoFileKey,
		&partner.LogoFileName,
		&partner.LogoContentType,
		&partner.LogoSizeBytes,
		&partner.CreatedAt,
		&partner.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Partner{}, apperr.NotFound(partnerNotFoundMsg)
		}
		return Partner{}, fmt.Errorf("update partner logo: %w", err)
	}

	return partner, nil
}

func (r *Repository) ClearLogo(ctx context.Context, organizationID, partnerID uuid.UUID) (Partner, error) {
	query := `
		UPDATE RAC_partners
		SET logo_file_key = NULL,
			logo_file_name = NULL,
			logo_content_type = NULL,
			logo_size_bytes = NULL,
			updated_at = now()
		WHERE id = $1 AND organization_id = $2
		RETURNING id, organization_id, business_name, kvk_number, vat_number,
			address_line1, address_line2, postal_code, city, country,
			contact_name, contact_email, contact_phone,
			logo_file_key, logo_file_name, logo_content_type, logo_size_bytes,
			created_at, updated_at
	`

	var partner Partner
	err := r.pool.QueryRow(ctx, query, partnerID, organizationID).Scan(
		&partner.ID,
		&partner.OrganizationID,
		&partner.BusinessName,
		&partner.KVKNumber,
		&partner.VATNumber,
		&partner.AddressLine1,
		&partner.AddressLine2,
		&partner.PostalCode,
		&partner.City,
		&partner.Country,
		&partner.ContactName,
		&partner.ContactEmail,
		&partner.ContactPhone,
		&partner.LogoFileKey,
		&partner.LogoFileName,
		&partner.LogoContentType,
		&partner.LogoSizeBytes,
		&partner.CreatedAt,
		&partner.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Partner{}, apperr.NotFound(partnerNotFoundMsg)
		}
		return Partner{}, fmt.Errorf("clear partner logo: %w", err)
	}

	return partner, nil
}

func (r *Repository) ValidateServiceTypeIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}

	uniqueIDs := make([]uuid.UUID, 0, len(ids))
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}

	query := `SELECT id FROM RAC_service_types WHERE organization_id = $1 AND id = ANY($2)`
	rows, err := r.pool.Query(ctx, query, organizationID, uniqueIDs)
	if err != nil {
		return fmt.Errorf("validate service types: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("validate service types: %w", err)
	}
	if count != len(uniqueIDs) {
		return apperr.Validation("invalid service type id")
	}

	return nil
}

func (r *Repository) ReplaceServiceTypes(ctx context.Context, partnerID uuid.UUID, ids []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("replace partner service types: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM RAC_partner_service_types WHERE partner_id = $1`, partnerID); err != nil {
		return fmt.Errorf("replace partner service types: %w", err)
	}

	for _, id := range ids {
		if _, err := tx.Exec(
			ctx,
			`INSERT INTO RAC_partner_service_types (partner_id, service_type_id) VALUES ($1, $2)`,
			partnerID,
			id,
		); err != nil {
			return fmt.Errorf("replace partner service types: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("replace partner service types: %w", err)
	}
	return nil
}

func (r *Repository) ListServiceTypeIDs(ctx context.Context, organizationID, partnerID uuid.UUID) ([]uuid.UUID, error) {
	query := `
		SELECT pst.service_type_id
		FROM RAC_partner_service_types pst
		JOIN RAC_service_types st ON st.id = pst.service_type_id
		WHERE pst.partner_id = $1 AND st.organization_id = $2
		ORDER BY st.display_order ASC, st.name ASC
	`
	rows, err := r.pool.Query(ctx, query, partnerID, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list partner service types: %w", err)
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan partner service type: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate partner service types: %w", err)
	}

	return ids, nil
}

func (r *Repository) GetOrganizationName(ctx context.Context, organizationID uuid.UUID) (string, error) {
	var name string
	query := `SELECT name FROM RAC_organizations WHERE id = $1`
	if err := r.pool.QueryRow(ctx, query, organizationID).Scan(&name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apperr.NotFound("organization not found")
		}
		return "", fmt.Errorf("get organization name: %w", err)
	}
	return name, nil
}

func resolveSortBy(value string) (string, error) {
	if value == "" {
		return "createdAt", nil
	}
	switch value {
	case "businessName", "createdAt", "updatedAt":
		return value, nil
	default:
		return "", apperr.BadRequest("invalid sort field")
	}
}

func resolveSortOrder(value string) (string, error) {
	if value == "" {
		return "desc", nil
	}
	switch value {
	case "asc", "desc":
		return value, nil
	default:
		return "", apperr.BadRequest("invalid sort order")
	}
}

func optionalSearch(value string) interface{} {
	if value == "" {
		return nil
	}
	return "%" + value + "%"
}
