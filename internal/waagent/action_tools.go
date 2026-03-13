package waagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"
)

const (
	errLeadMutationsNotConfigured  = "lead mutations not configured"
	errVisitMutationsNotConfigured = "visit mutations not configured"
)

type CreateLeadInput struct {
	FirstName    *string `json:"first_name,omitempty"`
	LastName     *string `json:"last_name,omitempty"`
	Phone        *string `json:"phone,omitempty"`
	Email        *string `json:"email,omitempty"`
	ConsumerRole *string `json:"consumer_role,omitempty"`
	Street       *string `json:"street,omitempty"`
	HouseNumber  *string `json:"house_number,omitempty"`
	ZipCode      *string `json:"zip_code,omitempty"`
	City         *string `json:"city,omitempty"`
	ServiceType  *string `json:"service_type,omitempty"`
	ConsumerNote *string `json:"consumer_note,omitempty"`
}

type CreateLeadResult struct {
	LeadID        string `json:"lead_id"`
	LeadServiceID string `json:"lead_service_id,omitempty"`
	CustomerName  string `json:"customer_name"`
	ServiceType   string `json:"service_type,omitempty"`
}

type CreateLeadOutput struct {
	Success       bool             `json:"success"`
	Message       string           `json:"message"`
	MissingFields []string         `json:"missing_fields,omitempty"`
	Lead          *CreateLeadResult `json:"lead,omitempty"`
}

type SearchProductMaterialsInput struct {
	Query      string   `json:"query"`
	Limit      int      `json:"limit,omitempty"`
	UseCatalog *bool    `json:"use_catalog,omitempty"`
	MinScore   *float64 `json:"min_score,omitempty"`
}

type ProductResult struct {
	ID               string   `json:"id,omitempty"`
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	Type             string   `json:"type"`
	PriceEuros       float64  `json:"price_euros"`
	PriceCents       int64    `json:"price_cents"`
	Unit             string   `json:"unit,omitempty"`
	LaborTime        string   `json:"labor_time,omitempty"`
	VatRateBps       int      `json:"vat_rate_bps,omitempty"`
	Materials        []string `json:"materials,omitempty"`
	Category         string   `json:"category,omitempty"`
	SourceURL        string   `json:"source_url,omitempty"`
	SourceCollection string   `json:"source_collection,omitempty"`
	Score            float64  `json:"score,omitempty"`
	HighConfidence   bool     `json:"high_confidence"`
}

type SearchProductMaterialsOutput struct {
	Products []ProductResult `json:"products"`
	Message  string          `json:"message"`
}

