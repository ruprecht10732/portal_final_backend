package service

import (
	"context"
	"strings"

	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/transport"

	"github.com/google/uuid"
)

func (s *Service) GetVersionHistory(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*transport.QuoteVersionHistoryResponse, error) {
	quote, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	rootID := resolveQuoteVersionRootID(quote)
	versionIDs, err := s.repo.ListVersionChainQuoteIDs(ctx, tenantID, rootID)
	if err != nil {
		return nil, err
	}

	versions := make([]transport.QuoteVersionSummaryResponse, 0, len(versionIDs))
	for _, versionID := range versionIDs {
		version, getErr := s.repo.GetByID(ctx, versionID, tenantID)
		if getErr != nil {
			return nil, getErr
		}
		versions = append(versions, transport.QuoteVersionSummaryResponse{
			QuoteID:       version.ID,
			QuoteNumber:   version.QuoteNumber,
			VersionNumber: version.VersionNumber,
			Status:        transport.QuoteStatus(version.Status),
			TotalCents:    version.TotalCents,
			CreatedAt:     version.CreatedAt,
			UpdatedAt:     version.UpdatedAt,
			IsCurrent:     version.ID == quote.ID,
		})
	}

	response := &transport.QuoteVersionHistoryResponse{Versions: versions}
	if quote.PreviousVersionQuoteID == nil || *quote.PreviousVersionQuoteID == uuid.Nil {
		return response, nil
	}

	previousQuote, err := s.repo.GetByID(ctx, *quote.PreviousVersionQuoteID, tenantID)
	if err != nil {
		return nil, err
	}
	currentItems, err := s.repo.GetItemsByQuoteID(ctx, quote.ID, tenantID)
	if err != nil {
		return nil, err
	}
	previousItems, err := s.repo.GetItemsByQuoteID(ctx, previousQuote.ID, tenantID)
	if err != nil {
		return nil, err
	}

	response.Diff = buildQuoteVersionDiff(previousQuote, previousItems, quote, currentItems)
	return response, nil
}

func buildQuoteVersionDiff(previousQuote *repository.Quote, previousItems []repository.QuoteItem, currentQuote *repository.Quote, currentItems []repository.QuoteItem) *transport.QuoteVersionDiffResponse {
	matches := mapQuoteVersionDiffItems(previousItems, currentItems)
	currentByID := make(map[uuid.UUID]repository.QuoteItem, len(currentItems))
	matchedCurrent := make(map[uuid.UUID]struct{}, len(matches))
	items := make([]transport.QuoteVersionDiffItemResponse, 0)
	addedCount := 0
	removedCount := 0
	changedCount := 0

	for _, item := range currentItems {
		currentByID[item.ID] = item
	}

	for _, previousItem := range previousItems {
		currentID, ok := matches[previousItem.ID]
		if !ok {
			removedCount++
			items = append(items, transport.QuoteVersionDiffItemResponse{
				ChangeType: "removed",
				Previous:   buildQuoteVersionItemSnapshot(previousItem, previousQuote.PricingMode),
			})
			continue
		}

		currentItem := currentByID[currentID]
		matchedCurrent[currentID] = struct{}{}
		if quoteVersionItemsEqual(previousItem, currentItem, previousQuote.PricingMode, currentQuote.PricingMode) {
			continue
		}

		changedCount++
		items = append(items, transport.QuoteVersionDiffItemResponse{
			ChangeType: "changed",
			Previous:   buildQuoteVersionItemSnapshot(previousItem, previousQuote.PricingMode),
			Current:    buildQuoteVersionItemSnapshot(currentItem, currentQuote.PricingMode),
		})
	}

	for _, currentItem := range currentItems {
		if _, ok := matchedCurrent[currentItem.ID]; ok {
			continue
		}
		addedCount++
		items = append(items, transport.QuoteVersionDiffItemResponse{
			ChangeType: "added",
			Current:    buildQuoteVersionItemSnapshot(currentItem, currentQuote.PricingMode),
		})
	}

	return &transport.QuoteVersionDiffResponse{
		PreviousQuoteID:       previousQuote.ID,
		PreviousQuoteNumber:   previousQuote.QuoteNumber,
		PreviousVersionNumber: previousQuote.VersionNumber,
		CurrentQuoteID:        currentQuote.ID,
		CurrentQuoteNumber:    currentQuote.QuoteNumber,
		CurrentVersionNumber:  currentQuote.VersionNumber,
		AddedCount:            addedCount,
		RemovedCount:          removedCount,
		ChangedCount:          changedCount,
		TotalDeltaCents:       currentQuote.TotalCents - previousQuote.TotalCents,
		Items:                 items,
	}
}

