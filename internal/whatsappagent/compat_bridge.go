package whatsappagent

import (
	"context"
	"time"

	"portal_final_backend/internal/whatsappagent/engine"

	"github.com/google/uuid"
)

type quotesReaderAdapter struct{ inner QuotesReader }

type quoteWorkflowWriterAdapter struct{ inner QuoteWorkflowWriter }

type appointmentsReaderAdapter struct{ inner AppointmentsReader }

type leadSearchReaderAdapter struct{ inner LeadSearchReader }

type navigationLinkReaderAdapter struct{ inner NavigationLinkReader }

type leadDetailsReaderAdapter struct{ inner LeadDetailsReader }

type catalogSearchReaderAdapter struct{ inner CatalogSearchReader }

type leadMutationWriterAdapter struct{ inner LeadMutationWriter }

type currentInboundPhotoAttacherAdapter struct{ inner CurrentInboundPhotoAttacher }

type visitSlotReaderAdapter struct{ inner VisitSlotReader }

type visitMutationWriterAdapter struct{ inner VisitMutationWriter }

type partnerPhoneReaderAdapter struct{ inner PartnerPhoneReader }

type partnerJobReaderAdapter struct{ inner PartnerJobReader }

type appointmentVisitReportWriterAdapter struct{ inner AppointmentVisitReportWriter }

type appointmentStatusWriterAdapter struct{ inner AppointmentStatusWriter }

func adaptQuotesReader(inner QuotesReader) engine.QuotesReader {
	if inner == nil {
		return nil
	}
	return quotesReaderAdapter{inner: inner}
}

func adaptQuoteWorkflowWriter(inner QuoteWorkflowWriter) engine.QuoteWorkflowWriter {
	if inner == nil {
		return nil
	}
	return quoteWorkflowWriterAdapter{inner: inner}
}

func adaptAppointmentsReader(inner AppointmentsReader) engine.AppointmentsReader {
	if inner == nil {
		return nil
	}
	return appointmentsReaderAdapter{inner: inner}
}

func adaptLeadSearchReader(inner LeadSearchReader) engine.LeadSearchReader {
	if inner == nil {
		return nil
	}
	return leadSearchReaderAdapter{inner: inner}
}

func adaptNavigationLinkReader(inner NavigationLinkReader) engine.NavigationLinkReader {
	if inner == nil {
		return nil
	}
	return navigationLinkReaderAdapter{inner: inner}
}

func adaptLeadDetailsReader(inner LeadDetailsReader) engine.LeadDetailsReader {
	if inner == nil {
		return nil
	}
	return leadDetailsReaderAdapter{inner: inner}
}

func adaptCatalogSearchReader(inner CatalogSearchReader) engine.CatalogSearchReader {
	if inner == nil {
		return nil
	}
	return catalogSearchReaderAdapter{inner: inner}
}

func adaptLeadMutationWriter(inner LeadMutationWriter) engine.LeadMutationWriter {
	if inner == nil {
		return nil
	}
	return leadMutationWriterAdapter{inner: inner}
}

func adaptCurrentInboundPhotoAttacher(inner CurrentInboundPhotoAttacher) engine.CurrentInboundPhotoAttacher {
	if inner == nil {
		return nil
	}
	return currentInboundPhotoAttacherAdapter{inner: inner}
}

func adaptVisitSlotReader(inner VisitSlotReader) engine.VisitSlotReader {
	if inner == nil {
		return nil
	}
	return visitSlotReaderAdapter{inner: inner}
}

func adaptVisitMutationWriter(inner VisitMutationWriter) engine.VisitMutationWriter {
	if inner == nil {
		return nil
	}
	return visitMutationWriterAdapter{inner: inner}
}

func adaptPartnerPhoneReader(inner PartnerPhoneReader) engine.PartnerPhoneReader {
	if inner == nil {
		return nil
	}
	return partnerPhoneReaderAdapter{inner: inner}
}

func adaptPartnerJobReader(inner PartnerJobReader) engine.PartnerJobReader {
	if inner == nil {
		return nil
	}
	return partnerJobReaderAdapter{inner: inner}
}

