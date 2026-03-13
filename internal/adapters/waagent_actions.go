package adapters

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	apptsvc "portal_final_backend/internal/appointments/service"
	appttransport "portal_final_backend/internal/appointments/transport"
	catalogsvc "portal_final_backend/internal/catalog/service"
	catalogtransport "portal_final_backend/internal/catalog/transport"
	leadsmgmt "portal_final_backend/internal/leads/management"
	"portal_final_backend/internal/leads/ports"
	leadsrepo "portal_final_backend/internal/leads/repository"
	leadtransport "portal_final_backend/internal/leads/transport"
	"portal_final_backend/internal/waagent"
	"portal_final_backend/platform/phone"

	"github.com/google/uuid"
)

const waagentActorName = "Reinout"

const errInvalidLeadID = "invalid lead_id"
const errLeadManagementNotConfigured = "lead management service not configured"

type WAAgentLeadActionsAdapter struct {
	mgmt *leadsmgmt.Service
	repo leadsrepo.LeadsRepository
}

func NewWAAgentLeadActionsAdapter(mgmt *leadsmgmt.Service, repo leadsrepo.LeadsRepository) *WAAgentLeadActionsAdapter {
	return &WAAgentLeadActionsAdapter{mgmt: mgmt, repo: repo}
}

func (a *WAAgentLeadActionsAdapter) SearchLeads(ctx context.Context, orgID uuid.UUID, query string, limit int) ([]waagent.LeadSearchResult, error) {
	if a.mgmt == nil {
		return nil, errors.New(errLeadManagementNotConfigured)
	}
	if limit <= 0 {
		limit = 10
	}
	query = strings.TrimSpace(query)
	log.Printf("waagent: SearchLeads org=%s query=%q limit=%d", orgID, query, limit)

	// Tier 1: ILIKE search via the management service.
	// Multi-word queries (e.g. "Johan Kuiper") are split into first/last name
	// filters so the AND-based ILIKE matching works correctly.
	// If the split search returns nothing, retry with the full query as a
	// combined OR-search across all fields.
	results := a.iLikeSearchLeads(ctx, orgID, query, limit)
	log.Printf("waagent: SearchLeads tier=ilike org=%s query=%q results=%d", orgID, query, len(results))

	// Tier 2: Fuzzy pg_trgm fallback for partial matches and minor typos.
	if len(results) == 0 && a.repo != nil {
		results = a.fuzzySearchLeads(ctx, orgID, query, limit)
		log.Printf("waagent: SearchLeads tier=fuzzy org=%s query=%q results=%d", orgID, query, len(results))
	}

	// Tier 3: Quote-based search — finds leads through their associated quotes
	// even when the lead itself has been soft-deleted.
	if len(results) == 0 && a.repo != nil {
		results = a.quoteBasedLeadSearch(ctx, orgID, query, limit)
		log.Printf("waagent: SearchLeads tier=quote org=%s query=%q results=%d", orgID, query, len(results))
	}

	log.Printf("waagent: SearchLeads final org=%s query=%q total_results=%d", orgID, query, len(results))
	return results, nil
}