func buildQuoteVersionItemSnapshot(item repository.QuoteItem, pricingMode string) *transport.QuoteVersionItemResponse {
	qty := parseQuantityNumber(item.Quantity)
	lineSubtotal := qty * computeLineNetPrice(item.UnitPriceCents, item.TaxRateBps, pricingMode)
	lineVat := lineSubtotal * (float64(item.TaxRateBps) / 10000.0)
	return &transport.QuoteVersionItemResponse{
		Title:          item.Title,
		Description:    item.Description,
		Quantity:       item.Quantity,
		UnitPriceCents: item.UnitPriceCents,
		TaxRateBps:     item.TaxRateBps,
		IsOptional:     item.IsOptional,
		IsSelected:     item.IsSelected,
		LineTotalCents: roundCents(lineSubtotal + lineVat),
	}
}

func quoteVersionItemsEqual(previousItem, currentItem repository.QuoteItem, previousPricingMode, currentPricingMode string) bool {
	previousSnapshot := buildQuoteVersionItemSnapshot(previousItem, previousPricingMode)
	currentSnapshot := buildQuoteVersionItemSnapshot(currentItem, currentPricingMode)
	return previousSnapshot.Title == currentSnapshot.Title &&
		previousSnapshot.Description == currentSnapshot.Description &&
		previousSnapshot.Quantity == currentSnapshot.Quantity &&
		previousSnapshot.UnitPriceCents == currentSnapshot.UnitPriceCents &&
		previousSnapshot.TaxRateBps == currentSnapshot.TaxRateBps &&
		previousSnapshot.IsOptional == currentSnapshot.IsOptional &&
		previousSnapshot.IsSelected == currentSnapshot.IsSelected &&
		previousSnapshot.LineTotalCents == currentSnapshot.LineTotalCents
}

func mapQuoteVersionDiffItems(previousItems, currentItems []repository.QuoteItem) map[uuid.UUID]uuid.UUID {
	mapping := make(map[uuid.UUID]uuid.UUID)
	usedTargets := make(map[uuid.UUID]struct{})
	matchers := []func(repository.QuoteItem, repository.QuoteItem) bool{
		quoteVersionItemsExactlyMatch,
		quoteVersionItemsMatchCatalogIdentity,
		quoteVersionItemsMatchTextIdentity,
		quoteVersionItemsMatchSortIdentity,
	}

	for _, matcher := range matchers {
		for _, previousItem := range previousItems {
			if _, alreadyMapped := mapping[previousItem.ID]; alreadyMapped {
				continue
			}
			targetID, ok := findQuoteVersionDiffTarget(previousItem, currentItems, usedTargets, matcher)
			if ok {
				mapping[previousItem.ID] = targetID
				usedTargets[targetID] = struct{}{}
			}
		}
	}

	return mapping
}

func findQuoteVersionDiffTarget(previousItem repository.QuoteItem, currentItems []repository.QuoteItem, usedTargets map[uuid.UUID]struct{}, matcher func(repository.QuoteItem, repository.QuoteItem) bool) (uuid.UUID, bool) {
	for _, currentItem := range currentItems {
		if _, alreadyUsed := usedTargets[currentItem.ID]; alreadyUsed {
			continue
		}
		if matcher(previousItem, currentItem) {
			return currentItem.ID, true
		}
	}
	return uuid.Nil, false
}

func quoteVersionItemsExactlyMatch(left, right repository.QuoteItem) bool {
	return quoteVersionCatalogIDsMatch(left.CatalogProductID, right.CatalogProductID) &&
		normalizeQuoteVersionDiffText(left.Title) == normalizeQuoteVersionDiffText(right.Title) &&
		normalizeQuoteVersionDiffText(left.Description) == normalizeQuoteVersionDiffText(right.Description) &&
		normalizeQuoteVersionDiffText(left.Quantity) == normalizeQuoteVersionDiffText(right.Quantity) &&
		left.UnitPriceCents == right.UnitPriceCents &&
		left.TaxRateBps == right.TaxRateBps &&
		left.IsOptional == right.IsOptional &&
		left.IsSelected == right.IsSelected &&
		left.SortOrder == right.SortOrder
}

func quoteVersionItemsMatchCatalogIdentity(left, right repository.QuoteItem) bool {
	return left.CatalogProductID != nil && right.CatalogProductID != nil &&
		*left.CatalogProductID == *right.CatalogProductID && left.IsOptional == right.IsOptional
}

func quoteVersionItemsMatchTextIdentity(left, right repository.QuoteItem) bool {
	leftLabel := normalizeQuoteVersionDiffLabel(left)
	rightLabel := normalizeQuoteVersionDiffLabel(right)
	return leftLabel != "" && leftLabel == rightLabel && left.IsOptional == right.IsOptional
}

func quoteVersionItemsMatchSortIdentity(left, right repository.QuoteItem) bool {
	return left.SortOrder == right.SortOrder && left.IsOptional == right.IsOptional
}

func normalizeQuoteVersionDiffLabel(item repository.QuoteItem) string {
	if title := normalizeQuoteVersionDiffText(item.Title); title != "" {
		return title
	}
	return normalizeQuoteVersionDiffText(item.Description)
}

func normalizeQuoteVersionDiffText(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}
	return strings.Join(strings.Fields(trimmed), " ")
}

func quoteVersionCatalogIDsMatch(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
