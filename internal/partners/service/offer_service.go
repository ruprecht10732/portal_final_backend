package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"portal_final_backend/internal/auth/token"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/partners/repository"
	"portal_final_backend/internal/partners/transport"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/sanitize"

	"github.com/google/uuid"
)

type OfferSummaryItem struct {
	Description string
	Quantity    string
}

// OfferSummaryInput defines the fields sent to the summary generator (no PII).
type OfferSummaryInput struct {
	LeadID        uuid.UUID
	LeadServiceID uuid.UUID
	ServiceType   string
	Scope         *string
	UrgencyLevel  *string
	Items         []OfferSummaryItem
}

// OfferSummaryGenerator generates a markdown summary for partner offers.
type OfferSummaryGenerator interface {
	GenerateSummary(ctx context.Context, tenantID uuid.UUID, input OfferSummaryInput) (string, error)
}

const (
	offerTokenBytes = 32
	// platformFeeMultiplier: vakman receives 90% of customer price (10% platform fee).
	platformFeeMultiplier = 0.90
)

// CreateOffer generates a new job offer for a vakman based on customer pricing.
func (s *Service) CreateOffer(ctx context.Context, tenantID uuid.UUID, req transport.CreateOfferRequest) (transport.CreateOfferResponse, error) {
	partner, err := s.repo.GetByID(ctx, req.PartnerID, tenantID)
	if err != nil {
		return transport.CreateOfferResponse{}, err
	}

	serviceCtx, err := s.resolveOfferContext(ctx, tenantID, req.LeadServiceID)
	if err != nil {
		return transport.CreateOfferResponse{}, err
	}

	if err := s.ensureOfferAvailable(ctx, req.LeadServiceID); err != nil {
		return transport.CreateOfferResponse{}, err
	}

	rawToken, err := token.GenerateRandomToken(offerTokenBytes)
	if err != nil {
		return transport.CreateOfferResponse{}, err
	}

	vakmanPrice := calculateVakmanPrice(req.CustomerPriceCents)
	expiry := time.Now().UTC().Add(time.Duration(req.ExpiresInHours) * time.Hour)
	items := s.fetchOfferItems(ctx, req.LeadServiceID, tenantID)

	scopeAssessment := buildScopeAssessment(items)
	builderSummaryPtr := s.resolveBuilderSummary(ctx, tenantID, items, scopeAssessment, serviceCtx, req.LeadServiceID)
	jobSummaryPtr := sanitizeJobSummary(req.JobSummaryShort)

	offer, err := s.repo.CreateOffer(ctx, repository.PartnerOffer{
		OrganizationID:     tenantID,
		PartnerID:          req.PartnerID,
		LeadServiceID:      req.LeadServiceID,
		PublicToken:        rawToken,
		ExpiresAt:          expiry,
		PricingSource:      req.PricingSource,
		CustomerPriceCents: req.CustomerPriceCents,
		VakmanPriceCents:   vakmanPrice,
		JobSummaryShort:    jobSummaryPtr,
		BuilderSummary:     builderSummaryPtr,
	})
	if err != nil {
		return transport.CreateOfferResponse{}, err
	}

	s.publishOfferCreated(ctx, offerCreatedParams{
		offerID:       offer.ID,
		tenantID:      tenantID,
		partnerID:     req.PartnerID,
		leadServiceID: req.LeadServiceID,
		leadID:        serviceCtx.LeadID,
		vakmanPrice:   vakmanPrice,
		rawToken:      rawToken,
		partner:       partner,
	})

	return transport.CreateOfferResponse{
		ID:               offer.ID,
		PublicToken:      rawToken,
		VakmanPriceCents: vakmanPrice,
		ExpiresAt:        expiry,
	}, nil
}