// iLikeSearchLeads performs a standard ILIKE-based lead search via the management service.
// For multi-word queries it first tries a split FirstName+LastName AND-match;
// if that returns nothing it falls back to a combined OR-search across all fields.
func (a *WAAgentLeadActionsAdapter) iLikeSearchLeads(ctx context.Context, orgID uuid.UUID, query string, limit int) []waagent.LeadSearchResult {
	req := leadtransport.ListLeadsRequest{Page: 1, PageSize: limit}
	parts := strings.Fields(query)
	if len(parts) >= 2 {
		req.FirstName = parts[0]
		req.LastName = strings.Join(parts[1:], " ")
	} else {
		req.Search = query
	}
	log.Printf("waagent: iLikeSearch org=%s firstName=%q lastName=%q search=%q", orgID, req.FirstName, req.LastName, req.Search)
	resp, err := a.mgmt.List(ctx, req, orgID)
	if err != nil {
		log.Printf("waagent: iLikeSearch error org=%s: %v", orgID, err)
		return nil
	}
	log.Printf("waagent: iLikeSearch org=%s split_results=%d", orgID, len(resp.Items))

	// Fallback: if split first/last returned nothing, retry with combined OR-search.
	if len(resp.Items) == 0 && len(parts) >= 2 {
		req = leadtransport.ListLeadsRequest{Page: 1, PageSize: limit, Search: query}
		log.Printf("waagent: iLikeSearch fallback org=%s search=%q", orgID, req.Search)
		resp, err = a.mgmt.List(ctx, req, orgID)
		if err != nil {
			log.Printf("waagent: iLikeSearch fallback error org=%s: %v", orgID, err)
			return nil
		}
		log.Printf("waagent: iLikeSearch fallback org=%s results=%d", orgID, len(resp.Items))
	}
	results := make([]waagent.LeadSearchResult, 0, len(resp.Items))
	for _, item := range resp.Items {
		serviceID := ""
		serviceType := ""
		status := ""
		if item.CurrentService != nil {
			serviceID = item.CurrentService.ID.String()
			serviceType = string(item.CurrentService.ServiceType)
			status = string(item.CurrentService.Status)
		}
		results = append(results, waagent.LeadSearchResult{
			LeadID:        item.ID.String(),
			LeadServiceID: serviceID,
			CustomerName:  strings.TrimSpace(item.Consumer.FirstName + " " + item.Consumer.LastName),
			Phone:         item.Consumer.Phone,
			City:          item.Address.City,
			ServiceType:   serviceType,
			Status:        status,
			CreatedAt:     item.CreatedAt.Format(time.RFC3339),
		})
	}
	return results
}

// fuzzySearchLeads performs a pg_trgm similarity-based lead search as a fallback.
// Returns nil (no error) if pg_trgm is unavailable so the caller degrades gracefully.
func (a *WAAgentLeadActionsAdapter) fuzzySearchLeads(ctx context.Context, orgID uuid.UUID, query string, limit int) []waagent.LeadSearchResult {
	matches, err := a.repo.FuzzySearchLeads(ctx, orgID, query, limit)
	if err != nil {
		log.Printf("waagent: fuzzySearch error org=%s query=%q: %v", orgID, query, err)
		return nil
	}
	if len(matches) == 0 {
		log.Printf("waagent: fuzzySearch org=%s query=%q no matches", orgID, query)
		return nil
	}
	results := make([]waagent.LeadSearchResult, 0, len(matches))
	for _, m := range matches {
		serviceID := ""
		if m.ServiceID != nil {
			serviceID = m.ServiceID.String()
		}
		results = append(results, waagent.LeadSearchResult{
			LeadID:        m.LeadID.String(),
			LeadServiceID: serviceID,
			CustomerName:  strings.TrimSpace(m.FirstName + " " + m.LastName),
			Phone:         m.Phone,
			City:          m.City,
			ServiceType:   m.ServiceType,
			Status:        m.ServiceStatus,
			CreatedAt:     m.CreatedAt.Format(time.RFC3339),
		})
	}
	return results
}

