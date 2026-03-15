// Package management handles lead CRUD operations.
// This is a vertically sliced feature package containing service logic
// for creating, reading, updating, and deleting RAC_leads.
package management

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	appointmentstransport "portal_final_backend/internal/appointments/transport"
	"portal_final_backend/internal/auth/token"
	"portal_final_backend/internal/events"
	identityrepo "portal_final_backend/internal/identity/repository"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/scoring"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/internal/maps"
	"portal_final_backend/internal/notification/inapp"
	quotestransport "portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/phone"

	"github.com/google/uuid"
)

const (
	leadNotFoundMsg               = "lead not found"
	leadServiceNotFoundMsg        = "lead service not found"
	energyLabelRefreshInterval    = 30 * 24 * time.Hour
	leadEnrichmentRefreshInterval = 365 * 24 * time.Hour
)

// Repository defines the data access interface needed by the management service.
// This is a consumer-driven interface - only what management needs.
type Repository interface {
	repository.LeadReader
	repository.LeadWriter
	repository.LeadViewTracker
	repository.ActivityLogger
	repository.LeadServiceReader
	repository.LeadServiceWriter
	repository.NoteStore
	repository.AIAnalysisStore
	repository.PhotoAnalysisStore
	repository.QuotePriceReader
	repository.MetricsReader
	repository.TimelineEventStore
	repository.ActivityFeedReader
	repository.FeedReactionStore
	repository.FeedCommentStore
	repository.OrgMemberReader
	UpdateEnergyLabel(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params repository.UpdateEnergyLabelParams) error
	UpdateLeadEnrichment(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params repository.UpdateLeadEnrichmentParams) error
}

// Service handles lead management operations (CRUD).
type Service struct {
	repo                   Repository
	eventBus               events.Bus
	maps                   *maps.Service
	inAppService           *inapp.Service
	leadDetailQuotes       LeadDetailQuotesReader
	leadDetailAppointments LeadDetailAppointmentsReader
	leadDetailWorkflow     LeadDetailWorkflowContextReader
	timelineWhatsAppSender TimelineWhatsAppSender
	partnerPhoneResolver   PartnerPhoneResolver
	acceptedQuoteUpdater   AcceptedQuoteUpdater
	energyEnricher         ports.EnergyLabelEnricher
	leadEnricher           ports.LeadEnricher
	scorer                 *scoring.Service
	workflowOverrideWriter LeadWorkflowOverrideWriter
}

type AcceptedQuoteUpdater interface {
	AddExtraWorkToQuote(ctx context.Context, quoteID uuid.UUID, organizationID uuid.UUID, actorID uuid.UUID, amountCents int64, notes *string) error
}

type LeadDetailQuotesReader interface {
	ListLeadQuotes(ctx context.Context, tenantID uuid.UUID, leadID uuid.UUID) ([]quotestransport.QuoteResponse, error)
}

type LeadDetailAppointmentsReader interface {
	ListLeadAppointments(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, leadID uuid.UUID) ([]appointmentstransport.AppointmentResponse, error)
}

type LeadDetailWorkflowContextReader interface {
	GetLeadWorkflowContext(ctx context.Context, tenantID uuid.UUID, leadID uuid.UUID, leadSource *string, leadServiceType *string, pipelineStage *string) (*transport.LeadDetailWorkflowContext, error)
}

type LeadWorkflowOverrideWriter interface {
	UpsertLeadWorkflowOverride(ctx context.Context, upsert identityrepo.LeadWorkflowOverrideUpsert) (identityrepo.LeadWorkflowOverride, error)
}

type TimelineWhatsAppSendParams struct {
	OrgID       uuid.UUID
	LeadID      uuid.UUID
	ServiceID   *uuid.UUID
	PhoneNumber string
	Message     string
	Category    string
	Audience    string
	Summary     string
	ActorType   string
	ActorName   string
	Metadata    map[string]any
}

type TimelineWhatsAppSender interface {
	SendLeadWhatsApp(ctx context.Context, params TimelineWhatsAppSendParams) error
}

type TimelineWhatsAppSenderFunc func(ctx context.Context, params TimelineWhatsAppSendParams) error

func (f TimelineWhatsAppSenderFunc) SendLeadWhatsApp(ctx context.Context, params TimelineWhatsAppSendParams) error {
	return f(ctx, params)
}

type PartnerPhoneResolver interface {
	ResolvePartnerPhone(ctx context.Context, organizationID uuid.UUID, partnerID uuid.UUID) (string, error)
}

type PartnerPhoneResolverFunc func(ctx context.Context, organizationID uuid.UUID, partnerID uuid.UUID) (string, error)

func (f PartnerPhoneResolverFunc) ResolvePartnerPhone(ctx context.Context, organizationID uuid.UUID, partnerID uuid.UUID) (string, error) {
	return f(ctx, organizationID, partnerID)
}

// New creates a new lead management service.
func New(repo Repository, eventBus events.Bus, mapsService *maps.Service) *Service {
	return &Service{repo: repo, eventBus: eventBus, maps: mapsService}
}

// SetEnergyLabelEnricher sets the energy label enricher for lead enrichment.
// This is called after module initialization to break circular dependencies.
func (s *Service) SetEnergyLabelEnricher(enricher ports.EnergyLabelEnricher) {
	s.energyEnricher = enricher
}

// SetLeadEnricher sets the lead enrichment provider.
func (s *Service) SetLeadEnricher(enricher ports.LeadEnricher) {
	s.leadEnricher = enricher
}

// SetLeadScorer sets the lead scoring service.
func (s *Service) SetLeadScorer(scorer *scoring.Service) {
	s.scorer = scorer
}

func (s *Service) SetWorkflowOverrideWriter(writer LeadWorkflowOverrideWriter) {
	s.workflowOverrideWriter = writer
}

// SetInAppNotificationService injects the in-app notification service.
func (s *Service) SetInAppNotificationService(svc *inapp.Service) {
	s.inAppService = svc
}

func (s *Service) SetTimelineWhatsAppSender(sender TimelineWhatsAppSender) {
	s.timelineWhatsAppSender = sender
}

func (s *Service) SetPartnerPhoneResolver(resolver PartnerPhoneResolver) {
	s.partnerPhoneResolver = resolver
}

func (s *Service) SetAcceptedQuoteUpdater(updater AcceptedQuoteUpdater) {
	s.acceptedQuoteUpdater = updater
}

func (s *Service) SetLeadDetailQuotesReader(reader LeadDetailQuotesReader) {
	s.leadDetailQuotes = reader
}

func (s *Service) SetLeadDetailAppointmentsReader(reader LeadDetailAppointmentsReader) {
	s.leadDetailAppointments = reader
}

func (s *Service) SetLeadDetailWorkflowContextReader(reader LeadDetailWorkflowContextReader) {
	s.leadDetailWorkflow = reader
}

// Create creates a new lead.
func (s *Service) Create(ctx context.Context, req transport.CreateLeadRequest, tenantID uuid.UUID) (transport.LeadResponse, error) {
	req.Phone = phone.NormalizeE164(req.Phone)

	whatsAppOptedIn := true
	if req.WhatsAppOptedIn != nil {
		whatsAppOptedIn = *req.WhatsAppOptedIn
	}

	params := repository.CreateLeadParams{
		OrganizationID:     tenantID,
		ConsumerFirstName:  req.FirstName,
		ConsumerLastName:   req.LastName,
		ConsumerPhone:      req.Phone,
		ConsumerRole:       string(req.ConsumerRole),
		AddressStreet:      req.Street,
		AddressHouseNumber: req.HouseNumber,
		AddressZipCode:     req.ZipCode,
		AddressCity:        req.City,
		Latitude:           req.Latitude,
		Longitude:          req.Longitude,
		Source:             toPtr(req.Source),
		GCLID:              toPtr(req.GCLID),
		UTMSource:          toPtr(req.UTMSource),
		UTMMedium:          toPtr(req.UTMMedium),
		UTMCampaign:        toPtr(req.UTMCampaign),
		UTMContent:         toPtr(req.UTMContent),
		UTMTerm:            toPtr(req.UTMTerm),
		AdLandingPage:      toPtr(req.AdLandingPage),
		ReferrerURL:        toPtr(req.ReferrerURL),
		WhatsAppOptedIn:    whatsAppOptedIn,
	}

	if req.AssigneeID.Set {
		params.AssignedAgentID = req.AssigneeID.Value
	}

	if req.Email != "" {
		params.ConsumerEmail = &req.Email
	}

	lead, err := s.repo.Create(ctx, params)
	if err != nil {
		return transport.LeadResponse{}, err
	}

	// Create the initial service for the lead
	initialService, err := s.repo.CreateLeadService(ctx, repository.CreateLeadServiceParams{
		LeadID:         lead.ID,
		OrganizationID: tenantID,
		ServiceType:    string(req.ServiceType),
		ConsumerNote:   toPtr(req.ConsumerNote),
	})
	if err != nil {
		return transport.LeadResponse{}, err
	}

	publicToken, err := token.GenerateRandomToken(32)
	if err != nil {
		return transport.LeadResponse{}, err
	}
	publicTokenExpiresAt := time.Now().Add(30 * 24 * time.Hour)
	if err := s.repo.SetPublicToken(ctx, lead.ID, tenantID, publicToken, publicTokenExpiresAt); err != nil {
		return transport.LeadResponse{}, err
	}

	if req.WorkflowID != nil && strings.TrimSpace(*req.WorkflowID) != "" && s.workflowOverrideWriter != nil {
		workflowID, err := uuid.Parse(*req.WorkflowID)
		if err != nil {
			return transport.LeadResponse{}, apperr.Validation("invalid workflowId")
		}
		if _, err := s.workflowOverrideWriter.UpsertLeadWorkflowOverride(ctx, identityrepo.LeadWorkflowOverrideUpsert{
			LeadID:         lead.ID,
			OrganizationID: tenantID,
			WorkflowID:     &workflowID,
			OverrideMode:   "manual",
		}); err != nil {
			return transport.LeadResponse{}, err
		}
	}

	consumerEmail := ""
	if lead.ConsumerEmail != nil {
		consumerEmail = *lead.ConsumerEmail
	}

	s.eventBus.Publish(ctx, events.LeadCreated{
		BaseEvent:       events.NewBaseEvent(),
		LeadID:          lead.ID,
		LeadServiceID:   initialService.ID,
		TenantID:        tenantID,
		AssignedAgentID: lead.AssignedAgentID,
		ServiceType:     string(req.ServiceType),
		Source:          strings.TrimSpace(req.Source),
		ConsumerName:    strings.TrimSpace(lead.ConsumerFirstName + " " + lead.ConsumerLastName),
		ConsumerPhone:   lead.ConsumerPhone,
		ConsumerEmail:   consumerEmail,
		WhatsAppOptedIn: lead.WhatsAppOptedIn,
		PublicToken:     publicToken,
	})

	services, _ := s.repo.ListLeadServices(ctx, lead.ID, tenantID)
	resp := ToLeadResponseWithServices(lead, services)

	// Enrich with energy label data (fire and forget - don't fail lead creation)
	s.enrichWithEnergyLabel(ctx, tenantID, &lead, &resp)
	// Enrich with lead data (fire and forget - don't fail lead creation)
	s.enrichWithLeadData(ctx, tenantID, &lead, &resp)

	return resp, nil
}

