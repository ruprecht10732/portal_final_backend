package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	partnersdb "portal_final_backend/internal/partners/db"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const partnerNotFoundMsg = "partner not found"
const partnerInviteNotFoundMsg = "partner invite not found"
const replacePartnerServiceTypesErr = "replace partner service types: %w"

// Repository provides database operations for partners.
type Repository struct {
	pool    *pgxpool.Pool
	queries *partnersdb.Queries
}

// New creates a new partners repository.
func New(pool *pgxpool.Pool) *Repository {
	if pool == nil {
		return &Repository{}
	}
	return &Repository{pool: pool, queries: partnersdb.New(pool)}
}

type Partner struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	BusinessName    string
	KVKNumber       *string
	VATNumber       *string
	AddressLine1    string
	AddressLine2    *string
	HouseNumber     *string
	PostalCode      string
	City            string
	Country         string
	Latitude        *float64
	Longitude       *float64
	ContactName     string
	ContactEmail    string
	ContactPhone    string
	WhatsAppOptedIn bool
	LogoFileKey     *string
	LogoFileName    *string
	LogoContentType *string
	LogoSizeBytes   *int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type PartnerUpdate struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	BusinessName    *string
	KVKNumber       *string
	VATNumber       *string
	AddressLine1    *string
	AddressLine2    *string
	HouseNumber     *string
	PostalCode      *string
	City            *string
	Country         *string
	Latitude        *float64
	Longitude       *float64
	ContactName     *string
	ContactEmail    *string
	ContactPhone    *string
	WhatsAppOptedIn *bool
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

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func toPgUUIDPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return toPgUUID(*id)
}

func toPgUUIDSlice(ids []uuid.UUID) []pgtype.UUID {
	if len(ids) == 0 {
		return nil
	}
	out := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		out = append(out, toPgUUID(id))
	}
	return out
}

func toPgText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func toPgFloat8Ptr(value *float64) pgtype.Float8 {
	if value == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *value, Valid: true}
}

func toPgBoolPtr(value *bool) pgtype.Bool {
	if value == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *value, Valid: true}
}

func toPgTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func toPgTimestampPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return toPgTimestamp(*value)
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

func optionalFloat64(value pgtype.Float8) *float64 {
	if !value.Valid {
		return nil
	}
	n := value.Float64
	return &n
}

func optionalInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	n := value.Int64
	return &n
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time
	return &timestamp
}

func partnerFromModel(model partnersdb.RacPartner) Partner {
	return Partner{
		ID:              uuid.UUID(model.ID.Bytes),
		OrganizationID:  uuid.UUID(model.OrganizationID.Bytes),
		BusinessName:    model.BusinessName,
		KVKNumber:       optionalString(model.KvkNumber),
		VATNumber:       optionalString(model.VatNumber),
		AddressLine1:    model.AddressLine1,
		AddressLine2:    optionalString(model.AddressLine2),
		HouseNumber:     optionalString(model.HouseNumber),
		PostalCode:      model.PostalCode,
		City:            model.City,
		Country:         model.Country,
		Latitude:        optionalFloat64(model.Latitude),
		Longitude:       optionalFloat64(model.Longitude),
		ContactName:     model.ContactName,
		ContactEmail:    model.ContactEmail,
		ContactPhone:    model.ContactPhone,
		WhatsAppOptedIn: model.WhatsappOptedIn,
		LogoFileKey:     optionalString(model.LogoFileKey),
		LogoFileName:    optionalString(model.LogoFileName),
		LogoContentType: optionalString(model.LogoContentType),
		LogoSizeBytes:   optionalInt64(model.LogoSizeBytes),
		CreatedAt:       model.CreatedAt.Time,
		UpdatedAt:       model.UpdatedAt.Time,
	}
}

func inviteFromModel(model partnersdb.RacPartnerInvite) PartnerInvite {
	return PartnerInvite{
		ID:             uuid.UUID(model.ID.Bytes),
		OrganizationID: uuid.UUID(model.OrganizationID.Bytes),
		PartnerID:      uuid.UUID(model.PartnerID.Bytes),
		Email:          model.Email,
		TokenHash:      model.TokenHash,
		ExpiresAt:      model.ExpiresAt.Time,
		CreatedBy:      uuid.UUID(model.CreatedBy.Bytes),
		CreatedAt:      model.CreatedAt.Time,
		UsedAt:         optionalTime(model.UsedAt),
		UsedBy:         optionalUUID(model.UsedBy),
		LeadID:         optionalUUID(model.LeadID),
		LeadServiceID:  optionalUUID(model.LeadServiceID),
	}
}