func (a *WAAgentLeadActionsAdapter) GetLeadDetails(ctx context.Context, orgID uuid.UUID, leadIDRaw string) (*waagent.LeadDetailsResult, error) {
	if a.mgmt == nil {
		return nil, errors.New(errLeadManagementNotConfigured)
	}
	leadID, err := uuid.Parse(strings.TrimSpace(leadIDRaw))
	if err != nil {
		return nil, errors.New(errInvalidLeadID)
	}
	resp, err := a.mgmt.GetByID(ctx, leadID, orgID)
	if err != nil {
		return nil, err
	}
	serviceType := ""
	status := ""
	if resp.CurrentService != nil {
		serviceType = string(resp.CurrentService.ServiceType)
		status = string(resp.CurrentService.Status)
	}
	fullAddress := strings.TrimSpace(strings.TrimSpace(resp.Address.Street) + " " + strings.TrimSpace(resp.Address.HouseNumber) + ", " + strings.TrimSpace(resp.Address.ZipCode) + " " + strings.TrimSpace(resp.Address.City))
	return &waagent.LeadDetailsResult{
		LeadID:       resp.ID.String(),
		CustomerName: strings.TrimSpace(resp.Consumer.FirstName + " " + resp.Consumer.LastName),
		Phone:        resp.Consumer.Phone,
		Email:        derefString(resp.Consumer.Email),
		Street:       resp.Address.Street,
		HouseNumber:  resp.Address.HouseNumber,
		ZipCode:      resp.Address.ZipCode,
		City:         resp.Address.City,
		FullAddress:  strings.Trim(fullAddress, ", "),
		ServiceType:  serviceType,
		Status:       status,
	}, nil
}

func (a *WAAgentLeadActionsAdapter) CreateLead(ctx context.Context, orgID uuid.UUID, input waagent.CreateLeadInput) (waagent.CreateLeadOutput, error) {
	if a.mgmt == nil {
		return waagent.CreateLeadOutput{}, errors.New(errLeadManagementNotConfigured)
	}
	missing := missingCreateLeadFields(input)
	if len(missing) > 0 {
		return waagent.CreateLeadOutput{
			Success:       false,
			Message:       "Er ontbreken nog gegevens om de lead aan te maken",
			MissingFields: missing,
		}, nil
	}
	req := leadtransport.CreateLeadRequest{
		FirstName:    strings.TrimSpace(*input.FirstName),
		LastName:     strings.TrimSpace(*input.LastName),
		Phone:        strings.TrimSpace(phone.NormalizeE164(*input.Phone)),
		ConsumerRole: leadtransport.ConsumerRole(strings.TrimSpace(*input.ConsumerRole)),
		Street:       strings.TrimSpace(*input.Street),
		HouseNumber:  strings.TrimSpace(*input.HouseNumber),
		ZipCode:      strings.TrimSpace(*input.ZipCode),
		City:         strings.TrimSpace(*input.City),
		ServiceType:  leadtransport.ServiceType(strings.TrimSpace(*input.ServiceType)),
		Source:       "whatsapp_agent",
	}
	if input.Email != nil {
		req.Email = strings.TrimSpace(*input.Email)
	}
	if input.ConsumerNote != nil {
		req.ConsumerNote = strings.TrimSpace(*input.ConsumerNote)
	}
	resp, err := a.mgmt.Create(ctx, req, orgID)
	if err != nil {
		return waagent.CreateLeadOutput{}, err
	}
	serviceID := ""
	serviceType := ""
	if resp.CurrentService != nil {
		serviceID = resp.CurrentService.ID.String()
		serviceType = string(resp.CurrentService.ServiceType)
	}
	if serviceType == "" && len(resp.Services) > 0 {
		serviceID = resp.Services[0].ID.String()
		serviceType = string(resp.Services[0].ServiceType)
	}
	return waagent.CreateLeadOutput{
		Success: true,
		Message: "Lead aangemaakt",
		Lead: &waagent.CreateLeadResult{
			LeadID:        resp.ID.String(),
			LeadServiceID: serviceID,
			CustomerName:  strings.TrimSpace(resp.Consumer.FirstName + " " + resp.Consumer.LastName),
			ServiceType:   serviceType,
		},
	}, nil
}

func missingCreateLeadFields(input waagent.CreateLeadInput) []string {
	missing := make([]string, 0, 9)
	appendMissing := func(name string, value *string) {
		if value == nil || strings.TrimSpace(*value) == "" {
			missing = append(missing, name)
		}
	}
	appendMissing("voornaam", input.FirstName)
	appendMissing("achternaam", input.LastName)
	appendMissing("telefoonnummer", input.Phone)
	appendMissing("rol van de klant", input.ConsumerRole)
	appendMissing("straat", input.Street)
	appendMissing("huisnummer", input.HouseNumber)
	appendMissing("postcode", input.ZipCode)
	appendMissing("plaats", input.City)
	appendMissing("dienst", input.ServiceType)
	return missing
}