func (s *Service) GetInboxCommunications(ctx context.Context, leadID, tenantID uuid.UUID) (transport.LeadInboxCommunicationsResponse, error) {
	if _, err := s.repo.GetByID(ctx, leadID, tenantID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadInboxCommunicationsResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadInboxCommunicationsResponse{}, err
	}

	whatsAppItems, err := s.repo.ListLinkedWhatsAppConversations(ctx, leadID, tenantID)
	if err != nil {
		return transport.LeadInboxCommunicationsResponse{}, err
	}
	emailItems, err := s.repo.ListLinkedIMAPMessages(ctx, leadID, tenantID)
	if err != nil {
		return transport.LeadInboxCommunicationsResponse{}, err
	}

	response := transport.LeadInboxCommunicationsResponse{
		WhatsAppConversations: make([]transport.LinkedWhatsAppConversationResponse, 0, len(whatsAppItems)),
		EmailMessages:         make([]transport.LinkedIMAPMessageResponse, 0, len(emailItems)),
	}
	for _, item := range whatsAppItems {
		response.WhatsAppConversations = append(response.WhatsAppConversations, ToLinkedWhatsAppConversationResponse(item))
	}
	for _, item := range emailItems {
		response.EmailMessages = append(response.EmailMessages, ToLinkedIMAPMessageResponse(item))
	}
	return response, nil
}

// GetByID retrieves a lead by ID.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (transport.LeadResponse, error) {
	lead, services, err := s.repo.GetByIDWithServices(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	resp := ToLeadResponseWithServices(lead, services)

	// Enrich with energy label data
	s.enrichWithEnergyLabel(ctx, tenantID, &lead, &resp)
	// Enrich with lead data
	s.enrichWithLeadData(ctx, tenantID, &lead, &resp)

	return resp, nil
}

func (s *Service) GetDetailContext(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, userID uuid.UUID, isAdmin bool) (transport.LeadDetailContextResponse, error) {
	lead, services, err := s.repo.GetByIDWithServices(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadDetailContextResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadDetailContextResponse{}, err
	}

	leadResponse := ToLeadResponseWithServices(lead, services)
	s.enrichWithEnergyLabel(ctx, tenantID, &lead, &leadResponse)
	s.enrichWithLeadData(ctx, tenantID, &lead, &leadResponse)

	notes, err := s.loadLeadDetailNotes(ctx, id, tenantID)
	if err != nil {
		return transport.LeadDetailContextResponse{}, err
	}

	communications, err := s.GetInboxCommunications(ctx, id, tenantID)
	if err != nil {
		return transport.LeadDetailContextResponse{}, err
	}

	quotes, err := s.loadLeadDetailQuotes(ctx, tenantID, id)
	if err != nil {
		return transport.LeadDetailContextResponse{}, err
	}

	appointments, err := s.loadLeadDetailAppointments(ctx, userID, isAdmin, tenantID, id)
	if err != nil {
		return transport.LeadDetailContextResponse{}, err
	}

	response := transport.LeadDetailContextResponse{
		Lead:           leadResponse,
		Notes:          notes,
		Appointments:   appointments,
		Quotes:         quotes,
		Communications: communications,
	}

	workflowContext, err := s.loadLeadWorkflowContext(ctx, leadResponse, tenantID)
	if err != nil {
		return transport.LeadDetailContextResponse{}, err
	}
	response.Workflow = workflowContext

	if leadResponse.CurrentService == nil {
		return response, nil
	}

	if err := s.populateCurrentServiceDetailContext(ctx, tenantID, leadResponse.CurrentService.ID, &response); err != nil {
		return transport.LeadDetailContextResponse{}, err
	}

	return response, nil
}

func (s *Service) loadLeadDetailNotes(ctx context.Context, leadID uuid.UUID, tenantID uuid.UUID) ([]transport.LeadNoteResponse, error) {
	notesList, err := s.repo.ListLeadNotes(ctx, leadID, tenantID)
	if err != nil {
		return nil, err
	}
	notes := make([]transport.LeadNoteResponse, len(notesList))
	for i, note := range notesList {
		notes[i] = toLeadNoteResponse(note)
	}
	return notes, nil
}

func (s *Service) loadLeadDetailQuotes(ctx context.Context, tenantID uuid.UUID, leadID uuid.UUID) ([]quotestransport.QuoteResponse, error) {
	if s.leadDetailQuotes == nil {
		return []quotestransport.QuoteResponse{}, nil
	}
	return s.leadDetailQuotes.ListLeadQuotes(ctx, tenantID, leadID)
}

func (s *Service) loadLeadDetailAppointments(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, leadID uuid.UUID) ([]appointmentstransport.AppointmentResponse, error) {
	if s.leadDetailAppointments == nil {
		return []appointmentstransport.AppointmentResponse{}, nil
	}
	return s.leadDetailAppointments.ListLeadAppointments(ctx, userID, isAdmin, tenantID, leadID)
}

func (s *Service) loadLeadWorkflowContext(ctx context.Context, lead transport.LeadResponse, tenantID uuid.UUID) (*transport.LeadDetailWorkflowContext, error) {
	if s.leadDetailWorkflow == nil {
		return nil, nil
	}

	var leadServiceType *string
	var pipelineStage *string
	if lead.CurrentService != nil {
		serviceType := string(lead.CurrentService.ServiceType)
		stage := string(lead.CurrentService.PipelineStage)
		leadServiceType = &serviceType
		pipelineStage = &stage
	}

	return s.leadDetailWorkflow.GetLeadWorkflowContext(ctx, tenantID, lead.ID, lead.Source, leadServiceType, pipelineStage)
}

func (s *Service) populateCurrentServiceDetailContext(ctx context.Context, tenantID uuid.UUID, serviceID uuid.UUID, response *transport.LeadDetailContextResponse) error {
	analysis, err := s.repo.GetLatestAIAnalysis(ctx, serviceID, tenantID)
	if err == nil {
		response.CurrentServiceAnalysis = &transport.LeadDetailAnalysisContext{
			Analysis:  toAIAnalysisResponsePtr(analysis),
			IsDefault: false,
		}
	} else if errors.Is(err, repository.ErrNotFound) {
		response.CurrentServiceAnalysis = &transport.LeadDetailAnalysisContext{IsDefault: true}
	} else {
		return err
	}

	photoAnalysis, err := s.repo.GetLatestPhotoAnalysis(ctx, serviceID, tenantID)
	if err == nil {
		response.CurrentServicePhotoAnalysis = &photoAnalysis
		return nil
	}
	if errors.Is(err, repository.ErrPhotoAnalysisNotFound) {
		return nil
	}
	return err
}

// enrichWithEnergyLabel ensures the lead has up-to-date energy label data.
// This is a best-effort operation - failures do not block the request flow.
func (s *Service) enrichWithEnergyLabel(ctx context.Context, tenantID uuid.UUID, lead *repository.Lead, resp *transport.LeadResponse) {
	// Always apply whatever data we currently have stored
	resp.EnergyLabel = energyLabelFromLead(*lead)

	if s.energyEnricher == nil {
		return
	}
	if !shouldRefreshEnergyLabel(lead) {
		return
	}

	params := ports.EnrichLeadParams{
		Postcode:   lead.AddressZipCode,
		Huisnummer: lead.AddressHouseNumber,
	}

	data, err := s.energyEnricher.EnrichLead(ctx, params)
	if err != nil {
		return
	}

	fetchedAt := time.Now().UTC()
	ptrs := buildEnergyLabelPointers(data)
	updateParams := repository.UpdateEnergyLabelParams{
		Class:          ptrs.class,
		Index:          ptrs.index,
		Bouwjaar:       ptrs.bouwjaar,
		Gebouwtype:     ptrs.gebouwtype,
		ValidUntil:     ptrs.validUntil,
		RegisteredAt:   ptrs.registeredAt,
		PrimairFossiel: ptrs.primairFossiel,
		BAGObjectID:    ptrs.bagObjectID,
		FetchedAt:      fetchedAt,
	}

	if err := s.repo.UpdateEnergyLabel(ctx, lead.ID, tenantID, updateParams); err != nil {
		return
	}

	applyEnergyLabelUpdate(lead, updateParams)

	resp.EnergyLabel = energyLabelFromLead(*lead)
}