func (r *Repository) Create(ctx context.Context, partner Partner) (Partner, error) {
	model, err := r.queries.CreatePartner(ctx, partnersdb.CreatePartnerParams{
		ID:              toPgUUID(partner.ID),
		OrganizationID:  toPgUUID(partner.OrganizationID),
		BusinessName:    partner.BusinessName,
		KvkNumber:       toPgText(partner.KVKNumber),
		VatNumber:       toPgText(partner.VATNumber),
		AddressLine1:    partner.AddressLine1,
		AddressLine2:    toPgText(partner.AddressLine2),
		HouseNumber:     toPgText(partner.HouseNumber),
		PostalCode:      partner.PostalCode,
		City:            partner.City,
		Country:         partner.Country,
		Latitude:        toPgFloat8Ptr(partner.Latitude),
		Longitude:       toPgFloat8Ptr(partner.Longitude),
		ContactName:     partner.ContactName,
		ContactEmail:    partner.ContactEmail,
		ContactPhone:    partner.ContactPhone,
		WhatsappOptedIn: partner.WhatsAppOptedIn,
		CreatedAt:       toPgTimestamp(partner.CreatedAt),
		UpdatedAt:       toPgTimestamp(partner.UpdatedAt),
	})
	if err != nil {
		return Partner{}, fmt.Errorf("create partner: %w", err)
	}

	return partnerFromModel(model), nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (Partner, error) {
	model, err := r.queries.GetPartnerByID(ctx, partnersdb.GetPartnerByIDParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Partner{}, apperr.NotFound(partnerNotFoundMsg)
		}
		return Partner{}, fmt.Errorf("get partner: %w", err)
	}

	return partnerFromModel(model), nil
}

