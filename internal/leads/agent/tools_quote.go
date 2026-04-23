package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"
	apptools "portal_final_backend/internal/tools"
)

// handleDraftQuote creates a draft quote via the QuoteDrafter port.
func handleDraftQuote(ctx tool.Context, deps *ToolDependencies, input DraftQuoteInput) (DraftQuoteOutput, error) {
	if deps.QuoteDrafter == nil {
		return DraftQuoteOutput{Success: false, Message: "Quote drafting is not configured"}, nil
	}

	tenantID, ok := deps.GetTenantID()
	if !ok || tenantID == nil {
		return DraftQuoteOutput{Success: false, Message: "Organization context not available"}, errors.New(missingTenantContextError)
	}

	leadID, serviceID, ok := deps.GetLeadContext()
	if !ok {
		return DraftQuoteOutput{Success: false, Message: "Lead context not available"}, errors.New(missingLeadContextError)
	}

	if !deps.ShouldForceDraftQuote() {
		if blocked, reason := shouldBlockDraftQuoteForInsufficientIntake(ctx, deps, serviceID, *tenantID); blocked {
			log.Printf("DraftQuote: blocked run=%s service=%s reason=%s", deps.GetRunID(), serviceID, reason)
			return DraftQuoteOutput{Success: false, Message: "Onvoldoende intakegegevens voor een betrouwbare conceptofferte"}, fmt.Errorf("draft quote blocked: %s", reason)
		}
	} else {
		log.Printf("DraftQuote: intake guard bypass enabled run=%s service=%s", deps.GetRunID(), serviceID)
	}

	if len(input.Items) == 0 {
		return DraftQuoteOutput{Success: false, Message: "At least one item is required"}, fmt.Errorf("empty items")
	}

	normalizedInput, quantityCorrections := normalizeDraftQuoteInput(input)
	for _, correction := range quantityCorrections {
		log.Printf("DraftQuote: defaulted missing quantity to 1 run=%s service=%s itemIndex=%d description=%q", deps.GetRunID(), serviceID, correction.Index, correction.Description)
	}
	if invalidQuantity, invalid := findInvalidDraftQuoteQuantity(normalizedInput.Items); invalid {
		log.Printf("DraftQuote: rejected vague quantity run=%s service=%s itemIndex=%d quantity=%q description=%q", deps.GetRunID(), serviceID, invalidQuantity.Index, invalidQuantity.Quantity, invalidQuantity.Description)
		return DraftQuoteOutput{Success: false, Message: "Conceptofferte vereist concrete hoeveelheden per regel"}, fmt.Errorf("draft quote invalid quantity at item %d: %q", invalidQuantity.Index, invalidQuantity.Quantity)
	}

	deps.SetLastDraftInput(normalizedInput)

	if blockedOutput, blockedErr := validateDraftQuoteGovernance(ctx, deps, leadID, serviceID, *tenantID, len(normalizedInput.Items)); blockedErr != nil {
		return blockedOutput, blockedErr
	}

	portItems := convertDraftItems(normalizedInput.Items)
	portItems, err := enforceCatalogUnitPrices(ctx, deps, *tenantID, portItems)
	if err != nil {
		return DraftQuoteOutput{Success: false, Message: err.Error()}, err
	}
	portAttachments, portURLs := collectCatalogAssetsForDraft(ctx, deps, tenantID, portItems)
	pricingSnapshot, pricingSnapshotErr := buildDraftPricingSnapshot(ctx, deps, *tenantID, leadID, serviceID)
	if pricingSnapshotErr != nil {
		log.Printf("DraftQuote: pricing snapshot context unavailable: %v", pricingSnapshotErr)
	}

	result, err := deps.QuoteDrafter.DraftQuote(ctx, ports.DraftQuoteParams{
		QuoteID:         deps.GetExistingQuoteID(),
		LeadID:          leadID,
		LeadServiceID:   serviceID,
		OrganizationID:  *tenantID,
		CreatedByID:     uuid.Nil,
		Notes:           normalizedInput.Notes,
		Items:           portItems,
		Attachments:     portAttachments,
		URLs:            portURLs,
		PricingSnapshot: pricingSnapshot,
	})
	if err != nil {
		log.Printf("DraftQuote: failed: %v", err)
		return DraftQuoteOutput{Success: false, Message: fmt.Sprintf("Failed to draft quote: %v", err)}, err
	}

	log.Printf("DraftQuote: created run=%s quote=%s items=%d lead=%s service=%s", deps.GetRunID(), result.QuoteNumber, result.ItemCount, leadID, serviceID)
	deps.SetLastDraftResult(result)
	deps.SetExistingQuoteID(&result.QuoteID)
	deps.MarkDraftQuoteCalled()

	return DraftQuoteOutput{
		Success:     true,
		Message:     fmt.Sprintf("Draft quote %s created with %d items", result.QuoteNumber, result.ItemCount),
		QuoteID:     result.QuoteID.String(),
		QuoteNumber: result.QuoteNumber,
		ItemCount:   result.ItemCount,
	}, nil
}

