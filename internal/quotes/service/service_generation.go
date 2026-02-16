package service

import (
	"context"
	"fmt"
	"time"

	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

func (s *Service) GetLatestNonDraftByLead(ctx context.Context, leadID uuid.UUID, orgID uuid.UUID) (*repository.Quote, error) {
	return s.repo.GetLatestNonDraftByLead(ctx, leadID, orgID)
}

func (s *Service) GenerateQuote(ctx context.Context, tenantID uuid.UUID, leadID uuid.UUID, serviceID uuid.UUID, prompt string, existingQuoteID *uuid.UUID) (*GenerateQuoteResult, error) {
	if s.promptGen == nil {
		return nil, apperr.Internal("quote generation is not configured")
	}
	return s.promptGen.GenerateFromPrompt(ctx, leadID, serviceID, tenantID, prompt, existingQuoteID)
}

func (s *Service) StartGenerateQuoteJob(ctx context.Context, tenantID, userID, leadID, serviceID uuid.UUID, prompt string, existingQuoteID *uuid.UUID) (uuid.UUID, error) {
	if s.promptGen == nil {
		return uuid.Nil, apperr.Internal("quote generation is not configured")
	}
	if s.jobQueue == nil {
		return uuid.Nil, apperr.Internal("quote generation job queue is not configured")
	}

	now := time.Now()
	job := &GenerateQuoteJob{
		JobID:           uuid.New(),
		TenantID:        tenantID,
		UserID:          userID,
		LeadID:          leadID,
		LeadServiceID:   serviceID,
		Status:          GenerateQuoteJobStatusPending,
		Step:            jobStepQueued,
		ProgressPercent: 0,
		StartedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.repo.CreateGenerateQuoteJob(ctx, &repository.GenerateQuoteJob{
		ID:              job.JobID,
		OrganizationID:  job.TenantID,
		UserID:          job.UserID,
		LeadID:          job.LeadID,
		LeadServiceID:   job.LeadServiceID,
		Status:          string(job.Status),
		Step:            job.Step,
		ProgressPercent: job.ProgressPercent,
		StartedAt:       job.StartedAt,
		UpdatedAt:       job.UpdatedAt,
	}); err != nil {
		return uuid.Nil, err
	}

	s.publishJobProgress(job)

	if err := s.jobQueue.EnqueueGenerateQuoteJobRequest(ctx, job.JobID, tenantID, userID, leadID, serviceID, prompt, existingQuoteID); err != nil {
		errText := err.Error()
		if progressErr := s.updateJobProgress(ctx, job.JobID, GenerateQuoteJobStatusFailed, jobStepQueueFailed, 100, &errText); progressErr != nil {
			return uuid.Nil, fmt.Errorf("enqueue generate quote job: %w (status update failed: %v)", err, progressErr)
		}
		return uuid.Nil, err
	}

	return job.JobID, nil
}

func (s *Service) GetGenerateQuoteJob(ctx context.Context, tenantID, userID, jobID uuid.UUID) (*GenerateQuoteJob, error) {
	job, err := s.repo.GetGenerateQuoteJob(ctx, tenantID, userID, jobID)
	if err != nil {
		return nil, err
	}

	return &GenerateQuoteJob{
		JobID:           job.ID,
		TenantID:        job.OrganizationID,
		UserID:          job.UserID,
		LeadID:          job.LeadID,
		LeadServiceID:   job.LeadServiceID,
		Status:          GenerateQuoteJobStatus(job.Status),
		Step:            job.Step,
		ProgressPercent: job.ProgressPercent,
		Error:           job.Error,
		QuoteID:         job.QuoteID,
		QuoteNumber:     job.QuoteNumber,
		ItemCount:       job.ItemCount,
		StartedAt:       job.StartedAt,
		UpdatedAt:       job.UpdatedAt,
		FinishedAt:      job.FinishedAt,
	}, nil
}

func (s *Service) ProcessGenerateQuoteJob(ctx context.Context, jobID uuid.UUID, prompt string, existingQuoteID *uuid.UUID) error {
	claimed, err := s.repo.ClaimGenerateQuoteJob(ctx, jobID, jobStepPreparingContext, 10, time.Now())
	if err != nil {
		return err
	}

	if claimed == nil {
		current, currentErr := s.repo.GetGenerateQuoteJobByID(ctx, jobID)
		if currentErr != nil {
			return currentErr
		}
		switch current.Status {
		case string(GenerateQuoteJobStatusRunning), string(GenerateQuoteJobStatusCompleted), string(GenerateQuoteJobStatusFailed):
			return nil
		default:
			return fmt.Errorf("generate quote job %s cannot be claimed from status %s", jobID, current.Status)
		}
	}

	s.publishJobProgress(repositoryJobToServiceJob(claimed))

	if err := s.updateJobProgress(ctx, jobID, GenerateQuoteJobStatusRunning, jobStepGeneratingAIQuote, 55, nil); err != nil {
		return err
	}

	result, err := s.GenerateQuote(ctx, claimed.OrganizationID, claimed.LeadID, claimed.LeadServiceID, prompt, existingQuoteID)
	if err != nil {
		errText := err.Error()
		if progressErr := s.updateJobProgress(ctx, jobID, GenerateQuoteJobStatusFailed, jobStepGenerationFailed, 100, &errText); progressErr != nil {
			return fmt.Errorf("generate quote failed: %w (status update failed: %v)", err, progressErr)
		}
		return err
	}

	if err := s.updateJobProgress(ctx, jobID, GenerateQuoteJobStatusRunning, jobStepFinalizingAndSaving, 90, nil); err != nil {
		return err
	}

	now := time.Now()
	entry := &repository.GenerateQuoteJob{
		ID:              jobID,
		Status:          string(GenerateQuoteJobStatusCompleted),
		Step:            jobStepCompleted,
		ProgressPercent: 100,
		QuoteID:         &result.QuoteID,
		QuoteNumber:     &result.QuoteNumber,
		ItemCount:       &result.ItemCount,
		UpdatedAt:       now,
		FinishedAt:      &now,
	}
	if err := s.repo.UpdateGenerateQuoteJob(ctx, entry); err != nil {
		return err
	}

	stored, err := s.repo.GetGenerateQuoteJobByID(ctx, jobID)
	if err != nil {
		return err
	}
	s.publishJobProgress(repositoryJobToServiceJob(stored))
	return nil
}

func repositoryJobToServiceJob(job *repository.GenerateQuoteJob) *GenerateQuoteJob {
	if job == nil {
		return nil
	}
	return &GenerateQuoteJob{
		JobID:           job.ID,
		TenantID:        job.OrganizationID,
		UserID:          job.UserID,
		LeadID:          job.LeadID,
		LeadServiceID:   job.LeadServiceID,
		Status:          GenerateQuoteJobStatus(job.Status),
		Step:            job.Step,
		ProgressPercent: job.ProgressPercent,
		Error:           job.Error,
		QuoteID:         job.QuoteID,
		QuoteNumber:     job.QuoteNumber,
		ItemCount:       job.ItemCount,
		StartedAt:       job.StartedAt,
		UpdatedAt:       job.UpdatedAt,
		FinishedAt:      job.FinishedAt,
	}
}

func (s *Service) updateJobProgress(ctx context.Context, jobID uuid.UUID, status GenerateQuoteJobStatus, step string, progress int, errText *string) error {
	stored, err := s.repo.GetGenerateQuoteJobByID(ctx, jobID)
	if err != nil {
		return err
	}

	now := time.Now()
	update := &repository.GenerateQuoteJob{
		ID:              jobID,
		Status:          string(status),
		Step:            step,
		ProgressPercent: progress,
		Error:           errText,
		QuoteID:         stored.QuoteID,
		QuoteNumber:     stored.QuoteNumber,
		ItemCount:       stored.ItemCount,
		UpdatedAt:       now,
		FinishedAt:      stored.FinishedAt,
	}
	if status == GenerateQuoteJobStatusFailed {
		update.FinishedAt = &now
	}
	if err := s.repo.UpdateGenerateQuoteJob(ctx, update); err != nil {
		return err
	}

	stored.Status = update.Status
	stored.Step = update.Step
	stored.ProgressPercent = update.ProgressPercent
	stored.Error = update.Error
	stored.UpdatedAt = update.UpdatedAt
	stored.FinishedAt = update.FinishedAt
	s.publishJobProgress(repositoryJobToServiceJob(stored))
	return nil
}

func (s *Service) publishJobProgress(job *GenerateQuoteJob) {
	if s.sse == nil || job == nil {
		return
	}
	data := map[string]interface{}{
		"job": map[string]interface{}{
			"jobId":           job.JobID,
			"status":          string(job.Status),
			"step":            job.Step,
			"progressPercent": job.ProgressPercent,
			"leadId":          job.LeadID,
			"leadServiceId":   job.LeadServiceID,
			"startedAt":       job.StartedAt,
			"updatedAt":       job.UpdatedAt,
			"finishedAt":      job.FinishedAt,
			"error":           job.Error,
			"quoteId":         job.QuoteID,
			"quoteNumber":     job.QuoteNumber,
			"itemCount":       job.ItemCount,
		},
	}
	s.sse.Publish(job.UserID, sse.Event{Type: sse.EventAIJobProgress, Message: "AI quote generation progress", Data: data})
}

func (s *Service) DraftQuote(ctx context.Context, params DraftQuoteParams) (*DraftQuoteResult, error) {
	if params.QuoteID != nil {
		return s.updateDraftQuote(ctx, params)
	}
	return s.createDraftQuote(ctx, params)
}

func (s *Service) createDraftQuote(ctx context.Context, params DraftQuoteParams) (*DraftQuoteResult, error) {
	quoteNumber, err := s.repo.NextQuoteNumber(ctx, params.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("generate quote number: %w", err)
	}

	calc := CalculateQuote(transport.QuoteCalculationRequest{Items: buildDraftCalcItems(params.Items), PricingMode: "exclusive"})
	now := time.Now()
	validUntil := s.resolveValidUntil(ctx, params.OrganizationID, params.LeadID, &params.LeadServiceID, now)
	createdBy := nilIfZeroUUID(params.CreatedByID)
	serviceID := &params.LeadServiceID

	quote := repository.Quote{
		ID:                  uuid.New(),
		OrganizationID:      params.OrganizationID,
		LeadID:              params.LeadID,
		LeadServiceID:       serviceID,
		CreatedByID:         createdBy,
		QuoteNumber:         quoteNumber,
		Status:              string(transport.QuoteStatusDraft),
		PricingMode:         "exclusive",
		DiscountType:        "percentage",
		DiscountValue:       0,
		SubtotalCents:       calc.SubtotalCents,
		DiscountAmountCents: calc.DiscountAmountCents,
		TaxTotalCents:       calc.VatTotalCents,
		TotalCents:          calc.TotalCents,
		ValidUntil:          validUntil,
		Notes:               nilIfEmpty(params.Notes),
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	items, catalogCount := buildDraftRepoItems(quote.ID, params.OrganizationID, params.Items, now)
	if err := s.repo.CreateWithItems(ctx, &quote, items); err != nil {
		return nil, fmt.Errorf("draft quote: %w", err)
	}
	if err := s.saveDraftAssets(ctx, quote.ID, params); err != nil {
		return nil, err
	}
	s.emitDraftTimelineEvent(ctx, quote, params.OrganizationID, len(items), catalogCount)
	return &DraftQuoteResult{QuoteID: quote.ID, QuoteNumber: quoteNumber, ItemCount: len(items)}, nil
}

func (s *Service) updateDraftQuote(ctx context.Context, params DraftQuoteParams) (*DraftQuoteResult, error) {
	quoteID := *params.QuoteID
	quote, err := s.repo.GetByID(ctx, quoteID, params.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("update draft quote: load existing: %w", err)
	}

	calc := CalculateQuote(transport.QuoteCalculationRequest{Items: buildDraftCalcItems(params.Items), PricingMode: quote.PricingMode})
	now := time.Now()
	quote.SubtotalCents = calc.SubtotalCents
	quote.DiscountAmountCents = calc.DiscountAmountCents
	quote.TaxTotalCents = calc.VatTotalCents
	quote.TotalCents = calc.TotalCents
	quote.Notes = nilIfEmpty(params.Notes)
	quote.UpdatedAt = now

	items, _ := buildDraftRepoItems(quoteID, params.OrganizationID, params.Items, now)
	if err := s.repo.UpdateWithItems(ctx, quote, items, true); err != nil {
		return nil, fmt.Errorf("update draft quote: %w", err)
	}
	if err := s.saveDraftAssets(ctx, quoteID, params); err != nil {
		return nil, err
	}
	return &DraftQuoteResult{QuoteID: quoteID, QuoteNumber: quote.QuoteNumber, ItemCount: len(items)}, nil
}

func buildDraftCalcItems(items []DraftQuoteItemParams) []transport.QuoteItemRequest {
	calcItems := make([]transport.QuoteItemRequest, len(items))
	for i, it := range items {
		calcItems[i] = transport.QuoteItemRequest{
			Description:      it.Description,
			Quantity:         it.Quantity,
			UnitPriceCents:   it.UnitPriceCents,
			TaxRateBps:       it.TaxRateBps,
			IsOptional:       it.IsOptional,
			IsSelected:       true,
			CatalogProductID: it.CatalogProductID,
		}
	}
	return calcItems
}

func buildDraftRepoItems(quoteID, orgID uuid.UUID, items []DraftQuoteItemParams, now time.Time) ([]repository.QuoteItem, int) {
	repoItems := make([]repository.QuoteItem, len(items))
	catalogCount := 0
	for i, it := range items {
		repoItems[i] = repository.QuoteItem{
			ID:               uuid.New(),
			QuoteID:          quoteID,
			OrganizationID:   orgID,
			Description:      it.Description,
			Quantity:         it.Quantity,
			QuantityNumeric:  parseQuantityNumber(it.Quantity),
			UnitPriceCents:   it.UnitPriceCents,
			TaxRateBps:       it.TaxRateBps,
			IsOptional:       it.IsOptional,
			IsSelected:       true,
			SortOrder:        i,
			CatalogProductID: it.CatalogProductID,
			CreatedAt:        now,
		}
		if it.CatalogProductID != nil {
			catalogCount++
		}
	}
	return repoItems, catalogCount
}

func (s *Service) resolveValidUntil(ctx context.Context, orgID uuid.UUID, leadID uuid.UUID, leadServiceID *uuid.UUID, now time.Time) *time.Time {
	_, validDays := s.resolveEffectiveQuoteTerms(ctx, orgID, leadID, leadServiceID)
	if validDays <= 0 {
		return nil
	}
	expiry := now.AddDate(0, 0, validDays)
	return &expiry
}

func (s *Service) resolveEffectiveQuoteTerms(ctx context.Context, organizationID uuid.UUID, leadID uuid.UUID, leadServiceID *uuid.UUID) (paymentDays int, validDays int) {
	if s.quoteTerms == nil {
		return defaultPaymentTermDays, defaultQuoteValidityDays
	}
	paymentDays, validDays, err := s.quoteTerms.ResolveQuoteTerms(ctx, organizationID, leadID, leadServiceID)
	if err != nil {
		return defaultPaymentTermDays, defaultQuoteValidityDays
	}
	if paymentDays <= 0 {
		paymentDays = defaultPaymentTermDays
	}
	if validDays <= 0 {
		validDays = defaultQuoteValidityDays
	}
	return paymentDays, validDays
}

func nilIfZeroUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func (s *Service) saveDraftAssets(ctx context.Context, quoteID uuid.UUID, params DraftQuoteParams) error {
	if len(params.Attachments) > 0 {
		attReqs := make([]transport.QuoteAttachmentRequest, len(params.Attachments))
		for i, att := range params.Attachments {
			attReqs[i] = transport.QuoteAttachmentRequest{Filename: att.Filename, FileKey: att.FileKey, Source: att.Source, CatalogProductID: att.CatalogProductID, Enabled: true, SortOrder: i}
		}
		if err := s.saveAttachments(ctx, quoteID, params.OrganizationID, attReqs); err != nil {
			return fmt.Errorf("draft quote save attachments: %w", err)
		}
	}
	if len(params.URLs) > 0 {
		urlReqs := make([]transport.QuoteURLRequest, len(params.URLs))
		for i, u := range params.URLs {
			urlReqs[i] = transport.QuoteURLRequest{Label: u.Label, Href: u.Href, CatalogProductID: u.CatalogProductID}
		}
		if err := s.saveURLs(ctx, quoteID, params.OrganizationID, urlReqs); err != nil {
			return fmt.Errorf("draft quote save urls: %w", err)
		}
	}
	return nil
}

func (s *Service) emitDraftTimelineEvent(ctx context.Context, quote repository.Quote, orgID uuid.UUID, itemCount, catalogCount int) {
	adHocCount := itemCount - catalogCount
	s.emitTimelineEvent(ctx, TimelineEventParams{
		LeadID:         quote.LeadID,
		ServiceID:      quote.LeadServiceID,
		OrganizationID: orgID,
		ActorType:      "AI",
		ActorName:      "Estimation Agent",
		EventType:      "quote_drafted",
		Title:          fmt.Sprintf("Draft quote %s created", quote.QuoteNumber),
		Summary:        toPtr(fmt.Sprintf("Total: €%.2f (%d items, %d from catalog, %d estimated)", float64(quote.TotalCents)/100, itemCount, catalogCount, adHocCount)),
		Metadata:       map[string]any{"quoteId": quote.ID, "quoteNumber": quote.QuoteNumber, "itemCount": itemCount, "catalogItems": catalogCount, "adHocItems": adHocCount, "status": quote.Status},
	})
}