type LeadSearchResult struct {
	LeadID        string `json:"lead_id"`
	LeadServiceID string `json:"lead_service_id,omitempty"`
	CustomerName  string `json:"customer_name"`
	Phone         string `json:"phone,omitempty"`
	City          string `json:"city,omitempty"`
	ServiceType   string `json:"service_type,omitempty"`
	Status        string `json:"status,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

type SearchLeadsInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

type SearchLeadsOutput struct {
	Leads []LeadSearchResult `json:"leads"`
	Count int                `json:"count"`
}

type GetLeadDetailsInput struct {
	LeadID string `json:"lead_id"`
}

type LeadDetailsResult struct {
	LeadID        string `json:"lead_id"`
	CustomerName  string `json:"customer_name"`
	Phone         string `json:"phone,omitempty"`
	Email         string `json:"email,omitempty"`
	Street        string `json:"street,omitempty"`
	HouseNumber   string `json:"house_number,omitempty"`
	ZipCode       string `json:"zip_code,omitempty"`
	City          string `json:"city,omitempty"`
	FullAddress   string `json:"full_address,omitempty"`
	ServiceType   string `json:"service_type,omitempty"`
	Status        string `json:"status,omitempty"`
}

type GetLeadDetailsOutput struct {
	Success bool              `json:"success"`
	Message string            `json:"message"`
	Lead    *LeadDetailsResult `json:"lead,omitempty"`
}

type VisitSlotSummary struct {
	AssignedUserID string `json:"assigned_user_id"`
	StartTime      string `json:"start_time"`
	EndTime        string `json:"end_time"`
	Date           string `json:"date"`
}

type GetAvailableVisitSlotsInput struct {
	StartDate    string `json:"start_date,omitempty"`
	EndDate      string `json:"end_date,omitempty"`
	SlotDuration int    `json:"slot_duration,omitempty"`
}

type GetAvailableVisitSlotsOutput struct {
	Slots []VisitSlotSummary `json:"slots"`
	Count int                `json:"count"`
}

type GetNavigationLinkInput struct {
	LeadID string `json:"lead_id"`
}

type NavigationLinkResult struct {
	LeadID             string `json:"lead_id"`
	DestinationAddress string `json:"destination_address"`
	URL                string `json:"url"`
}

type GetNavigationLinkOutput struct {
	Success bool                 `json:"success"`
	Message string               `json:"message"`
	Link    *NavigationLinkResult `json:"link,omitempty"`
}

type UpdateLeadDetailsInput struct {
	LeadID          string   `json:"lead_id"`
	FirstName       *string  `json:"first_name,omitempty"`
	LastName        *string  `json:"last_name,omitempty"`
	Phone           *string  `json:"phone,omitempty"`
	Email           *string  `json:"email,omitempty"`
	ConsumerRole    *string  `json:"consumer_role,omitempty"`
	Street          *string  `json:"street,omitempty"`
	HouseNumber     *string  `json:"house_number,omitempty"`
	ZipCode         *string  `json:"zip_code,omitempty"`
	City            *string  `json:"city,omitempty"`
	Latitude        *float64 `json:"latitude,omitempty"`
	Longitude       *float64 `json:"longitude,omitempty"`
	WhatsAppOptedIn *bool    `json:"whatsapp_opted_in,omitempty"`
	Reason          string   `json:"reason,omitempty"`
}

type UpdateLeadDetailsOutput struct {
	Success       bool     `json:"success"`
	Message       string   `json:"message"`
	UpdatedFields []string `json:"updated_fields,omitempty"`
}

type AskCustomerClarificationInput struct {
	LeadID            string   `json:"lead_id"`
	LeadServiceID     string   `json:"lead_service_id,omitempty"`
	Message           string   `json:"message"`
	MissingDimensions []string `json:"missing_dimensions,omitempty"`
}

type AskCustomerClarificationOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type SaveNoteInput struct {
	LeadID        string `json:"lead_id"`
	LeadServiceID string `json:"lead_service_id,omitempty"`
	Body          string `json:"body"`
}

type SaveNoteOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type UpdateStatusInput struct {
	LeadID        string `json:"lead_id"`
	LeadServiceID string `json:"lead_service_id,omitempty"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
}

type UpdateStatusOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Status  string `json:"status,omitempty"`
}

type ScheduleVisitInput struct {
	LeadID                string `json:"lead_id"`
	LeadServiceID         string `json:"lead_service_id"`
	AssignedUserID        string `json:"assigned_user_id"`
	StartTime             string `json:"start_time"`
	EndTime               string `json:"end_time"`
	Title                 string `json:"title,omitempty"`
	SendConfirmationEmail *bool  `json:"send_confirmation_email,omitempty"`
}

type ScheduleVisitOutput struct {
	Success     bool                `json:"success"`
	Message     string              `json:"message"`
	Appointment *AppointmentSummary `json:"appointment,omitempty"`
}

type RescheduleVisitInput struct {
	AppointmentID string  `json:"appointment_id"`
	StartTime     string  `json:"start_time"`
	EndTime       string  `json:"end_time"`
	Title         *string `json:"title,omitempty"`
	Description   *string `json:"description,omitempty"`
}

type RescheduleVisitOutput struct {
	Success     bool                `json:"success"`
	Message     string              `json:"message"`
	Appointment *AppointmentSummary `json:"appointment,omitempty"`
}