func buildDraftPricingSnapshot(ctx context.Context, deps *ToolDependencies, tenantID, leadID, serviceID uuid.UUID) (*ports.QuotePricingSnapshot, error) {
	lead, err := deps.Repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("load lead for pricing snapshot: %w", err)
	}

	service, err := deps.Repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("load service for pricing snapshot: %w", err)
	}

	var materialSubtotalCents *int64
	var laborSubtotalLowCents *int64
	var laborSubtotalHighCents *int64
	var extraCostsCents *int64
	if snapshot, ok := deps.GetLastEstimateSnapshot(); ok {
		materialSubtotalCents = &snapshot.MaterialSubtotalCents
		laborSubtotalLowCents = &snapshot.LaborSubtotalLowCents
		laborSubtotalHighCents = &snapshot.LaborSubtotalHighCents
		extraCostsCents = &snapshot.ExtraCostsCents
	}

	runID := deps.GetRunID()
	postcodeRaw := strings.TrimSpace(lead.AddressZipCode)

	return &ports.QuotePricingSnapshot{
		ServiceType:            service.ServiceType,
		PostcodeRaw:            postcodeRaw,
		PostcodePrefixZIP4:     derivePostcodePrefixZIP4(postcodeRaw),
		SourceType:             "ai_draft",
		MaterialSubtotalCents:  materialSubtotalCents,
		LaborSubtotalLowCents:  laborSubtotalLowCents,
		LaborSubtotalHighCents: laborSubtotalHighCents,
		ExtraCostsCents:        extraCostsCents,
		EstimatorRunID:         nilIfEmptyString(runID),
		CreatedByActor:         repository.ActorNameEstimator,
	}, nil
}

func derivePostcodePrefixZIP4(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	digits := make([]rune, 0, 4)
	for _, r := range trimmed {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
			if len(digits) == 4 {
				return string(digits)
			}
		}
	}
	return ""
}

func nilIfEmptyString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func createSubmitQuoteCritiqueTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewSubmitQuoteCritiqueTool(func(ctx tool.Context, input SubmitQuoteCritiqueInput) (SubmitQuoteCritiqueOutput, error) {
		deps, err := GetDependencies(ctx)
		if err != nil {
			return SubmitQuoteCritiqueOutput{}, err
		}
		return handleSubmitQuoteCritique(ctx, deps, input)
	})
}

