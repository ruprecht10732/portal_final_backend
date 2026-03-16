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