func (r *Repository) Update(ctx context.Context, update PartnerUpdate) (Partner, error) {
	model, err := r.queries.UpdatePartner(ctx, partnersdb.UpdatePartnerParams{
		BusinessName:    toPgText(update.BusinessName),
		KvkNumber:       toPgText(update.KVKNumber),
		VatNumber:       toPgText(update.VATNumber),
		AddressLine1:    toPgText(update.AddressLine1),
		AddressLine2:    toPgText(update.AddressLine2),
		HouseNumber:     toPgText(update.HouseNumber),
		PostalCode:      toPgText(update.PostalCode),
		City:            toPgText(update.City),
		Country:         toPgText(update.Country),
		Latitude:        toPgFloat8Ptr(update.Latitude),
		Longitude:       toPgFloat8Ptr(update.Longitude),
		ContactName:     toPgText(update.ContactName),
		ContactEmail:    toPgText(update.ContactEmail),
		ContactPhone:    toPgText(update.ContactPhone),
		WhatsappOptedIn: toPgBoolPtr(update.WhatsAppOptedIn),
		ID:              toPgUUID(update.ID),
		OrganizationID:  toPgUUID(update.OrganizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Partner{}, apperr.NotFound(partnerNotFoundMsg)
		}
		return Partner{}, fmt.Errorf("update partner: %w", err)
	}

	return partnerFromModel(model), nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error {
	rowsAffected, err := r.queries.DeletePartner(ctx, partnersdb.DeletePartnerParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return fmt.Errorf("delete partner: %w", err)
	}
	if rowsAffected == 0 {
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

	total, err := r.queries.CountPartners(ctx, partnersdb.CountPartnersParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		Search:         searchParam,
	})
	if err != nil {
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
	totalInt := int(total)
	pageTotal := 0
	if pageSize > 0 {
		pageTotal = (totalInt + pageSize - 1) / pageSize
	}

	models, err := r.queries.ListPartners(ctx, partnersdb.ListPartnersParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		Search:         searchParam,
		SortBy:         sortBy,
		SortOrder:      orderBy,
		OffsetCount:    int32(offset),
		LimitCount:     int32(pageSize),
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("list partners: %w", err)
	}

	items := make([]Partner, 0, len(models))
	for _, model := range models {
		items = append(items, partnerFromModel(model))
	}

	return ListResult{Items: items, Total: totalInt, Page: page, PageSize: pageSize, TotalPages: pageTotal}, nil
}

func (r *Repository) Exists(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (bool, error) {
	exists, err := r.queries.PartnerExists(ctx, partnersdb.PartnerExistsParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return false, fmt.Errorf("check partner exists: %w", err)
	}
	return exists, nil
}

func (r *Repository) LeadExists(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (bool, error) {
	exists, err := r.queries.LeadExists(ctx, partnersdb.LeadExistsParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return false, fmt.Errorf("check lead exists: %w", err)
	}
	return exists, nil
}

func (r *Repository) LeadServiceExists(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (bool, error) {
	exists, err := r.queries.LeadServiceExists(ctx, partnersdb.LeadServiceExistsParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return false, fmt.Errorf("check lead service exists: %w", err)
	}
	return exists, nil
}

// GetLeadIDForService resolves the parent lead_id for a lead-service row.
func (r *Repository) GetLeadIDForService(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) (uuid.UUID, error) {
	leadID, err := r.queries.GetLeadIDForService(ctx, partnersdb.GetLeadIDForServiceParams{
		ServiceID:      toPgUUID(serviceID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.UUID{}, apperr.NotFound("lead service not found")
		}
		return uuid.UUID{}, fmt.Errorf("get lead id for service: %w", err)
	}
	return uuid.UUID(leadID.Bytes), nil
}

func (r *Repository) LinkLead(ctx context.Context, organizationID, partnerID, leadID uuid.UUID) error {
	rowsAffected, err := r.queries.LinkPartnerLead(ctx, partnersdb.LinkPartnerLeadParams{
		OrganizationID: toPgUUID(organizationID),
		PartnerID:      toPgUUID(partnerID),
		LeadID:         toPgUUID(leadID),
	})
	if err != nil {
		return fmt.Errorf("link partner lead: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.Conflict("lead already linked to partner")
	}
	return nil
}

func (r *Repository) UnlinkLead(ctx context.Context, organizationID, partnerID, leadID uuid.UUID) error {
	rowsAffected, err := r.queries.UnlinkPartnerLead(ctx, partnersdb.UnlinkPartnerLeadParams{
		OrganizationID: toPgUUID(organizationID),
		PartnerID:      toPgUUID(partnerID),
		LeadID:         toPgUUID(leadID),
	})
	if err != nil {
		return fmt.Errorf("unlink partner lead: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.NotFound("link not found")
	}
	return nil
}

func (r *Repository) ListLeads(ctx context.Context, organizationID, partnerID uuid.UUID) ([]PartnerLead, error) {
	rows, err := r.queries.ListPartnerLeads(ctx, partnersdb.ListPartnerLeadsParams{
		OrganizationID: toPgUUID(organizationID),
		PartnerID:      toPgUUID(partnerID),
	})
	if err != nil {
		return nil, fmt.Errorf("list partner leads: %w", err)
	}

	leads := make([]PartnerLead, 0, len(rows))
	for _, row := range rows {
		leads = append(leads, PartnerLead{
			ID:          uuid.UUID(row.ID.Bytes),
			FirstName:   row.ConsumerFirstName,
			LastName:    row.ConsumerLastName,
			Phone:       row.ConsumerPhone,
			Street:      row.AddressStreet,
			HouseNumber: row.HouseNumber,
			City:        row.AddressCity,
		})
	}

	return leads, nil
}

func (r *Repository) CreateInvite(ctx context.Context, invite PartnerInvite) (PartnerInvite, error) {
	model, err := r.queries.CreatePartnerInvite(ctx, partnersdb.CreatePartnerInviteParams{
		ID:             toPgUUID(invite.ID),
		OrganizationID: toPgUUID(invite.OrganizationID),
		PartnerID:      toPgUUID(invite.PartnerID),
		Email:          invite.Email,
		TokenHash:      invite.TokenHash,
		ExpiresAt:      toPgTimestamp(invite.ExpiresAt),
		CreatedBy:      toPgUUID(invite.CreatedBy),
		CreatedAt:      toPgTimestamp(invite.CreatedAt),
		UsedAt:         toPgTimestampPtr(invite.UsedAt),
		UsedBy:         toPgUUIDPtr(invite.UsedBy),
		LeadID:         toPgUUIDPtr(invite.LeadID),
		LeadServiceID:  toPgUUIDPtr(invite.LeadServiceID),
	})
	if err != nil {
		return PartnerInvite{}, fmt.Errorf("create partner invite: %w", err)
	}

	return inviteFromModel(model), nil
}

func (r *Repository) ListInvites(ctx context.Context, organizationID, partnerID uuid.UUID) ([]PartnerInvite, error) {
	models, err := r.queries.ListPartnerInvites(ctx, partnersdb.ListPartnerInvitesParams{
		OrganizationID: toPgUUID(organizationID),
		PartnerID:      toPgUUID(partnerID),
	})
	if err != nil {
		return nil, fmt.Errorf("list partner invites: %w", err)
	}

	invites := make([]PartnerInvite, 0, len(models))
	for _, model := range models {
		invites = append(invites, inviteFromModel(model))
	}

	return invites, nil
}

func (r *Repository) RevokeInvite(ctx context.Context, organizationID, inviteID uuid.UUID) (PartnerInvite, error) {
	model, err := r.queries.RevokePartnerInvite(ctx, partnersdb.RevokePartnerInviteParams{
		InviteID:       toPgUUID(inviteID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PartnerInvite{}, apperr.NotFound(partnerInviteNotFoundMsg)
		}
		return PartnerInvite{}, fmt.Errorf("revoke partner invite: %w", err)
	}

	return inviteFromModel(model), nil
}

func (r *Repository) UpdateLogo(ctx context.Context, organizationID, partnerID uuid.UUID, logo PartnerLogo) (Partner, error) {
	model, err := r.queries.UpdatePartnerLogo(ctx, partnersdb.UpdatePartnerLogoParams{
		FileKey:        logo.FileKey,
		FileName:       logo.FileName,
		ContentType:    logo.ContentType,
		SizeBytes:      logo.SizeBytes,
		PartnerID:      toPgUUID(partnerID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Partner{}, apperr.NotFound(partnerNotFoundMsg)
		}
		return Partner{}, fmt.Errorf("update partner logo: %w", err)
	}

	return partnerFromModel(model), nil
}

func (r *Repository) ClearLogo(ctx context.Context, organizationID, partnerID uuid.UUID) (Partner, error) {
	model, err := r.queries.ClearPartnerLogo(ctx, partnersdb.ClearPartnerLogoParams{
		PartnerID:      toPgUUID(partnerID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Partner{}, apperr.NotFound(partnerNotFoundMsg)
		}
		return Partner{}, fmt.Errorf("clear partner logo: %w", err)
	}

	return partnerFromModel(model), nil
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

	count, err := r.queries.CountValidServiceTypes(ctx, partnersdb.CountValidServiceTypesParams{
		OrganizationID: toPgUUID(organizationID),
		ServiceTypeIds: toPgUUIDSlice(uniqueIDs),
	})
	if err != nil {
		return fmt.Errorf("validate service types: %w", err)
	}
	if int(count) != len(uniqueIDs) {
		return apperr.Validation("invalid service type id")
	}

	return nil
}

func (r *Repository) ReplaceServiceTypes(ctx context.Context, partnerID uuid.UUID, ids []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf(replacePartnerServiceTypesErr, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	queries := r.queries.WithTx(tx)
	if err := queries.DeletePartnerServiceTypes(ctx, toPgUUID(partnerID)); err != nil {
		return fmt.Errorf(replacePartnerServiceTypesErr, err)
	}

	for _, id := range ids {
		if err := queries.CreatePartnerServiceType(ctx, partnersdb.CreatePartnerServiceTypeParams{
			PartnerID:     toPgUUID(partnerID),
			ServiceTypeID: toPgUUID(id),
		}); err != nil {
			return fmt.Errorf(replacePartnerServiceTypesErr, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(replacePartnerServiceTypesErr, err)
	}
	return nil
}

func (r *Repository) ListServiceTypeIDs(ctx context.Context, organizationID, partnerID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.queries.ListPartnerServiceTypeIDs(ctx, partnersdb.ListPartnerServiceTypeIDsParams{
		PartnerID:      toPgUUID(partnerID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return nil, fmt.Errorf("list partner service types: %w", err)
	}

	ids := make([]uuid.UUID, 0, len(rows))
	for _, id := range rows {
		ids = append(ids, uuid.UUID(id.Bytes))
	}

	return ids, nil
}

func (r *Repository) GetOrganizationName(ctx context.Context, organizationID uuid.UUID) (string, error) {
	name, err := r.queries.GetOrganizationName(ctx, toPgUUID(organizationID))
	if err != nil {
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
	case "businessName", "contactName", "createdAt", "updatedAt":
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

func optionalSearch(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: "%" + value + "%", Valid: true}
}