type energyLabelPointers struct {
	class          *string
	index          *float64
	bouwjaar       *int
	gebouwtype     *string
	validUntil     *time.Time
	registeredAt   *time.Time
	primairFossiel *float64
	bagObjectID    *string
}

func shouldRefreshEnergyLabel(lead *repository.Lead) bool {
	if lead.EnergyLabelFetchedAt == nil {
		return true
	}
	return time.Since(*lead.EnergyLabelFetchedAt) >= energyLabelRefreshInterval
}

func buildEnergyLabelPointers(data *ports.LeadEnergyData) energyLabelPointers {
	var ptrs energyLabelPointers
	if data == nil {
		return ptrs
	}

	if data.Energieklasse != "" {
		val := data.Energieklasse
		ptrs.class = &val
	}
	if data.EnergieIndex != nil {
		val := *data.EnergieIndex
		ptrs.index = &val
	}
	if data.Bouwjaar != 0 {
		val := data.Bouwjaar
		ptrs.bouwjaar = &val
	}
	if data.Gebouwtype != "" {
		val := data.Gebouwtype
		ptrs.gebouwtype = &val
	}
	if data.GeldigTot != nil {
		val := *data.GeldigTot
		ptrs.validUntil = &val
	}
	if data.Registratiedatum != nil {
		val := *data.Registratiedatum
		ptrs.registeredAt = &val
	}
	if data.PrimaireFossieleEnergie != nil {
		val := *data.PrimaireFossieleEnergie
		ptrs.primairFossiel = &val
	}
	if data.BAGVerblijfsobjectID != "" {
		val := data.BAGVerblijfsobjectID
		ptrs.bagObjectID = &val
	}

	return ptrs
}

func applyEnergyLabelUpdate(lead *repository.Lead, params repository.UpdateEnergyLabelParams) {
	lead.EnergyClass = params.Class
	lead.EnergyIndex = params.Index
	lead.EnergyBouwjaar = params.Bouwjaar
	lead.EnergyGebouwtype = params.Gebouwtype
	lead.EnergyLabelValidUntil = params.ValidUntil
	lead.EnergyLabelRegisteredAt = params.RegisteredAt
	lead.EnergyPrimairFossiel = params.PrimairFossiel
	lead.EnergyBAGVerblijfsobjectID = params.BAGObjectID
	fetchedAt := params.FetchedAt
	lead.EnergyLabelFetchedAt = &fetchedAt
}

// enrichWithLeadData ensures the lead has up-to-date enrichment and score data.
// This is a best-effort operation - failures do not block the request flow.
func (s *Service) enrichWithLeadData(ctx context.Context, tenantID uuid.UUID, lead *repository.Lead, resp *transport.LeadResponse) {
	resp.LeadEnrichment = leadEnrichmentFromLead(*lead)
	resp.LeadScore = leadScoreFromLead(*lead)

	if s.leadEnricher == nil {
		return
	}

	if lead.LeadEnrichmentFetchedAt != nil {
		if time.Since(*lead.LeadEnrichmentFetchedAt) < leadEnrichmentRefreshInterval {
			return
		}
	}

	data, err := s.leadEnricher.EnrichLead(ctx, lead.AddressZipCode)
	if err != nil || data == nil {
		return
	}

	fetchedAt := time.Now().UTC()

	var serviceID *uuid.UUID
	if resp.CurrentService != nil {
		serviceID = &resp.CurrentService.ID
	}

	var scoreResult *scoring.Result
	if s.scorer != nil {
		scoreResult, _ = s.scorer.Recalculate(ctx, lead.ID, serviceID, tenantID, false)
	}

	updateParams := repository.UpdateLeadEnrichmentParams{
		Source:                    toPtrString(data.Source),
		Postcode6:                 toPtrString(data.Postcode6),
		Postcode4:                 toPtrString(data.Postcode4),
		Buurtcode:                 toPtrString(data.Buurtcode),
		DataYear:                  data.DataYear,
		GemAardgasverbruik:        data.GemAardgasverbruik,
		GemElektriciteitsverbruik: data.GemElektriciteitsverbruik,
		HuishoudenGrootte:         data.HuishoudenGrootte,
		KoopwoningenPct:           data.KoopwoningenPct,
		BouwjaarVanaf2000Pct:      data.BouwjaarVanaf2000Pct,
		WOZWaarde:                 data.WOZWaarde,
		MediaanVermogenX1000:      data.MediaanVermogenX1000,
		GemInkomen:                data.GemInkomenHuishouden,
		PctHoogInkomen:            data.PctHoogInkomen,
		PctLaagInkomen:            data.PctLaagInkomen,
		HuishoudensMetKinderenPct: data.HuishoudensMetKinderenPct,
		Stedelijkheid:             data.Stedelijkheid,
		Confidence:                data.Confidence,
		FetchedAt:                 fetchedAt,
	}

	if scoreResult != nil {
		updateParams.Score = &scoreResult.Score
		updateParams.ScorePreAI = &scoreResult.ScorePreAI
		updateParams.ScoreFactors = scoreResult.FactorsJSON
		updateParams.ScoreVersion = toPtrString(scoreResult.Version)
		updateParams.ScoreUpdatedAt = &scoreResult.UpdatedAt
	}

	if err := s.repo.UpdateLeadEnrichment(ctx, lead.ID, tenantID, updateParams); err != nil {
		return
	}

	lead.LeadEnrichmentSource = updateParams.Source
	lead.LeadEnrichmentPostcode6 = updateParams.Postcode6
	lead.LeadEnrichmentPostcode4 = updateParams.Postcode4
	lead.LeadEnrichmentBuurtcode = updateParams.Buurtcode
	lead.LeadEnrichmentDataYear = updateParams.DataYear
	lead.LeadEnrichmentGemAardgasverbruik = updateParams.GemAardgasverbruik
	lead.LeadEnrichmentGemElektriciteitsverbruik = updateParams.GemElektriciteitsverbruik
	lead.LeadEnrichmentHuishoudenGrootte = updateParams.HuishoudenGrootte
	lead.LeadEnrichmentKoopwoningenPct = updateParams.KoopwoningenPct
	lead.LeadEnrichmentBouwjaarVanaf2000Pct = updateParams.BouwjaarVanaf2000Pct
	lead.LeadEnrichmentWOZWaarde = updateParams.WOZWaarde
	lead.LeadEnrichmentMediaanVermogenX1000 = updateParams.MediaanVermogenX1000
	lead.LeadEnrichmentGemInkomen = updateParams.GemInkomen
	lead.LeadEnrichmentPctHoogInkomen = updateParams.PctHoogInkomen
	lead.LeadEnrichmentPctLaagInkomen = updateParams.PctLaagInkomen
	lead.LeadEnrichmentHuishoudensMetKinderenPct = updateParams.HuishoudensMetKinderenPct
	lead.LeadEnrichmentStedelijkheid = updateParams.Stedelijkheid
	lead.LeadEnrichmentConfidence = updateParams.Confidence
	lead.LeadEnrichmentFetchedAt = &fetchedAt

	if scoreResult != nil {
		lead.LeadScore = updateParams.Score
		lead.LeadScorePreAI = updateParams.ScorePreAI
		lead.LeadScoreFactors = updateParams.ScoreFactors
		lead.LeadScoreVersion = updateParams.ScoreVersion
		lead.LeadScoreUpdatedAt = updateParams.ScoreUpdatedAt
	}

	resp.LeadEnrichment = leadEnrichmentFromLead(*lead)
	resp.LeadScore = leadScoreFromLead(*lead)
}

// Update updates a lead's information.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req transport.UpdateLeadRequest, actorID uuid.UUID, tenantID uuid.UUID, actorRoles []string) (transport.LeadResponse, error) {
	params, current, err := s.prepareAssigneeUpdate(ctx, id, tenantID, req, actorID, actorRoles)
	if err != nil {
		return transport.LeadResponse{}, err
	}

	addressUpdateRequested, err := s.applyAddressGeocode(ctx, id, tenantID, req, &params, &current)
	if err != nil {
		return transport.LeadResponse{}, err
	}

	applyUpdateFields(&params, req, !addressUpdateRequested)

	lead, err := s.repo.Update(ctx, id, tenantID, params)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	if req.AssigneeID.Set && current != nil {
		if !equalUUIDPtrs(current.AssignedAgentID, req.AssigneeID.Value) {
			_ = s.repo.AddActivity(ctx, id, tenantID, actorID, "assigned", map[string]interface{}{
				"from": current.AssignedAgentID,
				"to":   req.AssigneeID.Value,
			})
		}
	}

	services, _ := s.repo.ListLeadServices(ctx, lead.ID, tenantID)
	return ToLeadResponseWithServices(lead, services), nil
}

// Delete soft-deletes a lead.
func (s *Service) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	err := s.repo.Delete(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.NotFound(leadNotFoundMsg)
		}
		return err
	}
	return nil
}