// GetPublicOffer retrieves offer details for the vakman-facing view.
// Only exposes the vakman's price — customer markup is never served.
func (s *Service) GetPublicOffer(ctx context.Context, publicToken string) (transport.PublicOfferResponse, error) {
	oc, err := s.repo.GetOfferByToken(ctx, publicToken)
	if err != nil {
		return transport.PublicOfferResponse{}, err
	}

	items, itemsErr := s.repo.GetLatestQuoteItemsForService(ctx, oc.LeadServiceID, oc.OrganizationID)
	if itemsErr != nil {
		items = nil
	}
	scopeAssessment := buildScopeAssessment(items)
	builderSummary := oc.BuilderSummary
	if builderSummary == nil {
		builderSummary = buildBuilderSummary(items, scopeAssessment, oc.UrgencyLevel)
	}

	return transport.PublicOfferResponse{
		OfferID:          oc.ID,
		OrganizationName: oc.OrganizationName,
		JobSummary:       oc.ServiceType,
		JobSummaryShort:  oc.JobSummaryShort,
		BuilderSummary:   builderSummary,
		City:             oc.LeadCity,
		Postcode4:        oc.LeadPostcode4,
		Buurtcode:        oc.LeadBuurtcode,
		ConstructionYear: oc.LeadEnergyBouwjaar,
		ScopeAssessment:  scopeAssessment,
		UrgencyLevel:     oc.UrgencyLevel,
		VakmanPriceCents: oc.VakmanPriceCents,
		PricingSource:    oc.PricingSource,
		Status:           oc.Status,
		ExpiresAt:        oc.ExpiresAt,
		CreatedAt:        oc.CreatedAt,
	}, nil
}

// AcceptOffer processes a vakman's acceptance, locks the job via the unique index.
func (s *Service) AcceptOffer(ctx context.Context, publicToken string, req transport.AcceptOfferRequest) error {
	oc, err := s.repo.GetOfferByToken(ctx, publicToken)
	if err != nil {
		return err
	}

	// Validation
	if time.Now().After(oc.ExpiresAt) {
		return apperr.Gone("this offer has expired")
	}
	if oc.Status != "pending" && oc.Status != "sent" {
		return apperr.Conflict("offer cannot be accepted in current state")
	}

	// Serialize availability slots
	inspectionJSON, err := json.Marshal(req.InspectionSlots)
	if err != nil {
		return apperr.Validation("invalid inspection slots")
	}

	var jobJSON []byte
	if len(req.JobSlots) > 0 {
		jobJSON, err = json.Marshal(req.JobSlots)
		if err != nil {
			return apperr.Validation("invalid job slots")
		}
	}

	// Atomic update (unique index enforces exclusivity)
	if err := s.repo.AcceptOffer(ctx, oc.ID, inspectionJSON, jobJSON); err != nil {
		return err
	}

	// Resolve lead ID for timeline/notification handlers
	leadID, _ := s.repo.GetLeadIDForService(ctx, oc.LeadServiceID, oc.OrganizationID)

	// Resolve partner details for confirmation
	var partnerEmail string
	var partnerPhone string
	var partnerWhatsAppOptedIn bool
	if partner, pErr := s.repo.GetByID(ctx, oc.PartnerID, oc.OrganizationID); pErr == nil {
		partnerEmail = partner.ContactEmail
		partnerPhone = partner.ContactPhone
		partnerWhatsAppOptedIn = partner.WhatsAppOptedIn
	}

	// Publish event
	s.eventBus.Publish(ctx, events.PartnerOfferAccepted{
		BaseEvent:              events.NewBaseEvent(),
		OfferID:                oc.ID,
		OrganizationID:         oc.OrganizationID,
		PartnerID:              oc.PartnerID,
		LeadServiceID:          oc.LeadServiceID,
		LeadID:                 leadID,
		PartnerName:            oc.PartnerName,
		PartnerEmail:           partnerEmail,
		PartnerPhone:           partnerPhone,
		PartnerWhatsAppOptedIn: partnerWhatsAppOptedIn,
	})

	return nil
}

// RejectOffer processes a vakman's rejection of an offer.
func (s *Service) RejectOffer(ctx context.Context, publicToken string, req transport.RejectOfferRequest) error {
	oc, err := s.repo.GetOfferByToken(ctx, publicToken)
	if err != nil {
		return err
	}

	if oc.Status != "pending" && oc.Status != "sent" {
		return apperr.Conflict("offer cannot be rejected in current state")
	}

	if err := s.repo.RejectOffer(ctx, oc.ID, req.Reason); err != nil {
		return err
	}

	// Resolve lead ID for timeline/notification handlers
	leadID, _ := s.repo.GetLeadIDForService(ctx, oc.LeadServiceID, oc.OrganizationID)

	s.eventBus.Publish(ctx, events.PartnerOfferRejected{
		BaseEvent:      events.NewBaseEvent(),
		OfferID:        oc.ID,
		OrganizationID: oc.OrganizationID,
		PartnerID:      oc.PartnerID,
		LeadServiceID:  oc.LeadServiceID,
		LeadID:         leadID,
		PartnerName:    oc.PartnerName,
		Reason:         req.Reason,
	})

	return nil
}