func handleSubmitQuoteCritique(ctx tool.Context, deps *ToolDependencies, input SubmitQuoteCritiqueInput) (SubmitQuoteCritiqueOutput, error) {
	if deps.QuoteDrafter == nil {
		return SubmitQuoteCritiqueOutput{Success: false, Message: "Quote drafting is not configured"}, nil
	}

	// Guard: prevent duplicate SubmitQuoteCritique calls within the same critic attempt.
	deps.mu.Lock()
	currentAttempt := deps.quoteCriticAttempt
	alreadySubmitted := deps.quoteCritiqueSubmittedForAt == currentAttempt && currentAttempt > 0
	if !alreadySubmitted {
		deps.quoteCritiqueSubmittedForAt = currentAttempt
	}
	deps.mu.Unlock()
	if alreadySubmitted {
		log.Printf("SubmitQuoteCritique: duplicate call blocked for attempt=%d", currentAttempt)
		if last := deps.GetLastQuoteReviewResult(); last != nil {
			return SubmitQuoteCritiqueOutput{
				Success:  true,
				Message:  "Already submitted for this attempt",
				Decision: last.Decision,
				ReviewID: last.ReviewID.String(),
			}, nil
		}
		return SubmitQuoteCritiqueOutput{Success: true, Message: "Already submitted for this attempt"}, nil
	}

	tenantID, ok := deps.GetTenantID()
	if !ok || tenantID == nil {
		return SubmitQuoteCritiqueOutput{Success: false, Message: "Organization context not available"}, errors.New(missingTenantContextError)
	}

	draftResult := deps.GetLastDraftResult()
	if draftResult == nil {
		return SubmitQuoteCritiqueOutput{Success: false, Message: "No draft quote available for review"}, fmt.Errorf("missing draft quote context")
	}

	decision := "needs_repair"
	if input.Approved {
		decision = "approved"
	}

	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		if input.Approved {
			summary = "AI-review akkoord: conceptofferte is klaar voor menselijke controle."
		} else {
			summary = "AI-review afgekeurd: conceptofferte heeft nog herstel nodig."
		}
	}

	findings := make([]ports.QuoteAIReviewFinding, 0, len(input.Findings))
	for _, finding := range input.Findings {
		message := strings.TrimSpace(finding.Message)
		if message == "" {
			continue
		}
		findings = append(findings, ports.QuoteAIReviewFinding{
			Code:      strings.TrimSpace(finding.Code),
			Message:   message,
			Severity:  strings.TrimSpace(finding.Severity),
			ItemIndex: finding.ItemIndex,
		})
	}
	deps.SetLastQuoteCritiqueInput(input)

	runID := deps.GetRunID()
	reviewerName := "QuoteCritic"
	modelName := "moonshot"
	attemptCount := deps.GetQuoteCriticAttempt()
	if attemptCount <= 0 {
		attemptCount = 1
	}
	reviewResult, err := deps.QuoteDrafter.RecordQuoteAIReview(ctx, ports.RecordQuoteAIReviewParams{
		QuoteID:        draftResult.QuoteID,
		OrganizationID: *tenantID,
		Decision:       decision,
		Summary:        summary,
		Findings:       findings,
		Signals:        normalizeMissingInformation(input.Signals),
		AttemptCount:   attemptCount,
		RunID:          &runID,
		ReviewerName:   &reviewerName,
		ModelName:      &modelName,
	})
	if err != nil {
		return SubmitQuoteCritiqueOutput{Success: false, Message: err.Error()}, err
	}

	deps.SetLastQuoteReviewResult(reviewResult)
	return SubmitQuoteCritiqueOutput{
		Success:  true,
		Message:  summary,
		Decision: reviewResult.Decision,
		ReviewID: reviewResult.ReviewID.String(),
	}, nil
}

func validateDraftQuoteGovernance(ctx tool.Context, deps *ToolDependencies, leadID, serviceID, tenantID uuid.UUID, itemCount int) (DraftQuoteOutput, error) {
	if deps.ShouldForceDraftQuote() {
		log.Printf("DraftQuote: manual governance bypass enabled run=%s service=%s", deps.GetRunID(), serviceID)
		return DraftQuoteOutput{}, nil
	}

	if blocked, reason := shouldBlockDraftQuoteForInsufficientIntake(ctx, deps, serviceID, tenantID); blocked {
		log.Printf("DraftQuote: blocked run=%s service=%s reason=%s", deps.GetRunID(), serviceID, reason)
		return DraftQuoteOutput{Success: false, Message: "Onvoldoende intakegegevens voor een betrouwbare conceptofferte"}, fmt.Errorf("draft quote blocked: %s", reason)
	}

	councilEval, councilErr := evaluateCouncilForDraftQuote(ctx, deps, leadID, serviceID, tenantID, itemCount)
	if councilErr != nil {
		return DraftQuoteOutput{Success: false, Message: "Council evaluatie mislukt"}, councilErr
	}
	if councilEval.Decision == CouncilDecisionAllow {
		return DraftQuoteOutput{}, nil
	}

	summary := strings.TrimSpace(councilEval.Summary)
	if summary == "" {
		summary = "Council blokkeert conceptofferte: handmatige beoordeling vereist."
	}
	if deps.MarkAlertEmitted("council_draft_quote", councilEval.ReasonCode, summary) {
		_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
			LeadID:         leadID,
			ServiceID:      &serviceID,
			OrganizationID: tenantID,
			ActorType:      repository.ActorTypeSystem,
			ActorName:      "Council",
			EventType:      repository.EventTypeAlert,
			Title:          repository.EventTitleManualIntervention,
			Summary:        &summary,
			Metadata: repository.CouncilAdviceMetadata{
				Decision:         councilEval.Decision,
				ReasonCode:       councilEval.ReasonCode,
				Summary:          councilEval.Summary,
				EstimatorSignals: councilEval.EstimatorSignals,
				RiskSignals:      councilEval.RiskSignals,
				ReadinessSignals: councilEval.ReadinessSignals,
			}.ToMap(),
		})
	}

	return DraftQuoteOutput{Success: false, Message: summary}, fmt.Errorf("council blocked draft quote: %s", councilEval.ReasonCode)
}