// BulkDelete deletes multiple RAC_leads.
func (s *Service) BulkDelete(ctx context.Context, ids []uuid.UUID, tenantID uuid.UUID) (int, error) {
	deletedCount, err := s.repo.BulkDelete(ctx, ids, tenantID)
	if err != nil {
		return 0, err
	}
	if deletedCount == 0 {
		return 0, apperr.NotFound("no RAC_leads found to delete")
	}
	return deletedCount, nil
}

// List retrieves a paginated list of RAC_leads.
func (s *Service) List(ctx context.Context, req transport.ListLeadsRequest, tenantID uuid.UUID) (transport.LeadListResponse, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 {
		req.PageSize = 20
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}

	params, err := buildListParams(req)
	if err != nil {
		return transport.LeadListResponse{}, err
	}
	params.OrganizationID = tenantID

	leads, total, err := s.repo.List(ctx, params)
	if err != nil {
		return transport.LeadListResponse{}, err
	}

	items := make([]transport.LeadResponse, len(leads))
	for i, lead := range leads {
		services, _ := s.repo.ListLeadServices(ctx, lead.ID, tenantID)
		items[i] = ToLeadResponseWithServices(lead, services)
	}

	totalPages := (total + req.PageSize - 1) / req.PageSize

	return transport.LeadListResponse{
		Items:      items,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: totalPages,
	}, nil
}

func buildListParams(req transport.ListLeadsRequest) (repository.ListParams, error) {
	params := repository.ListParams{
		Search:    req.Search,
		Offset:    (req.Page - 1) * req.PageSize,
		Limit:     req.PageSize,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	if req.Status != nil {
		status := string(*req.Status)
		params.Status = &status
	}
	if req.ServiceType != nil {
		serviceType := string(*req.ServiceType)
		params.ServiceType = &serviceType
	}

	params.FirstName = optionalString(req.FirstName)
	params.LastName = optionalString(req.LastName)
	params.Phone = optionalString(req.Phone)
	params.Email = optionalString(req.Email)
	if req.Role != nil {
		role := string(*req.Role)
		params.Role = &role
	}
	params.Street = optionalString(req.Street)
	params.HouseNumber = optionalString(req.HouseNumber)
	params.ZipCode = optionalString(req.ZipCode)
	params.City = optionalString(req.City)
	params.AssignedAgentID = req.AssignedAgentID

	createdFrom, createdTo, err := parseDateRange(req.CreatedAtFrom, req.CreatedAtTo)
	if err != nil {
		return repository.ListParams{}, apperr.Validation(err.Error())
	}
	params.CreatedAtFrom = createdFrom
	params.CreatedAtTo = createdTo

	return params, nil
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	return &trimmed
}

func parseDateRange(from string, to string) (*time.Time, *time.Time, error) {
	const dateLayout = "2006-01-02"

	var start *time.Time
	var end *time.Time

	if from != "" {
		parsed, err := time.Parse(dateLayout, from)
		if err != nil {
			return nil, nil, err
		}
		start = &parsed
	}

	if to != "" {
		parsed, err := time.Parse(dateLayout, to)
		if err != nil {
			return nil, nil, err
		}
		endExclusive := parsed.AddDate(0, 0, 1)
		end = &endExclusive
	}

	if start != nil && end != nil && start.After(*end) {
		return nil, nil, errors.New("createdAtFrom must be before createdAtTo")
	}

	return start, end, nil
}

// CheckDuplicate checks if a lead with the given phone already exists.
func (s *Service) CheckDuplicate(ctx context.Context, phoneNumber string, tenantID uuid.UUID) (transport.DuplicateCheckResponse, error) {
	normalizedPhone := phone.NormalizeE164(phoneNumber)
	lead, err := s.repo.GetByPhone(ctx, normalizedPhone, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.DuplicateCheckResponse{IsDuplicate: false}, nil
		}
		return transport.DuplicateCheckResponse{}, err
	}

	resp := ToLeadResponse(lead)
	return transport.DuplicateCheckResponse{
		IsDuplicate:  true,
		ExistingLead: &resp,
	}, nil
}

// CheckReturningCustomer checks if a lead with the given phone or email already exists.
// This is used to detect returning customers when creating a new service request.
func (s *Service) CheckReturningCustomer(ctx context.Context, phoneNumber string, email string, tenantID uuid.UUID) (transport.ReturningCustomerResponse, error) {
	normalizedPhone := phone.NormalizeE164(phoneNumber)

	summary, services, err := s.repo.GetByPhoneOrEmail(ctx, normalizedPhone, email, tenantID)
	if err != nil {
		return transport.ReturningCustomerResponse{}, err
	}

	if summary == nil {
		return transport.ReturningCustomerResponse{Found: false}, nil
	}

	serviceBriefs := make([]transport.ServiceBrief, len(services))
	for i, svc := range services {
		serviceBriefs[i] = transport.ServiceBrief{
			ServiceType: transport.ServiceType(svc.ServiceType),
			Status:      transport.LeadStatus(svc.Status),
			CreatedAt:   svc.CreatedAt,
		}
	}

	return transport.ReturningCustomerResponse{
		Found:         true,
		LeadID:        &summary.ID,
		FullName:      summary.ConsumerName,
		TotalServices: summary.ServiceCount,
		Services:      serviceBriefs,
	}, nil
}

// Assign assigns or unassigns a lead to an agent.
func (s *Service) Assign(ctx context.Context, id uuid.UUID, assigneeID *uuid.UUID, actorID uuid.UUID, tenantID uuid.UUID, actorRoles []string) (transport.LeadResponse, error) {
	current, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	if !hasRole(actorRoles, "admin") {
		if current.AssignedAgentID == nil || *current.AssignedAgentID != actorID {
			return transport.LeadResponse{}, apperr.Forbidden("forbidden")
		}
	}

	params := repository.UpdateLeadParams{
		AssignedAgentID:    assigneeID,
		AssignedAgentIDSet: true,
	}
	updated, err := s.repo.Update(ctx, id, tenantID, params)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	_ = s.repo.AddActivity(ctx, id, tenantID, actorID, "assigned", map[string]interface{}{
		"from": current.AssignedAgentID,
		"to":   assigneeID,
	})

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.LeadAssigned{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        id,
			TenantID:      tenantID,
			PreviousAgent: current.AssignedAgentID,
			NewAgent:      assigneeID,
			AssignedByID:  actorID,
		})
	}

	return ToLeadResponse(updated), nil
}

// AssignIfUnassigned assigns a lead to the agent if it is currently unassigned.
func (s *Service) AssignIfUnassigned(ctx context.Context, id uuid.UUID, agentID uuid.UUID, tenantID uuid.UUID) error {
	lead, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.NotFound(leadNotFoundMsg)
		}
		return err
	}

	if lead.AssignedAgentID != nil {
		return apperr.Forbidden("lead is already assigned")
	}

	params := repository.UpdateLeadParams{
		AssignedAgentID:    &agentID,
		AssignedAgentIDSet: true,
	}

	if _, err := s.repo.Update(ctx, id, tenantID, params); err != nil {
		return err
	}

	_ = s.repo.AddActivity(ctx, id, tenantID, agentID, "assigned", map[string]interface{}{
		"from": nil,
		"to":   agentID,
	})

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.LeadAssigned{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        id,
			TenantID:      tenantID,
			PreviousAgent: nil,
			NewAgent:      &agentID,
			AssignedByID:  agentID,
		})
	}

	return nil
}

// SetViewedBy marks a lead as viewed by a user.
func (s *Service) SetViewedBy(ctx context.Context, id uuid.UUID, userID uuid.UUID, tenantID uuid.UUID) error {
	return s.repo.SetViewedBy(ctx, id, tenantID, userID)
}

// GetLeadServiceByID retrieves a lead service by its ID.
func (s *Service) GetLeadServiceByID(ctx context.Context, serviceID uuid.UUID, tenantID uuid.UUID) (repository.LeadService, error) {
	svc, err := s.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return repository.LeadService{}, apperr.NotFound(leadServiceNotFoundMsg)
		}
		return repository.LeadService{}, err
	}
	return svc, nil
}

// AddService adds a new service to an existing lead.
func (s *Service) AddService(ctx context.Context, leadID uuid.UUID, req transport.AddServiceRequest, tenantID uuid.UUID) (transport.LeadResponse, error) {
	lead, err := s.repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	if req.CloseCurrentStatus {
		if err := s.repo.CloseAllActiveServices(ctx, leadID, tenantID); err != nil {
			return transport.LeadResponse{}, err
		}
	}

	newService, err := s.repo.CreateLeadService(ctx, repository.CreateLeadServiceParams{
		LeadID:         leadID,
		OrganizationID: tenantID,
		ServiceType:    string(req.ServiceType),
		ConsumerNote:   toPtr(req.ConsumerNote),
		Source:         toPtr(req.Source),
	})
	if err != nil {
		return transport.LeadResponse{}, err
	}

	// Publish event so the gatekeeper agent triages the new service
	s.eventBus.Publish(ctx, events.LeadServiceAdded{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        leadID,
		LeadServiceID: newService.ID,
		TenantID:      tenantID,
		ServiceType:   string(req.ServiceType),
	})

	services, _ := s.repo.ListLeadServices(ctx, leadID, tenantID)
	return ToLeadResponseWithServices(lead, services), nil
}