type CancelVisitInput struct {
	AppointmentID string `json:"appointment_id"`
	Reason        string `json:"reason,omitempty"`
}

type CancelVisitOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (h *ToolHandler) HandleSearchLeads(ctx tool.Context, orgID uuid.UUID, input SearchLeadsInput) (SearchLeadsOutput, error) {
	if h.leadSearchReader == nil {
		return SearchLeadsOutput{}, fmt.Errorf("lead search not configured")
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return SearchLeadsOutput{Leads: []LeadSearchResult{}, Count: 0}, nil
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 5
	}
	leads, err := h.leadSearchReader.SearchLeads(context.Background(), orgID, query, limit)
	if err != nil {
		return SearchLeadsOutput{}, err
	}
	h.recordLeadHintIfUnambiguous(ctx, orgID, leads)
	return SearchLeadsOutput{Leads: leads, Count: len(leads)}, nil
}

func (h *ToolHandler) HandleGetLeadDetails(ctx tool.Context, orgID uuid.UUID, input GetLeadDetailsInput) (GetLeadDetailsOutput, error) {
	if h.leadDetailsReader == nil {
		return GetLeadDetailsOutput{}, fmt.Errorf("lead details reader not configured")
	}
	leadID := strings.TrimSpace(input.LeadID)
	if leadID == "" {
		return GetLeadDetailsOutput{Success: false, Message: "lead_id is verplicht"}, fmt.Errorf("lead_id is required")
	}
	lead, err := h.leadDetailsReader.GetLeadDetails(context.Background(), orgID, leadID)
	if err != nil {
		return GetLeadDetailsOutput{Success: false, Message: err.Error()}, err
	}
	h.recordLeadHint(ctx, orgID, lead.LeadID, lead.CustomerName)
	return GetLeadDetailsOutput{Success: true, Message: "Leadgegevens gevonden", Lead: lead}, nil
}

func (h *ToolHandler) HandleGetAvailableVisitSlots(_ tool.Context, orgID uuid.UUID, input GetAvailableVisitSlotsInput) (GetAvailableVisitSlotsOutput, error) {
	if h.visitSlotReader == nil {
		return GetAvailableVisitSlotsOutput{}, fmt.Errorf("visit slot reader not configured")
	}
	startDate := strings.TrimSpace(input.StartDate)
	endDate := strings.TrimSpace(input.EndDate)
	if startDate == "" {
		startDate = time.Now().Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Add(14 * 24 * time.Hour).Format("2006-01-02")
	}
	slotDuration := input.SlotDuration
	if slotDuration <= 0 {
		slotDuration = 60
	}
	slots, err := h.visitSlotReader.GetAvailableVisitSlots(context.Background(), orgID, startDate, endDate, slotDuration)
	if err != nil {
		return GetAvailableVisitSlotsOutput{}, err
	}
	return GetAvailableVisitSlotsOutput{Slots: slots, Count: len(slots)}, nil
}

func (h *ToolHandler) HandleGetNavigationLink(ctx tool.Context, orgID uuid.UUID, input GetNavigationLinkInput) (GetNavigationLinkOutput, error) {
	if h.navigationLinkReader == nil {
		return GetNavigationLinkOutput{}, fmt.Errorf("navigation link reader not configured")
	}
	leadID := strings.TrimSpace(input.LeadID)
	if leadID == "" {
		return GetNavigationLinkOutput{Success: false, Message: "lead_id is verplicht"}, fmt.Errorf("lead_id is required")
	}
	link, err := h.navigationLinkReader.GetNavigationLink(context.Background(), orgID, leadID)
	if err != nil {
		return GetNavigationLinkOutput{Success: false, Message: err.Error()}, err
	}
	h.recordLeadHint(ctx, orgID, link.LeadID, "")
	return GetNavigationLinkOutput{Success: true, Message: "Navigatielink gevonden", Link: link}, nil
}