func evaluateCouncilForDraftQuote(ctx tool.Context, deps *ToolDependencies, leadID, serviceID, tenantID uuid.UUID, itemCount int) (CouncilEvaluation, error) {
	settings := deps.GetOrganizationAISettingsOrDefault()
	if !settings.AICouncilMode || deps.CouncilService == nil {
		deps.SetCouncilMetadata(nil)
		return CouncilEvaluation{Decision: CouncilDecisionAllow, ReasonCode: "council_disabled", Summary: "Council uitgeschakeld."}, nil
	}

	evaluation, err := deps.CouncilService.Evaluate(ctx, CouncilEvaluationInput{
		Action:    CouncilActionDraftQuote,
		LeadID:    leadID,
		ServiceID: serviceID,
		TenantID:  tenantID,
		Mode:      settings.AICouncilConsensusMode,
		ItemCount: itemCount,
	})
	if err != nil {
		return CouncilEvaluation{}, err
	}

	deps.SetCouncilMetadata(repository.CouncilAdviceMetadata{
		Decision:         evaluation.Decision,
		ReasonCode:       evaluation.ReasonCode,
		Summary:          evaluation.Summary,
		EstimatorSignals: evaluation.EstimatorSignals,
		RiskSignals:      evaluation.RiskSignals,
		ReadinessSignals: evaluation.ReadinessSignals,
	}.ToMap())

	return evaluation, nil
}

func shouldBlockDraftQuoteForInsufficientIntake(ctx context.Context, deps *ToolDependencies, serviceID, tenantID uuid.UUID) (bool, string) {
	analysis, err := deps.Repo.GetLatestAIAnalysis(ctx, serviceID, tenantID)
	if err != nil {
		return true, "latest analysis unavailable"
	}
	if reason := domain.ValidateAnalysisStageTransition(analysis.RecommendedAction, analysis.MissingInformation, domain.PipelineStageEstimation); reason != "" {
		return true, reason
	}
	return false, ""
}

// enforceCatalogUnitPrices ensures catalog-linked quote items use authoritative
// catalog pricing metadata (unit price + VAT). Ad-hoc items (without
// catalogProductId) are left unchanged so they can be estimated.
func enforceCatalogUnitPrices(ctx context.Context, deps *ToolDependencies, tenantID uuid.UUID, items []ports.DraftQuoteItem) ([]ports.DraftQuoteItem, error) {
	if deps.CatalogReader == nil || len(items) == 0 {
		return items, nil
	}

	catalogIDs := collectCatalogProductIDs(items)
	if len(catalogIDs) == 0 {
		return items, nil
	}

	details, err := deps.CatalogReader.GetProductDetails(ctx, tenantID, catalogIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to validate catalog-linked quote items: %w", err)
	}

	detailByID := mapCatalogDetailsByID(details)

	priceAdjusted, vatAdjusted, unresolvedCatalogIDs := normalizeCatalogLinkedItems(items, detailByID)
	if unresolvedCatalogIDs > 0 {
		return nil, fmt.Errorf("failed to resolve %d catalog-linked quote item(s)", unresolvedCatalogIDs)
	}

	logCatalogNormalizationSummary(priceAdjusted, vatAdjusted, unresolvedCatalogIDs)
	return items, nil
}