func (a *WAAgentLeadActionsAdapter) GetNavigationLink(ctx context.Context, orgID uuid.UUID, leadIDRaw string) (*waagent.NavigationLinkResult, error) {
	leadID, err := uuid.Parse(strings.TrimSpace(leadIDRaw))
	if err != nil {
		return nil, errors.New(errInvalidLeadID)
	}
	lead, err := a.repo.GetByID(ctx, leadID, orgID)
	if err != nil {
		return nil, err
	}
	destination := formatLeadDestination(lead)
	if destination == "" {
		return nil, fmt.Errorf("lead heeft geen bruikbaar adres")
	}
	return &waagent.NavigationLinkResult{
		LeadID:             leadID.String(),
		DestinationAddress: destination,
		URL:                "https://www.google.com/maps/dir/?api=1&destination=" + url.QueryEscape(destination),
	}, nil
}

func formatLeadDestination(lead leadsrepo.Lead) string {
	parts := make([]string, 0, 4)
	streetLine := strings.TrimSpace(strings.TrimSpace(lead.AddressStreet) + " " + strings.TrimSpace(lead.AddressHouseNumber))
	if streetLine != "" {
		parts = append(parts, streetLine)
	}
	zipCity := strings.TrimSpace(strings.TrimSpace(lead.AddressZipCode) + " " + strings.TrimSpace(lead.AddressCity))
	if zipCity != "" {
		parts = append(parts, zipCity)
	}
	if len(parts) == 0 {
		return ""
	}
	parts = append(parts, "Netherlands")
	return strings.Join(parts, ", ")
}

type WAAgentCatalogSearchAdapter struct {
	svc           *catalogsvc.Service
	catalogReader ports.CatalogReader
}

func NewWAAgentCatalogSearchAdapter(svc *catalogsvc.Service, catalogReader ports.CatalogReader) *WAAgentCatalogSearchAdapter {
	return &WAAgentCatalogSearchAdapter{svc: svc, catalogReader: catalogReader}
}

func (a *WAAgentCatalogSearchAdapter) SearchProductMaterials(ctx context.Context, orgID uuid.UUID, input waagent.SearchProductMaterialsInput) (waagent.SearchProductMaterialsOutput, error) {
	if a.svc == nil {
		return waagent.SearchProductMaterialsOutput{Products: []waagent.ProductResult{}, Message: "Catalogus zoeken is niet beschikbaar"}, nil
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return waagent.SearchProductMaterialsOutput{Products: []waagent.ProductResult{}, Message: "Query mag niet leeg zijn"}, nil
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 5
	}
	items, err := a.svc.SearchForAutocomplete(ctx, orgID, catalogtransport.AutocompleteSearchRequest{Query: query, Limit: limit})
	if err != nil {
		return waagent.SearchProductMaterialsOutput{}, err
	}
	products := a.mapAutocompleteProducts(ctx, orgID, items)
	message := "Resultaten gevonden"
	if len(products) == 0 {
		message = "Geen passende producten gevonden"
	}
	return waagent.SearchProductMaterialsOutput{Products: products, Message: message}, nil
}

func (a *WAAgentCatalogSearchAdapter) mapAutocompleteProducts(ctx context.Context, orgID uuid.UUID, items []catalogtransport.AutocompleteItemResponse) []waagent.ProductResult {
	materialsByID := a.hydrateCatalogMaterials(ctx, orgID, items)
	products := make([]waagent.ProductResult, 0, len(items))
	for _, item := range items {
		products = append(products, autocompleteItemToProductResult(item, materialsByID))
	}
	return products
}