// GetOfferPreview returns the same vakman-facing view but requires authentication.
// This lets admin users preview what the vakman sees.
func (s *Service) GetOfferPreview(ctx context.Context, tenantID uuid.UUID, offerID uuid.UUID) (transport.PublicOfferResponse, error) {
	oc, err := s.repo.GetOfferByIDWithContext(ctx, offerID, tenantID)
	if err != nil {
		return transport.PublicOfferResponse{}, err
	}

	items, itemsErr := s.repo.GetLatestQuoteItemsForService(ctx, oc.LeadServiceID, oc.OrganizationID)
	if itemsErr != nil {
		items = nil
	}
	scopeAssessment := buildScopeAssessment(items)
	builderSummary := oc.BuilderSummary
	if builderSummary == nil {
		builderSummary = buildBuilderSummary(items, scopeAssessment, oc.UrgencyLevel)
	}

	return transport.PublicOfferResponse{
		OfferID:          oc.ID,
		OrganizationName: oc.OrganizationName,
		JobSummary:       oc.ServiceType,
		JobSummaryShort:  oc.JobSummaryShort,
		BuilderSummary:   builderSummary,
		City:             oc.LeadCity,
		Postcode4:        oc.LeadPostcode4,
		Buurtcode:        oc.LeadBuurtcode,
		ConstructionYear: oc.LeadEnergyBouwjaar,
		ScopeAssessment:  scopeAssessment,
		UrgencyLevel:     oc.UrgencyLevel,
		VakmanPriceCents: oc.VakmanPriceCents,
		PricingSource:    oc.PricingSource,
		Status:           oc.Status,
		ExpiresAt:        oc.ExpiresAt,
		CreatedAt:        oc.CreatedAt,
	}, nil
}

func buildBuilderSummary(items []repository.QuoteItemSummary, scopeAssessment *string, urgencyLevel *string) *string {
	if len(items) == 0 {
		return nil
	}

	lines := buildSummaryHeader(scopeAssessment, urgencyLevel)
	lines = append(lines, buildSummaryItems(items)...)
	if len(lines) == 0 {
		return nil
	}

	result := strings.Join(lines, "\n")
	return &result
}

func (s *Service) resolveOfferContext(ctx context.Context, tenantID, leadServiceID uuid.UUID) (repository.LeadServiceSummaryContext, error) {
	if err := s.ensureLeadServiceExists(ctx, tenantID, leadServiceID); err != nil {
		return repository.LeadServiceSummaryContext{}, err
	}
	return s.repo.GetLeadServiceSummaryContext(ctx, leadServiceID, tenantID)
}

func (s *Service) ensureOfferAvailable(ctx context.Context, leadServiceID uuid.UUID) error {
	hasActive, err := s.repo.HasActiveOffer(ctx, leadServiceID)
	if err != nil {
		return err
	}
	if hasActive {
		return apperr.Conflict("an active offer already exists for this service")
	}
	return nil
}

func (s *Service) fetchOfferItems(ctx context.Context, leadServiceID, tenantID uuid.UUID) []repository.QuoteItemSummary {
	items, err := s.repo.GetLatestQuoteItemsForService(ctx, leadServiceID, tenantID)
	if err != nil {
		return nil
	}
	return items
}

