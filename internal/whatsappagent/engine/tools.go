package engine

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"
)

// GetQuotesInput — no organization field (compile-time safety).
type GetQuotesInput struct {
	Status string `json:"status,omitempty"`
}

// GetQuotesOutput is the tool output returned to the LLM.
type GetQuotesOutput struct {
	Quotes []QuoteSummary `json:"quotes"`
	Count  int            `json:"count"`
}

// GetAppointmentsInput — no organization field (compile-time safety).
type GetAppointmentsInput struct {
	DateFrom string `json:"date_from,omitempty"`
	DateTo   string `json:"date_to,omitempty"`
}

// GetAppointmentsOutput is the tool output returned to the LLM.
type GetAppointmentsOutput struct {
	Appointments []AppointmentSummary `json:"appointments"`
	Count        int                  `json:"count"`
}

type GetEnergyLabelInput struct {
	LeadID      string `json:"lead_id,omitempty"`
	Postcode    string `json:"postcode,omitempty"`
	HouseNumber string `json:"house_number,omitempty"`
	HouseLetter string `json:"house_letter,omitempty"`
	Addition    string `json:"addition,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

type EnergyLabelSummary struct {
	EnergyClass        string `json:"energy_class,omitempty"`
	EnergyIndex        string `json:"energy_index,omitempty"`
	RegistrationDate   string `json:"registration_date,omitempty"`
	ValidUntil         string `json:"valid_until,omitempty"`
	BuildYear          int    `json:"build_year,omitempty"`
	BuildingType       string `json:"building_type,omitempty"`
	BuildingSubType    string `json:"building_sub_type,omitempty"`
	AddressPostcode    string `json:"address_postcode,omitempty"`
	AddressHouseNo     int    `json:"address_house_number,omitempty"`
	AddressHouseLetter string `json:"address_house_letter,omitempty"`
	AddressAddition    string `json:"address_addition,omitempty"`
}

type GetEnergyLabelOutput struct {
	Success bool                `json:"success"`
	Message string              `json:"message"`
	Found   bool                `json:"found"`
	Label   *EnergyLabelSummary `json:"label,omitempty"`
}

type GetLeadTasksInput struct {
	LeadID        string `json:"lead_id"`
	LeadServiceID string `json:"lead_service_id,omitempty"`
	Status        string `json:"status,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

type LeadTaskSummary struct {
	TaskID         string `json:"task_id"`
	LeadID         string `json:"lead_id,omitempty"`
	LeadServiceID  string `json:"lead_service_id,omitempty"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	Status         string `json:"status"`
	Priority       string `json:"priority,omitempty"`
	AssignedUserID string `json:"assigned_user_id,omitempty"`
	DueAt          string `json:"due_at,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
}

type GetLeadTasksOutput struct {
	Tasks []LeadTaskSummary `json:"tasks"`
	Count int               `json:"count"`
}

type GetISDEInput struct {
	ExecutionYear                   *int                        `json:"execution_year,omitempty"`
	PreviousSubsidiesWithin24Months bool                        `json:"previous_subsidies_within_24_months"`
	HasExistingWarmtenetConnection  bool                        `json:"has_existing_warmtenet_connection"`
	HasReceivedWarmtenetSubsidy     bool                        `json:"has_received_warmtenet_subsidy"`
	Measures                        []ISDERequestedMeasure      `json:"measures,omitempty"`
	Installations                   []ISDERequestedInstallation `json:"installations,omitempty"`
}

type ISDERequestedMeasure struct {
	MeasureID                string   `json:"measure_id"`
	AreaM2                   float64  `json:"area_m2"`
	PerformanceValue         *float64 `json:"performance_value,omitempty"`
	FramePerformanceValue    *float64 `json:"frame_performance_value,omitempty"`
	HasMKIBonus              bool     `json:"has_mki_bonus"`
	FrameReplaced            bool     `json:"frame_replaced"`
	StackedWithPairedMeasure bool     `json:"stacked_with_paired_measure"`
}

type ISDERequestedInstallation struct {
	Kind                string   `json:"kind,omitempty"`
	Meldcode            string   `json:"meldcode,omitempty"`
	HeatPumpType        string   `json:"heat_pump_type,omitempty"`
	HeatPumpEnergyLabel string   `json:"heat_pump_energy_label,omitempty"`
	ThermalPowerKW      *float64 `json:"thermal_power_kw,omitempty"`
	IsAdditionalUnit    bool     `json:"is_additional_unit"`
	IsSplitSystem       bool     `json:"is_split_system"`
	RefrigerantChargeKg *float64 `json:"refrigerant_charge_kg,omitempty"`
	RefrigerantGWP      *float64 `json:"refrigerant_gwp,omitempty"`
}

type ISDELineItem struct {
	Description string  `json:"description"`
	AreaM2      float64 `json:"area_m2,omitempty"`
	AmountCents int64   `json:"amount_cents"`
}

type GetISDEOutput struct {
	TotalAmountCents     int64          `json:"total_amount_cents"`
	IsDoubled            bool           `json:"is_doubled"`
	EligibleMeasureCount int            `json:"eligible_measure_count"`
	InsulationBreakdown  []ISDELineItem `json:"insulation_breakdown"`
	GlassBreakdown       []ISDELineItem `json:"glass_breakdown"`
	Installations        []ISDELineItem `json:"installations"`
	ValidationMessages   []string       `json:"validation_messages,omitempty"`
	UnknownMeasureIDs    []string       `json:"unknown_measure_ids,omitempty"`
	UnknownMeldcodes     []string       `json:"unknown_meldcodes,omitempty"`
}

var reHouseNumberParts = regexp.MustCompile(`^(\d+)\s*([a-zA-Z]?)\s*(.*)$`)

type LeadHintStore interface {
	Get(ctx context.Context, orgID, phoneKey string) (*ConversationLeadHint, bool)
	Set(ctx context.Context, orgID, phoneKey string, hint ConversationLeadHint)
	RememberQuotes(ctx context.Context, orgID, phoneKey string, quotes []QuoteSummary)
	RememberAppointments(ctx context.Context, orgID, phoneKey string, appointments []AppointmentSummary)
	Clear(ctx context.Context, orgID, phoneKey string)
}

// ToolHandler implements the function-calling tool handlers.
type ToolHandler struct {
	quotesReader                 QuotesReader
	appointmentsReader           AppointmentsReader
	leadSearchReader             LeadSearchReader
	leadHintStore                LeadHintStore
	leadDetailsReader            LeadDetailsReader
	navigationLinkReader         NavigationLinkReader
	catalogSearchReader          CatalogSearchReader
	leadMutationWriter           LeadMutationWriter
	taskWriter                   TaskWriter
	taskReader                   TaskReader
	energyLabelReader            EnergyLabelReader
	isdeCalculator               ISDECalculator
	quoteWorkflowWriter          QuoteWorkflowWriter
	currentInboundPhotoAttacher  CurrentInboundPhotoAttacher
	sender                       *Sender
	visitSlotReader              VisitSlotReader
	visitMutationWriter          VisitMutationWriter
	partnerJobReader             PartnerJobReader
	appointmentVisitReportWriter AppointmentVisitReportWriter
	appointmentStatusWriter      AppointmentStatusWriter
}

// HandleGetQuotes retrieves quotes scoped to the org from context.
func (h *ToolHandler) HandleGetQuotes(ctx tool.Context, orgID uuid.UUID, input GetQuotesInput) (GetQuotesOutput, error) {
	var status *string
	if input.Status != "" {
		normalized := normalizeQuoteStatusFilter(input.Status)
		status = &normalized
	}

	quotes, err := h.quotesReader.ListQuotesByOrganization(context.Background(), orgID, status)
	if err != nil {
		return GetQuotesOutput{}, err
	}
	h.recordLeadHintFromQuotes(ctx, orgID, quotes)

	return GetQuotesOutput{
		Quotes: quotes,
		Count:  len(quotes),
	}, nil
}

func normalizeQuoteStatusFilter(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "draft", "concept", "conceptofferte":
		return "Draft"
	case "sent", "pending", "open", "openstaand", "openstaande", "verstuurd":
		return "Sent"
	case "accepted", "accept", "goedgekeurd", "geaccepteerd", "akkoord":
		return "Accepted"
	case "rejected", "afgewezen", "geweigerd":
		return "Rejected"
	case "expired", "verlopen", "expired_quote":
		return "Expired"
	default:
		if normalized == "" {
			return ""
		}
		return strings.ToUpper(normalized[:1]) + normalized[1:]
	}
}