func adaptAppointmentVisitReportWriter(inner AppointmentVisitReportWriter) engine.AppointmentVisitReportWriter {
	if inner == nil {
		return nil
	}
	return appointmentVisitReportWriterAdapter{inner: inner}
}

func adaptAppointmentStatusWriter(inner AppointmentStatusWriter) engine.AppointmentStatusWriter {
	if inner == nil {
		return nil
	}
	return appointmentStatusWriterAdapter{inner: inner}
}

func (a quotesReaderAdapter) ListQuotesByOrganization(ctx context.Context, orgID uuid.UUID, status *string) ([]engine.QuoteSummary, error) {
	items, err := a.inner.ListQuotesByOrganization(ctx, orgID, status)
	if err != nil {
		return nil, err
	}
	result := make([]engine.QuoteSummary, 0, len(items))
	for _, item := range items {
		result = append(result, engine.QuoteSummary(item))
	}
	return result, nil
}

func (a quoteWorkflowWriterAdapter) DraftQuote(ctx context.Context, orgID uuid.UUID, input engine.DraftQuoteInput) (engine.DraftQuoteOutput, error) {
	output, err := a.inner.DraftQuote(ctx, orgID, toLocalDraftQuoteInput(input))
	return engine.DraftQuoteOutput(output), err
}

func (a quoteWorkflowWriterAdapter) GenerateQuote(ctx context.Context, orgID uuid.UUID, input engine.GenerateQuoteInput) (engine.GenerateQuoteOutput, error) {
	output, err := a.inner.GenerateQuote(ctx, orgID, GenerateQuoteInput(input))
	return engine.GenerateQuoteOutput(output), err
}

func (a quoteWorkflowWriterAdapter) GetQuotePDF(ctx context.Context, orgID uuid.UUID, input engine.SendQuotePDFInput) (engine.QuotePDFResult, error) {
	output, err := a.inner.GetQuotePDF(ctx, orgID, SendQuotePDFInput(input))
	return engine.QuotePDFResult(output), err
}

func (a appointmentsReaderAdapter) ListAppointmentsByOrganization(ctx context.Context, orgID uuid.UUID, from, to *time.Time) ([]engine.AppointmentSummary, error) {
	items, err := a.inner.ListAppointmentsByOrganization(ctx, orgID, from, to)
	if err != nil {
		return nil, err
	}
	result := make([]engine.AppointmentSummary, 0, len(items))
	for _, item := range items {
		result = append(result, engine.AppointmentSummary(item))
	}
	return result, nil
}

func (a leadSearchReaderAdapter) SearchLeads(ctx context.Context, orgID uuid.UUID, query string, limit int) ([]engine.LeadSearchResult, error) {
	items, err := a.inner.SearchLeads(ctx, orgID, query, limit)
	if err != nil {
		return nil, err
	}
	result := make([]engine.LeadSearchResult, 0, len(items))
	for _, item := range items {
		result = append(result, engine.LeadSearchResult(item))
	}
	return result, nil
}

func (a navigationLinkReaderAdapter) GetNavigationLink(ctx context.Context, orgID uuid.UUID, leadID string) (*engine.NavigationLinkResult, error) {
	item, err := a.inner.GetNavigationLink(ctx, orgID, leadID)
	if err != nil || item == nil {
		return nil, err
	}
	converted := engine.NavigationLinkResult(*item)
	return &converted, nil
}

func (a leadDetailsReaderAdapter) GetLeadDetails(ctx context.Context, orgID uuid.UUID, leadID string) (*engine.LeadDetailsResult, error) {
	item, err := a.inner.GetLeadDetails(ctx, orgID, leadID)
	if err != nil || item == nil {
		return nil, err
	}
	converted := engine.LeadDetailsResult(*item)
	return &converted, nil
}

func (a catalogSearchReaderAdapter) SearchProductMaterials(ctx context.Context, orgID uuid.UUID, input engine.SearchProductMaterialsInput) (engine.SearchProductMaterialsOutput, error) {
	output, err := a.inner.SearchProductMaterials(ctx, orgID, SearchProductMaterialsInput(input))
	if err != nil {
		return engine.SearchProductMaterialsOutput{}, err
	}
	result := engine.SearchProductMaterialsOutput{Message: output.Message}
	if len(output.Products) > 0 {
		result.Products = make([]engine.ProductResult, 0, len(output.Products))
		for _, item := range output.Products {
			result.Products = append(result.Products, engine.ProductResult(item))
		}
	}
	return result, nil
}