// UpdateServiceStatus updates the status of a specific service.
func (s *Service) UpdateServiceStatus(ctx context.Context, leadID uuid.UUID, serviceID uuid.UUID, req transport.UpdateServiceStatusRequest, tenantID uuid.UUID) (transport.LeadResponse, error) {
	svc, err := s.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadServiceNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}
	if svc.LeadID != leadID {
		return transport.LeadResponse{}, apperr.NotFound(leadServiceNotFoundMsg)
	}

	if domain.IsTerminal(svc.Status, svc.PipelineStage) {
		return transport.LeadResponse{}, apperr.Validation("cannot update status for a service in terminal state")
	}

	// Disqualified <-> Lost is an invariant. When a user sets Disqualified we auto-move the stage to Lost.
	// Do it atomically to avoid invalid intermediate combinations that would otherwise fail validation.
	if string(req.Status) == domain.LeadStatusDisqualified && svc.PipelineStage != domain.PipelineStageLost {
		return s.disqualifyServiceAndMarkLost(ctx, leadID, serviceID, tenantID, leadServiceNotFoundMsg)
	}

	if reason := domain.ValidateStateCombination(string(req.Status), svc.PipelineStage); reason != "" {
		return transport.LeadResponse{}, apperr.Validation(reason)
	}

	_, err = s.repo.UpdateServiceStatus(ctx, serviceID, tenantID, string(req.Status))
	if err != nil {
		return transport.LeadResponse{}, err
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.LeadServiceStatusChanged{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        leadID,
			LeadServiceID: serviceID,
			TenantID:      tenantID,
			OldStatus:     svc.Status,
			NewStatus:     string(req.Status),
		})
	}

	return s.GetByID(ctx, leadID, tenantID)
}

// UpdateServiceType updates the service type of a specific service.
func (s *Service) UpdateServiceType(ctx context.Context, leadID uuid.UUID, serviceID uuid.UUID, req transport.UpdateServiceTypeRequest, tenantID uuid.UUID) (transport.LeadResponse, error) {
	svc, err := s.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadServiceNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}
	if svc.LeadID != leadID {
		return transport.LeadResponse{}, apperr.NotFound(leadServiceNotFoundMsg)
	}

	_, err = s.repo.UpdateLeadServiceType(ctx, serviceID, tenantID, string(req.ServiceType))
	if err != nil {
		if errors.Is(err, repository.ErrServiceTypeNotFound) {
			return transport.LeadResponse{}, apperr.Validation("service type not found or inactive")
		}
		return transport.LeadResponse{}, err
	}

	// Publish event for potential workflow reassessment and AI re-analysis
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.LeadDataChanged{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        leadID,
			LeadServiceID: serviceID,
			TenantID:      tenantID,
			Source:        "service_type_update",
		})
	}

	return s.GetByID(ctx, leadID, tenantID)
}

// CompleteService moves a service from the Fulfillment pipeline stage to Completed,
// optionally recording extra work amount and notes.
func (s *Service) CompleteService(ctx context.Context, leadID uuid.UUID, serviceID uuid.UUID, actorID uuid.UUID, req transport.CompleteServiceRequest, tenantID uuid.UUID) (transport.LeadResponse, error) {
	svc, err := s.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadServiceNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}
	if svc.LeadID != leadID {
		return transport.LeadResponse{}, apperr.NotFound(leadServiceNotFoundMsg)
	}

	if svc.PipelineStage != domain.PipelineStageFulfillment {
		return transport.LeadResponse{}, apperr.Validation("only services in the Fulfillment stage can be marked as complete")
	}

	if err := s.updateAcceptedQuoteExtraWork(ctx, serviceID, tenantID, actorID, req); err != nil {
		return transport.LeadResponse{}, err
	}

	updatedService, err := s.repo.CompleteLeadService(ctx, serviceID, tenantID, req.ExtraWorkAmountCents, req.ExtraWorkNotes)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadServiceNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.LeadServiceStatusChanged{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        leadID,
			LeadServiceID: serviceID,
			TenantID:      tenantID,
			OldStatus:     svc.Status,
			NewStatus:     updatedService.Status,
		})
	}

	return s.GetByID(ctx, leadID, tenantID)
}

func (s *Service) updateAcceptedQuoteExtraWork(ctx context.Context, serviceID uuid.UUID, tenantID uuid.UUID, actorID uuid.UUID, req transport.CompleteServiceRequest) error {
	if req.ExtraWorkAmountCents == nil || *req.ExtraWorkAmountCents <= 0 {
		return nil
	}
	if s.acceptedQuoteUpdater == nil {
		return apperr.Internal("accepted quote updater is not configured")
	}

	quoteID, err := s.repo.GetLatestAcceptedQuoteIDForService(ctx, serviceID, tenantID)
	if err != nil {
		if apperr.Is(err, apperr.KindNotFound) {
			return apperr.Validation("accepted quote not found for service")
		}
		return err
	}

	return s.acceptedQuoteUpdater.AddExtraWorkToQuote(ctx, quoteID, tenantID, actorID, *req.ExtraWorkAmountCents, req.ExtraWorkNotes)
}

func (s *Service) disqualifyServiceAndMarkLost(ctx context.Context, leadID uuid.UUID, serviceID uuid.UUID, tenantID uuid.UUID, notFoundMsg string) (transport.LeadResponse, error) {
	svc, err := s.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(notFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	if _, err := s.repo.UpdateServiceStatusAndPipelineStage(ctx, serviceID, tenantID, domain.LeadStatusDisqualified, domain.PipelineStageLost); err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(notFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.LeadServiceStatusChanged{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        leadID,
			LeadServiceID: serviceID,
			TenantID:      tenantID,
			OldStatus:     svc.Status,
			NewStatus:     domain.LeadStatusDisqualified,
		})
	}

	return s.GetByID(ctx, leadID, tenantID)
}

// UpdateStatus updates the status of the lead's current service.
func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, req transport.UpdateLeadStatusRequest, tenantID uuid.UUID) (transport.LeadResponse, error) {
	service, err := s.repo.GetCurrentLeadService(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) || errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	if domain.IsTerminal(service.Status, service.PipelineStage) {
		return transport.LeadResponse{}, apperr.Validation("cannot update status for a service in terminal state")
	}

	// Disqualified <-> Lost is an invariant. When a user sets Disqualified we auto-move the stage to Lost.
	// Do it atomically to avoid invalid intermediate combinations that would otherwise fail validation.
	if string(req.Status) == domain.LeadStatusDisqualified && service.PipelineStage != domain.PipelineStageLost {
		return s.disqualifyServiceAndMarkLost(ctx, id, service.ID, tenantID, leadNotFoundMsg)
	}

	if reason := domain.ValidateStateCombination(string(req.Status), service.PipelineStage); reason != "" {
		return transport.LeadResponse{}, apperr.Validation(reason)
	}

	if _, err := s.repo.UpdateServiceStatus(ctx, service.ID, tenantID, string(req.Status)); err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.LeadServiceStatusChanged{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        id,
			LeadServiceID: service.ID,
			TenantID:      tenantID,
			OldStatus:     service.Status,
			NewStatus:     string(req.Status),
		})
	}

	return s.GetByID(ctx, id, tenantID)
}

// GetMetrics returns aggregated KPI metrics for the dashboard.
func (s *Service) GetMetrics(ctx context.Context, tenantID uuid.UUID) (transport.LeadMetricsResponse, error) {
	metrics, err := s.repo.GetMetrics(ctx, tenantID)
	if err != nil {
		return transport.LeadMetricsResponse{}, err
	}

	var conversionRate float64
	totalQuoted := metrics.AcceptedQuotes + metrics.SentQuotes
	if totalQuoted > 0 {
		conversionRate = float64(metrics.AcceptedQuotes) / float64(totalQuoted)
	}

	trendLength := len(metrics.AcceptedQuotesTrend)
	if len(metrics.SentQuotesTrend) > trendLength {
		trendLength = len(metrics.SentQuotesTrend)
	}
	conversionTrend := make([]float64, 0, trendLength)
	for index := 0; index < trendLength; index++ {
		acceptedQuotes := 0
		if index < len(metrics.AcceptedQuotesTrend) {
			acceptedQuotes = metrics.AcceptedQuotesTrend[index]
		}

		sentQuotes := 0
		if index < len(metrics.SentQuotesTrend) {
			sentQuotes = metrics.SentQuotesTrend[index]
		}

		totalQuotes := acceptedQuotes + sentQuotes
		if totalQuotes <= 0 {
			conversionTrend = append(conversionTrend, 0)
			continue
		}

		rate := float64(acceptedQuotes) / float64(totalQuotes)
		conversionTrend = append(conversionTrend, roundToOneDecimal(rate*100))
	}

	return transport.LeadMetricsResponse{
		ActiveLeads:        metrics.ActiveLeads,
		QuotePipelineCents: metrics.QuotePipelineCents,
		ConversionRate:     roundToOneDecimal(conversionRate * 100),
		AvgQuoteValueCents: metrics.AvgQuoteValueCents,
		ActiveLeadsTrend:   metrics.ActiveLeadsTrend,
		QuotePipelineTrend: metrics.QuotePipelineTrend,
		ConversionTrend:    conversionTrend,
		AvgQuoteValueTrend: metrics.AvgQuoteValueTrend,
	}, nil
}

// GetHeatmap returns geocoded lead points for the dashboard heatmap.
func (s *Service) GetHeatmap(ctx context.Context, startDate *time.Time, endDate *time.Time, tenantID uuid.UUID) (transport.LeadHeatmapResponse, error) {
	var endExclusive *time.Time
	if endDate != nil {
		end := endDate.AddDate(0, 0, 1)
		endExclusive = &end
	}

	points, err := s.repo.ListHeatmapPoints(ctx, tenantID, startDate, endExclusive)
	if err != nil {
		return transport.LeadHeatmapResponse{}, err
	}

	resp := transport.LeadHeatmapResponse{Points: make([]transport.LeadHeatmapPointResponse, 0, len(points))}
	for _, point := range points {
		resp.Points = append(resp.Points, transport.LeadHeatmapPointResponse{
			Latitude:  point.Latitude,
			Longitude: point.Longitude,
		})
	}

	return resp, nil
}