// HandleGetAppointments retrieves appointments scoped to the org from context.
func (h *ToolHandler) HandleGetAppointments(ctx tool.Context, orgID uuid.UUID, input GetAppointmentsInput) (GetAppointmentsOutput, error) {
	var from, to *time.Time

	if input.DateFrom != "" {
		t, err := parseAppointmentDateInput(input.DateFrom)
		if err == nil {
			from = &t
		}
	}
	if input.DateTo != "" {
		t, err := parseAppointmentDateInput(input.DateTo)
		if err == nil {
			to = &t
		}
	}

	// Default: today to 30 days from now
	if from == nil {
		t := time.Now().Truncate(24 * time.Hour)
		from = &t
	}
	if to == nil {
		t := from.Add(30 * 24 * time.Hour)
		to = &t
	}

	appointments, err := h.appointmentsReader.ListAppointmentsByOrganization(context.Background(), orgID, from, to)
	if err != nil {
		return GetAppointmentsOutput{}, fmt.Errorf("ik kan de afspraken nu niet ophalen. probeer het later opnieuw")
	}
	if partnerID, ok := partnerIDFromToolContext(ctx); ok {
		appointments, err = h.filterPartnerAppointments(orgID, partnerID, appointments)
		if err != nil {
			return GetAppointmentsOutput{}, err
		}
	}
	h.recordLeadHintFromAppointments(ctx, orgID, appointments)

	return GetAppointmentsOutput{
		Appointments: appointments,
		Count:        len(appointments),
	}, nil
}