func (a leadMutationWriterAdapter) CreateLead(ctx context.Context, orgID uuid.UUID, input engine.CreateLeadInput) (engine.CreateLeadOutput, error) {
	output, err := a.inner.CreateLead(ctx, orgID, CreateLeadInput(input))
	if err != nil {
		return engine.CreateLeadOutput{}, err
	}
	result := engine.CreateLeadOutput{Success: output.Success, Message: output.Message, MissingFields: output.MissingFields}
	if output.Lead != nil {
		lead := engine.CreateLeadResult(*output.Lead)
		result.Lead = &lead
	}
	return result, nil
}

func (a leadMutationWriterAdapter) ResolveServiceID(ctx context.Context, leadID, organizationID uuid.UUID, requestedServiceID *uuid.UUID) (uuid.UUID, error) {
	return a.inner.ResolveServiceID(ctx, leadID, organizationID, requestedServiceID)
}

func (a leadMutationWriterAdapter) UpdateLeadDetails(ctx context.Context, orgID uuid.UUID, input engine.UpdateLeadDetailsInput) ([]string, error) {
	return a.inner.UpdateLeadDetails(ctx, orgID, UpdateLeadDetailsInput(input))
}

func (a leadMutationWriterAdapter) AskCustomerClarification(ctx context.Context, orgID uuid.UUID, input engine.AskCustomerClarificationInput) error {
	return a.inner.AskCustomerClarification(ctx, orgID, AskCustomerClarificationInput(input))
}

func (a leadMutationWriterAdapter) SaveNote(ctx context.Context, orgID uuid.UUID, input engine.SaveNoteInput) error {
	return a.inner.SaveNote(ctx, orgID, SaveNoteInput(input))
}

func (a leadMutationWriterAdapter) UpdateLeadStatus(ctx context.Context, orgID uuid.UUID, input engine.UpdateStatusInput) (string, error) {
	return a.inner.UpdateLeadStatus(ctx, orgID, UpdateStatusInput(input))
}

func (a currentInboundPhotoAttacherAdapter) AttachCurrentWhatsAppPhoto(ctx context.Context, orgID uuid.UUID, input engine.AttachCurrentWhatsAppPhotoInput, message engine.CurrentInboundMessage) (engine.AttachCurrentWhatsAppPhotoOutput, error) {
	output, err := a.inner.AttachCurrentWhatsAppPhoto(ctx, orgID, AttachCurrentWhatsAppPhotoInput(input), CurrentInboundMessage(message))
	return engine.AttachCurrentWhatsAppPhotoOutput(output), err
}

func (a visitSlotReaderAdapter) GetAvailableVisitSlots(ctx context.Context, orgID uuid.UUID, startDate, endDate string, slotDuration int) ([]engine.VisitSlotSummary, error) {
	items, err := a.inner.GetAvailableVisitSlots(ctx, orgID, startDate, endDate, slotDuration)
	if err != nil {
		return nil, err
	}
	result := make([]engine.VisitSlotSummary, 0, len(items))
	for _, item := range items {
		result = append(result, engine.VisitSlotSummary(item))
	}
	return result, nil
}

func (a visitMutationWriterAdapter) ScheduleVisit(ctx context.Context, orgID uuid.UUID, input engine.ScheduleVisitInput) (*engine.AppointmentSummary, error) {
	item, err := a.inner.ScheduleVisit(ctx, orgID, ScheduleVisitInput(input))
	if err != nil || item == nil {
		return nil, err
	}
	converted := engine.AppointmentSummary(*item)
	return &converted, nil
}

func (a visitMutationWriterAdapter) RescheduleVisit(ctx context.Context, orgID uuid.UUID, input engine.RescheduleVisitInput) (*engine.AppointmentSummary, error) {
	item, err := a.inner.RescheduleVisit(ctx, orgID, RescheduleVisitInput(input))
	if err != nil || item == nil {
		return nil, err
	}
	converted := engine.AppointmentSummary(*item)
	return &converted, nil
}

