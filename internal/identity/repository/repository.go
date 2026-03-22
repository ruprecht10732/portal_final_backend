package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	identitydb "portal_final_backend/internal/identity/db"
	"portal_final_backend/platform/apperr"
)

var ErrNotFound = errors.New("not found")

const whatsappAccountJIDUniqueIndex = "idx_rac_organization_settings_whatsapp_account_jid_unique"

type DBTX = identitydb.DBTX

type Repository struct {
	pool    *pgxpool.Pool
	queries *identitydb.Queries
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: identitydb.New(pool)}
}

func (r *Repository) queriesFor(q DBTX) *identitydb.Queries {
	if q != nil {
		return identitydb.New(q)
	}
	return r.queries
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
	OrganizationID                                    uuid.UUID
	QuotePaymentDays                                  int
	QuoteValidDays                                    int
	OfferMarginBasisPoints                            int
	AIAutoDisqualifyJunk                              bool
	AIAutoDispatch                                    bool
	AIAutoEstimate                                    bool
	AIConfidenceGateEnabled                           bool
	AIAdaptiveReasoningEnabled                        bool
	AIExperienceMemoryEnabled                         bool
	AICouncilEnabled                                  bool
	AICouncilConsensusMode                            string
	WhatsAppToneOfVoice                               string
	CatalogGapThreshold                               int
	CatalogGapLookbackDays                            int
	PhotoAnalysisPreprocessingEnabled                 bool
	PhotoAnalysisOCRAssistEnabled                     bool
	PhotoAnalysisOCRAssistServiceTypes                []string
	PhotoAnalysisLensCorrectionEnabled                bool
	PhotoAnalysisLensCorrectionServiceTypes           []string
	PhotoAnalysisPerspectiveNormalizationEnabled      bool
	PhotoAnalysisPerspectiveNormalizationServiceTypes []string
	NotificationEmail                                 *string
	WhatsAppDeviceID                                  *string
	WhatsAppAccountJID                                *string
	WhatsAppPresence                                  string
	WhatsAppWelcomeDelayMinutes                       int
	WhatsAppDefaultReplyScenario                      string
	EmailDefaultReplyScenario                         string
	QuoteRelatedReplyScenario                         string
	AppointmentRelatedReplyScenario                   string
	DailyDigestEnabled                                bool
	SMTPHost                                          *string
	SMTPPort                                          *int
	SMTPUsername                                      *string
	SMTPPassword                                      *string
	SMTPFromEmail                                     *string
	SMTPFromName                                      *string
	CreatedAt                                         time.Time
	UpdatedAt                                         time.Time
}

type OrganizationSettingsUpdate struct {
	QuotePaymentDays                                  *int
	QuoteValidDays                                    *int
	OfferMarginBasisPoints                            *int
	AIAutoDisqualifyJunk                              *bool
	AIAutoDispatch                                    *bool
	AIAutoEstimate                                    *bool
	AIConfidenceGateEnabled                           *bool
	AIAdaptiveReasoningEnabled                        *bool
	AIExperienceMemoryEnabled                         *bool
	AICouncilEnabled                                  *bool
	AICouncilConsensusMode                            *string
	WhatsAppToneOfVoice                               *string
	CatalogGapThreshold                               *int
	CatalogGapLookbackDays                            *int
	PhotoAnalysisPreprocessingEnabled                 *bool
	PhotoAnalysisOCRAssistEnabled                     *bool
	PhotoAnalysisOCRAssistServiceTypes                *[]string
	PhotoAnalysisLensCorrectionEnabled                *bool
	PhotoAnalysisLensCorrectionServiceTypes           *[]string
	PhotoAnalysisPerspectiveNormalizationEnabled      *bool
	PhotoAnalysisPerspectiveNormalizationServiceTypes *[]string
	NotificationEmail                                 *string
	WhatsAppDeviceID                                  *string
	WhatsAppAccountJID                                *string
	WhatsAppPresence                                  *string
	WhatsAppWelcomeDelayMinutes                       *int
	WhatsAppDefaultReplyScenario                      *string
	EmailDefaultReplyScenario                         *string
	QuoteRelatedReplyScenario                         *string
	AppointmentRelatedReplyScenario                   *string
	DailyDigestEnabled                                *bool
}

type ReplyScenarioAnalyticsItem struct {
	Scenario    string
	SentCount   int
	EditedCount int
	LastUsedAt  *time.Time
}

type ReplyScenarioAnalytics struct {
	WhatsApp []ReplyScenarioAnalyticsItem
	Email    []ReplyScenarioAnalyticsItem
}