func autocompleteItemToProductResult(item catalogtransport.AutocompleteItemResponse, materialsByID map[uuid.UUID][]string) waagent.ProductResult {
	priceCents := item.UnitPriceCents
	if priceCents == 0 {
		priceCents = item.PriceCents
	}
	materials := []string{}
	if item.CatalogProductID != nil {
		materials = materialsByID[*item.CatalogProductID]
	}
	score := 0.0
	if item.Score != nil {
		score = *item.Score
	}
	result := waagent.ProductResult{
		ID:               item.ID,
		Name:             item.Title,
		Description:      derefString(item.Description),
		Type:             autocompleteItemType(item.SourceType),
		PriceEuros:       float64(priceCents) / 100,
		PriceCents:       priceCents,
		Unit:             derefString(item.UnitLabel),
		VatRateBps:       item.VatRateBps,
		Materials:        materials,
		SourceURL:        derefString(item.SourceURL),
		SourceCollection: item.SourceCollection,
		Score:            score,
		HighConfidence:   score >= 0.55,
		Category:         item.SourceLabel,
	}
	if item.CatalogProductID != nil {
		result.ID = item.CatalogProductID.String()
	}
	return result
}

func autocompleteItemType(sourceType string) string {
	if sourceType == "reference" {
		return "material"
	}
	return "product"
}

func (a *WAAgentCatalogSearchAdapter) hydrateCatalogMaterials(ctx context.Context, orgID uuid.UUID, items []catalogtransport.AutocompleteItemResponse) map[uuid.UUID][]string {
	if a.catalogReader == nil {
		return map[uuid.UUID][]string{}
	}
	ids := make([]uuid.UUID, 0)
	seen := make(map[uuid.UUID]struct{})
	for _, item := range items {
		if item.CatalogProductID == nil {
			continue
		}
		if _, ok := seen[*item.CatalogProductID]; ok {
			continue
		}
		seen[*item.CatalogProductID] = struct{}{}
		ids = append(ids, *item.CatalogProductID)
	}
	if len(ids) == 0 {
		return map[uuid.UUID][]string{}
	}
	details, err := a.catalogReader.GetProductDetails(ctx, orgID, ids)
	if err != nil {
		return map[uuid.UUID][]string{}
	}
	out := make(map[uuid.UUID][]string, len(details))
	for _, detail := range details {
		out[detail.ID] = detail.Materials
	}
	return out
}

func (a *WAAgentLeadActionsAdapter) UpdateLeadDetails(ctx context.Context, orgID uuid.UUID, input waagent.UpdateLeadDetailsInput) ([]string, error) {
	if a.mgmt == nil {
		return nil, errors.New(errLeadManagementNotConfigured)
	}
	leadID, err := uuid.Parse(strings.TrimSpace(input.LeadID))
	if err != nil {
		return nil, errors.New(errInvalidLeadID)
	}
	req := leadtransport.UpdateLeadRequest{
		FirstName:       input.FirstName,
		LastName:        input.LastName,
		Email:           input.Email,
		Street:          input.Street,
		HouseNumber:     input.HouseNumber,
		ZipCode:         input.ZipCode,
		City:            input.City,
		Latitude:        input.Latitude,
		Longitude:       input.Longitude,
		WhatsAppOptedIn: input.WhatsAppOptedIn,
	}
	if input.Phone != nil {
		normalized := strings.TrimSpace(phone.NormalizeE164(*input.Phone))
		req.Phone = &normalized
	}
	if input.ConsumerRole != nil {
		role := leadtransport.ConsumerRole(strings.TrimSpace(*input.ConsumerRole))
		req.ConsumerRole = &role
	}
	if _, err := a.mgmt.Update(ctx, leadID, req, uuid.Nil, orgID, nil); err != nil {
		return nil, err
	}
	updatedFields := make([]string, 0, 10)
	appendField := func(name string, present bool) {
		if present {
			updatedFields = append(updatedFields, name)
		}
	}
	appendField("firstName", input.FirstName != nil)
	appendField("lastName", input.LastName != nil)
	appendField("phone", input.Phone != nil)
	appendField("email", input.Email != nil)
	appendField("consumerRole", input.ConsumerRole != nil)
	appendField("street", input.Street != nil)
	appendField("houseNumber", input.HouseNumber != nil)
	appendField("zipCode", input.ZipCode != nil)
	appendField("city", input.City != nil)
	appendField("latitude", input.Latitude != nil)
	appendField("longitude", input.Longitude != nil)
	appendField("whatsAppOptedIn", input.WhatsAppOptedIn != nil)
	return updatedFields, nil
}