func (a visitMutationWriterAdapter) CancelVisit(ctx context.Context, orgID uuid.UUID, input engine.CancelVisitInput) error {
	return a.inner.CancelVisit(ctx, orgID, CancelVisitInput(input))
}

func (a partnerPhoneReaderAdapter) GetPartnerPhone(ctx context.Context, orgID, partnerID uuid.UUID) (*engine.PartnerPhoneRecord, error) {
	item, err := a.inner.GetPartnerPhone(ctx, orgID, partnerID)
	if err != nil || item == nil {
		return nil, err
	}
	converted := engine.PartnerPhoneRecord(*item)
	return &converted, nil
}

func (a partnerJobReaderAdapter) ListPartnerJobs(ctx context.Context, orgID, partnerID uuid.UUID) ([]engine.PartnerJobSummary, error) {
	items, err := a.inner.ListPartnerJobs(ctx, orgID, partnerID)
	if err != nil {
		return nil, err
	}
	result := make([]engine.PartnerJobSummary, 0, len(items))
	for _, item := range items {
		result = append(result, engine.PartnerJobSummary(item))
	}
	return result, nil
}

func (a partnerJobReaderAdapter) GetPartnerJobByService(ctx context.Context, orgID, partnerID, leadServiceID uuid.UUID) (*engine.PartnerJobSummary, error) {
	item, err := a.inner.GetPartnerJobByService(ctx, orgID, partnerID, leadServiceID)
	if err != nil || item == nil {
		return nil, err
	}
	converted := engine.PartnerJobSummary(*item)
	return &converted, nil
}

func (a partnerJobReaderAdapter) GetPartnerJobByAppointment(ctx context.Context, orgID, partnerID, appointmentID uuid.UUID) (*engine.PartnerJobSummary, error) {
	item, err := a.inner.GetPartnerJobByAppointment(ctx, orgID, partnerID, appointmentID)
	if err != nil || item == nil {
		return nil, err
	}
	converted := engine.PartnerJobSummary(*item)
	return &converted, nil
}

func (a partnerJobReaderAdapter) GetPartnerJobByLead(ctx context.Context, orgID, partnerID, leadID uuid.UUID) (*engine.PartnerJobSummary, error) {
	item, err := a.inner.GetPartnerJobByLead(ctx, orgID, partnerID, leadID)
	if err != nil || item == nil {
		return nil, err
	}
	converted := engine.PartnerJobSummary(*item)
	return &converted, nil
}

func (a appointmentVisitReportWriterAdapter) UpsertVisitReport(ctx context.Context, orgID, appointmentID uuid.UUID, input engine.SaveMeasurementInput) error {
	return a.inner.UpsertVisitReport(ctx, orgID, appointmentID, SaveMeasurementInput(input))
}

func (a appointmentStatusWriterAdapter) UpdateAppointmentStatus(ctx context.Context, orgID, appointmentID uuid.UUID, input engine.UpdateAppointmentStatusInput) (*engine.AppointmentSummary, error) {
	item, err := a.inner.UpdateAppointmentStatus(ctx, orgID, appointmentID, UpdateAppointmentStatusInput(input))
	if err != nil || item == nil {
		return nil, err
	}
	converted := engine.AppointmentSummary(*item)
	return &converted, nil
}

func toLocalDraftQuoteInput(input engine.DraftQuoteInput) DraftQuoteInput {
	result := DraftQuoteInput{
		LeadID:        input.LeadID,
		LeadServiceID: input.LeadServiceID,
		Notes:         input.Notes,
	}
	if len(input.Items) > 0 {
		result.Items = make([]DraftQuoteItem, 0, len(input.Items))
		for _, item := range input.Items {
			result.Items = append(result.Items, DraftQuoteItem{
				Description:      item.Description,
				Quantity:         item.Quantity,
				UnitPriceCents:   item.UnitPriceCents,
				TaxRateBps:       item.TaxRateBps,
				IsOptional:       item.IsOptional,
				CatalogProductID: item.CatalogProductID,
			})
		}
	}
	return result
}
