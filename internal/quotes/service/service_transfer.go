package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	
	leadrepo "portal_final_backend/internal/leads/repository"
	leadstransport "portal_final_backend/internal/leads/transport"
	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/apperr"
	
	"github.com/google/uuid"
)

func (s *Service) TransferToOrganization(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, destinationTenantID uuid.UUID, actorID uuid.UUID) (*transport.TransferQuoteResponse, error) {
	if tenantID == destinationTenantID {
		return nil, apperr.Validation("destination organization must differ from source organization")
	}
	if s.leadCreator == nil || s.leadRepo == nil {
		return nil, apperr.Internal("quote transfer dependencies are not configured")
	}

	transferCtx, err := s.loadQuoteTransferContext(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	createdLead, err := s.createTransferredQuoteLead(ctx, transferCtx, destinationTenantID)
	if err != nil {
		return nil, err
	}

	createdQuote, err := s.Create(ctx, destinationTenantID, actorID, buildTransferredQuoteRequest(transferCtx, createdLead))
	if err != nil {
		return nil, err
	}
	s.recordQuoteTransferAudit(ctx, quoteTransferAuditParams{
		quoteID:               transferCtx.sourceQuote.ID,
		leadID:                transferCtx.sourceQuote.LeadID,
		serviceID:             nil,
		tenantID:              tenantID,
		relatedOrganizationID: destinationTenantID,
		actorID:               actorID,
		action:                "transferred_out",
		message:               "Quote transferred to another organization",
	})
	s.recordQuoteTransferAudit(ctx, quoteTransferAuditParams{
		quoteID:               createdQuote.ID,
		leadID:                createdQuote.LeadID,
		serviceID:             createdQuote.LeadServiceID,
		tenantID:              destinationTenantID,
		relatedOrganizationID: tenantID,
		actorID:               actorID,
		action:                "transferred_in",
		message:               "Quote received from another organization",
	})

	if err := s.Delete(ctx, id, tenantID, actorID); err != nil {
		return nil, err
	}

	cleanupPlan := planQuoteTransferSourceCleanup(transferCtx.sourceService.ID, transferCtx.sourceServices)
	if cleanupPlan.DeleteLead {
		if err := s.leadRepo.Delete(ctx, transferCtx.sourceLead.ID, tenantID); err != nil {
			return nil, err
		}
	} else if cleanupPlan.ServiceID != nil {
		if err := s.leadRepo.DeleteLeadService(ctx, *cleanupPlan.ServiceID, tenantID); err != nil {
			return nil, err
		}
	}

	return &transport.TransferQuoteResponse{
		Quote:                     *createdQuote,
		DestinationLeadID:         createdLead.ID,
		DestinationOrganizationID: destinationTenantID,
		SourceLeadDeleted:         cleanupPlan.DeleteLead,
	}, nil
}

type quoteTransferCleanupPlan struct {
	DeleteLead bool
	ServiceID  *uuid.UUID
}

func planQuoteTransferSourceCleanup(sourceServiceID uuid.UUID, services []leadrepo.LeadService) quoteTransferCleanupPlan {
	if len(services) <= 1 {
		return quoteTransferCleanupPlan{DeleteLead: true}
	}
	serviceID := sourceServiceID
	return quoteTransferCleanupPlan{DeleteLead: false, ServiceID: &serviceID}
}

type quoteTransferAuditParams struct {
	quoteID               uuid.UUID
	leadID                uuid.UUID
	serviceID             *uuid.UUID
	tenantID              uuid.UUID
	relatedOrganizationID uuid.UUID
	actorID               uuid.UUID
	action                string
	message               string
}

func (s *Service) recordQuoteTransferAudit(ctx context.Context, params quoteTransferAuditParams) {
	metadata, _ := json.Marshal(map[string]any{
		"action":                params.action,
		"relatedOrganizationId": params.relatedOrganizationID,
	})
	_ = s.repo.CreateActivity(ctx, &repository.QuoteActivity{
		ID:             uuid.New(),
		QuoteID:        params.quoteID,
		OrganizationID: params.tenantID,
		EventType:      "transfer",
		Message:        params.message,
		Metadata:       metadata,
		CreatedAt:      time.Now(),
	})
	s.emitTimelineEvent(ctx, TimelineEventParams{
		LeadID:         params.leadID,
		ServiceID:      params.serviceID,
		OrganizationID: params.tenantID,
		ActorType:      "User",
		ActorName:      params.actorID.String(),
		EventType:      "quote_transfer",
		Title:          params.message,
		Summary:        nil,
		Metadata: map[string]any{
			"quoteId":               params.quoteID,
			"action":                params.action,
			"relatedOrganizationId": params.relatedOrganizationID,
		},
		Visibility: "internal",
	})
}

type quoteTransferContext struct {
	sourceQuote    *repository.Quote
	sourceLead     leadrepo.Lead
	sourceServices []leadrepo.LeadService
	sourceService  *leadrepo.LeadService
	items          []repository.QuoteItem
	attachments    []repository.QuoteAttachment
	urls           []repository.QuoteURL
}

func (s *Service) loadQuoteTransferContext(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (quoteTransferContext, error) {
	sourceQuote, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return quoteTransferContext{}, err
	}
	items, attachments, urls, err := s.loadQuoteCloneData(ctx, id, tenantID)
	if err != nil {
		return quoteTransferContext{}, err
	}
	sourceLead, sourceServices, err := s.leadRepo.GetByIDWithServices(ctx, sourceQuote.LeadID, tenantID)
	if err != nil {
		return quoteTransferContext{}, err
	}
	sourceService := resolveTransferredQuoteService(sourceQuote.LeadServiceID, sourceServices)
	if sourceService == nil {
		return quoteTransferContext{}, apperr.Validation("quote has no transferable service context")
	}

	return quoteTransferContext{
		sourceQuote:    sourceQuote,
		sourceLead:     sourceLead,
		sourceServices: sourceServices,
		sourceService:  sourceService,
		items:          items,
		attachments:    attachments,
		urls:           urls,
	}, nil
}

func (s *Service) createTransferredQuoteLead(ctx context.Context, transfer quoteTransferContext, destinationTenantID uuid.UUID) (leadstransport.LeadResponse, error) {
	createdLead, err := s.leadCreator.Create(ctx, leadstransport.CreateLeadRequest{
		FirstName:       firstNonEmptyQuoteString(transfer.sourceQuote.CustomerFirstName, &transfer.sourceLead.ConsumerFirstName),
		LastName:        firstNonEmptyQuoteString(transfer.sourceQuote.CustomerLastName, &transfer.sourceLead.ConsumerLastName),
		Phone:           firstNonEmptyQuoteString(transfer.sourceQuote.CustomerPhone, &transfer.sourceLead.ConsumerPhone),
		Email:           firstNonEmptyQuoteString(transfer.sourceQuote.CustomerEmail, transfer.sourceLead.ConsumerEmail),
		ConsumerRole:    leadstransport.ConsumerRole(transfer.sourceLead.ConsumerRole),
		Street:          firstNonEmptyQuoteString(transfer.sourceQuote.CustomerAddressStreet, &transfer.sourceLead.AddressStreet),
		HouseNumber:     firstNonEmptyQuoteString(transfer.sourceQuote.CustomerAddressHouseNumber, &transfer.sourceLead.AddressHouseNumber),
		ZipCode:         firstNonEmptyQuoteString(transfer.sourceQuote.CustomerAddressZipCode, &transfer.sourceLead.AddressZipCode),
		City:            firstNonEmptyQuoteString(transfer.sourceQuote.CustomerAddressCity, &transfer.sourceLead.AddressCity),
		Latitude:        transfer.sourceLead.Latitude,
		Longitude:       transfer.sourceLead.Longitude,
		ServiceType:     leadstransport.ServiceType(transfer.sourceService.ServiceType),
		ConsumerNote:    ptrToString(transfer.sourceService.ConsumerNote),
		Source:          firstNonEmptyQuoteString(transfer.sourceService.Source, transfer.sourceLead.Source),
		WhatsAppOptedIn: quoteBoolPtr(transfer.sourceLead.WhatsAppOptedIn),
	}, destinationTenantID)
	if err != nil {
		return leadstransport.LeadResponse{}, err
	}

	if createdLead.CurrentService != nil {
		if _, err := s.leadRepo.UpdateServiceStatusAndPipelineStage(ctx, createdLead.CurrentService.ID, destinationTenantID, transfer.sourceService.Status, transfer.sourceService.PipelineStage); err != nil {
			return leadstransport.LeadResponse{}, err
		}
	}

	return createdLead, nil
}

func buildTransferredQuoteRequest(transfer quoteTransferContext, createdLead leadstransport.LeadResponse) transport.CreateQuoteRequest {
	request := transport.CreateQuoteRequest{
		LeadID:              createdLead.ID,
		PricingMode:         transfer.sourceQuote.PricingMode,
		DiscountType:        transfer.sourceQuote.DiscountType,
		DiscountValue:       transfer.sourceQuote.DiscountValue,
		ValidUntil:          transport.DateFromTime(transfer.sourceQuote.ValidUntil),
		Notes:               ptrToString(transfer.sourceQuote.Notes),
		FinancingDisclaimer: transfer.sourceQuote.FinancingDisclaimer,
		Items:               make([]transport.QuoteItemRequest, len(transfer.items)),
		Attachments:         cloneAttachmentRequests(transfer.attachments),
		URLs:                cloneURLRequests(transfer.urls),
	}
	if subsidySnapshot, err := unmarshalQuoteSubsidySnapshot(transfer.sourceQuote.SubsidyData); err == nil {
		request.ISDESubsidy = subsidySnapshot
	}
	if createdLead.CurrentService != nil {
		request.LeadServiceID = &createdLead.CurrentService.ID
	}
	for index, item := range transfer.items {
		request.Items[index] = transport.QuoteItemRequest{
			Title:            item.Title,
			Description:      item.Description,
			Quantity:         item.Quantity,
			UnitPriceCents:   item.UnitPriceCents,
			TaxRateBps:       item.TaxRateBps,
			IsOptional:       item.IsOptional,
			IsSelected:       item.IsSelected,
			CatalogProductID: item.CatalogProductID,
		}
	}
	return request
}

func resolveTransferredQuoteService(leadServiceID *uuid.UUID, services []leadrepo.LeadService) *leadrepo.LeadService {
	if leadServiceID != nil {
		for _, service := range services {
			if service.ID == *leadServiceID {
				serviceCopy := service
				return &serviceCopy
			}
		}
	}
	for _, service := range services {
		if service.PipelineStage != "Completed" && service.PipelineStage != "Lost" {
			serviceCopy := service
			return &serviceCopy
		}
	}
	if len(services) == 0 {
		return nil
	}
	serviceCopy := services[0]
	return &serviceCopy
}

func firstNonEmptyQuoteString(values ...*string) string {
	for _, value := range values {
		if value == nil {
			continue
		}
		trimmed := strings.TrimSpace(*value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func quoteBoolPtr(value bool) *bool {
	result := value
	return &result
}