// GetActionItems returns urgent or recent RAC_leads for the dashboard widget.
func (s *Service) GetActionItems(ctx context.Context, page int, pageSize int, newLeadDays int, tenantID uuid.UUID) (transport.ActionItemsResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 5
	}
	if pageSize > 50 {
		pageSize = 50
	}

	offset := (page - 1) * pageSize
	result, err := s.repo.ListActionItems(ctx, tenantID, newLeadDays, pageSize, offset)
	if err != nil {
		return transport.ActionItemsResponse{}, err
	}

	items := make([]transport.ActionItemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		name := strings.TrimSpace(item.FirstName + " " + item.LastName)
		isUrgent := item.UrgencyLevel != nil && *item.UrgencyLevel == "High"
		items = append(items, transport.ActionItemResponse{
			ID:            item.ID,
			Name:          name,
			UrgencyReason: item.UrgencyReason,
			CreatedAt:     item.CreatedAt,
			IsUrgent:      isUrgent,
		})
	}

	return transport.ActionItemsResponse{
		Items:    items,
		Total:    result.Total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetTimeline returns the lead timeline events in reverse chronological order.
// When serviceID is provided, only events for that service (plus unscoped events) are returned.
func (s *Service) GetTimeline(ctx context.Context, leadID uuid.UUID, tenantID uuid.UUID, serviceID *uuid.UUID) ([]transport.TimelineItem, error) {
	if _, err := s.repo.GetByID(ctx, leadID, tenantID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.NotFound(leadNotFoundMsg)
		}
		return nil, err
	}

	var events []repository.TimelineEvent
	var err error
	if serviceID != nil {
		events, err = s.repo.ListTimelineEventsByService(ctx, leadID, *serviceID, tenantID)
	} else {
		events, err = s.repo.ListTimelineEvents(ctx, leadID, tenantID)
	}
	if err != nil {
		return nil, err
	}

	return buildTimelineItems(events), nil
}

func (s *Service) SendTimelineWhatsAppDraft(ctx context.Context, leadID uuid.UUID, eventID uuid.UUID, tenantID uuid.UUID) error {
	lead, err := s.repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.NotFound(leadNotFoundMsg)
		}
		return err
	}
	if s.timelineWhatsAppSender == nil {
		return apperr.Internal("WhatsApp is niet geconfigureerd")
	}

	events, err := s.repo.ListTimelineEvents(ctx, leadID, tenantID)
	if err != nil {
		return err
	}

	event, found := findTimelineEventByID(events, eventID)
	if !found {
		return apperr.NotFound("tijdlijnitem niet gevonden")
	}
	if hasSentTimelineWhatsAppEvent(events, eventID) {
		return apperr.Conflict("dit WhatsApp-concept is al verstuurd")
	}

	params, err := s.buildTimelineWhatsAppSendParams(ctx, event, tenantID, lead.ConsumerPhone)
	if err != nil {
		return err
	}
	return s.timelineWhatsAppSender.SendLeadWhatsApp(ctx, params)
}

const timelineStageMergeWindow = 5 * time.Minute

type timelineStageContext struct {
	serviceID     string
	actorName     string
	createdAt     time.Time
	hasAnalysis   bool
	hasEstimation bool
	hasDraftQuote bool
}

func buildTimelineItems(events []repository.TimelineEvent) []transport.TimelineItem {
	stageContexts := make([]timelineStageContext, 0, len(events))
	items := make([]transport.TimelineItem, 0, len(events))
	for _, event := range events {
		if shouldSkipTimelineEvent(event, stageContexts) {
			continue
		}
		items = append(items, transportTimelineItem(event))
		if context, ok := buildTimelineStageContext(event); ok {
			stageContexts = append(stageContexts, context)
		}
	}
	return items
}

func findTimelineEventByID(events []repository.TimelineEvent, eventID uuid.UUID) (repository.TimelineEvent, bool) {
	for _, event := range events {
		if event.ID == eventID {
			return event, true
		}
	}
	return repository.TimelineEvent{}, false
}

func hasSentTimelineWhatsAppEvent(events []repository.TimelineEvent, sourceEventID uuid.UUID) bool {
	sourceID := sourceEventID.String()
	for _, event := range events {
		metadata := event.Metadata
		if readTimelineStringValue(metadata["sourceTimelineEventId"]) != sourceID {
			continue
		}
		if readTimelineStringValue(metadata["status"]) == "sent" {
			return true
		}
	}
	return false
}

func (s *Service) buildTimelineWhatsAppSendParams(ctx context.Context, event repository.TimelineEvent, tenantID uuid.UUID, leadPhone string) (TimelineWhatsAppSendParams, error) {
	metadata := event.Metadata
	if metadata == nil {
		return TimelineWhatsAppSendParams{}, apperr.Validation("tijdlijnitem bevat geen verstuurbaar WhatsApp-concept")
	}

	draft := resolveTimelineWhatsAppDraft(metadata)
	if strings.EqualFold(draft.audience, "internal") {
		return TimelineWhatsAppSendParams{}, apperr.Validation("dit WhatsApp-concept is alleen intern en kan niet rechtstreeks worden verstuurd")
	}
	if strings.EqualFold(draft.status, "sent") {
		return TimelineWhatsAppSendParams{}, apperr.Conflict("dit WhatsApp-concept is al verstuurd")
	}
	if strings.TrimSpace(draft.message) == "" {
		return TimelineWhatsAppSendParams{}, apperr.Validation("tijdlijnitem bevat geen verstuurbaar WhatsApp-concept")
	}

	phoneNumber, err := s.resolveTimelineWhatsAppPhoneNumber(ctx, metadata, tenantID, leadPhone, draft.audience)
	if err != nil {
		return TimelineWhatsAppSendParams{}, err
	}

	if draft.category == "" {
		draft.category = event.EventType
	}
	if draft.language == "" {
		draft.language = "nl"
	}

	return TimelineWhatsAppSendParams{
		OrgID:       tenantID,
		LeadID:      event.LeadID,
		ServiceID:   event.ServiceID,
		PhoneNumber: phoneNumber,
		Message:     draft.message,
		Category:    draft.category,
		Audience:    draft.audience,
		Summary:     buildTimelineWhatsAppSummary(event),
		ActorType:   "System",
		ActorName:   "WhatsApp",
		Metadata:    buildTimelineWhatsAppSentMetadata(event, draft.category, draft.audience, draft.language),
	}, nil
}

type timelineWhatsAppDraft struct {
	message  string
	category string
	audience string
	language string
	status   string
}

func (s *Service) resolveTimelineWhatsAppPhoneNumber(ctx context.Context, metadata map[string]any, tenantID uuid.UUID, leadPhone string, audience string) (string, error) {
	phoneNumber := readTimelineStringValue(metadata["phoneNumber"])
	if phoneNumber == "" {
		phoneNumber = extractPhoneFromTimelineWhatsAppURL(readTimelineStringValue(metadata["whatsappUrl"]))
	}
	if phoneNumber == "" {
		phoneNumber = s.resolveLegacyTimelinePartnerPhone(ctx, metadata, tenantID)
	}
	if phoneNumber == "" {
		phoneNumber = resolveTimelineLeadPhone(audience, leadPhone)
	}
	if phoneNumber == "" {
		return "", apperr.Validation("tijdlijnitem bevat geen telefoonnummer voor WhatsApp")
	}
	return phoneNumber, nil
}