func (h *ToolHandler) HandleGetEnergyLabel(_ tool.Context, orgID uuid.UUID, input GetEnergyLabelInput) (GetEnergyLabelOutput, error) {
	if h.energyLabelReader == nil {
		return GetEnergyLabelOutput{}, errors.New(errEnergyLabelReaderNotConfigured)
	}

	resolved, failure, err := h.resolveEnergyLabelInput(orgID, input)
	if err != nil {
		if failure != nil {
			return *failure, err
		}
		return GetEnergyLabelOutput{}, err
	}

	if strings.TrimSpace(resolved.Postcode) == "" || strings.TrimSpace(resolved.HouseNumber) == "" {
		return GetEnergyLabelOutput{Success: false, Message: "postcode en huisnummer zijn verplicht", Found: false}, fmt.Errorf("postcode and house_number are required")
	}

	return h.energyLabelReader.GetEnergyLabel(context.Background(), orgID, resolved)
}

func (h *ToolHandler) resolveEnergyLabelInput(orgID uuid.UUID, input GetEnergyLabelInput) (GetEnergyLabelInput, *GetEnergyLabelOutput, error) {
	resolved := input
	resolved.Postcode = normalizeEnergyLabelPostcode(resolved.Postcode)
	if strings.TrimSpace(resolved.HouseNumber) != "" && strings.TrimSpace(resolved.Postcode) != "" {
		return resolved, nil, nil
	}

	leadID := strings.TrimSpace(resolved.LeadID)
	if leadID == "" {
		return GetEnergyLabelInput{}, energyLabelFailure("lead_id of adresgegevens ontbreken", false), fmt.Errorf("lead_id or address fields are required")
	}
	if h.leadDetailsReader == nil {
		return GetEnergyLabelInput{}, nil, fmt.Errorf("lead details reader is not configured")
	}
	details, err := h.leadDetailsReader.GetLeadDetails(context.Background(), orgID, leadID)
	if err != nil {
		return GetEnergyLabelInput{}, energyLabelFailure(err.Error(), false), err
	}
	if details == nil {
		return GetEnergyLabelInput{}, energyLabelFailure("lead niet gevonden", false), fmt.Errorf("lead not found")
	}
	applyLeadAddressToEnergyLabelInput(&resolved, details.ZipCode, details.HouseNumber)
	return resolved, nil, nil
}

func normalizeEnergyLabelPostcode(value string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
}

func applyLeadAddressToEnergyLabelInput(input *GetEnergyLabelInput, zipCode, houseNumber string) {
	input.Postcode = normalizeEnergyLabelPostcode(zipCode)
	number, letter, addition := parseHouseNumberParts(houseNumber)
	input.HouseNumber = number
	if strings.TrimSpace(input.HouseLetter) == "" {
		input.HouseLetter = letter
	}
	if strings.TrimSpace(input.Addition) == "" {
		input.Addition = addition
	}
}

func energyLabelFailure(message string, found bool) *GetEnergyLabelOutput {
	return &GetEnergyLabelOutput{Success: false, Message: message, Found: found}
}

