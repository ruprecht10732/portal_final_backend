package tools

import (
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"portal_final_backend/platform/adk/confirmation"
	"portal_final_backend/platform/adk/plugins"
)

func newDomainTool[In any, Out any](name, description string, handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        name,
		Description: description,
	}, handler)
}

func NewPreloadMemoryTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("PreloadMemory", "Searches the agent's long-term memory for past interactions and preferences from the user. Use this at the start of a session.", handler)
}

func NewSaveAnalysisTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("SaveAnalysis", "Saves the gatekeeper triage analysis to the database. Call this ONCE after completing your full analysis. Include urgency, lead quality, recommended action, missing information, resolved information, extracted facts, preferred contact channel, message, and summary.", handler)
}

func NewUpdateLeadServiceTypeTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("UpdateLeadServiceType", "Updates the service type for a lead service when there is a confident mismatch. The service type must match an active service type name or slug.", handler)
}

func NewUpdateLeadDetailsTool[In any, Out any](description string, handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("UpdateLeadDetails", description, handler)
}

func NewCreateLeadTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("CreateLead", "Creates a new lead in the current organization after collecting the required customer and address details. Required fields: first_name, last_name, phone, consumer_role (must be exactly one of: Owner, Tenant, Landlord), street, house_number, zip_code, city, service_type (the product/service name as known in the organization catalog, e.g. Zonnepanelen, Warmtepomp, Isolatie). Optional: email, consumer_note. If required fields are missing, returns which fields are still needed. Do NOT provide a tenant or organization identifier. When the caller's own phone number is known and no other phone is provided, use the caller's phone.", handler)
}

func NewCreateTaskTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("CreateTask", "Creates a follow-up task in the current organization. Use this for internal follow-up items. For lead-related tasks, provide lead_id and lead_service_id. If assigned_user_id is omitted, the system only defaults to the current lead assignee when that is already known.", handler)
}

func NewUpdatePipelineStageTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("UpdatePipelineStage", "Updates the pipeline stage for the lead service and records a timeline event.", confirmation.WrapToolHandler("UpdatePipelineStage", handler))
}

func NewFindMatchingPartnersTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("FindMatchingPartners", "Finds partner matches by service type and distance radius. Allows excluding specific partner IDs.", plugins.WrapHandler(handler, plugins.DefaultRetryPolicy()))
}

func NewCreatePartnerOfferTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("CreatePartnerOffer", "Creates a formal job offer for a specific partner. This generates the unique link they use to accept the job.", confirmation.WrapToolHandler("CreatePartnerOffer", handler))
}

func NewSaveEstimationTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("SaveEstimation", "Saves estimation metadata (scope and price range) to the lead timeline.", handler)
}

func NewCommitScopeArtifactTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("CommitScopeArtifact", "Commits structured scope analysis output for the quote builder phase.", handler)
}

func NewAskCustomerClarificationTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("AskCustomerClarification", "Stores a Dutch clarification request for the customer when intake is incomplete.", handler)
}

func NewSearchLeadsTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("SearchLeads", "Searches leads by customer name, phone number, or quote number in the current organization. Also finds leads linked to quotes. Returns matching lead and current service identifiers for follow-up actions. Do NOT provide a tenant or organization identifier.", handler)
}

func NewGetAvailableVisitSlotsTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("GetAvailableVisitSlots", "Lists available appointment slots for the current organization. Returns start_time, end_time, and assigned_user_id needed to request a visit. Do NOT provide a tenant or organization identifier.", handler)
}

func NewGetLeadDetailsTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("GetLeadDetails", "Returns contact and address details for a lead in the current organization. Use the lead_id from SearchLeads or quote context. Do NOT provide a tenant or organization identifier.", handler)
}

func NewGetEnergyLabelTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("GetEnergyLabel", "Returns the energy label for a resolved lead or Dutch address in the current organization. Use this when customers ask about energy class, label validity, or building-energy details.", plugins.WrapHandler(handler, plugins.DefaultRetryPolicy()))
}

func NewGetLeadTasksTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("GetLeadTasks", "Lists follow-up tasks for a specific lead in the current organization. Optionally filter by lead_service_id or status.", handler)
}

func NewGetISDETool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("GetISDE", "Calculates an ISDE subsidy estimate from the provided insulation and installation measures for the current organization.", plugins.WrapHandler(handler, plugins.DefaultRetryPolicy()))
}

func NewGetNavigationLinkTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("GetNavigationLink", "Returns a Google Maps directions link for a lead in the current organization using the lead's address. Provide the lead_id returned by SearchLeads. Do NOT provide a tenant or organization identifier.", handler)
}

func NewCalculatorTool[In any, Out any](description string, handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("Calculator", description, handler)
}

func NewCalculateEstimateTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("CalculateEstimate", "Calculates material subtotal, labor subtotal range, and total range from raw structured inputs (unit prices, quantities, hour ranges, hourly rate ranges). Do NOT pre-calculate subtotals; this tool performs all multiplication.", handler)
}

func NewListCatalogGapsTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("ListCatalogGaps", "Records structured catalog gaps when no reliable product/material matches are available.", handler)
}

func NewSearchProductMaterialsTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("SearchProductMaterials", "Searches the product/material catalog and returns ranked matches with pricing and confidence metadata.", plugins.WrapHandler(handler, plugins.DefaultRetryPolicy()))
}

func NewSubmitQuoteCritiqueTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("SubmitQuoteCritique", "Stores structured quote critique feedback for a generated draft quote.", handler)
}

func NewDraftQuoteTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("DraftQuote", "Creates or updates a structured draft quote from the provided line items and pricing metadata.", confirmation.WrapToolHandler("DraftQuote", handler))
}

func NewSaveNoteTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("SaveNote", "Saves a note on the lead or service timeline. Use this to record important customer context or follow-up information. The body will be normalized/cleaned server-side.", handler)
}

func NewSetCallOutcomeTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("SetCallOutcome", "Stores a short call outcome label on the timeline.", handler)
}

func NewUpdateStatusTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("UpdateStatus", "Updates the status of the lead service. Valid statuses: New, Pending, In_Progress, Attempted_Contact, Appointment_Scheduled, Needs_Rescheduling, Disqualified", handler)
}

func NewScheduleVisitTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("ScheduleVisit", "Books an inspection/visit appointment for the lead. Provide start and end times in ISO 8601 format. Set sendConfirmationEmail to false if the call notes mention not sending email; otherwise it defaults to true.", handler)
}

func NewRescheduleVisitTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("RescheduleVisit", "Reschedules an existing lead visit appointment. Provide start and end times in ISO 8601 format.", handler)
}

func NewCancelVisitTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("CancelVisit", "Cancels the existing lead visit appointment.", confirmation.WrapToolHandler("CancelVisit", handler))
}

func NewSavePhotoAnalysisTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("SavePhotoAnalysis", "Save the analysis of photos for a lead service. Call this after analyzing all photos. Include measurements, discrepancies, extracted text, and suggested search terms.", handler)
}

func NewFlagOnsiteMeasurementTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("FlagOnsiteMeasurement", "Flag that a specific measurement cannot be determined from photos alone and requires on-site measurement. Call this for EACH measurement that needs on-site verification.", handler)
}

func NewSubmitAuditResultTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("SubmitAuditResult", "Submit the audit result. If required info is missing, flag Manual_Intervention and explain what is missing.", handler)
}

func NewGetPendingQuotesTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("GetPendingQuotes",
		"Lists quotes for the current organization filtered by status. Returns quote_number, client_name, total_cents, and status. Do NOT provide a tenant or organization identifier.",
		handler)
}

func NewGetQuotesTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("GetQuotes",
		"Lists quotes for the current organization filtered by status. Returns quote_number, client_name, total_cents, status, created_at, and a readable summary of what the quote covers. Do NOT provide a tenant or organization identifier.",
		handler)
}

func NewGetAppointmentsTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("GetAppointments",
		"Lists upcoming appointments for the current organization within an optional date range. Returns title, description, start_time, end_time, status, location. Do NOT provide a tenant or organization identifier.",
		handler)
}

func NewGetMyJobsTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("GetMyJobs",
		"Lists the authenticated partner's accepted jobs and their latest appointment context. Use this first when the partner refers to one of their own jobs.",
		handler)
}

func NewGetPartnerJobDetailsTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("GetPartnerJobDetails",
		"Returns the details for one accepted partner job, resolved by appointment_id, lead_service_id, or lead_id.",
		handler)
}

func NewAttachCurrentWhatsAppPhotoTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("AttachCurrentWhatsAppPhoto", "Attaches the current inbound WhatsApp image message to a resolved lead service in the current organization. Use only when the current inbound message is an image from the customer.", handler)
}

func NewSaveMeasurementTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("SaveMeasurement", "Stores measurements, access difficulty, and notes on the selected appointment visit report for the authenticated partner's job.", handler)
}

func NewUpdateAppointmentStatusTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("UpdateAppointmentStatus", "Updates the status of an appointment for the authenticated partner's accepted job. Valid values: scheduled, requested, completed, cancelled, no_show.", handler)
}

func NewGenerateQuoteTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("GenerateQuote", "Generates a draft quote from a grounded natural-language prompt for a resolved lead service in the current organization. Prefer this when the customer asks for a quote without a full explicit item list.", confirmation.WrapToolHandler("GenerateQuote", handler))
}

func NewSendQuotePDFTool[In any, Out any](handler func(tool.Context, In) (Out, error)) (tool.Tool, error) {
	return newDomainTool("SendQuotePDF", "Retrieves or lazily generates a quote PDF for a resolved quote in the current organization and sends it back to the current WhatsApp customer as a document attachment.", handler)
}