func (s *Service) resolveBuilderSummary(ctx context.Context, tenantID uuid.UUID, items []repository.QuoteItemSummary, scopeAssessment *string, serviceCtx repository.LeadServiceSummaryContext, leadServiceID uuid.UUID) *string {
	if s.summaryGenerator != nil && len(items) > 0 {
		inputItems := make([]OfferSummaryItem, 0, len(items))
		for _, it := range items {
			inputItems = append(inputItems, OfferSummaryItem{
				Description: it.Description,
				Quantity:    it.Quantity,
			})
		}
		summary, err := s.summaryGenerator.GenerateSummary(ctx, tenantID, OfferSummaryInput{
			LeadID:        serviceCtx.LeadID,
			LeadServiceID: leadServiceID,
			ServiceType:   serviceCtx.ServiceType,
			Scope:         scopeAssessment,
			UrgencyLevel:  serviceCtx.UrgencyLevel,
			Items:         inputItems,
		})
		if err == nil {
			clean := strings.TrimSpace(sanitize.Text(summary))
			if clean != "" {
				return &clean
			}
		}
	}

	return buildBuilderSummary(items, scopeAssessment, serviceCtx.UrgencyLevel)
}

func sanitizeJobSummary(value string) *string {
	jobSummary := strings.TrimSpace(value)
	jobSummary = sanitize.Text(jobSummary)
	if jobSummary == "" {
		return nil
	}
	return &jobSummary
}

type offerCreatedParams struct {
	offerID       uuid.UUID
	tenantID      uuid.UUID
	partnerID     uuid.UUID
	leadServiceID uuid.UUID
	leadID        uuid.UUID
	vakmanPrice   int64
	rawToken      string
	partner       repository.Partner
}

func (s *Service) publishOfferCreated(ctx context.Context, params offerCreatedParams) {
	if s.eventBus == nil {
		return
	}
	s.eventBus.Publish(ctx, events.PartnerOfferCreated{
		BaseEvent:        events.NewBaseEvent(),
		OfferID:          params.offerID,
		OrganizationID:   params.tenantID,
		PartnerID:        params.partnerID,
		LeadServiceID:    params.leadServiceID,
		LeadID:           params.leadID,
		VakmanPriceCents: params.vakmanPrice,
		PublicToken:      params.rawToken,
		PartnerName:      params.partner.BusinessName,
		PartnerPhone:     params.partner.ContactPhone,
	})
}

func buildSummaryHeader(scopeAssessment *string, urgencyLevel *string) []string {
	scopeLabel := mapScopeLabel(scopeAssessment)
	urgencyLabel := mapUrgencyLabel(urgencyLevel)
	if scopeLabel == "" && urgencyLabel == "" {
		return nil
	}
	if scopeLabel == "" {
		scopeLabel = "Onbekend"
	}
	if urgencyLabel == "" {
		urgencyLabel = "Onbekend"
	}

	return []string{
		fmt.Sprintf("**Omvang** %s  **Urgentie** %s", scopeLabel, urgencyLabel),
		"",
	}
}

func buildSummaryItems(items []repository.QuoteItemSummary) []string {
	const maxItems = 3
	remaining := len(items) - maxItems
	if remaining < 0 {
		remaining = 0
	}

	lines := make([]string, 0, len(items))
	for i, it := range items {
		if i >= maxItems {
			break
		}
		main, inclusions := buildSummaryItem(it, remaining > 0 && i == maxItems-1, remaining)
		if main == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, main))
		for _, inc := range inclusions {
			lines = append(lines, "   - "+inc)
		}
	}

	return lines
}

func buildSummaryItem(item repository.QuoteItemSummary, isLast bool, remaining int) (string, []string) {
	quantity := strings.TrimSpace(item.Quantity)
	main, inclusions := splitInclusions(item.Description)
	main = strings.TrimSpace(main)
	if main == "" {
		return "", nil
	}
	if quantity != "" {
		main = strings.TrimSpace(quantity + " " + main)
	}
	if len(inclusions) > 0 {
		main += " Inclusief:"
	}
	if isLast {
		main = fmt.Sprintf("%s (+%d)", main, remaining)
	}

	return main, inclusions
}

func buildScopeAssessment(items []repository.QuoteItemSummary) *string {
	count := len(items)
	if count == 0 {
		return nil
	}
	var scope string
	switch {
	case count <= 2:
		scope = "Small"
	case count <= 5:
		scope = "Medium"
	default:
		scope = "Large"
	}
	return &scope
}

func mapScopeLabel(scope *string) string {
	if scope == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(*scope)) {
	case "small":
		return "Klein"
	case "medium":
		return "Middel"
	case "large":
		return "Groot"
	default:
		return ""
	}
}