func (a *WAAgentLeadActionsAdapter) AskCustomerClarification(ctx context.Context, orgID uuid.UUID, input waagent.AskCustomerClarificationInput) error {
	leadID, serviceID, err := a.resolveLeadAndService(ctx, orgID, input.LeadID, input.LeadServiceID)
	if err != nil {
		return err
	}
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return fmt.Errorf("message is required")
	}
	_, err = a.repo.CreateTimelineEvent(ctx, leadsrepo.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: orgID,
		ActorType:      leadsrepo.ActorTypeAI,
		ActorName:      waagentActorName,
		EventType:      leadsrepo.EventTypeNote,
		Title:          leadsrepo.EventTitleNoteAdded,
		Summary:        &message,
		Metadata: map[string]any{
			"noteType":          "ai_clarification_request",
			"missingDimensions": input.MissingDimensions,
			"source":            "whatsapp_agent",
		},
	})
	return err
}

func (a *WAAgentLeadActionsAdapter) SaveNote(ctx context.Context, orgID uuid.UUID, input waagent.SaveNoteInput) error {
	leadID, serviceID, err := a.resolveLeadAndService(ctx, orgID, input.LeadID, input.LeadServiceID)
	if err != nil {
		return err
	}
	body := strings.TrimSpace(input.Body)
	if body == "" {
		return fmt.Errorf("body is required")
	}
	_, err = a.repo.CreateTimelineEvent(ctx, leadsrepo.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: orgID,
		ActorType:      leadsrepo.ActorTypeAI,
		ActorName:      waagentActorName,
		EventType:      leadsrepo.EventTypeNote,
		Title:          leadsrepo.EventTitleNoteAdded,
		Summary:        &body,
		Metadata: map[string]any{
			"noteType": "whatsapp_agent_note",
			"source":   "whatsapp_agent",
		},
	})
	return err
}

func (a *WAAgentLeadActionsAdapter) UpdateLeadStatus(ctx context.Context, orgID uuid.UUID, input waagent.UpdateStatusInput) (string, error) {
	leadID, serviceID, err := a.resolveLeadAndService(ctx, orgID, input.LeadID, input.LeadServiceID)
	if err != nil {
		return "", err
	}
	resp, err := a.mgmt.UpdateServiceStatus(ctx, leadID, serviceID, leadtransport.UpdateServiceStatusRequest{Status: leadtransport.LeadStatus(input.Status)}, orgID)
	if err != nil {
		return "", err
	}
	if resp.CurrentService != nil {
		return string(resp.CurrentService.Status), nil
	}
	return input.Status, nil
}