func (h *ToolHandler) HandleCreateLead(ctx tool.Context, orgID uuid.UUID, input CreateLeadInput) (CreateLeadOutput, error) {
	if h.leadMutationWriter == nil {
		return CreateLeadOutput{}, errors.New(errLeadMutationsNotConfigured)
	}
	output, err := h.leadMutationWriter.CreateLead(context.Background(), orgID, input)
	if err == nil && output.Success && output.Lead != nil {
		h.recordLeadHint(ctx, orgID, output.Lead.LeadID, output.Lead.CustomerName)
	}
	return output, err
}

func (h *ToolHandler) HandleSearchProductMaterials(_ tool.Context, orgID uuid.UUID, input SearchProductMaterialsInput) (SearchProductMaterialsOutput, error) {
	if h.catalogSearchReader == nil {
		return SearchProductMaterialsOutput{}, fmt.Errorf("catalog search not configured")
	}
	return h.catalogSearchReader.SearchProductMaterials(context.Background(), orgID, input)
}

func (h *ToolHandler) HandleUpdateLeadDetails(_ tool.Context, orgID uuid.UUID, input UpdateLeadDetailsInput) (UpdateLeadDetailsOutput, error) {
	if h.leadMutationWriter == nil {
		return UpdateLeadDetailsOutput{}, errors.New(errLeadMutationsNotConfigured)
	}
	if input.ConsumerRole != nil {
		normalized := normalizeConsumerRole(*input.ConsumerRole)
		input.ConsumerRole = &normalized
	}
	updatedFields, err := h.leadMutationWriter.UpdateLeadDetails(context.Background(), orgID, input)
	if err != nil {
		return UpdateLeadDetailsOutput{Success: false, Message: err.Error()}, err
	}
	return UpdateLeadDetailsOutput{Success: true, Message: "Leadgegevens bijgewerkt", UpdatedFields: updatedFields}, nil
}

func (h *ToolHandler) HandleAskCustomerClarification(_ tool.Context, orgID uuid.UUID, input AskCustomerClarificationInput) (AskCustomerClarificationOutput, error) {
	if h.leadMutationWriter == nil {
		return AskCustomerClarificationOutput{}, errors.New(errLeadMutationsNotConfigured)
	}
	if err := h.leadMutationWriter.AskCustomerClarification(context.Background(), orgID, input); err != nil {
		return AskCustomerClarificationOutput{Success: false, Message: err.Error()}, err
	}
	return AskCustomerClarificationOutput{Success: true, Message: "Verduidelijkingsverzoek opgeslagen"}, nil
}

func (h *ToolHandler) HandleSaveNote(_ tool.Context, orgID uuid.UUID, input SaveNoteInput) (SaveNoteOutput, error) {
	if h.leadMutationWriter == nil {
		return SaveNoteOutput{}, errors.New(errLeadMutationsNotConfigured)
	}
	if err := h.leadMutationWriter.SaveNote(context.Background(), orgID, input); err != nil {
		return SaveNoteOutput{Success: false, Message: err.Error()}, err
	}
	return SaveNoteOutput{Success: true, Message: "Notitie opgeslagen"}, nil
}

func (h *ToolHandler) HandleUpdateStatus(_ tool.Context, orgID uuid.UUID, input UpdateStatusInput) (UpdateStatusOutput, error) {
	if h.leadMutationWriter == nil {
		return UpdateStatusOutput{}, errors.New(errLeadMutationsNotConfigured)
	}
	normalized := normalizeLeadStatus(input.Status)
	if normalized == "Disqualified" {
		return UpdateStatusOutput{Success: false, Message: "Status Disqualified wordt niet autonoom via WhatsApp aangepast"}, fmt.Errorf("disqualified is not allowed via whatsapp agent")
	}
	input.Status = normalized
	status, err := h.leadMutationWriter.UpdateLeadStatus(context.Background(), orgID, input)
	if err != nil {
		return UpdateStatusOutput{Success: false, Message: err.Error()}, err
	}
	return UpdateStatusOutput{Success: true, Message: "Status bijgewerkt", Status: status}, nil
}