type OrganizationSMTPUpdate struct {
	SMTPHost      string
	SMTPPort      int
	SMTPUsername  string
	SMTPPassword  string
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

type organizationSnapshot struct {
	ID              pgtype.UUID
	Name            string
	Email           pgtype.Text
	Phone           pgtype.Text
	VatNumber       pgtype.Text
	KvkNumber       pgtype.Text
	AddressLine1    pgtype.Text
	AddressLine2    pgtype.Text
	PostalCode      pgtype.Text
	City            pgtype.Text
	Country         pgtype.Text
	LogoFileKey     pgtype.Text
	LogoFileName    pgtype.Text
	LogoContentType pgtype.Text
	LogoSizeBytes   pgtype.Int8
	CreatedBy       pgtype.UUID
	CreatedAt       pgtype.Timestamptz
	UpdatedAt       pgtype.Timestamptz
}

type settingsSnapshot struct {
	OrganizationID                                    pgtype.UUID
	QuotePaymentDays                                  int32
	QuoteValidDays                                    int32
	OfferMarginBasisPoints                            int32
	AIAutoDisqualifyJunk                              bool
	AIAutoDispatch                                    bool
	AIAutoEstimate                                    bool
	AIConfidenceGateEnabled                           bool
	AIAdaptiveReasoningEnabled                        bool
	AIExperienceMemoryEnabled                         bool
	AICouncilEnabled                                  bool
	AICouncilConsensusMode                            string
	WhatsAppToneOfVoice                               string
	CatalogGapThreshold                               int32
	CatalogGapLookbackDays                            int32
	PhotoAnalysisPreprocessingEnabled                 bool
	PhotoAnalysisOCRAssistEnabled                     bool
	PhotoAnalysisOCRAssistServiceTypes                []string
	PhotoAnalysisLensCorrectionEnabled                bool
	PhotoAnalysisLensCorrectionServiceTypes           []string
	PhotoAnalysisPerspectiveNormalizationEnabled      bool
	PhotoAnalysisPerspectiveNormalizationServiceTypes []string
	NotificationEmail                                 pgtype.Text
	WhatsAppDeviceID                                  pgtype.Text
	WhatsAppAccountJID                                pgtype.Text
	WhatsAppPresence                                  string
	WhatsAppWelcomeDelayMinutes                       int32
	WhatsAppDefaultReplyScenario                      string
	EmailDefaultReplyScenario                         string
	QuoteRelatedReplyScenario                         string
	AppointmentRelatedReplyScenario                   string
	DailyDigestEnabled                                bool
	SMTPHost                                          pgtype.Text
	SMTPPort                                          pgtype.Int4
	SMTPUsername                                      pgtype.Text
	SMTPPassword                                      pgtype.Text
	SMTPFromEmail                                     pgtype.Text
	SMTPFromName                                      pgtype.Text
	CreatedAt                                         pgtype.Timestamptz
	UpdatedAt                                         pgtype.Timestamptz
}

func (r *Repository) CreateOrganization(ctx context.Context, q DBTX, name string, createdBy uuid.UUID) (Organization, error) {
	row, err := r.queriesFor(q).CreateOrganization(ctx, identitydb.CreateOrganizationParams{
		Name:      name,
		CreatedBy: toPgUUID(createdBy),
	})
	if err != nil {
		return Organization{}, err
	}
	return organizationFromSnapshot(organizationSnapshot{
		ID:              row.ID,
		Name:            row.Name,
		Email:           row.Email,
		Phone:           row.Phone,
		VatNumber:       row.VatNumber,
		KvkNumber:       row.KvkNumber,
		AddressLine1:    row.AddressLine1,
		AddressLine2:    row.AddressLine2,
		PostalCode:      row.PostalCode,
		City:            row.City,
		Country:         row.Country,
		LogoFileKey:     row.LogoFileKey,
		LogoFileName:    row.LogoFileName,
		LogoContentType: row.LogoContentType,
		LogoSizeBytes:   row.LogoSizeBytes,
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}), nil
}

func (r *Repository) GetOrganization(ctx context.Context, organizationID uuid.UUID) (Organization, error) {
	row, err := r.queries.GetOrganization(ctx, toPgUUID(organizationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	if err != nil {
		return Organization{}, err
	}
	return organizationFromSnapshot(organizationSnapshot{
		ID:              row.ID,
		Name:            row.Name,
		Email:           row.Email,
		Phone:           row.Phone,
		VatNumber:       row.VatNumber,
		KvkNumber:       row.KvkNumber,
		AddressLine1:    row.AddressLine1,
		AddressLine2:    row.AddressLine2,
		PostalCode:      row.PostalCode,
		City:            row.City,
		Country:         row.Country,
		LogoFileKey:     row.LogoFileKey,
		LogoFileName:    row.LogoFileName,
		LogoContentType: row.LogoContentType,
		LogoSizeBytes:   row.LogoSizeBytes,
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}), nil
}

func (r *Repository) UpdateOrganizationProfile(ctx context.Context, organizationID uuid.UUID, update OrganizationProfileUpdate) (Organization, error) {
	row, err := r.queries.UpdateOrganizationProfile(ctx, identitydb.UpdateOrganizationProfileParams{
		ID:           toPgUUID(organizationID),
		Name:         toPgTextPtr(update.Name),
		Email:        toPgTextPtr(update.Email),
		Phone:        toPgTextPtr(update.Phone),
		VatNumber:    toPgTextPtr(update.VatNumber),
		KvkNumber:    toPgTextPtr(update.KvkNumber),
		AddressLine1: toPgTextPtr(update.AddressLine1),
		AddressLine2: toPgTextPtr(update.AddressLine2),
		PostalCode:   toPgTextPtr(update.PostalCode),
		City:         toPgTextPtr(update.City),
		Country:      toPgTextPtr(update.Country),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	if err != nil {
		return Organization{}, err
	}
	return organizationFromSnapshot(organizationSnapshot{
		ID:              row.ID,
		Name:            row.Name,
		Email:           row.Email,
		Phone:           row.Phone,
		VatNumber:       row.VatNumber,
		KvkNumber:       row.KvkNumber,
		AddressLine1:    row.AddressLine1,
		AddressLine2:    row.AddressLine2,
		PostalCode:      row.PostalCode,
		City:            row.City,
		Country:         row.Country,
		LogoFileKey:     row.LogoFileKey,
		LogoFileName:    row.LogoFileName,
		LogoContentType: row.LogoContentType,
		LogoSizeBytes:   row.LogoSizeBytes,
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}), nil
}

func (r *Repository) UpdateOrganizationLogo(ctx context.Context, organizationID uuid.UUID, logo OrganizationLogo) (Organization, error) {
	row, err := r.queries.UpdateOrganizationLogo(ctx, identitydb.UpdateOrganizationLogoParams{
		ID:              toPgUUID(organizationID),
		LogoFileKey:     toPgTextValue(logo.FileKey),
		LogoFileName:    toPgTextValue(logo.FileName),
		LogoContentType: toPgTextValue(logo.ContentType),
		LogoSizeBytes:   toPgInt8Value(logo.SizeBytes),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	if err != nil {
		return Organization{}, err
	}
	return organizationFromSnapshot(organizationSnapshot{
		ID:              row.ID,
		Name:            row.Name,
		Email:           row.Email,
		Phone:           row.Phone,
		VatNumber:       row.VatNumber,
		KvkNumber:       row.KvkNumber,
		AddressLine1:    row.AddressLine1,
		AddressLine2:    row.AddressLine2,
		PostalCode:      row.PostalCode,
		City:            row.City,
		Country:         row.Country,
		LogoFileKey:     row.LogoFileKey,
		LogoFileName:    row.LogoFileName,
		LogoContentType: row.LogoContentType,
		LogoSizeBytes:   row.LogoSizeBytes,
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}), nil
}

func (r *Repository) ClearOrganizationLogo(ctx context.Context, organizationID uuid.UUID) (Organization, error) {
	row, err := r.queries.ClearOrganizationLogo(ctx, toPgUUID(organizationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	if err != nil {
		return Organization{}, err
	}
	return organizationFromSnapshot(organizationSnapshot{
		ID:              row.ID,
		Name:            row.Name,
		Email:           row.Email,
		Phone:           row.Phone,
		VatNumber:       row.VatNumber,
		KvkNumber:       row.KvkNumber,
		AddressLine1:    row.AddressLine1,
		AddressLine2:    row.AddressLine2,
		PostalCode:      row.PostalCode,
		City:            row.City,
		Country:         row.Country,
		LogoFileKey:     row.LogoFileKey,
		LogoFileName:    row.LogoFileName,
		LogoContentType: row.LogoContentType,
		LogoSizeBytes:   row.LogoSizeBytes,
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}), nil
}

func (r *Repository) GetOrganizationSettings(ctx context.Context, organizationID uuid.UUID) (OrganizationSettings, error) {
	const query = `
		SELECT organization_id, quote_payment_days, quote_valid_days,
		       offer_margin_basis_points,
		       ai_auto_disqualify_junk, ai_auto_dispatch, ai_auto_estimate, ai_confidence_gate_enabled,
		       ai_adaptive_reasoning_enabled, ai_experience_memory_enabled, ai_council_enabled,
		       ai_council_consensus_mode, whatsapp_tone_of_voice,
		       catalog_gap_threshold, catalog_gap_lookback_days,
		       photo_analysis_preprocessing_enabled,
		       photo_analysis_ocr_assist_enabled,
		       photo_analysis_ocr_assist_service_types,
		       photo_analysis_lens_correction_enabled,
		       photo_analysis_lens_correction_service_types,
		       photo_analysis_perspective_normalization_enabled,
		       photo_analysis_perspective_normalization_service_types,
		       notification_email, whatsapp_device_id, whatsapp_account_jid, whatsapp_presence, whatsapp_welcome_delay_minutes,
		       whatsapp_default_reply_scenario, email_default_reply_scenario, quote_related_reply_scenario, appointment_related_reply_scenario,
		       daily_digest_enabled,
		       smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_email, smtp_from_name,
		       created_at, updated_at
		FROM RAC_organization_settings
		WHERE organization_id = $1`

	var row settingsSnapshot
	err := r.pool.QueryRow(ctx, query, organizationID).Scan(
		&row.OrganizationID,
		&row.QuotePaymentDays,
		&row.QuoteValidDays,
		&row.OfferMarginBasisPoints,
		&row.AIAutoDisqualifyJunk,
		&row.AIAutoDispatch,
		&row.AIAutoEstimate,
		&row.AIConfidenceGateEnabled,
		&row.AIAdaptiveReasoningEnabled,
		&row.AIExperienceMemoryEnabled,
		&row.AICouncilEnabled,
		&row.AICouncilConsensusMode,
		&row.WhatsAppToneOfVoice,
		&row.CatalogGapThreshold,
		&row.CatalogGapLookbackDays,
		&row.PhotoAnalysisPreprocessingEnabled,
		&row.PhotoAnalysisOCRAssistEnabled,
		&row.PhotoAnalysisOCRAssistServiceTypes,
		&row.PhotoAnalysisLensCorrectionEnabled,
		&row.PhotoAnalysisLensCorrectionServiceTypes,
		&row.PhotoAnalysisPerspectiveNormalizationEnabled,
		&row.PhotoAnalysisPerspectiveNormalizationServiceTypes,
		&row.NotificationEmail,
		&row.WhatsAppDeviceID,
		&row.WhatsAppAccountJID,
		&row.WhatsAppPresence,
		&row.WhatsAppWelcomeDelayMinutes,
		&row.WhatsAppDefaultReplyScenario,
		&row.EmailDefaultReplyScenario,
		&row.QuoteRelatedReplyScenario,
		&row.AppointmentRelatedReplyScenario,
		&row.DailyDigestEnabled,
		&row.SMTPHost,
		&row.SMTPPort,
		&row.SMTPUsername,
		&row.SMTPPassword,
		&row.SMTPFromEmail,
		&row.SMTPFromName,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrganizationSettings{
			OrganizationID:                                    organizationID,
			QuotePaymentDays:                                  7,
			QuoteValidDays:                                    14,
			OfferMarginBasisPoints:                            1000,
			AIAutoDisqualifyJunk:                              true,
			AIAutoDispatch:                                    false,
			AIAutoEstimate:                                    true,
			AIConfidenceGateEnabled:                           false,
			AIAdaptiveReasoningEnabled:                        true,
			AIExperienceMemoryEnabled:                         true,
			AICouncilEnabled:                                  true,
			AICouncilConsensusMode:                            "weighted",
			WhatsAppToneOfVoice:                               "warm, practical, and professional",
			CatalogGapThreshold:                               3,
			CatalogGapLookbackDays:                            30,
			PhotoAnalysisPreprocessingEnabled:                 true,
			PhotoAnalysisOCRAssistEnabled:                     false,
			PhotoAnalysisOCRAssistServiceTypes:                []string{},
			PhotoAnalysisLensCorrectionEnabled:                false,
			PhotoAnalysisLensCorrectionServiceTypes:           []string{},
			PhotoAnalysisPerspectiveNormalizationEnabled:      false,
			PhotoAnalysisPerspectiveNormalizationServiceTypes: []string{},
			WhatsAppDeviceID:                                  nil,
			WhatsAppAccountJID:                                nil,
			WhatsAppPresence:                                  "available",
			WhatsAppWelcomeDelayMinutes:                       2,
			WhatsAppDefaultReplyScenario:                      "generic",
			EmailDefaultReplyScenario:                         "generic",
			QuoteRelatedReplyScenario:                         "quote_reminder",
			AppointmentRelatedReplyScenario:                   "appointment_reminder",
			DailyDigestEnabled:                                true,
		}, nil
	}
	if err != nil {
		return OrganizationSettings{}, err
	}
	return organizationSettingsFromSnapshot(row), nil
}

func (r *Repository) UpsertOrganizationSettings(ctx context.Context, organizationID uuid.UUID, update OrganizationSettingsUpdate) (OrganizationSettings, error) {
	const query = `
		INSERT INTO RAC_organization_settings (
		  organization_id,
		  quote_payment_days,
		  quote_valid_days,
		  offer_margin_basis_points,
		  ai_auto_disqualify_junk,
		  ai_auto_dispatch,
		  ai_auto_estimate,
		  ai_confidence_gate_enabled,
		  ai_adaptive_reasoning_enabled,
		  ai_experience_memory_enabled,
		  ai_council_enabled,
		  ai_council_consensus_mode,
		  whatsapp_tone_of_voice,
		  catalog_gap_threshold,
		  catalog_gap_lookback_days,
		  photo_analysis_preprocessing_enabled,
		  photo_analysis_ocr_assist_enabled,
		  photo_analysis_ocr_assist_service_types,
		  photo_analysis_lens_correction_enabled,
		  photo_analysis_lens_correction_service_types,
		  photo_analysis_perspective_normalization_enabled,
		  photo_analysis_perspective_normalization_service_types,
		  notification_email,
		  whatsapp_device_id,
		  whatsapp_account_jid,
		  whatsapp_presence,
		  whatsapp_welcome_delay_minutes,
		  whatsapp_default_reply_scenario,
		  email_default_reply_scenario,
		  quote_related_reply_scenario,
		  appointment_related_reply_scenario,
		  daily_digest_enabled
		)
		VALUES (
		  $1,
		  COALESCE($2::int, 7),
		  COALESCE($3::int, 14),
		  COALESCE($4::int, 1000),
		  COALESCE($5::boolean, true),
		  COALESCE($6::boolean, false),
		  COALESCE($7::boolean, true),
		  COALESCE($8::boolean, false),
		  COALESCE($9::boolean, true),
		  COALESCE($10::boolean, true),
		  COALESCE($11::boolean, true),
		  COALESCE(NULLIF($12::text, ''), 'weighted'),
		  COALESCE(NULLIF($13::text, ''), 'warm, practical, and professional'),
		  COALESCE($14::int, 3),
		  COALESCE($15::int, 30),
		  COALESCE($16::boolean, true),
		  COALESCE($17::boolean, false),
		  COALESCE($18::text[], '{}'::text[]),
		  COALESCE($19::boolean, false),
		  COALESCE($20::text[], '{}'::text[]),
		  COALESCE($21::boolean, false),
		  COALESCE($22::text[], '{}'::text[]),
		  NULLIF($23::text, ''),
		  NULLIF($24::text, ''),
		  NULLIF($25::text, ''),
		  COALESCE(NULLIF($26::text, ''), 'available'),
		  COALESCE($27::int, 2),
		  COALESCE(NULLIF($28::text, ''), 'generic'),
		  COALESCE(NULLIF($29::text, ''), 'generic'),
		  COALESCE(NULLIF($30::text, ''), 'quote_reminder'),
		  COALESCE(NULLIF($31::text, ''), 'appointment_reminder'),
		  COALESCE($32::boolean, true)
		)
		ON CONFLICT (organization_id) DO UPDATE SET
		  quote_payment_days = COALESCE($2::int, RAC_organization_settings.quote_payment_days),
		  quote_valid_days = COALESCE($3::int, RAC_organization_settings.quote_valid_days),
		  offer_margin_basis_points = COALESCE($4::int, RAC_organization_settings.offer_margin_basis_points),
		  ai_auto_disqualify_junk = COALESCE($5::boolean, RAC_organization_settings.ai_auto_disqualify_junk),
		  ai_auto_dispatch = COALESCE($6::boolean, RAC_organization_settings.ai_auto_dispatch),
		  ai_auto_estimate = COALESCE($7::boolean, RAC_organization_settings.ai_auto_estimate),
		  ai_confidence_gate_enabled = COALESCE($8::boolean, RAC_organization_settings.ai_confidence_gate_enabled),
		  ai_adaptive_reasoning_enabled = COALESCE($9::boolean, RAC_organization_settings.ai_adaptive_reasoning_enabled),
		  ai_experience_memory_enabled = COALESCE($10::boolean, RAC_organization_settings.ai_experience_memory_enabled),
		  ai_council_enabled = COALESCE($11::boolean, RAC_organization_settings.ai_council_enabled),
		  ai_council_consensus_mode = COALESCE(NULLIF($12::text, ''), RAC_organization_settings.ai_council_consensus_mode),
		  whatsapp_tone_of_voice = COALESCE(NULLIF($13::text, ''), RAC_organization_settings.whatsapp_tone_of_voice),
		  catalog_gap_threshold = COALESCE($14::int, RAC_organization_settings.catalog_gap_threshold),
		  catalog_gap_lookback_days = COALESCE($15::int, RAC_organization_settings.catalog_gap_lookback_days),
		  photo_analysis_preprocessing_enabled = COALESCE($16::boolean, RAC_organization_settings.photo_analysis_preprocessing_enabled),
		  photo_analysis_ocr_assist_enabled = COALESCE($17::boolean, RAC_organization_settings.photo_analysis_ocr_assist_enabled),
		  photo_analysis_ocr_assist_service_types = COALESCE($18::text[], RAC_organization_settings.photo_analysis_ocr_assist_service_types),
		  photo_analysis_lens_correction_enabled = COALESCE($19::boolean, RAC_organization_settings.photo_analysis_lens_correction_enabled),
		  photo_analysis_lens_correction_service_types = COALESCE($20::text[], RAC_organization_settings.photo_analysis_lens_correction_service_types),
		  photo_analysis_perspective_normalization_enabled = COALESCE($21::boolean, RAC_organization_settings.photo_analysis_perspective_normalization_enabled),
		  photo_analysis_perspective_normalization_service_types = COALESCE($22::text[], RAC_organization_settings.photo_analysis_perspective_normalization_service_types),
		  notification_email = CASE WHEN $23::text IS NULL THEN RAC_organization_settings.notification_email ELSE NULLIF($23::text, '') END,
		  whatsapp_device_id = CASE WHEN $24::text IS NULL THEN RAC_organization_settings.whatsapp_device_id ELSE NULLIF($24::text, '') END,
		  whatsapp_account_jid = CASE WHEN $25::text IS NULL THEN RAC_organization_settings.whatsapp_account_jid ELSE NULLIF($25::text, '') END,
		  whatsapp_presence = COALESCE(NULLIF($26::text, ''), RAC_organization_settings.whatsapp_presence),
		  whatsapp_welcome_delay_minutes = COALESCE($27::int, RAC_organization_settings.whatsapp_welcome_delay_minutes),
		  whatsapp_default_reply_scenario = COALESCE(NULLIF($28::text, ''), RAC_organization_settings.whatsapp_default_reply_scenario),
		  email_default_reply_scenario = COALESCE(NULLIF($29::text, ''), RAC_organization_settings.email_default_reply_scenario),
		  quote_related_reply_scenario = COALESCE(NULLIF($30::text, ''), RAC_organization_settings.quote_related_reply_scenario),
		  appointment_related_reply_scenario = COALESCE(NULLIF($31::text, ''), RAC_organization_settings.appointment_related_reply_scenario),
		  daily_digest_enabled = COALESCE($32::boolean, RAC_organization_settings.daily_digest_enabled),
		  updated_at = now()
		RETURNING organization_id, quote_payment_days, quote_valid_days,
		  offer_margin_basis_points,
		  ai_auto_disqualify_junk, ai_auto_dispatch, ai_auto_estimate, ai_confidence_gate_enabled,
		  ai_adaptive_reasoning_enabled, ai_experience_memory_enabled, ai_council_enabled,
		  ai_council_consensus_mode, whatsapp_tone_of_voice,
		  catalog_gap_threshold, catalog_gap_lookback_days,
		  photo_analysis_preprocessing_enabled,
		  photo_analysis_ocr_assist_enabled,
		  photo_analysis_ocr_assist_service_types,
		  photo_analysis_lens_correction_enabled,
		  photo_analysis_lens_correction_service_types,
		  photo_analysis_perspective_normalization_enabled,
		  photo_analysis_perspective_normalization_service_types,
		  notification_email, whatsapp_device_id, whatsapp_account_jid, whatsapp_presence, whatsapp_welcome_delay_minutes,
		  whatsapp_default_reply_scenario, email_default_reply_scenario, quote_related_reply_scenario, appointment_related_reply_scenario,
		  daily_digest_enabled,
		  smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_email, smtp_from_name,
		  created_at, updated_at`

	var row settingsSnapshot
	err := r.pool.QueryRow(ctx, query,
		organizationID,
		update.QuotePaymentDays,
		update.QuoteValidDays,
		update.OfferMarginBasisPoints,
		update.AIAutoDisqualifyJunk,
		update.AIAutoDispatch,
		update.AIAutoEstimate,
		update.AIConfidenceGateEnabled,
		update.AIAdaptiveReasoningEnabled,
		update.AIExperienceMemoryEnabled,
		update.AICouncilEnabled,
		normalizedTextValue(update.AICouncilConsensusMode),
		normalizedTextValue(update.WhatsAppToneOfVoice),
		update.CatalogGapThreshold,
		update.CatalogGapLookbackDays,
		update.PhotoAnalysisPreprocessingEnabled,
		update.PhotoAnalysisOCRAssistEnabled,
		normalizedStringSlice(update.PhotoAnalysisOCRAssistServiceTypes),
		update.PhotoAnalysisLensCorrectionEnabled,
		normalizedStringSlice(update.PhotoAnalysisLensCorrectionServiceTypes),
		update.PhotoAnalysisPerspectiveNormalizationEnabled,
		normalizedStringSlice(update.PhotoAnalysisPerspectiveNormalizationServiceTypes),
		normalizedTextValue(update.NotificationEmail),
		normalizedTextValue(update.WhatsAppDeviceID),
		normalizedTextValue(update.WhatsAppAccountJID),
		normalizedTextValue(update.WhatsAppPresence),
		update.WhatsAppWelcomeDelayMinutes,
		normalizedTextValue(update.WhatsAppDefaultReplyScenario),
		normalizedTextValue(update.EmailDefaultReplyScenario),
		normalizedTextValue(update.QuoteRelatedReplyScenario),
		normalizedTextValue(update.AppointmentRelatedReplyScenario),
		update.DailyDigestEnabled,
	).Scan(
		&row.OrganizationID,
		&row.QuotePaymentDays,
		&row.QuoteValidDays,
		&row.OfferMarginBasisPoints,
		&row.AIAutoDisqualifyJunk,
		&row.AIAutoDispatch,
		&row.AIAutoEstimate,
		&row.AIConfidenceGateEnabled,
		&row.AIAdaptiveReasoningEnabled,
		&row.AIExperienceMemoryEnabled,
		&row.AICouncilEnabled,
		&row.AICouncilConsensusMode,
		&row.WhatsAppToneOfVoice,
		&row.CatalogGapThreshold,
		&row.CatalogGapLookbackDays,
		&row.PhotoAnalysisPreprocessingEnabled,
		&row.PhotoAnalysisOCRAssistEnabled,
		&row.PhotoAnalysisOCRAssistServiceTypes,
		&row.PhotoAnalysisLensCorrectionEnabled,
		&row.PhotoAnalysisLensCorrectionServiceTypes,
		&row.PhotoAnalysisPerspectiveNormalizationEnabled,
		&row.PhotoAnalysisPerspectiveNormalizationServiceTypes,
		&row.NotificationEmail,
		&row.WhatsAppDeviceID,
		&row.WhatsAppAccountJID,
		&row.WhatsAppPresence,
		&row.WhatsAppWelcomeDelayMinutes,
		&row.WhatsAppDefaultReplyScenario,
		&row.EmailDefaultReplyScenario,
		&row.QuoteRelatedReplyScenario,
		&row.AppointmentRelatedReplyScenario,
		&row.DailyDigestEnabled,
		&row.SMTPHost,
		&row.SMTPPort,
		&row.SMTPUsername,
		&row.SMTPPassword,
		&row.SMTPFromEmail,
		&row.SMTPFromName,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if isDuplicateWhatsAppAccountJID(err) {
		return OrganizationSettings{}, apperr.Validation("this WhatsApp account is already linked to another organization")
	}
	if err != nil {
		return OrganizationSettings{}, err
	}
	return organizationSettingsFromSnapshot(row), nil
}

func (r *Repository) GetOrganizationIDByWhatsAppAccountJID(ctx context.Context, accountJID string) (uuid.UUID, error) {
	trimmed := strings.TrimSpace(accountJID)
	if trimmed == "" {
		return uuid.UUID{}, ErrNotFound
	}

	const query = `
		SELECT organization_id
		FROM RAC_organization_settings
		WHERE whatsapp_account_jid = $1
		LIMIT 1`

	var organizationID uuid.UUID
	if err := r.pool.QueryRow(ctx, query, trimmed).Scan(&organizationID); errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, ErrNotFound
	} else if err != nil {
		return uuid.UUID{}, err
	}

	return organizationID, nil
}

func isDuplicateWhatsAppAccountJID(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != "23505" {
		return false
	}
	return pgErr.ConstraintName == whatsappAccountJIDUniqueIndex
}

func (r *Repository) UpsertOrganizationSMTP(ctx context.Context, organizationID uuid.UUID, update OrganizationSMTPUpdate) (OrganizationSettings, error) {
	row, err := r.queries.UpsertOrganizationSMTP(ctx, identitydb.UpsertOrganizationSMTPParams{
		OrganizationID: toPgUUID(organizationID),
		SmtpHost:       toPgTextValue(update.SMTPHost),
		SmtpPort:       toPgInt4Value(update.SMTPPort),
		SmtpUsername:   toPgTextValue(update.SMTPUsername),
		SmtpPassword:   toPgTextValue(update.SMTPPassword),
		SmtpFromEmail:  toPgTextValue(update.SMTPFromEmail),
		SmtpFromName:   toPgTextValue(update.SMTPFromName),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return OrganizationSettings{}, ErrNotFound
	}
	if err != nil {
		return OrganizationSettings{}, err
	}
	return organizationSettingsFromSnapshot(settingsSnapshot{
		OrganizationID:                                    row.OrganizationID,
		QuotePaymentDays:                                  row.QuotePaymentDays,
		QuoteValidDays:                                    row.QuoteValidDays,
		AIAutoDisqualifyJunk:                              row.AiAutoDisqualifyJunk,
		AIAutoDispatch:                                    row.AiAutoDispatch,
		AIAutoEstimate:                                    row.AiAutoEstimate,
		AIConfidenceGateEnabled:                           row.AiConfidenceGateEnabled,
		AIAdaptiveReasoningEnabled:                        row.AiAdaptiveReasoningEnabled,
		AIExperienceMemoryEnabled:                         row.AiExperienceMemoryEnabled,
		AICouncilEnabled:                                  row.AiCouncilEnabled,
		AICouncilConsensusMode:                            row.AiCouncilConsensusMode,
		WhatsAppToneOfVoice:                               row.WhatsappToneOfVoice,
		CatalogGapThreshold:                               row.CatalogGapThreshold,
		CatalogGapLookbackDays:                            row.CatalogGapLookbackDays,
		PhotoAnalysisPreprocessingEnabled:                 row.PhotoAnalysisPreprocessingEnabled,
		PhotoAnalysisOCRAssistEnabled:                     row.PhotoAnalysisOcrAssistEnabled,
		PhotoAnalysisOCRAssistServiceTypes:                row.PhotoAnalysisOcrAssistServiceTypes,
		PhotoAnalysisLensCorrectionEnabled:                row.PhotoAnalysisLensCorrectionEnabled,
		PhotoAnalysisLensCorrectionServiceTypes:           row.PhotoAnalysisLensCorrectionServiceTypes,
		PhotoAnalysisPerspectiveNormalizationEnabled:      row.PhotoAnalysisPerspectiveNormalizationEnabled,
		PhotoAnalysisPerspectiveNormalizationServiceTypes: row.PhotoAnalysisPerspectiveNormalizationServiceTypes,
		NotificationEmail:                                 row.NotificationEmail,
		WhatsAppDeviceID:                                  row.WhatsappDeviceID,
		WhatsAppAccountJID:                                row.WhatsappAccountJid,
		WhatsAppPresence:                                  row.WhatsappPresence,
		WhatsAppWelcomeDelayMinutes:                       row.WhatsappWelcomeDelayMinutes,
		SMTPHost:                                          row.SmtpHost,
		SMTPPort:                                          row.SmtpPort,
		SMTPUsername:                                      row.SmtpUsername,
		SMTPPassword:                                      row.SmtpPassword,
		SMTPFromEmail:                                     row.SmtpFromEmail,
		SMTPFromName:                                      row.SmtpFromName,
		CreatedAt:                                         row.CreatedAt,
		UpdatedAt:                                         row.UpdatedAt,
	}), nil
}

func (r *Repository) ClearOrganizationSMTP(ctx context.Context, organizationID uuid.UUID) error {
	return r.queries.ClearOrganizationSMTP(ctx, toPgUUID(organizationID))
}

func (r *Repository) AddMember(ctx context.Context, q DBTX, organizationID, userID uuid.UUID) error {
	return r.queriesFor(q).AddMember(ctx, identitydb.AddMemberParams{
		OrganizationID: toPgUUID(organizationID),
		UserID:         toPgUUID(userID),
	})
}

func (r *Repository) GetUserOrganizationID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	orgID, err := r.queries.GetUserOrganizationID(ctx, toPgUUID(userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, ErrNotFound
	}
	if err != nil {
		return uuid.UUID{}, err
	}
	return uuidFromPg(orgID), nil
}

func (r *Repository) CreateInvite(ctx context.Context, organizationID uuid.UUID, email, tokenHash string, expiresAt time.Time, createdBy uuid.UUID) (Invite, error) {
	row, err := r.queries.CreateInvite(ctx, identitydb.CreateInviteParams{
		OrganizationID: toPgUUID(organizationID),
		Email:          email,
		TokenHash:      tokenHash,
		ExpiresAt:      toPgTimestamp(expiresAt),
		CreatedBy:      toPgUUID(createdBy),
	})
	if err != nil {
		return Invite{}, err
	}
	return inviteFromModel(row), nil
}

func (r *Repository) GetInviteByToken(ctx context.Context, tokenHash string) (Invite, error) {
	row, err := r.queries.GetInviteByToken(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	if err != nil {
		return Invite{}, err
	}
	return inviteFromModel(row), nil
}

func (r *Repository) UseInvite(ctx context.Context, q DBTX, inviteID, usedBy uuid.UUID) error {
	return r.queriesFor(q).UseInvite(ctx, identitydb.UseInviteParams{
		ID:     toPgUUID(inviteID),
		UsedBy: toPgUUID(usedBy),
	})
}

func (r *Repository) ListInvites(ctx context.Context, organizationID uuid.UUID) ([]Invite, error) {
	rows, err := r.queries.ListInvites(ctx, toPgUUID(organizationID))
	if err != nil {
		return nil, err
	}
	invites := make([]Invite, 0, len(rows))
	for _, row := range rows {
		invites = append(invites, inviteFromModel(row))
	}
	return invites, nil
}

func (r *Repository) UpdateInvite(ctx context.Context, organizationID uuid.UUID, inviteID uuid.UUID, email *string, tokenHash *string, expiresAt *time.Time) (Invite, error) {
	row, err := r.queries.UpdateInvite(ctx, identitydb.UpdateInviteParams{
		ID:             toPgUUID(inviteID),
		OrganizationID: toPgUUID(organizationID),
		Email:          toPgTextPtr(email),
		TokenHash:      toPgTextPtr(tokenHash),
		ExpiresAt:      toPgTimestampPtr(expiresAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	if err != nil {
		return Invite{}, err
	}
	return inviteFromModel(row), nil
}

func (r *Repository) RevokeInvite(ctx context.Context, organizationID, inviteID uuid.UUID) (Invite, error) {
	row, err := r.queries.RevokeInvite(ctx, identitydb.RevokeInviteParams{
		ID:             toPgUUID(inviteID),
		OrganizationID: toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	if err != nil {
		return Invite{}, err
	}
	return inviteFromModel(row), nil
}

func organizationFromSnapshot(snapshot organizationSnapshot) Organization {
	return Organization{
		ID:              uuidFromPg(snapshot.ID),
		Name:            snapshot.Name,
		Email:           optionalString(snapshot.Email),
		Phone:           optionalString(snapshot.Phone),
		VatNumber:       optionalString(snapshot.VatNumber),
		KvkNumber:       optionalString(snapshot.KvkNumber),
		AddressLine1:    optionalString(snapshot.AddressLine1),
		AddressLine2:    optionalString(snapshot.AddressLine2),
		PostalCode:      optionalString(snapshot.PostalCode),
		City:            optionalString(snapshot.City),
		Country:         optionalString(snapshot.Country),
		LogoFileKey:     optionalString(snapshot.LogoFileKey),
		LogoFileName:    optionalString(snapshot.LogoFileName),
		LogoContentType: optionalString(snapshot.LogoContentType),
		LogoSizeBytes:   optionalInt64(snapshot.LogoSizeBytes),
		CreatedBy:       uuidFromPg(snapshot.CreatedBy),
		CreatedAt:       timeFromPg(snapshot.CreatedAt),
		UpdatedAt:       timeFromPg(snapshot.UpdatedAt),
	}
}

func organizationSettingsFromSnapshot(snapshot settingsSnapshot) OrganizationSettings {
	return OrganizationSettings{
		OrganizationID:                                    uuidFromPg(snapshot.OrganizationID),
		QuotePaymentDays:                                  int(snapshot.QuotePaymentDays),
		QuoteValidDays:                                    int(snapshot.QuoteValidDays),
		OfferMarginBasisPoints:                            int(snapshot.OfferMarginBasisPoints),
		AIAutoDisqualifyJunk:                              snapshot.AIAutoDisqualifyJunk,
		AIAutoDispatch:                                    snapshot.AIAutoDispatch,
		AIAutoEstimate:                                    snapshot.AIAutoEstimate,
		AIConfidenceGateEnabled:                           snapshot.AIConfidenceGateEnabled,
		AIAdaptiveReasoningEnabled:                        snapshot.AIAdaptiveReasoningEnabled,
		AIExperienceMemoryEnabled:                         snapshot.AIExperienceMemoryEnabled,
		AICouncilEnabled:                                  snapshot.AICouncilEnabled,
		AICouncilConsensusMode:                            snapshot.AICouncilConsensusMode,
		WhatsAppToneOfVoice:                               snapshot.WhatsAppToneOfVoice,
		CatalogGapThreshold:                               int(snapshot.CatalogGapThreshold),
		CatalogGapLookbackDays:                            int(snapshot.CatalogGapLookbackDays),
		PhotoAnalysisPreprocessingEnabled:                 snapshot.PhotoAnalysisPreprocessingEnabled,
		PhotoAnalysisOCRAssistEnabled:                     snapshot.PhotoAnalysisOCRAssistEnabled,
		PhotoAnalysisOCRAssistServiceTypes:                cloneStrings(snapshot.PhotoAnalysisOCRAssistServiceTypes),
		PhotoAnalysisLensCorrectionEnabled:                snapshot.PhotoAnalysisLensCorrectionEnabled,
		PhotoAnalysisLensCorrectionServiceTypes:           cloneStrings(snapshot.PhotoAnalysisLensCorrectionServiceTypes),
		PhotoAnalysisPerspectiveNormalizationEnabled:      snapshot.PhotoAnalysisPerspectiveNormalizationEnabled,
		PhotoAnalysisPerspectiveNormalizationServiceTypes: cloneStrings(snapshot.PhotoAnalysisPerspectiveNormalizationServiceTypes),
		NotificationEmail:                                 optionalString(snapshot.NotificationEmail),
		WhatsAppDeviceID:                                  optionalString(snapshot.WhatsAppDeviceID),
		WhatsAppAccountJID:                                optionalString(snapshot.WhatsAppAccountJID),
		WhatsAppPresence:                                  normalizePresenceSnapshot(snapshot.WhatsAppPresence),
		WhatsAppWelcomeDelayMinutes:                       int(snapshot.WhatsAppWelcomeDelayMinutes),
		WhatsAppDefaultReplyScenario:                      strings.TrimSpace(snapshot.WhatsAppDefaultReplyScenario),
		EmailDefaultReplyScenario:                         strings.TrimSpace(snapshot.EmailDefaultReplyScenario),
		QuoteRelatedReplyScenario:                         strings.TrimSpace(snapshot.QuoteRelatedReplyScenario),
		AppointmentRelatedReplyScenario:                   strings.TrimSpace(snapshot.AppointmentRelatedReplyScenario),
		DailyDigestEnabled:                                snapshot.DailyDigestEnabled,
		SMTPHost:                                          optionalString(snapshot.SMTPHost),
		SMTPPort:                                          optionalInt(snapshot.SMTPPort),
		SMTPUsername:                                      optionalString(snapshot.SMTPUsername),
		SMTPPassword:                                      optionalString(snapshot.SMTPPassword),
		SMTPFromEmail:                                     optionalString(snapshot.SMTPFromEmail),
		SMTPFromName:                                      optionalString(snapshot.SMTPFromName),
		CreatedAt:                                         timeFromPg(snapshot.CreatedAt),
		UpdatedAt:                                         timeFromPg(snapshot.UpdatedAt),
	}
}

func inviteFromModel(row identitydb.RacOrganizationInvite) Invite {
	return Invite{
		ID:             uuidFromPg(row.ID),
		OrganizationID: uuidFromPg(row.OrganizationID),
		Email:          row.Email,
		TokenHash:      row.TokenHash,
		ExpiresAt:      timeFromPg(row.ExpiresAt),
		CreatedBy:      uuidFromPg(row.CreatedBy),
		CreatedAt:      timeFromPg(row.CreatedAt),
		UsedAt:         optionalTime(row.UsedAt),
		UsedBy:         optionalUUID(row.UsedBy),
	}
}

func toPgUUID(value uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: value, Valid: value != uuid.Nil}
}

func toPgUUIDPtr(value *uuid.UUID) pgtype.UUID {
	if value == nil || *value == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *value, Valid: true}
}

func toPgTextValue(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

func toPgTextPtr(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func normalizePresenceSnapshot(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "unavailable" {
		return "unavailable"
	}
	return "available"
}

func toPgInt4Value(value int) pgtype.Int4 {
	return pgtype.Int4{Int32: int32(value), Valid: true}
}

func toPgInt4Ptr(value *int) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*value), Valid: true}
}

func toPgInt8Value(value int64) pgtype.Int8 {
	return pgtype.Int8{Int64: value, Valid: true}
}

func toPgTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func toPgTimestampPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func uuidFromPg(value pgtype.UUID) uuid.UUID {
	if !value.Valid {
		return uuid.Nil
	}
	return uuid.UUID(value.Bytes)
}

func optionalUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	v := uuid.UUID(value.Bytes)
	return &v
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	v := value.String
	return &v
}

func optionalInt(value pgtype.Int4) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int32)
	return &v
}

func optionalInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}

func normalizedTextValue(value *string) any {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return ""
	}
	return trimmed
}

func normalizedStringSlice(values *[]string) any {
	if values == nil {
		return nil
	}
	return toStringSlice(values)
}

func toStringSlice(values *[]string) []string {
	if values == nil {
		return nil
	}
	return cleanStrings(*values)
}

func cleanStrings(values []string) []string {
	cleaned := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		cleaned = append(cleaned, trimmed)
	}
	return cleaned
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	v := value.Time
	return &v
}

func timeFromPg(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}