func mapUrgencyLabel(level *string) string {
	if level == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(*level)) {
	case "low":
		return "Laag"
	case "medium":
		return "Gemiddeld"
	case "high":
		return "Hoog"
	default:
		return ""
	}
}

func splitInclusions(description string) (string, []string) {
	parts := strings.SplitN(description, "Inclusief:", 2)
	main := strings.TrimSpace(parts[0])
	if len(parts) < 2 {
		return main, nil
	}

	return main, parseInclusionList(parts[1])
}

func parseInclusionList(value string) []string {
	cleaned := strings.ReplaceAll(value, "\n\n", "\n")
	cleaned = strings.ReplaceAll(cleaned, " - ", "\n- ")
	lines := strings.Split(cleaned, "\n")

	items := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "-") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		}
		if line != "" {
			items = append(items, line)
		}
	}

	return items
}

// ListOffersByPartner returns all offers for a given partner (admin view).
func (s *Service) ListOffersByPartner(ctx context.Context, tenantID uuid.UUID, partnerID uuid.UUID) (transport.ListOffersResponse, error) {
	if err := s.ensurePartnerExists(ctx, tenantID, partnerID); err != nil {
		return transport.ListOffersResponse{}, err
	}

	offers, err := s.repo.ListOffersByPartner(ctx, partnerID, tenantID)
	if err != nil {
		return transport.ListOffersResponse{}, err
	}

	items := make([]transport.OfferResponse, 0, len(offers))
	for _, o := range offers {
		items = append(items, mapOfferResponse(o))
	}

	return transport.ListOffersResponse{Items: items}, nil
}

// ListOffersForService returns all offers for a given lead service (admin view).
func (s *Service) ListOffersForService(ctx context.Context, tenantID uuid.UUID, leadServiceID uuid.UUID) (transport.ListOffersResponse, error) {
	offers, err := s.repo.ListOffersForService(ctx, leadServiceID, tenantID)
	if err != nil {
		return transport.ListOffersResponse{}, err
	}

	items := make([]transport.OfferResponse, 0, len(offers))
	for _, o := range offers {
		items = append(items, mapOfferResponse(o))
	}

	return transport.ListOffersResponse{Items: items}, nil
}

// ExpireOffers is called by a background job to expire stale offers.
func (s *Service) ExpireOffers(ctx context.Context) (int, error) {
	expired, err := s.repo.ExpireOffers(ctx)
	if err != nil {
		return 0, err
	}

	for _, o := range expired {
		// Resolve lead ID and partner name for timeline handlers
		leadID, _ := s.repo.GetLeadIDForService(ctx, o.LeadServiceID, o.OrganizationID)
		var partnerName string
		if p, err := s.repo.GetByID(ctx, o.PartnerID, o.OrganizationID); err == nil {
			partnerName = p.BusinessName
		}

		s.eventBus.Publish(ctx, events.PartnerOfferExpired{
			BaseEvent:      events.NewBaseEvent(),
			OfferID:        o.ID,
			OrganizationID: o.OrganizationID,
			PartnerID:      o.PartnerID,
			LeadServiceID:  o.LeadServiceID,
			LeadID:         leadID,
			PartnerName:    partnerName,
		})
	}

	return len(expired), nil
}

func mapOfferResponse(oc repository.PartnerOfferWithContext) transport.OfferResponse {
	resp := transport.OfferResponse{
		ID:                 oc.ID,
		PartnerID:          oc.PartnerID,
		PartnerName:        oc.PartnerName,
		LeadServiceID:      oc.LeadServiceID,
		PricingSource:      oc.PricingSource,
		CustomerPriceCents: oc.CustomerPriceCents,
		VakmanPriceCents:   oc.VakmanPriceCents,
		Status:             oc.Status,
		PublicToken:        oc.PublicToken,
		ExpiresAt:          oc.ExpiresAt,
		AcceptedAt:         oc.AcceptedAt,
		RejectedAt:         oc.RejectedAt,
		CreatedAt:          oc.CreatedAt,
	}
	if oc.RejectionReason != nil {
		resp.RejectionReason = *oc.RejectionReason
	}
	return resp
}

func calculateVakmanPrice(customerPriceCents int64) int64 {
	return int64(float64(customerPriceCents) * platformFeeMultiplier)
}