func (h *ToolHandler) HandleScheduleVisit(_ tool.Context, orgID uuid.UUID, input ScheduleVisitInput) (ScheduleVisitOutput, error) {
	if h.visitMutationWriter == nil {
		return ScheduleVisitOutput{}, errors.New(errVisitMutationsNotConfigured)
	}
	appointment, err := h.visitMutationWriter.ScheduleVisit(context.Background(), orgID, input)
	if err != nil {
		return ScheduleVisitOutput{Success: false, Message: err.Error()}, err
	}
	return ScheduleVisitOutput{Success: true, Message: "Afspraak aangevraagd", Appointment: appointment}, nil
}

func (h *ToolHandler) HandleRescheduleVisit(_ tool.Context, orgID uuid.UUID, input RescheduleVisitInput) (RescheduleVisitOutput, error) {
	if h.visitMutationWriter == nil {
		return RescheduleVisitOutput{}, errors.New(errVisitMutationsNotConfigured)
	}
	appointment, err := h.visitMutationWriter.RescheduleVisit(context.Background(), orgID, input)
	if err != nil {
		return RescheduleVisitOutput{Success: false, Message: err.Error()}, err
	}
	return RescheduleVisitOutput{Success: true, Message: "Afspraak verplaatst", Appointment: appointment}, nil
}

func (h *ToolHandler) HandleCancelVisit(_ tool.Context, orgID uuid.UUID, input CancelVisitInput) (CancelVisitOutput, error) {
	if h.visitMutationWriter == nil {
		return CancelVisitOutput{}, errors.New(errVisitMutationsNotConfigured)
	}
	if err := h.visitMutationWriter.CancelVisit(context.Background(), orgID, input); err != nil {
		return CancelVisitOutput{Success: false, Message: err.Error()}, err
	}
	return CancelVisitOutput{Success: true, Message: "Afspraak geannuleerd"}, nil
}

func normalizeConsumerRole(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "owner", "eigenaar":
		return "Owner"
	case "tenant", "huurder":
		return "Tenant"
	case "landlord", "verhuurder":
		return "Landlord"
	default:
		if normalized == "" {
			return ""
		}
		return strings.ToUpper(normalized[:1]) + normalized[1:]
	}
}

func normalizeLeadStatus(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	switch normalized {
	case "new", "nieuw":
		return "New"
	case "pending", "open", "wachtend":
		return "Pending"
	case "in_progress", "bezig", "in_behandeling":
		return "In_Progress"
	case "attempted_contact", "contact_geprobeerd", "poging_contact":
		return "Attempted_Contact"
	case "appointment_scheduled", "afspraak_ingepland", "afspraak_gepland":
		return "Appointment_Scheduled"
	case "needs_rescheduling", "herplannen", "opnieuw_inplannen":
		return "Needs_Rescheduling"
	case "disqualified", "afgewezen", "ongeschikt":
		return "Disqualified"
	default:
		return raw
	}
}

func (h *ToolHandler) recordLeadHintIfUnambiguous(ctx tool.Context, orgID uuid.UUID, leads []LeadSearchResult) {
	if len(leads) != 1 {
		return
	}
	h.recordLeadHint(ctx, orgID, leads[0].LeadID, leads[0].CustomerName)
}

func (h *ToolHandler) recordLeadHint(ctx tool.Context, orgID uuid.UUID, leadID, customerName string) {
	if h == nil || h.leadHintStore == nil {
		return
	}
	phoneKey, ok := phoneKeyFromToolContext(ctx)
	if !ok {
		return
	}
	h.leadHintStore.Set(orgID.String(), phoneKey, ConversationLeadHint{
		LeadID:       strings.TrimSpace(leadID),
		CustomerName: strings.TrimSpace(customerName),
	})
}