func (h *ToolHandler) HandleGetLeadTasks(_ tool.Context, orgID uuid.UUID, input GetLeadTasksInput) (GetLeadTasksOutput, error) {
	if h.taskReader == nil {
		return GetLeadTasksOutput{}, errors.New(errTaskReaderNotConfigured)
	}
	if strings.TrimSpace(input.LeadID) == "" {
		return GetLeadTasksOutput{}, fmt.Errorf("lead_id is required")
	}
	if input.Limit <= 0 {
		input.Limit = 20
	}
	return h.taskReader.GetLeadTasks(context.Background(), orgID, input)
}

func (h *ToolHandler) HandleGetISDE(_ tool.Context, orgID uuid.UUID, input GetISDEInput) (GetISDEOutput, error) {
	if h.isdeCalculator == nil {
		return GetISDEOutput{}, errors.New(errISDECalculatorNotConfigured)
	}
	return h.isdeCalculator.GetISDE(context.Background(), orgID, input)
}

func parseHouseNumberParts(raw string) (number, letter, addition string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", ""
	}
	match := reHouseNumberParts.FindStringSubmatch(trimmed)
	if len(match) < 4 {
		return trimmed, "", ""
	}
	return strings.TrimSpace(match[1]), strings.TrimSpace(match[2]), strings.TrimSpace(match[3])
}

func (h *ToolHandler) filterPartnerAppointments(orgID, partnerID uuid.UUID, appointments []AppointmentSummary) ([]AppointmentSummary, error) {
	if h.partnerJobReader == nil {
		return nil, errors.New(errPartnerJobReaderNotConfigured)
	}
	jobs, err := h.partnerJobReader.ListPartnerJobs(context.Background(), orgID, partnerID)
	if err != nil {
		return nil, err
	}
	allowedAppointments := make(map[string]struct{}, len(jobs))
	allowedServices := make(map[string]struct{}, len(jobs))
	for _, job := range jobs {
		if strings.TrimSpace(job.AppointmentID) != "" {
			allowedAppointments[strings.TrimSpace(job.AppointmentID)] = struct{}{}
		}
		if strings.TrimSpace(job.LeadServiceID) != "" {
			allowedServices[strings.TrimSpace(job.LeadServiceID)] = struct{}{}
		}
	}
	filtered := make([]AppointmentSummary, 0, len(appointments))
	for _, appointment := range appointments {
		if _, ok := allowedAppointments[strings.TrimSpace(appointment.AppointmentID)]; ok {
			filtered = append(filtered, appointment)
			continue
		}
		if _, ok := allowedServices[strings.TrimSpace(appointment.LeadServiceID)]; ok {
			filtered = append(filtered, appointment)
		}
	}
	return filtered, nil
}

func (h *ToolHandler) recordLeadHintFromQuotes(ctx tool.Context, orgID uuid.UUID, quotes []QuoteSummary) {
	if h == nil || h.leadHintStore == nil {
		return
	}
	if phoneKey, ok := phoneKeyFromToolContext(ctx); ok {
		h.leadHintStore.RememberQuotes(ctx, orgID.String(), phoneKey, quotes)
	}
	if len(quotes) != 1 {
		return
	}
	quote := quotes[0]
	if strings.TrimSpace(quote.LeadID) == "" {
		return
	}
	h.recordLeadHint(ctx, orgID, quote.LeadID, quote.ClientName, quote.LeadServiceID)
}

func (h *ToolHandler) recordLeadHintFromAppointments(ctx tool.Context, orgID uuid.UUID, appointments []AppointmentSummary) {
	if h == nil || h.leadHintStore == nil {
		return
	}
	if phoneKey, ok := phoneKeyFromToolContext(ctx); ok {
		h.leadHintStore.RememberAppointments(ctx, orgID.String(), phoneKey, appointments)
	}
	if len(appointments) != 1 {
		return
	}
	appointment := appointments[0]
	if strings.TrimSpace(appointment.LeadID) == "" {
		return
	}
	h.recordLeadHint(ctx, orgID, appointment.LeadID, "", appointment.LeadServiceID)
}

func parseAppointmentDateInput(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	if parsed, err := time.Parse("2006-01-02", trimmed); err == nil {
		return parsed, nil
	}
	if parsed := parseDateFact(trimmed); parsed != nil {
		return time.Date(parsed.year, time.Month(parsed.month), parsed.day, 0, 0, 0, 0, time.UTC), nil
	}
	return time.Time{}, fmt.Errorf("unsupported date format")
}