func (a *WAAgentLeadActionsAdapter) resolveLeadAndService(ctx context.Context, orgID uuid.UUID, leadIDRaw string, serviceIDRaw string) (uuid.UUID, uuid.UUID, error) {
	leadID, err := uuid.Parse(strings.TrimSpace(leadIDRaw))
	if err != nil {
		return uuid.Nil, uuid.Nil, errors.New(errInvalidLeadID)
	}
	if trimmed := strings.TrimSpace(serviceIDRaw); trimmed != "" {
		serviceID, parseErr := uuid.Parse(trimmed)
		if parseErr != nil {
			return uuid.Nil, uuid.Nil, fmt.Errorf("invalid lead_service_id")
		}
		return leadID, serviceID, nil
	}
	svc, err := a.repo.GetCurrentLeadService(ctx, leadID, orgID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return leadID, svc.ID, nil
}

type WAAgentVisitActionsAdapter struct {
	slots ports.AppointmentSlotProvider
	svc   *apptsvc.Service
	repo  leadsrepo.LeadsRepository
}

func NewWAAgentVisitActionsAdapter(slots ports.AppointmentSlotProvider, svc *apptsvc.Service, repo leadsrepo.LeadsRepository) *WAAgentVisitActionsAdapter {
	return &WAAgentVisitActionsAdapter{slots: slots, svc: svc, repo: repo}
}

func (a *WAAgentVisitActionsAdapter) GetAvailableVisitSlots(ctx context.Context, orgID uuid.UUID, startDate, endDate string, slotDuration int) ([]waagent.VisitSlotSummary, error) {
	if a.slots == nil {
		return nil, fmt.Errorf("appointment slot provider not configured")
	}
	resp, err := a.slots.GetAvailableSlots(ctx, orgID, startDate, endDate, slotDuration)
	if err != nil {
		return nil, err
	}
	slots := make([]waagent.VisitSlotSummary, 0)
	for _, day := range resp.Days {
		for _, slot := range day.Slots {
			slots = append(slots, waagent.VisitSlotSummary{
				AssignedUserID: slot.UserID.String(),
				StartTime:      slot.StartTime.Format(time.RFC3339),
				EndTime:        slot.EndTime.Format(time.RFC3339),
				Date:           day.Date,
			})
		}
	}
	return slots, nil
}

func (a *WAAgentVisitActionsAdapter) ScheduleVisit(ctx context.Context, orgID uuid.UUID, input waagent.ScheduleVisitInput) (*waagent.AppointmentSummary, error) {
	if a.slots == nil {
		return nil, fmt.Errorf("appointment slot provider not configured")
	}
	leadID, err := uuid.Parse(strings.TrimSpace(input.LeadID))
	if err != nil {
		return nil, errors.New(errInvalidLeadID)
	}
	serviceID, err := uuid.Parse(strings.TrimSpace(input.LeadServiceID))
	if err != nil {
		return nil, fmt.Errorf("invalid lead_service_id")
	}
	userID, err := uuid.Parse(strings.TrimSpace(input.AssignedUserID))
	if err != nil {
		return nil, fmt.Errorf("invalid assigned_user_id")
	}
	startTime, err := time.Parse(time.RFC3339, strings.TrimSpace(input.StartTime))
	if err != nil {
		return nil, fmt.Errorf("invalid start_time")
	}
	endTime, err := time.Parse(time.RFC3339, strings.TrimSpace(input.EndTime))
	if err != nil {
		return nil, fmt.Errorf("invalid end_time")
	}
	appointment, err := a.slots.CreateRequestedAppointment(ctx, userID, orgID, leadID, serviceID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	return &waagent.AppointmentSummary{
		AppointmentID:  appointment.ID.String(),
		LeadID:         leadID.String(),
		LeadServiceID:  serviceID.String(),
		AssignedUserID: userID.String(),
		Title:          appointment.Title,
		Description:    derefString(appointment.Description),
		Location:       derefString(appointment.Location),
		StartTime:      appointment.StartTime.Format(time.RFC3339),
		EndTime:        appointment.EndTime.Format(time.RFC3339),
		Status:         appointment.Status,
	}, nil
}

func (a *WAAgentVisitActionsAdapter) RescheduleVisit(ctx context.Context, orgID uuid.UUID, input waagent.RescheduleVisitInput) (*waagent.AppointmentSummary, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("appointment service not configured")
	}
	appointmentID, err := uuid.Parse(strings.TrimSpace(input.AppointmentID))
	if err != nil {
		return nil, fmt.Errorf("invalid appointment_id")
	}
	startTime, err := time.Parse(time.RFC3339, strings.TrimSpace(input.StartTime))
	if err != nil {
		return nil, fmt.Errorf("invalid start_time")
	}
	endTime, err := time.Parse(time.RFC3339, strings.TrimSpace(input.EndTime))
	if err != nil {
		return nil, fmt.Errorf("invalid end_time")
	}
	resp, err := a.svc.Update(ctx, appointmentID, uuid.Nil, true, orgID, appttransport.UpdateAppointmentRequest{
		Title:       input.Title,
		Description: input.Description,
		StartTime:   &startTime,
		EndTime:     &endTime,
	})
	if err != nil {
		return nil, err
	}
	return appointmentSummaryFromResponse(resp), nil
}

func (a *WAAgentVisitActionsAdapter) CancelVisit(ctx context.Context, orgID uuid.UUID, input waagent.CancelVisitInput) error {
	if a.svc == nil {
		return fmt.Errorf("appointment service not configured")
	}
	appointmentID, err := uuid.Parse(strings.TrimSpace(input.AppointmentID))
	if err != nil {
		return fmt.Errorf("invalid appointment_id")
	}
	resp, err := a.svc.UpdateStatus(ctx, appointmentID, uuid.Nil, true, orgID, appttransport.UpdateAppointmentStatusRequest{Status: appttransport.AppointmentStatusCancelled})
	if err != nil {
		return err
	}
	if strings.TrimSpace(input.Reason) != "" && resp.LeadID != nil {
		serviceID := ""
		if resp.LeadServiceID != nil {
			serviceID = resp.LeadServiceID.String()
		}
		return (&WAAgentLeadActionsAdapter{repo: a.repo}).SaveNote(ctx, orgID, waagent.SaveNoteInput{
			LeadID:        resp.LeadID.String(),
			LeadServiceID: serviceID,
			Body:          strings.TrimSpace(input.Reason),
		})
	}
	return nil
}

func appointmentSummaryFromResponse(resp *appttransport.AppointmentResponse) *waagent.AppointmentSummary {
	if resp == nil {
		return nil
	}
	leadID := ""
	if resp.LeadID != nil {
		leadID = resp.LeadID.String()
	}
	serviceID := ""
	if resp.LeadServiceID != nil {
		serviceID = resp.LeadServiceID.String()
	}
	return &waagent.AppointmentSummary{
		AppointmentID:  resp.ID.String(),
		LeadID:         leadID,
		LeadServiceID:  serviceID,
		AssignedUserID: resp.UserID.String(),
		Title:          resp.Title,
		Description:    derefString(resp.Description),
		Location:       derefString(resp.Location),
		StartTime:      resp.StartTime.Format(time.RFC3339),
		EndTime:        resp.EndTime.Format(time.RFC3339),
		Status:         string(resp.Status),
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// quoteBasedLeadSearch finds leads via their associated quotes. This catches
// soft-deleted leads that still have active quotes referencing them.
func (a *WAAgentLeadActionsAdapter) quoteBasedLeadSearch(ctx context.Context, orgID uuid.UUID, query string, limit int) []waagent.LeadSearchResult {
	matches, err := a.repo.QuoteBasedLeadSearch(ctx, orgID, query, limit)
	if err != nil {
		log.Printf("waagent: quoteBasedSearch error org=%s query=%q: %v", orgID, query, err)
		return nil
	}
	if len(matches) == 0 {
		log.Printf("waagent: quoteBasedSearch org=%s query=%q no matches", orgID, query)
		return nil
	}
	log.Printf("waagent: quoteBasedSearch org=%s query=%q matches=%d", orgID, query, len(matches))
	results := make([]waagent.LeadSearchResult, 0, len(matches))
	for _, m := range matches {
		serviceID := ""
		if m.ServiceID != nil {
			serviceID = m.ServiceID.String()
		}
		results = append(results, waagent.LeadSearchResult{
			LeadID:        m.LeadID.String(),
			LeadServiceID: serviceID,
			CustomerName:  strings.TrimSpace(m.FirstName + " " + m.LastName),
			Phone:         m.Phone,
			City:          m.City,
			ServiceType:   m.ServiceType,
			Status:        m.ServiceStatus,
			CreatedAt:     m.CreatedAt.Format(time.RFC3339),
		})
	}
	return results
}