func (s *Service) resolveLegacyTimelinePartnerPhone(ctx context.Context, metadata map[string]any, tenantID uuid.UUID) string {
	if s.partnerPhoneResolver == nil {
		return ""
	}
	partnerIDText := readTimelineStringValue(metadata["partnerId"])
	if partnerIDText == "" {
		return ""
	}
	partnerID, err := uuid.Parse(partnerIDText)
	if err != nil {
		return ""
	}
	phoneNumber, err := s.partnerPhoneResolver.ResolvePartnerPhone(ctx, tenantID, partnerID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(phone.NormalizeE164(phoneNumber))
}

func resolveTimelineLeadPhone(audience string, leadPhone string) string {
	normalizedLeadPhone := strings.TrimSpace(phone.NormalizeE164(leadPhone))
	if normalizedLeadPhone == "" {
		return ""
	}
	audience = strings.TrimSpace(strings.ToLower(audience))
	if audience == "partner" || audience == "internal" {
		return ""
	}
	return normalizedLeadPhone
}

func resolveTimelineWhatsAppDraft(metadata map[string]any) timelineWhatsAppDraft {
	draft := timelineWhatsAppDraft{
		category: readTimelineStringValue(metadata["messageCategory"]),
		audience: readTimelineStringValue(metadata["messageAudience"]),
		language: readTimelineStringValue(metadata["messageLanguage"]),
		status:   readTimelineStringValue(metadata["status"]),
	}

	if drafts := readTimelineMapValue(metadata["drafts"]); drafts != nil {
		draft.message = readTimelineStringValue(drafts["whatsappMessage"])
		draft.category = fallbackTimelineString(draft.category, drafts["messageCategory"])
		draft.audience = fallbackTimelineString(draft.audience, drafts["messageAudience"])
		draft.language = fallbackTimelineString(draft.language, drafts["messageLanguage"])
		draft.status = fallbackTimelineString(draft.status, drafts["status"])
		return draft
	}

	if readTimelineStringValue(metadata["preferredContactChannel"]) == "WhatsApp" {
		draft.message = readTimelineStringValue(metadata["suggestedContactMessage"])
	}

	return draft
}

func fallbackTimelineString(current string, candidate any) string {
	if current != "" {
		return current
	}
	return readTimelineStringValue(candidate)
}

func buildTimelineWhatsAppSummary(event repository.TimelineEvent) string {
	if event.Summary != nil && strings.TrimSpace(*event.Summary) != "" {
		return fmt.Sprintf("WhatsApp verstuurd: %s", strings.TrimSpace(*event.Summary))
	}
	return "WhatsApp verstuurd"
}

func buildTimelineWhatsAppSentMetadata(event repository.TimelineEvent, category, audience, language string) map[string]any {
	sentMetadata := map[string]any{
		"sourceTimelineEventId":    event.ID.String(),
		"sourceTimelineEventType":  event.EventType,
		"sourceTimelineEventTitle": event.Title,
		"messageCategory":          category,
		"messageAudience":          audience,
		"messageLanguage":          language,
	}
	for _, key := range []string{"offerId", "partnerId", "partnerName", "acceptanceUrl", "publicToken"} {
		if value, ok := event.Metadata[key]; ok {
			sentMetadata[key] = value
		}
	}
	return sentMetadata
}

func readTimelineStringValue(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func readTimelineMapValue(value any) map[string]any {
	result, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return result
}

func extractPhoneFromTimelineWhatsAppURL(value string) string {
	if value == "" {
		return ""
	}
	const marker = "wa.me/"
	markerIndex := strings.Index(value, marker)
	if markerIndex == -1 {
		return ""
	}
	afterMarker := value[markerIndex+len(marker):]
	phonePart := strings.SplitN(afterMarker, "?", 2)[0]
	return strings.TrimSpace(phone.NormalizeE164(phonePart))
}

func shouldSkipTimelineEvent(event repository.TimelineEvent, contexts []timelineStageContext) bool {
	if event.Visibility == repository.TimelineVisibilityDebug {
		return true
	}
	return matchesTimelineStageContext(event, contexts)
}

func matchesTimelineStageContext(event repository.TimelineEvent, contexts []timelineStageContext) bool {
	if event.ServiceID == nil {
		return false
	}
	serviceKey := event.ServiceID.String()
	for _, context := range contexts {
		if context.serviceID != serviceKey || context.actorName != event.ActorName {
			continue
		}
		if context.createdAt.Before(event.CreatedAt) || context.createdAt.Sub(event.CreatedAt) > timelineStageMergeWindow {
			continue
		}
		switch {
		case event.EventType == repository.EventTypeAI && context.hasAnalysis:
			return true
		case event.EventType == repository.EventTypeAnalysis && event.Title == repository.EventTitleEstimationSaved && context.hasEstimation:
			return true
		case event.EventType == "quote_drafted" && context.hasDraftQuote:
			return true
		}
	}
	return false
}

func buildTimelineStageContext(event repository.TimelineEvent) (timelineStageContext, bool) {
	if event.EventType != repository.EventTypeStageChange {
		return timelineStageContext{}, false
	}
	context := timelineStageContext{
		actorName:     event.ActorName,
		createdAt:     event.CreatedAt,
		hasAnalysis:   event.Metadata != nil && event.Metadata["analysis"] != nil,
		hasEstimation: event.Metadata != nil && event.Metadata["estimation"] != nil,
		hasDraftQuote: event.Metadata != nil && event.Metadata["draftQuote"] != nil,
	}
	if event.ServiceID != nil {
		context.serviceID = event.ServiceID.String()
	}
	return context, true
}

func transportTimelineItem(event repository.TimelineEvent) transport.TimelineItem {
	return transport.TimelineItem{
		ID:         event.ID,
		ServiceID:  event.ServiceID,
		Type:       timelineTypeForEvent(event),
		Title:      event.Title,
		Summary:    summaryValue(event.Summary),
		Timestamp:  event.CreatedAt,
		Actor:      event.ActorName,
		Metadata:   event.Metadata,
		Visibility: event.Visibility,
	}
}

func timelineTypeForEvent(event repository.TimelineEvent) string {
	if event.EventType == repository.EventTypeStageChange {
		return "stage"
	}
	if event.EventType == repository.EventTypeAlert || event.ActorType == repository.ActorTypeSystem {
		return "system"
	}
	if event.ActorType == repository.ActorTypeAI {
		return "ai"
	}
	return "user"
}

func summaryValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func roundToOneDecimal(value float64) float64 {
	return math.Round(value*10) / 10
}

func (s *Service) prepareAssigneeUpdate(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, req transport.UpdateLeadRequest, actorID uuid.UUID, actorRoles []string) (repository.UpdateLeadParams, *repository.Lead, error) {
	params := repository.UpdateLeadParams{}
	if !req.AssigneeID.Set {
		return params, nil, nil
	}

	lead, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return repository.UpdateLeadParams{}, nil, apperr.NotFound(leadNotFoundMsg)
		}
		return repository.UpdateLeadParams{}, nil, err
	}

	if !hasRole(actorRoles, "admin") {
		if lead.AssignedAgentID == nil || *lead.AssignedAgentID != actorID {
			return repository.UpdateLeadParams{}, nil, apperr.Forbidden("forbidden")
		}
	}

	params.AssignedAgentID = req.AssigneeID.Value
	params.AssignedAgentIDSet = true
	return params, &lead, nil
}

func applyUpdateFields(params *repository.UpdateLeadParams, req transport.UpdateLeadRequest, applyCoords bool) {
	if req.FirstName != nil {
		params.ConsumerFirstName = req.FirstName
	}
	if req.LastName != nil {
		params.ConsumerLastName = req.LastName
	}
	if req.Phone != nil {
		normalized := phone.NormalizeE164(*req.Phone)
		params.ConsumerPhone = &normalized
	}
	if req.Email != nil {
		params.ConsumerEmail = req.Email
	}
	if applyCoords {
		if req.Latitude != nil {
			params.Latitude = req.Latitude
		}
		if req.Longitude != nil {
			params.Longitude = req.Longitude
		}
	}
	if req.ConsumerRole != nil {
		role := string(*req.ConsumerRole)
		params.ConsumerRole = &role
	}
	if req.Street != nil {
		params.AddressStreet = req.Street
	}
	if req.HouseNumber != nil {
		params.AddressHouseNumber = req.HouseNumber
	}
	if req.ZipCode != nil {
		params.AddressZipCode = req.ZipCode
	}
	if req.City != nil {
		params.AddressCity = req.City
	}
	if req.WhatsAppOptedIn != nil {
		params.WhatsAppOptedIn = req.WhatsAppOptedIn
		params.WhatsAppOptedInSet = true
	}
}

func (s *Service) applyAddressGeocode(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, req transport.UpdateLeadRequest, params *repository.UpdateLeadParams, current **repository.Lead) (bool, error) {
	if !hasAddressUpdate(req) {
		return false, nil
	}

	if *current == nil {
		lead, err := s.repo.GetByID(ctx, id, tenantID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return true, apperr.NotFound(leadNotFoundMsg)
			}
			return true, err
		}
		*current = &lead
	}

	updatedAddress, changed := buildUpdatedAddress(**current, req)
	if changed {
		if lat, lon, ok := s.geocodeAddress(ctx, updatedAddress); ok {
			params.Latitude = &lat
			params.Longitude = &lon
		}
	}

	return true, nil
}

type addressUpdate struct {
	street      string
	houseNumber string
	zipCode     string
	city        string
}

func hasAddressUpdate(req transport.UpdateLeadRequest) bool {
	return req.Street != nil || req.HouseNumber != nil || req.ZipCode != nil || req.City != nil
}

func buildUpdatedAddress(current repository.Lead, req transport.UpdateLeadRequest) (addressUpdate, bool) {
	updated := addressUpdate{
		street:      current.AddressStreet,
		houseNumber: current.AddressHouseNumber,
		zipCode:     current.AddressZipCode,
		city:        current.AddressCity,
	}

	changed := false
	if req.Street != nil {
		updated.street = strings.TrimSpace(*req.Street)
		changed = changed || updated.street != current.AddressStreet
	}
	if req.HouseNumber != nil {
		updated.houseNumber = strings.TrimSpace(*req.HouseNumber)
		changed = changed || updated.houseNumber != current.AddressHouseNumber
	}
	if req.ZipCode != nil {
		updated.zipCode = strings.TrimSpace(*req.ZipCode)
		changed = changed || updated.zipCode != current.AddressZipCode
	}
	if req.City != nil {
		updated.city = strings.TrimSpace(*req.City)
		changed = changed || updated.city != current.AddressCity
	}

	return updated, changed
}

func (s *Service) geocodeAddress(ctx context.Context, address addressUpdate) (float64, float64, bool) {
	if s.maps == nil {
		return 0, 0, false
	}

	if address.street == "" || address.city == "" {
		return 0, 0, false
	}

	query := formatGeocodeQuery(address)
	suggestions, err := s.maps.SearchAddress(ctx, query)
	if err != nil || len(suggestions) == 0 {
		return 0, 0, false
	}

	lat, err := strconv.ParseFloat(suggestions[0].Lat, 64)
	if err != nil {
		return 0, 0, false
	}
	lon, err := strconv.ParseFloat(suggestions[0].Lon, 64)
	if err != nil {
		return 0, 0, false
	}

	return lat, lon, true
}