func mapCatalogDetailsByID(details []ports.CatalogProductDetails) map[uuid.UUID]ports.CatalogProductDetails {
	detailByID := make(map[uuid.UUID]ports.CatalogProductDetails, len(details))
	for _, d := range details {
		detailByID[d.ID] = d
	}
	return detailByID
}

func normalizeCatalogLinkedItems(items []ports.DraftQuoteItem, detailByID map[uuid.UUID]ports.CatalogProductDetails) (priceAdjusted int, vatAdjusted int, unresolvedCatalogIDs int) {
	for i := range items {
		if items[i].CatalogProductID == nil {
			continue
		}

		priceChanged, vatChanged, resolved := applyCatalogDetailToDraftItem(&items[i], detailByID)
		if !resolved {
			unresolvedCatalogIDs++
			continue
		}
		if priceChanged {
			priceAdjusted++
		}
		if vatChanged {
			vatAdjusted++
		}
	}
	return priceAdjusted, vatAdjusted, unresolvedCatalogIDs
}

func applyCatalogDetailToDraftItem(item *ports.DraftQuoteItem, detailByID map[uuid.UUID]ports.CatalogProductDetails) (priceChanged bool, vatChanged bool, resolved bool) {
	d, ok := detailByID[*item.CatalogProductID]
	if !ok {
		return false, false, false
	}

	if item.UnitPriceCents != d.UnitPriceCents {
		item.UnitPriceCents = d.UnitPriceCents
		priceChanged = true
	}
	if d.VatRateBps > 0 && item.TaxRateBps != d.VatRateBps {
		item.TaxRateBps = d.VatRateBps
		vatChanged = true
	}

	return priceChanged, vatChanged, true
}

func logCatalogNormalizationSummary(priceAdjusted int, vatAdjusted int, unresolvedCatalogIDs int) {
	if priceAdjusted > 0 || vatAdjusted > 0 {
		log.Printf("DraftQuote: normalized catalog-linked metadata (prices=%d vat=%d)", priceAdjusted, vatAdjusted)
	}
	if unresolvedCatalogIDs > 0 {
		log.Printf("DraftQuote: %d catalog-linked item(s) could not be resolved; kept input values to avoid breaking flow", unresolvedCatalogIDs)
	}
}

type draftQuoteQuantityCorrection struct {
	Index       int
	Description string
}

type invalidDraftQuoteQuantity struct {
	Index       int
	Description string
	Quantity    string
}

func normalizeDraftQuoteInput(input DraftQuoteInput) (DraftQuoteInput, []draftQuoteQuantityCorrection) {
	normalized := input
	normalized.Notes = strings.TrimSpace(input.Notes)
	normalized.Items = make([]DraftQuoteItem, len(input.Items))
	corrections := make([]draftQuoteQuantityCorrection, 0)
	for i, item := range input.Items {
		normalizedItem, corrected := normalizeDraftQuoteItem(item)
		normalized.Items[i] = normalizedItem
		if corrected {
			corrections = append(corrections, draftQuoteQuantityCorrection{
				Index:       i,
				Description: normalizedItem.Description,
			})
		}
	}
	return normalized, corrections
}

func normalizeDraftQuoteItem(item DraftQuoteItem) (DraftQuoteItem, bool) {
	item.Description = strings.TrimSpace(item.Description)
	item.Quantity = strings.TrimSpace(item.Quantity)
	if item.Quantity != "" {
		return item, false
	}
	item.Quantity = "1"
	return item, true
}

func findInvalidDraftQuoteQuantity(items []DraftQuoteItem) (invalidDraftQuoteQuantity, bool) {
	for i, item := range items {
		if isVagueDraftQuoteQuantity(item.Quantity) {
			return invalidDraftQuoteQuantity{
				Index:       i,
				Description: item.Description,
				Quantity:    item.Quantity,
			}, true
		}
	}
	return invalidDraftQuoteQuantity{}, false
}

