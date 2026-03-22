package whatsappagent

import "github.com/google/uuid"

type QuoteSummary struct {
	QuoteID       string `json:"quote_id,omitempty"`
	LeadID        string `json:"lead_id,omitempty"`
	LeadServiceID string `json:"lead_service_id,omitempty"`
	QuoteNumber   string `json:"quote_number"`
	ClientName    string `json:"client_name"`
	ClientPhone   string `json:"client_phone,omitempty"`
	ClientEmail   string `json:"client_email,omitempty"`
	ClientCity    string `json:"client_city,omitempty"`
	TotalCents    int64  `json:"total_cents"`
	Status        string `json:"status"`
	Summary       string `json:"summary,omitempty"`
	CreatedAt     string `json:"created_at"`
}

type AppointmentSummary struct {
	AppointmentID  string `json:"appointment_id,omitempty"`
	LeadID         string `json:"lead_id,omitempty"`
	LeadServiceID  string `json:"lead_service_id,omitempty"`
	AssignedUserID string `json:"assigned_user_id,omitempty"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	StartTime      string `json:"start_time"`
	EndTime        string `json:"end_time"`
	Status         string `json:"status"`
	Location       string `json:"location,omitempty"`
}

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
	Success       bool              `json:"success"`
	Message       string            `json:"message"`
	MissingFields []string          `json:"missing_fields,omitempty"`
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

type AttachCurrentWhatsAppPhotoInput struct {
	AppointmentID string `json:"appointment_id,omitempty"`
	LeadID        string `json:"lead_id,omitempty"`
	LeadServiceID string `json:"lead_service_id,omitempty"`
}

type AttachCurrentWhatsAppPhotoOutput struct {
	Success       bool     `json:"success"`
	Message       string   `json:"message"`
	AttachmentID  string   `json:"attachment_id,omitempty"`
	LeadID        string   `json:"lead_id,omitempty"`
	LeadServiceID string   `json:"lead_service_id,omitempty"`
	MissingFields []string `json:"missing_fields,omitempty"`
}

type DraftQuoteItem struct {
	Description      string  `json:"description"`
	Quantity         string  `json:"quantity"`
	UnitPriceCents   int64   `json:"unit_price_cents"`
	TaxRateBps       int     `json:"tax_rate_bps,omitempty"`
	IsOptional       bool    `json:"is_optional,omitempty"`
	CatalogProductID *string `json:"catalog_product_id,omitempty"`
}

type DraftQuoteInput struct {
	LeadID        string           `json:"lead_id,omitempty"`
	LeadServiceID string           `json:"lead_service_id,omitempty"`
	Notes         string           `json:"notes,omitempty"`
	Items         []DraftQuoteItem `json:"items"`
}

type DraftQuoteOutput struct {
	Success       bool     `json:"success"`
	Message       string   `json:"message"`
	QuoteID       string   `json:"quote_id,omitempty"`
	QuoteNumber   string   `json:"quote_number,omitempty"`
	ItemCount     int      `json:"item_count,omitempty"`
	MissingFields []string `json:"missing_fields,omitempty"`
}

type GenerateQuoteInput struct {
	LeadID        string `json:"lead_id,omitempty"`
	LeadServiceID string `json:"lead_service_id,omitempty"`
	Prompt        string `json:"prompt"`
	Force         *bool  `json:"force,omitempty"`
}

type GenerateQuoteOutput struct {
	Success            bool     `json:"success"`
	Message            string   `json:"message"`
	QuoteID            string   `json:"quote_id,omitempty"`
	QuoteNumber        string   `json:"quote_number,omitempty"`
	ItemCount          int      `json:"item_count,omitempty"`
	MissingInformation []string `json:"missing_information,omitempty"`
}

type SendQuotePDFInput struct {
	QuoteID string `json:"quote_id"`
	Caption string `json:"caption,omitempty"`
}

type QuotePDFResult struct {
	QuoteID     string
	QuoteNumber string
	FileName    string
	Data        []byte
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

type LeadDetailsResult struct {
	LeadID       string `json:"lead_id"`
	CustomerName string `json:"customer_name"`
	Phone        string `json:"phone,omitempty"`
	Email        string `json:"email,omitempty"`
	Street       string `json:"street,omitempty"`
	HouseNumber  string `json:"house_number,omitempty"`
	ZipCode      string `json:"zip_code,omitempty"`
	City         string `json:"city,omitempty"`
	FullAddress  string `json:"full_address,omitempty"`
	ServiceType  string `json:"service_type,omitempty"`
	Status       string `json:"status,omitempty"`
}

type VisitSlotSummary struct {
	AssignedUserID string `json:"assigned_user_id"`
	StartTime      string `json:"start_time"`
	EndTime        string `json:"end_time"`
	Date           string `json:"date"`
}

type NavigationLinkResult struct {
	LeadID             string `json:"lead_id"`
	DestinationAddress string `json:"destination_address"`
	URL                string `json:"url"`
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

type AskCustomerClarificationInput struct {
	LeadID            string   `json:"lead_id"`
	LeadServiceID     string   `json:"lead_service_id,omitempty"`
	Message           string   `json:"message"`
	MissingDimensions []string `json:"missing_dimensions,omitempty"`
}

type SaveNoteInput struct {
	LeadID        string `json:"lead_id"`
	LeadServiceID string `json:"lead_service_id,omitempty"`
	Body          string `json:"body"`
}

type CreateTaskInput struct {
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	LeadID         string `json:"lead_id,omitempty"`
	LeadServiceID  string `json:"lead_service_id,omitempty"`
	AssignedUserID string `json:"assigned_user_id,omitempty"`
	DueAt          string `json:"due_at,omitempty"`
	ReminderAt     string `json:"reminder_at,omitempty"`
	RepeatDaily    *bool  `json:"repeat_daily,omitempty"`
	SendEmail      *bool  `json:"send_email,omitempty"`
	SendWhatsApp   *bool  `json:"send_whatsapp,omitempty"`
	Priority       string `json:"priority,omitempty"`
}

type CreateTaskOutput struct {
	Success        bool     `json:"success"`
	Message        string   `json:"message"`
	TaskID         string   `json:"task_id,omitempty"`
	AssignedUserID string   `json:"assigned_user_id,omitempty"`
	MissingFields  []string `json:"missing_fields,omitempty"`
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

type UpdateStatusInput struct {
	LeadID        string `json:"lead_id"`
	LeadServiceID string `json:"lead_service_id,omitempty"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
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

type RescheduleVisitInput struct {
	AppointmentID string  `json:"appointment_id"`
	StartTime     string  `json:"start_time"`
	EndTime       string  `json:"end_time"`
	Title         *string `json:"title,omitempty"`
	Description   *string `json:"description,omitempty"`
}

type CancelVisitInput struct {
	AppointmentID string `json:"appointment_id"`
	Reason        string `json:"reason,omitempty"`
}

type PartnerPhoneRecord struct {
	PartnerID    uuid.UUID
	DisplayName  string
	PhoneNumber  string
	BusinessName string
}

type PartnerJobSummary struct {
	OfferID            string `json:"offer_id,omitempty"`
	PartnerID          string `json:"partner_id,omitempty"`
	LeadID             string `json:"lead_id,omitempty"`
	LeadServiceID      string `json:"lead_service_id,omitempty"`
	AppointmentID      string `json:"appointment_id,omitempty"`
	CustomerName       string `json:"customer_name,omitempty"`
	ServiceType        string `json:"service_type,omitempty"`
	City               string `json:"city,omitempty"`
	AppointmentTitle   string `json:"appointment_title,omitempty"`
	AppointmentStatus  string `json:"appointment_status,omitempty"`
	AppointmentStart   string `json:"appointment_start,omitempty"`
	AppointmentEnd     string `json:"appointment_end,omitempty"`
	DestinationAddress string `json:"destination_address,omitempty"`
	VakmanPriceCents   int64  `json:"vakman_price_cents,omitempty"`
	OfferStatus        string `json:"offer_status,omitempty"`
}

type SaveMeasurementInput struct {
	AppointmentID    string `json:"appointment_id"`
	Measurements     string `json:"measurements,omitempty"`
	AccessDifficulty string `json:"access_difficulty,omitempty"`
	Notes            string `json:"notes,omitempty"`
}

type UpdateAppointmentStatusInput struct {
	AppointmentID string `json:"appointment_id"`
	Status        string `json:"status"`
}