func formatGeocodeQuery(address addressUpdate) string {
	streetPart := strings.TrimSpace(strings.Join([]string{address.street, address.houseNumber}, " "))
	cityPart := strings.TrimSpace(strings.Join([]string{address.zipCode, address.city}, " "))
	query := strings.TrimSpace(fmt.Sprintf("%s, %s", streetPart, cityPart))
	return strings.Trim(query, ", ")
}

func hasRole(roles []string, target string) bool {
	for _, role := range roles {
		if role == target {
			return true
		}
	}
	return false
}

func equalUUIDPtrs(a *uuid.UUID, b *uuid.UUID) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func toPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func toPtrString(value string) *string {
	return toPtr(value)
}

// GetActivityFeed returns the most recent org-wide activity for the dashboard feed card.
func (s *Service) GetActivityFeed(ctx context.Context, tenantID, userID uuid.UUID, page int, limit int) (transport.ActivityFeedResponse, error) {
	page, limit, offset := normalizePagination(page, limit)

	entries, err := s.repo.ListRecentActivity(ctx, tenantID, limit, offset)
	if err != nil {
		return transport.ActivityFeedResponse{}, err
	}

	if page == 1 {
		entries, err = s.mergeUpcomingAppointments(ctx, tenantID, entries)
		if err != nil {
			return transport.ActivityFeedResponse{}, err
		}
	}

	items := buildFeedItems(entries)
	if err := s.enrichFeedItems(ctx, tenantID, userID, items); err != nil {
		return transport.ActivityFeedResponse{}, err
	}

	return transport.ActivityFeedResponse{Items: items}, nil
}

func normalizePagination(page, limit int) (int, int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	offset := (page - 1) * limit
	return page, limit, offset
}

func buildFeedItems(entries []repository.ActivityFeedEntry) []transport.ActivityFeedItem {
	items := make([]transport.ActivityFeedItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, mapEntryToFeedItem(e))
	}
	return items
}

func (s *Service) enrichFeedItems(ctx context.Context, tenantID, userID uuid.UUID, items []transport.ActivityFeedItem) error {
	if len(items) == 0 {
		return nil
	}

	eventIDs := make([]string, len(items))
	for i, it := range items {
		eventIDs[i] = it.ID
	}

	reactions, err := s.repo.ListReactionsByEvents(ctx, eventIDs, tenantID)
	if err != nil {
		return err
	}

	commentCounts, err := s.repo.ListCommentCountsByEvents(ctx, eventIDs, tenantID)
	if err != nil {
		return err
	}

	reactionsByEvent := map[string][]repository.FeedReaction{}
	for _, r := range reactions {
		reactionsByEvent[r.EventID] = append(reactionsByEvent[r.EventID], r)
	}

	for i := range items {
		items[i].Reactions = buildReactionSummary(reactionsByEvent[items[i].ID], userID)
		if cnt, ok := commentCounts[items[i].ID]; ok {
			items[i].CommentCount = cnt
		}
	}

	return nil
}

// mergeUpcomingAppointments prepends upcoming appointments to the feed entries (page 1 only).
func (s *Service) mergeUpcomingAppointments(ctx context.Context, tenantID uuid.UUID, entries []repository.ActivityFeedEntry) ([]repository.ActivityFeedEntry, error) {
	upcoming, err := s.repo.ListUpcomingAppointments(ctx, tenantID, 5)
	if err != nil {
		return nil, err
	}
	if len(upcoming) == 0 {
		return entries, nil
	}

	existing := make(map[uuid.UUID]struct{}, len(entries))
	for _, entry := range entries {
		existing[entry.EntityID] = struct{}{}
	}

	filtered := make([]repository.ActivityFeedEntry, 0, len(upcoming))
	for _, entry := range upcoming {
		if _, seen := existing[entry.EntityID]; seen {
			continue
		}
		filtered = append(filtered, entry)
	}

	if len(filtered) > 0 {
		return append(filtered, entries...), nil
	}
	return entries, nil
}

// mapEntryToFeedItem converts a repository entry into a transport feed item.
func mapEntryToFeedItem(e repository.ActivityFeedEntry) transport.ActivityFeedItem {
	item := transport.ActivityFeedItem{
		ID:          e.ID.String(),
		Type:        e.EventType,
		Category:    e.Category,
		Title:       mapActivityTitle(e.Category, e.EventType, e.Title, e.ScheduledAt),
		Description: e.Description,
		Timestamp:   e.CreatedAt.Format(time.RFC3339),
	}

	populateFeedItemFields(&item, &e)
	assignFeedItemLink(&item, &e)
	enrichFeedItem(&item, &e)

	return item
}

// populateFeedItemFields sets optional fields from the entry onto the feed item.
func populateFeedItemFields(item *transport.ActivityFeedItem, e *repository.ActivityFeedEntry) {
	if e.LeadName != "" {
		item.LeadName = e.LeadName
	}
	if e.Phone != "" {
		item.Phone = e.Phone
	}
	if e.Email != "" {
		item.Email = e.Email
	}
	if e.LeadStatus != "" {
		item.LeadStatus = e.LeadStatus
	}
	if e.ServiceType != "" {
		item.ServiceType = e.ServiceType
	}
	if e.LeadScore != nil {
		item.LeadScore = e.LeadScore
	}
	if e.Address != "" {
		item.Address = e.Address
	}
	if e.Latitude != nil {
		item.Latitude = e.Latitude
	}
	if e.Longitude != nil {
		item.Longitude = e.Longitude
	}
	if e.ScheduledAt != nil {
		item.ScheduledAt = e.ScheduledAt.Format(time.RFC3339)
	}
	if e.Priority > 0 {
		item.Priority = e.Priority
	}
}

// assignFeedItemLink builds the navigation link for the feed item based on category.
func assignFeedItemLink(item *transport.ActivityFeedItem, e *repository.ActivityFeedEntry) {
	switch e.Category {
	case "leads":
		item.Link = []string{"leads", e.EntityID.String()}
	case "quotes":
		item.Link = []string{"offertes", e.EntityID.String()}
	case "appointments":
		item.Link = []string{"appointments"}
	case "ai":
		item.Link = []string{"leads", e.EntityID.String()}
	}
}

// mapActivityTitle translates raw event types into human-readable Dutch titles.
func mapActivityTitle(category, eventType, rawTitle string, scheduledAt *time.Time) string {
	switch eventType {
	// Lead events
	case "lead_created":
		return "Nieuwe lead aangemaakt"
	case "lead_updated":
		return "Lead bijgewerkt"
	case "status_change":
		switch category {
		case "quotes":
			return "Offerte status gewijzigd"
		case "appointments":
			return "Afspraak status gewijzigd"
		default:
			return "Lead bijgewerkt"
		}
	case "lead_assigned":
		return "Lead toegewezen"
	case "lead_viewed":
		return "Lead bekeken"
	case "note_added":
		return "Notitie toegevoegd"
	case "call_logged":
		return "Gesprek gelogd"
	// AI events
	case "analysis_complete":
		return "Gatekeeper analyse voltooid"
	case "photo_analysis_complete", "photo_analysis_completed":
		return "Foto-analyse voltooid"
	// Partner events
	case "partner_offer_accepted":
		return "Partner offerte geaccepteerd"
	case "partner_offer_rejected":
		return "Partner offerte afgewezen"
	// Pipeline / triage events
	case "manual_intervention":
		return "Handmatige interventie vereist"
	case "gatekeeper_rejected":
		return "Gatekeeper heeft lead afgewezen"
	case "lead_lost":
		return "Lead verloren"
	// Quote events (rawTitle already contains the human-readable message)
	case "quote_sent", "quote_viewed", "quote_accepted", "quote_rejected",
		"quote_item_toggled", "quote_annotated":
		if rawTitle != "" {
			return rawTitle
		}
		return "Offerte activiteit"
	// Appointment events
	case "appointment_created":
		if rawTitle != "" {
			return "Nieuwe afspraak: " + rawTitle
		}
		return "Nieuwe afspraak"
	case "appointment_updated":
		if rawTitle != "" {
			return "Afspraak bijgewerkt: " + rawTitle
		}
		return "Afspraak bijgewerkt"
	case "appointment_upcoming":
		return formatUpcomingTitle(scheduledAt, rawTitle)
	default:
		if rawTitle != "" {
			return rawTitle
		}
		// Use category for a friendlier fallback than the raw eventType
		switch category {
		case "leads":
			return "Lead activiteit"
		case "quotes":
			return "Offerte activiteit"
		case "appointments":
			return "Afspraak activiteit"
		default:
			return eventType
		}
	}
}

func formatUpcomingTitle(scheduledAt *time.Time, fallback string) string {
	if scheduledAt == nil {
		if fallback != "" {
			return "Afspraak binnenkort: " + fallback
		}
		return "Afspraak binnenkort"
	}

	start := *scheduledAt
	until := time.Until(start)
	minutes := int(math.Round(until.Minutes()))
	if minutes <= 60 {
		return appendTitle("Afspraak begint zo", fallback)
	}
	if minutes <= 180 {
		hours := int(math.Round(float64(minutes) / 60.0))
		return appendTitle("Afspraak over "+strconv.Itoa(hours)+" uur", fallback)
	}

	datePart := start.Format("02 Jan")
	timePart := start.Format("15:04")
	if minutes <= 24*60 {
		return appendTitle("Afspraak vandaag om "+timePart, fallback)
	}
	if minutes <= 48*60 {
		return appendTitle("Afspraak morgen om "+timePart, fallback)
	}
	return appendTitle("Afspraak op "+datePart+" om "+timePart, fallback)
}

func appendTitle(label string, fallback string) string {
	if fallback == "" {
		return label
	}
	return label + ": " + fallback
}