func isVagueDraftQuoteQuantity(quantity string) bool {
	normalized := strings.ToLower(strings.TrimSpace(quantity))
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "?") {
		return true
	}
	replacer := strings.NewReplacer(".", "", "-", " ", "_", " ", "/", " ")
	normalized = strings.Join(strings.Fields(replacer.Replace(normalized)), " ")
	switch normalized {
	case "nader te bepalen", "nog te bepalen", "onbekend", "unknown", "tbd", "ntb", "nvt":
		return true
	default:
		return false
	}
}

// convertDraftItems converts tool-level DraftQuoteItems to port-level items.
func convertDraftItems(items []DraftQuoteItem) []ports.DraftQuoteItem {
	portItems := make([]ports.DraftQuoteItem, len(items))
	for i, it := range items {
		portItems[i] = ports.DraftQuoteItem{
			Description:    it.Description,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
			TaxRateBps:     it.TaxRateBps,
			IsOptional:     it.IsOptional,
		}
		if it.CatalogProductID != nil && *it.CatalogProductID != "" {
			uid, err := uuid.Parse(*it.CatalogProductID)
			if err == nil {
				portItems[i].CatalogProductID = &uid
			}
		}
	}
	return portItems
}

// collectCatalogAssetsForDraft auto-collects catalog product attachments and URLs.
func collectCatalogAssetsForDraft(ctx context.Context, deps *ToolDependencies, tenantID *uuid.UUID, items []ports.DraftQuoteItem) ([]ports.DraftQuoteAttachment, []ports.DraftQuoteURL) {
	if deps.CatalogReader == nil {
		return nil, nil
	}
	catalogIDs := collectCatalogProductIDs(items)
	if len(catalogIDs) == 0 {
		return nil, nil
	}
	details, err := deps.CatalogReader.GetProductDetails(ctx, *tenantID, catalogIDs)
	if err != nil {
		log.Printf("DraftQuote: catalog details fetch failed (non-fatal): %v", err)
		return nil, nil
	}
	return collectCatalogAssets(details)
}

func createDraftQuoteTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewDraftQuoteTool(func(ctx tool.Context, input DraftQuoteInput) (DraftQuoteOutput, error) {
		deps, err := GetDependencies(ctx)
		if err != nil {
			return DraftQuoteOutput{}, err
		}
		return handleDraftQuote(ctx, deps, input)
	})
}

// collectCatalogProductIDs extracts unique, non-nil catalog product UUIDs from draft items.
func collectCatalogProductIDs(items []ports.DraftQuoteItem) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(items))
	ids := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		if it.CatalogProductID == nil {
			continue
		}
		if _, dup := seen[*it.CatalogProductID]; dup {
			continue
		}
		seen[*it.CatalogProductID] = struct{}{}
		ids = append(ids, *it.CatalogProductID)
	}
	return ids
}

// collectCatalogAssets de-duplicates document attachments and URLs across all
// catalog product details and returns them as port-level types.
func collectCatalogAssets(details []ports.CatalogProductDetails) ([]ports.DraftQuoteAttachment, []ports.DraftQuoteURL) {
	seenFileKeys := make(map[string]struct{})
	seenHrefs := make(map[string]struct{})

	var attachments []ports.DraftQuoteAttachment
	var urls []ports.DraftQuoteURL

	for _, d := range details {
		pid := d.ID
		for _, doc := range d.Documents {
			if _, dup := seenFileKeys[doc.FileKey]; dup {
				continue
			}
			seenFileKeys[doc.FileKey] = struct{}{}
			attachments = append(attachments, ports.DraftQuoteAttachment{
				Filename:         doc.Filename,
				FileKey:          doc.FileKey,
				Source:           "catalog",
				CatalogProductID: &pid,
			})
		}
		for _, u := range d.URLs {
			if _, dup := seenHrefs[u.Href]; dup {
				continue
			}
			seenHrefs[u.Href] = struct{}{}
			urls = append(urls, ports.DraftQuoteURL{
				Label:            u.Label,
				Href:             u.Href,
				CatalogProductID: &pid,
			})
		}
	}

	return attachments, urls
}
