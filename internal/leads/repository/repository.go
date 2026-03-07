package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	leadsdb "portal_final_backend/internal/leads/db"
	"portal_final_backend/platform/apperr"
)

var ErrNotFound = errors.New("lead not found")

type Repository struct {
	pool    *pgxpool.Pool
	queries *leadsdb.Queries
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: leadsdb.New(pool)}
}

// ListActiveServiceTypes returns active service types with intake guidelines for AI context.
func (r *Repository) ListActiveServiceTypes(ctx context.Context, organizationID uuid.UUID) ([]ServiceContextDefinition, error) {
	rows, err := r.queries.ListActiveServiceTypes(ctx, toPgUUID(organizationID))
	if err != nil {
		return nil, err
	}

	items := make([]ServiceContextDefinition, 0, len(rows))
	for _, row := range rows {
		items = append(items, ServiceContextDefinition{
			Name:                 row.Name,
			Description:          optionalString(row.Description),
			IntakeGuidelines:     optionalString(row.IntakeGuidelines),
			EstimationGuidelines: optionalString(row.EstimationGuidelines),
		})
	}

	return items, nil
}

type Lead struct {
	ID                                      uuid.UUID
	OrganizationID                          uuid.UUID
	ConsumerFirstName                       string
	ConsumerLastName                        string
	ConsumerPhone                           string
	ConsumerEmail                           *string
	ConsumerRole                            string
	AddressStreet                           string
	AddressHouseNumber                      string
	AddressZipCode                          string
	AddressCity                             string
	Latitude                                *float64
	Longitude                               *float64
	AssignedAgentID                         *uuid.UUID
	Source                                  *string
	WhatsAppOptedIn                         bool
	EnergyClass                             *string
	EnergyIndex                             *float64
	EnergyBouwjaar                          *int
	EnergyGebouwtype                        *string
	EnergyLabelValidUntil                   *time.Time
	EnergyLabelRegisteredAt                 *time.Time
	EnergyPrimairFossiel                    *float64
	EnergyBAGVerblijfsobjectID              *string
	EnergyLabelFetchedAt                    *time.Time
	LeadEnrichmentSource                    *string
	LeadEnrichmentPostcode6                 *string
	LeadEnrichmentPostcode4                 *string
	LeadEnrichmentBuurtcode                 *string
	LeadEnrichmentDataYear                  *int
	LeadEnrichmentGemAardgasverbruik        *float64
	LeadEnrichmentGemElektriciteitsverbruik *float64
	LeadEnrichmentHuishoudenGrootte         *float64
	LeadEnrichmentKoopwoningenPct           *float64
	LeadEnrichmentBouwjaarVanaf2000Pct      *float64
	LeadEnrichmentWOZWaarde                 *float64
	LeadEnrichmentMediaanVermogenX1000      *float64
	LeadEnrichmentGemInkomen                *float64
	LeadEnrichmentPctHoogInkomen            *float64
	LeadEnrichmentPctLaagInkomen            *float64
	LeadEnrichmentHuishoudensMetKinderenPct *float64
	LeadEnrichmentStedelijkheid             *int
	LeadEnrichmentConfidence                *float64
	LeadEnrichmentFetchedAt                 *time.Time
	LeadScore                               *int
	LeadScorePreAI                          *int
	LeadScoreFactors                        []byte
	LeadScoreVersion                        *string
	LeadScoreUpdatedAt                      *time.Time
	ViewedByID                              *uuid.UUID
	ViewedAt                                *time.Time
	CreatedAt                               time.Time
	UpdatedAt                               time.Time
}

// LeadSummary is a lightweight lead representation for returning customer detection
type LeadSummary struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	ConsumerName    string
	ConsumerPhone   string
	ConsumerEmail   *string
	AddressCity     string
	ServiceCount    int
	LastServiceType *string
	LastStatus      *string
	CreatedAt       time.Time
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

func toPgTextValue(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

func toPgInt8Ptr(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func toPgInt8Value(value int64) pgtype.Int8 {
	return pgtype.Int8{Int64: value, Valid: true}
}

func toPgFloat8Ptr(value *float64) pgtype.Float8 {
	if value == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *value, Valid: true}
}

func toPgInt4Ptr(value *int) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*value), Valid: true}
}

func toPgNumericPtr(value *float64) pgtype.Numeric {
	if value == nil {
		return pgtype.Numeric{}
	}
	var numeric pgtype.Numeric
	if err := numeric.Scan(*value); err != nil {
		return pgtype.Numeric{}
	}
	return numeric
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

func optionalInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	n := value.Int64
	return &n
}

func optionalFloat64(value pgtype.Float8) *float64 {
	if !value.Valid {
		return nil
	}
	n := value.Float64
	return &n
}

func optionalNumericFloat64(value pgtype.Numeric) *float64 {
	if !value.Valid {
		return nil
	}
	floatValue, err := value.Float64Value()
	if err != nil || !floatValue.Valid {
		return nil
	}
	n := floatValue.Float64
	return &n
}

func optionalInt(value pgtype.Int4) *int {
	if !value.Valid {
		return nil
	}
	n := int(value.Int32)
	return &n
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time
	return &timestamp
}

func optionalTimeValue(value interface{}) *time.Time {
	switch v := value.(type) {
	case time.Time:
		return &v
	case *time.Time:
		return v
	case pgtype.Timestamptz:
		return optionalTime(v)
	default:
		return nil
	}
}

func interfaceString(value interface{}) string {
	if value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

func emptyStringAsNil(value string) *string {
	if value == "" {
		return nil
	}
	text := value
	return &text
}

func nonEmptyStringOrNil(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func interfaceTimeOrZero(value interface{}) time.Time {
	if timestamp := optionalTimeValue(value); timestamp != nil {
		return *timestamp
	}
	return time.Time{}
}

func activityFeedEntryFromRecentRow(row leadsdb.ListRecentActivityRow) ActivityFeedEntry {
	return ActivityFeedEntry{
		ID:          uuid.UUID(row.ID.Bytes),
		Category:    row.Category,
		EventType:   row.EventType,
		Title:       row.Title,
		Description: row.Description,
		EntityID:    uuid.UUID(row.EntityID.Bytes),
		ServiceID:   optionalUUID(row.ServiceID),
		LeadName:    interfaceString(row.LeadName),
		Phone:       row.Phone,
		Email:       row.Email,
		LeadStatus:  row.LeadStatus,
		ServiceType: row.ServiceType,
		LeadScore:   optionalInt(row.LeadScore),
		Address:     row.Address,
		Latitude:    optionalFloat64(row.Latitude),
		Longitude:   optionalFloat64(row.Longitude),
		ScheduledAt: optionalTime(row.ScheduledAt),
		CreatedAt:   row.CreatedAt.Time,
		Priority:    int(row.Priority),
		GroupCount:  int(row.GroupCount),
		ActorName:   interfaceString(row.ActorName),
		RawMetadata: row.RawMetadata,
	}
}

func activityFeedEntryFromUpcomingRow(row leadsdb.ListUpcomingAppointmentsRow) ActivityFeedEntry {
	return ActivityFeedEntry{
		ID:          uuid.UUID(row.ID.Bytes),
		Category:    row.Category,
		EventType:   row.EventType,
		Title:       row.Title,
		Description: row.Description,
		EntityID:    uuid.UUID(row.EntityID.Bytes),
		LeadName:    interfaceString(row.LeadName),
		Phone:       row.Phone,
		Email:       row.Email,
		LeadStatus:  row.LeadStatus,
		ServiceType: row.ServiceType,
		LeadScore:   optionalInt(row.LeadScore),
		Address:     interfaceString(row.Address),
		Latitude:    optionalFloat64(row.Latitude),
		Longitude:   optionalFloat64(row.Longitude),
		ScheduledAt: optionalTime(row.ScheduledAt),
		CreatedAt:   interfaceTimeOrZero(row.CreatedAt),
		Priority:    int(row.Priority),
	}
}

func leadFromDB(row leadsdb.RacLead) Lead {
	return Lead{
		ID:                                      row.ID.Bytes,
		OrganizationID:                          row.OrganizationID.Bytes,
		ConsumerFirstName:                       row.ConsumerFirstName,
		ConsumerLastName:                        row.ConsumerLastName,
		ConsumerPhone:                           row.ConsumerPhone,
		ConsumerEmail:                           optionalString(row.ConsumerEmail),
		ConsumerRole:                            row.ConsumerRole,
		AddressStreet:                           row.AddressStreet,
		AddressHouseNumber:                      row.AddressHouseNumber,
		AddressZipCode:                          row.AddressZipCode,
		AddressCity:                             row.AddressCity,
		Latitude:                                optionalFloat64(row.Latitude),
		Longitude:                               optionalFloat64(row.Longitude),
		AssignedAgentID:                         optionalUUID(row.AssignedAgentID),
		Source:                                  optionalString(row.Source),
		WhatsAppOptedIn:                         row.WhatsappOptedIn,
		EnergyClass:                             optionalString(row.EnergyClass),
		EnergyIndex:                             optionalFloat64(row.EnergyIndex),
		EnergyBouwjaar:                          optionalInt(row.EnergyBouwjaar),
		EnergyGebouwtype:                        optionalString(row.EnergyGebouwtype),
		EnergyLabelValidUntil:                   optionalTime(row.EnergyLabelValidUntil),
		EnergyLabelRegisteredAt:                 optionalTime(row.EnergyLabelRegisteredAt),
		EnergyPrimairFossiel:                    optionalFloat64(row.EnergyPrimairFossiel),
		EnergyBAGVerblijfsobjectID:              optionalString(row.EnergyBagVerblijfsobjectID),
		EnergyLabelFetchedAt:                    optionalTime(row.EnergyLabelFetchedAt),
		LeadEnrichmentSource:                    optionalString(row.LeadEnrichmentSource),
		LeadEnrichmentPostcode6:                 optionalString(row.LeadEnrichmentPostcode6),
		LeadEnrichmentPostcode4:                 optionalString(row.LeadEnrichmentPostcode4),
		LeadEnrichmentBuurtcode:                 optionalString(row.LeadEnrichmentBuurtcode),
		LeadEnrichmentDataYear:                  optionalInt(row.LeadEnrichmentDataYear),
		LeadEnrichmentGemAardgasverbruik:        optionalFloat64(row.LeadEnrichmentGemAardgasverbruik),
		LeadEnrichmentGemElektriciteitsverbruik: optionalFloat64(row.LeadEnrichmentGemElektriciteitsverbruik),
		LeadEnrichmentHuishoudenGrootte:         optionalFloat64(row.LeadEnrichmentHuishoudenGrootte),
		LeadEnrichmentKoopwoningenPct:           optionalFloat64(row.LeadEnrichmentKoopwoningenPct),
		LeadEnrichmentBouwjaarVanaf2000Pct:      optionalFloat64(row.LeadEnrichmentBouwjaarVanaf2000Pct),
		LeadEnrichmentWOZWaarde:                 optionalFloat64(row.LeadEnrichmentWozWaarde),
		LeadEnrichmentMediaanVermogenX1000:      optionalFloat64(row.LeadEnrichmentMediaanVermogenX1000),
		LeadEnrichmentGemInkomen:                optionalFloat64(row.LeadEnrichmentGemInkomen),
		LeadEnrichmentPctHoogInkomen:            optionalFloat64(row.LeadEnrichmentPctHoogInkomen),
		LeadEnrichmentPctLaagInkomen:            optionalFloat64(row.LeadEnrichmentPctLaagInkomen),
		LeadEnrichmentHuishoudensMetKinderenPct: optionalFloat64(row.LeadEnrichmentHuishoudensMetKinderenPct),
		LeadEnrichmentStedelijkheid:             optionalInt(row.LeadEnrichmentStedelijkheid),
		LeadEnrichmentConfidence:                optionalFloat64(row.LeadEnrichmentConfidence),
		LeadEnrichmentFetchedAt:                 optionalTime(row.LeadEnrichmentFetchedAt),
		LeadScore:                               optionalInt(row.LeadScore),
		LeadScorePreAI:                          optionalInt(row.LeadScorePreAi),
		LeadScoreFactors:                        row.LeadScoreFactors,
		LeadScoreVersion:                        optionalString(row.LeadScoreVersion),
		LeadScoreUpdatedAt:                      optionalTime(row.LeadScoreUpdatedAt),
		ViewedByID:                              optionalUUID(row.ViewedByID),
		ViewedAt:                                optionalTime(row.ViewedAt),
		CreatedAt:                               row.CreatedAt.Time,
		UpdatedAt:                               row.UpdatedAt.Time,
	}
}

type CreateLeadParams struct {
	OrganizationID     uuid.UUID
	ConsumerFirstName  string
	ConsumerLastName   string
	ConsumerPhone      string
	ConsumerEmail      *string
	ConsumerRole       string
	AddressStreet      string
	AddressHouseNumber string
	AddressZipCode     string
	AddressCity        string
	Latitude           *float64
	Longitude          *float64
	AssignedAgentID    *uuid.UUID
	Source             *string
	GCLID              *string
	UTMSource          *string
	UTMMedium          *string
	UTMCampaign        *string
	UTMContent         *string
	UTMTerm            *string
	AdLandingPage      *string
	ReferrerURL        *string
	WhatsAppOptedIn    bool
}

func (r *Repository) Create(ctx context.Context, params CreateLeadParams) (Lead, error) {
	row, err := r.queries.CreateLead(ctx, leadsdb.CreateLeadParams{
		OrganizationID:     toPgUUID(params.OrganizationID),
		ConsumerFirstName:  params.ConsumerFirstName,
		ConsumerLastName:   params.ConsumerLastName,
		ConsumerPhone:      params.ConsumerPhone,
		ConsumerEmail:      toPgText(params.ConsumerEmail),
		ConsumerRole:       params.ConsumerRole,
		AddressStreet:      params.AddressStreet,
		AddressHouseNumber: params.AddressHouseNumber,
		AddressZipCode:     params.AddressZipCode,
		AddressCity:        params.AddressCity,
		Latitude:           toPgFloat8Ptr(params.Latitude),
		Longitude:          toPgFloat8Ptr(params.Longitude),
		AssignedAgentID:    toPgUUIDPtr(params.AssignedAgentID),
		Source:             toPgText(params.Source),
		Gclid:              toPgText(params.GCLID),
		UtmSource:          toPgText(params.UTMSource),
		UtmMedium:          toPgText(params.UTMMedium),
		UtmCampaign:        toPgText(params.UTMCampaign),
		UtmContent:         toPgText(params.UTMContent),
		UtmTerm:            toPgText(params.UTMTerm),
		AdLandingPage:      toPgText(params.AdLandingPage),
		ReferrerUrl:        toPgText(params.ReferrerURL),
		WhatsappOptedIn:    params.WhatsAppOptedIn,
	})
	if err != nil {
		return Lead{}, err
	}

	return leadFromDB(row), nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (Lead, error) {
	row, err := r.queries.GetLeadByID(ctx, leadsdb.GetLeadByIDParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	if err != nil {
		return Lead{}, err
	}
	if row.DeletedAt.Valid {
		return Lead{}, ErrNotFound
	}
	return leadFromDB(row), nil
}

// GetByIDWithServices returns a lead with all its services populated
func (r *Repository) GetByIDWithServices(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (Lead, []LeadService, error) {
	lead, err := r.GetByID(ctx, id, organizationID)
	if err != nil {
		return Lead{}, nil, err
	}

	services, err := r.ListLeadServices(ctx, id, organizationID)
	if err != nil {
		return Lead{}, nil, err
	}

	return lead, services, nil
}

func (r *Repository) GetByPhone(ctx context.Context, phone string, organizationID uuid.UUID) (Lead, error) {
	row, err := r.queries.GetLeadByPhone(ctx, leadsdb.GetLeadByPhoneParams{ConsumerPhone: phone, OrganizationID: toPgUUID(organizationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	if err != nil {
		return Lead{}, err
	}
	return leadFromDB(row), nil
}

func (r *Repository) IsWhatsAppOptedIn(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (bool, error) {
	optedIn, err := r.queries.GetLeadWhatsAppOptIn(ctx, leadsdb.GetLeadWhatsAppOptInParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	return optedIn, err
}

// GetByPhoneOrEmail finds a lead matching the given phone or email for returning customer detection.
// Returns the first matching lead with its services, or nil if not found.
func (r *Repository) GetByPhoneOrEmail(ctx context.Context, phone string, email string, organizationID uuid.UUID) (*LeadSummary, []LeadService, error) {
	if phone == "" && email == "" {
		return nil, nil, nil
	}

	row, err := r.queries.GetLeadSummaryByPhoneOrEmail(ctx, leadsdb.GetLeadSummaryByPhoneOrEmailParams{
		Column1:        nonEmptyStringOrNil(phone),
		Column2:        nonEmptyStringOrNil(email),
		OrganizationID: toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	summary := LeadSummary{
		ID:              uuid.UUID(row.ID.Bytes),
		OrganizationID:  uuid.UUID(row.OrganizationID.Bytes),
		ConsumerName:    interfaceString(row.ConsumerName),
		ConsumerPhone:   row.ConsumerPhone,
		ConsumerEmail:   optionalString(row.ConsumerEmail),
		AddressCity:     row.AddressCity,
		ServiceCount:    int(row.ServiceCount),
		LastServiceType: emptyStringAsNil(interfaceString(row.LastServiceType)),
		LastStatus:      emptyStringAsNil(interfaceString(row.LastStatus)),
		CreatedAt:       row.CreatedAt.Time,
	}

	// Fetch services for the found lead
	services, err := r.ListLeadServices(ctx, summary.ID, organizationID)
	if err != nil {
		return nil, nil, err
	}

	return &summary, services, nil
}

// GetLatestAcceptedQuoteIDForService returns the most recent Accepted quote ID for a lead service.
// This is used by agent tooling to create partner offers in quote-only mode.
func (r *Repository) GetLatestAcceptedQuoteIDForService(ctx context.Context, serviceID, organizationID uuid.UUID) (uuid.UUID, error) {
	quoteID, err := r.queries.GetLatestAcceptedQuoteIDForService(ctx, leadsdb.GetLatestAcceptedQuoteIDForServiceParams{
		LeadServiceID:  toPgUUID(serviceID),
		OrganizationID: toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, apperr.NotFound("accepted quote not found")
	}
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.UUID(quoteID.Bytes), nil
}

func (r *Repository) HasNonDraftQuote(ctx context.Context, serviceID, organizationID uuid.UUID) (bool, error) {
	return r.queries.HasNonDraftQuote(ctx, leadsdb.HasNonDraftQuoteParams{
		LeadServiceID:  toPgUUID(serviceID),
		OrganizationID: toPgUUID(organizationID),
	})
}

func (r *Repository) GetLatestDraftQuoteID(ctx context.Context, serviceID, organizationID uuid.UUID) (*uuid.UUID, error) {
	id, err := r.queries.GetLatestDraftQuoteID(ctx, leadsdb.GetLatestDraftQuoteIDParams{
		LeadServiceID:  toPgUUID(serviceID),
		OrganizationID: toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	value := uuid.UUID(id.Bytes)
	return &value, nil
}

type UpdateLeadParams struct {
	ConsumerFirstName  *string
	ConsumerLastName   *string
	ConsumerPhone      *string
	ConsumerEmail      *string
	ConsumerRole       *string
	AddressStreet      *string
	AddressHouseNumber *string
	AddressZipCode     *string
	AddressCity        *string
	Latitude           *float64
	Longitude          *float64
	AssignedAgentID    *uuid.UUID
	AssignedAgentIDSet bool
	WhatsAppOptedIn    *bool
	WhatsAppOptedInSet bool
}

type UpdateEnergyLabelParams struct {
	Class          *string
	Index          *float64
	Bouwjaar       *int
	Gebouwtype     *string
	ValidUntil     *time.Time
	RegisteredAt   *time.Time
	PrimairFossiel *float64
	BAGObjectID    *string
	FetchedAt      time.Time
}

type UpdateLeadEnrichmentParams struct {
	Source                    *string
	Postcode6                 *string
	Postcode4                 *string
	Buurtcode                 *string
	DataYear                  *int
	GemAardgasverbruik        *float64
	GemElektriciteitsverbruik *float64
	HuishoudenGrootte         *float64
	KoopwoningenPct           *float64
	BouwjaarVanaf2000Pct      *float64
	WOZWaarde                 *float64
	MediaanVermogenX1000      *float64
	GemInkomen                *float64
	PctHoogInkomen            *float64
	PctLaagInkomen            *float64
	HuishoudensMetKinderenPct *float64
	Stedelijkheid             *int
	Confidence                *float64
	FetchedAt                 time.Time
	Score                     *int
	ScorePreAI                *int
	ScoreFactors              []byte
	ScoreVersion              *string
	ScoreUpdatedAt            *time.Time
}

type UpdateLeadScoreParams struct {
	Score          *int
	ScorePreAI     *int
	ScoreFactors   []byte
	ScoreVersion   *string
	ScoreUpdatedAt time.Time
}

func nullable[T any](value *T) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func boolValue(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}

func (r *Repository) Update(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params UpdateLeadParams) (Lead, error) {
	hasUpdates := params.ConsumerFirstName != nil ||
		params.ConsumerLastName != nil ||
		params.ConsumerPhone != nil ||
		params.ConsumerEmail != nil ||
		params.ConsumerRole != nil ||
		params.AddressStreet != nil ||
		params.AddressHouseNumber != nil ||
		params.AddressZipCode != nil ||
		params.AddressCity != nil ||
		params.Latitude != nil ||
		params.Longitude != nil ||
		params.AssignedAgentIDSet ||
		params.WhatsAppOptedInSet

	if !hasUpdates {
		return r.GetByID(ctx, id, organizationID)
	}

	row, err := r.queries.UpdateLead(ctx, leadsdb.UpdateLeadParams{
		ConsumerFirstName:  toPgText(params.ConsumerFirstName),
		ConsumerLastName:   toPgText(params.ConsumerLastName),
		ConsumerPhone:      toPgText(params.ConsumerPhone),
		ConsumerEmail:      toPgText(params.ConsumerEmail),
		ConsumerRole:       toPgText(params.ConsumerRole),
		AddressStreet:      toPgText(params.AddressStreet),
		AddressHouseNumber: toPgText(params.AddressHouseNumber),
		AddressZipCode:     toPgText(params.AddressZipCode),
		AddressCity:        toPgText(params.AddressCity),
		Latitude:           toPgFloat8Ptr(params.Latitude),
		Longitude:          toPgFloat8Ptr(params.Longitude),
		AssignedAgentIDSet: params.AssignedAgentIDSet,
		AssignedAgentID:    toPgUUIDPtr(params.AssignedAgentID),
		WhatsappOptedInSet: params.WhatsAppOptedInSet,
		WhatsappOptedIn:    boolValue(params.WhatsAppOptedIn),
		ID:                 toPgUUID(id),
		OrganizationID:     toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	if err != nil {
		return Lead{}, err
	}
	return leadFromDB(row), nil
}

func (r *Repository) UpdateEnergyLabel(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params UpdateEnergyLabelParams) error {
	result, err := r.queries.UpdateEnergyLabel(ctx, leadsdb.UpdateEnergyLabelParams{
		ID:                         toPgUUID(id),
		OrganizationID:             toPgUUID(organizationID),
		EnergyClass:                toPgText(params.Class),
		EnergyIndex:                toPgFloat8Ptr(params.Index),
		EnergyBouwjaar:             toPgInt4Ptr(params.Bouwjaar),
		EnergyGebouwtype:           toPgText(params.Gebouwtype),
		EnergyLabelValidUntil:      toPgTimestampPtr(params.ValidUntil),
		EnergyLabelRegisteredAt:    toPgTimestampPtr(params.RegisteredAt),
		EnergyPrimairFossiel:       toPgFloat8Ptr(params.PrimairFossiel),
		EnergyBagVerblijfsobjectID: toPgText(params.BAGObjectID),
		EnergyLabelFetchedAt:       toPgTimestamp(params.FetchedAt),
		UpdatedAt:                  toPgTimestamp(params.FetchedAt),
	})
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateLeadEnrichment(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params UpdateLeadEnrichmentParams) error {
	result, err := r.queries.UpdateLeadEnrichment(ctx, leadsdb.UpdateLeadEnrichmentParams{
		ID:                                      toPgUUID(id),
		OrganizationID:                          toPgUUID(organizationID),
		LeadEnrichmentSource:                    toPgText(params.Source),
		LeadEnrichmentPostcode6:                 toPgText(params.Postcode6),
		LeadEnrichmentPostcode4:                 toPgText(params.Postcode4),
		LeadEnrichmentBuurtcode:                 toPgText(params.Buurtcode),
		LeadEnrichmentDataYear:                  toPgInt4Ptr(params.DataYear),
		LeadEnrichmentGemAardgasverbruik:        toPgFloat8Ptr(params.GemAardgasverbruik),
		LeadEnrichmentGemElektriciteitsverbruik: toPgFloat8Ptr(params.GemElektriciteitsverbruik),
		LeadEnrichmentHuishoudenGrootte:         toPgFloat8Ptr(params.HuishoudenGrootte),
		LeadEnrichmentKoopwoningenPct:           toPgFloat8Ptr(params.KoopwoningenPct),
		LeadEnrichmentBouwjaarVanaf2000Pct:      toPgFloat8Ptr(params.BouwjaarVanaf2000Pct),
		LeadEnrichmentWozWaarde:                 toPgFloat8Ptr(params.WOZWaarde),
		LeadEnrichmentMediaanVermogenX1000:      toPgFloat8Ptr(params.MediaanVermogenX1000),
		LeadEnrichmentGemInkomen:                toPgFloat8Ptr(params.GemInkomen),
		LeadEnrichmentPctHoogInkomen:            toPgFloat8Ptr(params.PctHoogInkomen),
		LeadEnrichmentPctLaagInkomen:            toPgFloat8Ptr(params.PctLaagInkomen),
		LeadEnrichmentHuishoudensMetKinderenPct: toPgFloat8Ptr(params.HuishoudensMetKinderenPct),
		LeadEnrichmentStedelijkheid:             toPgInt4Ptr(params.Stedelijkheid),
		LeadEnrichmentConfidence:                toPgFloat8Ptr(params.Confidence),
		LeadEnrichmentFetchedAt:                 toPgTimestamp(params.FetchedAt),
		LeadScore:                               toPgInt4Ptr(params.Score),
		LeadScorePreAi:                          toPgInt4Ptr(params.ScorePreAI),
		LeadScoreFactors:                        params.ScoreFactors,
		LeadScoreVersion:                        toPgText(params.ScoreVersion),
		LeadScoreUpdatedAt:                      toPgTimestampPtr(params.ScoreUpdatedAt),
		UpdatedAt:                               toPgTimestamp(params.FetchedAt),
	})
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateLeadScore(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params UpdateLeadScoreParams) error {
	result, err := r.queries.UpdateLeadScore(ctx, leadsdb.UpdateLeadScoreParams{
		ID:                 toPgUUID(id),
		OrganizationID:     toPgUUID(organizationID),
		LeadScore:          toPgInt4Ptr(params.Score),
		LeadScorePreAi:     toPgInt4Ptr(params.ScorePreAI),
		LeadScoreFactors:   params.ScoreFactors,
		LeadScoreVersion:   toPgText(params.ScoreVersion),
		LeadScoreUpdatedAt: toPgTimestamp(params.ScoreUpdatedAt),
		UpdatedAt:          toPgTimestamp(params.ScoreUpdatedAt),
	})
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateProjectedValueCents(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, projectedValueCents int64) error {
	result, err := r.queries.UpdateProjectedValueCents(ctx, leadsdb.UpdateProjectedValueCentsParams{
		ID:                  toPgUUID(id),
		OrganizationID:      toPgUUID(organizationID),
		ProjectedValueCents: projectedValueCents,
	})
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) SetViewedBy(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, userID uuid.UUID) error {
	return r.queries.SetLeadViewedBy(ctx, leadsdb.SetLeadViewedByParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
		ViewedByID:     toPgUUID(userID),
	})
}

func (r *Repository) AddActivity(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID, userID uuid.UUID, action string, meta map[string]interface{}) error {
	var metaJSON []byte
	if meta != nil {
		encoded, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		metaJSON = encoded
	}

	return r.queries.AddLeadActivity(ctx, leadsdb.AddLeadActivityParams{
		LeadID:         toPgUUID(leadID),
		OrganizationID: toPgUUID(organizationID),
		UserID:         toPgUUID(userID),
		Action:         action,
		Meta:           metaJSON,
	})
}

type ListParams struct {
	OrganizationID  uuid.UUID
	Status          *string
	ServiceType     *string
	Search          string
	FirstName       *string
	LastName        *string
	Phone           *string
	Email           *string
	Role            *string
	Street          *string
	HouseNumber     *string
	ZipCode         *string
	City            *string
	AssignedAgentID *uuid.UUID
	CreatedAtFrom   *time.Time
	CreatedAtTo     *time.Time
	Offset          int
	Limit           int
	SortBy          string
	SortOrder       string
}

func (r *Repository) List(ctx context.Context, params ListParams) ([]Lead, int, error) {
	filters := buildLeadListFilters(params)

	sortBy, err := resolveLeadSortBy(params.SortBy)
	if err != nil {
		return nil, 0, err
	}

	sortOrder, err := resolveLeadSortOrder(params.SortOrder)
	if err != nil {
		return nil, 0, err
	}

	total, err := r.queries.CountLeads(ctx, leadsdb.CountLeadsParams{
		OrganizationID:  toPgUUID(params.OrganizationID),
		Status:          filters.status,
		ServiceType:     filters.serviceType,
		Search:          filters.search,
		FirstName:       filters.firstName,
		LastName:        filters.lastName,
		Phone:           filters.phone,
		Email:           filters.email,
		Role:            filters.role,
		Street:          filters.street,
		HouseNumber:     filters.houseNumber,
		ZipCode:         filters.zipCode,
		City:            filters.city,
		AssignedAgentID: filters.assignedAgentID,
		CreatedAtFrom:   filters.createdAtFrom,
		CreatedAtTo:     filters.createdAtTo,
	})
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.queries.ListLeads(ctx, leadsdb.ListLeadsParams{
		OrganizationID:  toPgUUID(params.OrganizationID),
		Status:          filters.status,
		ServiceType:     filters.serviceType,
		Search:          filters.search,
		FirstName:       filters.firstName,
		LastName:        filters.lastName,
		Phone:           filters.phone,
		Email:           filters.email,
		Role:            filters.role,
		Street:          filters.street,
		HouseNumber:     filters.houseNumber,
		ZipCode:         filters.zipCode,
		City:            filters.city,
		AssignedAgentID: filters.assignedAgentID,
		CreatedAtFrom:   filters.createdAtFrom,
		CreatedAtTo:     filters.createdAtTo,
		SortBy:          sortBy,
		SortOrder:       sortOrder,
		OffsetCount:     int32(params.Offset),
		LimitCount:      int32(params.Limit),
	})
	if err != nil {
		return nil, 0, err
	}

	leads := make([]Lead, 0, len(rows))
	for _, row := range rows {
		leads = append(leads, leadFromDB(row))
	}

	return leads, int(total), nil
}

type leadListFilters struct {
	status          pgtype.Text
	serviceType     pgtype.Text
	search          pgtype.Text
	firstName       pgtype.Text
	lastName        pgtype.Text
	phone           pgtype.Text
	email           pgtype.Text
	role            pgtype.Text
	street          pgtype.Text
	houseNumber     pgtype.Text
	zipCode         pgtype.Text
	city            pgtype.Text
	assignedAgentID pgtype.UUID
	createdAtFrom   pgtype.Timestamptz
	createdAtTo     pgtype.Timestamptz
}

func buildLeadListFilters(params ListParams) leadListFilters {
	return leadListFilters{
		status:          toPgText(params.Status),
		serviceType:     toPgText(params.ServiceType),
		search:          optionalSearchParam(params.Search),
		firstName:       optionalLikeParam(params.FirstName),
		lastName:        optionalLikeParam(params.LastName),
		phone:           optionalLikeParam(params.Phone),
		email:           optionalLikeParam(params.Email),
		role:            toPgText(params.Role),
		street:          optionalLikeParam(params.Street),
		houseNumber:     optionalLikeParam(params.HouseNumber),
		zipCode:         optionalLikeParam(params.ZipCode),
		city:            optionalLikeParam(params.City),
		assignedAgentID: toPgUUIDPtr(params.AssignedAgentID),
		createdAtFrom:   toPgTimestampPtr(params.CreatedAtFrom),
		createdAtTo:     toPgTimestampPtr(params.CreatedAtTo),
	}
}

func optionalLikeParam(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	text := "%" + *value + "%"
	return pgtype.Text{String: text, Valid: true}
}

func optionalSearchParam(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: "%" + value + "%", Valid: true}
}

func resolveLeadSortBy(sortBy string) (string, error) {
	if sortBy == "" {
		return "createdAt", nil
	}
	switch sortBy {
	case "createdAt", "firstName", "lastName", "phone", "email", "role", "street", "houseNumber", "zipCode", "city", "assignedAgentId":
		return sortBy, nil
	default:
		return "", apperr.BadRequest("invalid sort field")
	}
}

func resolveLeadSortOrder(sortOrder string) (string, error) {
	if sortOrder == "" {
		return "desc", nil
	}
	switch sortOrder {
	case "asc", "desc":
		return sortOrder, nil
	default:
		return "", apperr.BadRequest("invalid sort order")
	}
}

type HeatmapPoint struct {
	Latitude  float64
	Longitude float64
}

func (r *Repository) ListHeatmapPoints(ctx context.Context, organizationID uuid.UUID, startDate *time.Time, endDate *time.Time) ([]HeatmapPoint, error) {
	rows, err := r.queries.ListHeatmapPoints(ctx, leadsdb.ListHeatmapPointsParams{
		OrganizationID: toPgUUID(organizationID),
		Column2:        toPgTimestampPtr(startDate),
		Column3:        toPgTimestampPtr(endDate),
	})
	if err != nil {
		return nil, err
	}

	points := make([]HeatmapPoint, 0, len(rows))
	for _, row := range rows {
		points = append(points, HeatmapPoint{Latitude: row.Latitude.Float64, Longitude: row.Longitude.Float64})
	}

	return points, nil
}

type ActionItem struct {
	ID            uuid.UUID
	FirstName     string
	LastName      string
	UrgencyLevel  *string
	UrgencyReason *string
	CreatedAt     time.Time
}

type ActionItemListResult struct {
	Items []ActionItem
	Total int
}

func (r *Repository) ListActionItems(ctx context.Context, organizationID uuid.UUID, newLeadDays int, limit int, offset int) (ActionItemListResult, error) {
	total, err := r.queries.CountActionItems(ctx, leadsdb.CountActionItemsParams{
		OrganizationID: toPgUUID(organizationID),
		Column2:        int32(newLeadDays),
	})
	if err != nil {
		return ActionItemListResult{}, err
	}

	rows, err := r.queries.ListActionItems(ctx, leadsdb.ListActionItemsParams{
		OrganizationID: toPgUUID(organizationID),
		Column2:        int32(newLeadDays),
		Limit:          int32(limit),
		Offset:         int32(offset),
	})
	if err != nil {
		return ActionItemListResult{}, err
	}

	items := make([]ActionItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, ActionItem{
			ID:            uuid.UUID(row.ID.Bytes),
			FirstName:     row.ConsumerFirstName,
			LastName:      row.ConsumerLastName,
			UrgencyLevel:  emptyStringAsNil(row.UrgencyLevel),
			UrgencyReason: optionalString(row.UrgencyReason),
			CreatedAt:     row.CreatedAt.Time,
		})
	}

	return ActionItemListResult{Items: items, Total: int(total)}, nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error {
	result, err := r.queries.DeleteLead(ctx, leadsdb.DeleteLeadParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrNotFound
	}
	return nil
}

// ListRecentActivity returns the most recent org-wide activity by unioning
// lead activity, quote activity, and recent appointments.
// Events are clustered: sequential events with the same lead, event type, and category
// within a 15-minute window are grouped into a single row with a group_count.
func (r *Repository) ListRecentActivity(ctx context.Context, organizationID uuid.UUID, limit int, offset int) ([]ActivityFeedEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.queries.ListRecentActivity(ctx, leadsdb.ListRecentActivityParams{
		OrganizationID: toPgUUID(organizationID),
		Limit:          int32(limit),
		Offset:         int32(offset),
	})
	if err != nil {
		return nil, err
	}

	entries := make([]ActivityFeedEntry, 0, limit)
	for _, row := range rows {
		entries = append(entries, activityFeedEntryFromRecentRow(row))
	}

	return entries, nil
}

// ListUpcomingAppointments returns soon upcoming scheduled appointments for the org.
func (r *Repository) ListUpcomingAppointments(ctx context.Context, organizationID uuid.UUID, limit int) ([]ActivityFeedEntry, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := r.queries.ListUpcomingAppointments(ctx, leadsdb.ListUpcomingAppointmentsParams{
		OrganizationID: toPgUUID(organizationID),
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, err
	}

	entries := make([]ActivityFeedEntry, 0, limit)
	for _, row := range rows {
		entries = append(entries, activityFeedEntryFromUpcomingRow(row))
	}

	return entries, nil
}

func (r *Repository) BulkDelete(ctx context.Context, ids []uuid.UUID, organizationID uuid.UUID) (int, error) {
	result, err := r.queries.BulkDeleteLeads(ctx, leadsdb.BulkDeleteLeadsParams{Column1: toPgUUIDSlice(ids), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		return 0, err
	}
	return int(result), nil
}
