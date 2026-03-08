package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	quotesdb "portal_final_backend/internal/quotes/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	pricingSourceManualUpdate = "manual_update"
	pricingSourceAIDraft      = "ai_draft"
	pricingItemIndexFmt       = "items[%d]"
)

type quotePricingCorrection struct {
	QuoteItemID     *uuid.UUID
	FieldName       string
	AIValue         map[string]any
	HumanValue      map[string]any
	DeltaCents      *int64
	DeltaPercentage *float64
	Reason          *string
	AIFindingCode   *string
}

func (r *Repository) insertPricingCorrections(ctx context.Context, qtx *quotesdb.Queries, quote *Quote, items []QuoteItem, previousSnapshot *quotesdb.RacQuotePricingSnapshot, pricingSnapshot *QuotePricingSnapshot) error {
	if pricingSnapshot == nil || pricingSnapshot.SourceType != pricingSourceManualUpdate || previousSnapshot == nil {
		return nil
	}
	if previousSnapshot.SourceType != pricingSourceAIDraft {
		return nil
	}

	corrections, err := buildPricingCorrections(*previousSnapshot, quote, items)
	if err != nil {
		return fmt.Errorf("build pricing corrections: %w", err)
	}
	if len(corrections) == 0 {
		return nil
	}

	createdAt := quote.UpdatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	for _, correction := range corrections {
		_, err := qtx.CreateQuotePricingCorrection(ctx, quotesdb.CreateQuotePricingCorrectionParams{
			ID:              toPgUUID(uuid.New()),
			QuoteID:         toPgUUID(quote.ID),
			SnapshotID:      pgtype.UUID{Bytes: previousSnapshot.ID.Bytes, Valid: previousSnapshot.ID.Valid},
			OrganizationID:  toPgUUID(quote.OrganizationID),
			QuoteItemID:     toPgUUIDPtr(correction.QuoteItemID),
			FieldName:       correction.FieldName,
			AiValue:         marshalJSON(correction.AIValue),
			HumanValue:      marshalJSON(correction.HumanValue),
			DeltaCents:      toPgInt8Ptr(correction.DeltaCents),
			DeltaPercentage: toPgFloat8Ptr(correction.DeltaPercentage),
			Reason:          toPgTextPtr(correction.Reason),
			AiFindingCode:   toPgTextPtr(correction.AIFindingCode),
			CreatedByUserID: toPgUUIDPtr(pricingSnapshot.CreatedByUserID),
			CreatedAt:       toPgTimestamp(createdAt),
		})
		if err != nil {
			return fmt.Errorf("create quote pricing correction: %w", err)
		}
	}

	return nil
}

func buildPricingCorrections(previousSnapshot quotesdb.RacQuotePricingSnapshot, quote *Quote, items []QuoteItem) ([]quotePricingCorrection, error) {
	var previousItems []QuotePricingSnapshotItem
	if len(previousSnapshot.StructuredItems) > 0 {
		if err := json.Unmarshal(previousSnapshot.StructuredItems, &previousItems); err != nil {
			return nil, err
		}
	}

	corrections := make([]quotePricingCorrection, 0)
	corrections = append(corrections, buildInt64Correction("discount_value", previousSnapshot.DiscountValue, quote.DiscountValue, "discount_value_changed")...)
	corrections = append(corrections, buildInt64Correction("subtotal_cents", previousSnapshot.SubtotalCents, quote.SubtotalCents, "quote_total_adjusted")...)
	corrections = append(corrections, buildInt64Correction("discount_amount_cents", previousSnapshot.DiscountAmountCents, quote.DiscountAmountCents, "discount_amount_adjusted")...)
	corrections = append(corrections, buildInt64Correction("tax_total_cents", previousSnapshot.TaxTotalCents, quote.TaxTotalCents, "tax_total_adjusted")...)
	corrections = append(corrections, buildInt64Correction("total_cents", previousSnapshot.TotalCents, quote.TotalCents, "quote_total_adjusted")...)

	if int(previousSnapshot.ItemCount) != len(items) {
		corrections = append(corrections, quotePricingCorrection{
			FieldName:       "item_count",
			AIValue:         map[string]any{"value": previousSnapshot.ItemCount},
			HumanValue:      map[string]any{"value": len(items)},
			DeltaPercentage: deltaPercentage(float64(previousSnapshot.ItemCount), float64(len(items))),
			Reason:          stringPtr("manual item count changed"),
			AIFindingCode:   stringPtr("item_count_changed"),
		})
	}

	matchedPairs, addedItems, removedItems := matchPricingSnapshotItems(previousItems, items)
	for _, pair := range matchedPairs {
		prefix := fmt.Sprintf(pricingItemIndexFmt, pair.ComparisonIndex)
		quoteItemID := pair.CurrentItem.ID

		corrections = append(corrections, buildItemInt64Correction(&quoteItemID, prefix+".unit_price_cents", pair.PreviousItem.UnitPriceCents, pair.CurrentItem.UnitPriceCents, pair.CurrentItem.Description, "unit_price_adjusted")...)
		corrections = append(corrections, buildItemFloatCorrection(&quoteItemID, prefix+".quantity_numeric", pair.PreviousItem.QuantityNumeric, pair.CurrentItem.QuantityNumeric, pair.CurrentItem.Description, "quantity_adjusted")...)
		corrections = append(corrections, buildItemIntCorrection(&quoteItemID, prefix+".tax_rate_bps", pair.PreviousItem.TaxRateBps, pair.CurrentItem.TaxRateBps, pair.CurrentItem.Description, "tax_rate_adjusted")...)

		if pair.PreviousItem.IsOptional != pair.CurrentItem.IsOptional {
			corrections = append(corrections, quotePricingCorrection{
				QuoteItemID:   &quoteItemID,
				FieldName:     prefix + ".is_optional",
				AIValue:       itemContextValue(pair.CurrentItem.Description, pair.PreviousItem.IsOptional),
				HumanValue:    itemContextValue(pair.CurrentItem.Description, pair.CurrentItem.IsOptional),
				Reason:        stringPtr("manual item optionality changed"),
				AIFindingCode: stringPtr("item_optionality_changed"),
			})
		}
		if pair.PreviousItem.IsSelected != pair.CurrentItem.IsSelected {
			corrections = append(corrections, quotePricingCorrection{
				QuoteItemID:   &quoteItemID,
				FieldName:     prefix + ".is_selected",
				AIValue:       itemContextValue(pair.CurrentItem.Description, pair.PreviousItem.IsSelected),
				HumanValue:    itemContextValue(pair.CurrentItem.Description, pair.CurrentItem.IsSelected),
				Reason:        stringPtr("manual item selection changed"),
				AIFindingCode: stringPtr("item_selection_changed"),
			})
		}
	}

	for _, addedItem := range addedItems {
		itemID := addedItem.Item.ID
		corrections = append(corrections, quotePricingCorrection{
			QuoteItemID:   &itemID,
			FieldName:     fmt.Sprintf(pricingItemIndexFmt, addedItem.Index),
			AIValue:       map[string]any{"exists": false},
			HumanValue:    itemSnapshotValue(addedItem.Item),
			Reason:        stringPtr("manual item added"),
			AIFindingCode: stringPtr("item_added"),
		})
	}
	for _, removedItem := range removedItems {
		corrections = append(corrections, quotePricingCorrection{
			FieldName:     fmt.Sprintf(pricingItemIndexFmt, removedItem.Index),
			AIValue:       previousItemSnapshotValue(removedItem.Item),
			HumanValue:    map[string]any{"exists": false},
			Reason:        stringPtr("manual item removed"),
			AIFindingCode: stringPtr("item_removed"),
		})
	}

	return corrections, nil
}

type matchedPricingItemPair struct {
	ComparisonIndex int
	PreviousItem    QuotePricingSnapshotItem
	CurrentItem     QuoteItem
}

type addedPricingItem struct {
	Index int
	Item  QuoteItem
}

type removedPricingItem struct {
	Index int
	Item  QuotePricingSnapshotItem
}

func matchPricingSnapshotItems(previousItems []QuotePricingSnapshotItem, currentItems []QuoteItem) ([]matchedPricingItemPair, []addedPricingItem, []removedPricingItem) {
	orderedKeys := make([]string, 0, len(previousItems)+len(currentItems))
	seenKeys := make(map[string]struct{}, len(previousItems)+len(currentItems))
	previousDescriptionCounts, currentDescriptionCounts := buildPricingItemDescriptionCounts(previousItems, currentItems)

	previousGroups := make(map[string][]removedPricingItem, len(previousItems))
	for index, item := range previousItems {
		key := pricingSnapshotItemMatchKey(item, index, previousDescriptionCounts, currentDescriptionCounts)
		if _, seen := seenKeys[key]; !seen {
			orderedKeys = append(orderedKeys, key)
			seenKeys[key] = struct{}{}
		}
		previousGroups[key] = append(previousGroups[key], removedPricingItem{Index: index, Item: item})
	}

	currentGroups := make(map[string][]addedPricingItem, len(currentItems))
	for index, item := range currentItems {
		key := quoteItemMatchKey(item, index, previousDescriptionCounts, currentDescriptionCounts)
		if _, seen := seenKeys[key]; !seen {
			orderedKeys = append(orderedKeys, key)
			seenKeys[key] = struct{}{}
		}
		currentGroups[key] = append(currentGroups[key], addedPricingItem{Index: index, Item: item})
	}

	matchedPairs := make([]matchedPricingItemPair, 0, minInt(len(previousItems), len(currentItems)))
	addedItems := make([]addedPricingItem, 0)
	removedItems := make([]removedPricingItem, 0)

	for _, key := range orderedKeys {
		previousGroup := previousGroups[key]
		currentGroup := currentGroups[key]
		pairCount := minInt(len(previousGroup), len(currentGroup))
		for i := 0; i < pairCount; i++ {
			matchedPairs = append(matchedPairs, matchedPricingItemPair{
				ComparisonIndex: currentGroup[i].Index,
				PreviousItem:    previousGroup[i].Item,
				CurrentItem:     currentGroup[i].Item,
			})
		}
		addedItems = append(addedItems, currentGroup[pairCount:]...)
		removedItems = append(removedItems, previousGroup[pairCount:]...)
	}

	return matchedPairs, addedItems, removedItems
}

func buildPricingItemDescriptionCounts(previousItems []QuotePricingSnapshotItem, currentItems []QuoteItem) (map[string]int, map[string]int) {
	previousCounts := make(map[string]int)
	currentCounts := make(map[string]int)

	for _, item := range previousItems {
		if item.CatalogProductID != nil {
			continue
		}
		description := normalizePricingItemDescription(item.Description)
		if description != "" {
			previousCounts[description]++
		}
	}
	for _, item := range currentItems {
		if item.CatalogProductID != nil {
			continue
		}
		description := normalizePricingItemDescription(item.Description)
		if description != "" {
			currentCounts[description]++
		}
	}

	return previousCounts, currentCounts
}

func pricingSnapshotItemMatchKey(item QuotePricingSnapshotItem, fallbackIndex int, previousDescriptionCounts map[string]int, currentDescriptionCounts map[string]int) string {
	if item.CatalogProductID != nil {
		return "catalog:" + item.CatalogProductID.String()
	}
	description := normalizePricingItemDescription(item.Description)
	if description != "" && previousDescriptionCounts[description] == 1 && currentDescriptionCounts[description] == 1 {
		return "description:" + description
	}
	return pricingSnapshotItemFingerprint(item, fallbackIndex)
}

func quoteItemMatchKey(item QuoteItem, fallbackIndex int, previousDescriptionCounts map[string]int, currentDescriptionCounts map[string]int) string {
	if item.CatalogProductID != nil {
		return "catalog:" + item.CatalogProductID.String()
	}
	description := normalizePricingItemDescription(item.Description)
	if description != "" && previousDescriptionCounts[description] == 1 && currentDescriptionCounts[description] == 1 {
		return "description:" + description
	}
	return quoteItemFingerprint(item, fallbackIndex)
}

func pricingSnapshotItemFingerprint(item QuotePricingSnapshotItem, fallbackIndex int) string {
	return strings.Join([]string{
		"adhoc",
		normalizePricingItemDescription(item.Description),
		item.Quantity,
		strconv.FormatFloat(item.QuantityNumeric, 'f', -1, 64),
		strconv.FormatInt(item.UnitPriceCents, 10),
		strconv.Itoa(item.TaxRateBps),
		strconv.FormatBool(item.IsOptional),
		strconv.FormatBool(item.IsSelected),
		strconv.Itoa(fallbackIndex),
	}, "|")
}

func quoteItemFingerprint(item QuoteItem, fallbackIndex int) string {
	return strings.Join([]string{
		"adhoc",
		normalizePricingItemDescription(item.Description),
		item.Quantity,
		strconv.FormatFloat(item.QuantityNumeric, 'f', -1, 64),
		strconv.FormatInt(item.UnitPriceCents, 10),
		strconv.Itoa(item.TaxRateBps),
		strconv.FormatBool(item.IsOptional),
		strconv.FormatBool(item.IsSelected),
		strconv.Itoa(fallbackIndex),
	}, "|")
}

func normalizePricingItemDescription(description string) string {
	return strings.Join(strings.Fields(strings.ToLower(description)), " ")
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func buildInt64Correction(fieldName string, aiValue, humanValue int64, findingCode string) []quotePricingCorrection {
	if aiValue == humanValue {
		return nil
	}
	delta := humanValue - aiValue
	return []quotePricingCorrection{{
		FieldName:       fieldName,
		AIValue:         map[string]any{"value": aiValue},
		HumanValue:      map[string]any{"value": humanValue},
		DeltaCents:      &delta,
		DeltaPercentage: deltaPercentage(float64(aiValue), float64(humanValue)),
		Reason:          stringPtr("manual quote pricing adjusted"),
		AIFindingCode:   &findingCode,
	}}
}

func buildItemInt64Correction(quoteItemID *uuid.UUID, fieldName string, aiValue, humanValue int64, description string, findingCode string) []quotePricingCorrection {
	if aiValue == humanValue {
		return nil
	}
	delta := humanValue - aiValue
	return []quotePricingCorrection{{
		QuoteItemID:     quoteItemID,
		FieldName:       fieldName,
		AIValue:         itemContextValue(description, aiValue),
		HumanValue:      itemContextValue(description, humanValue),
		DeltaCents:      &delta,
		DeltaPercentage: deltaPercentage(float64(aiValue), float64(humanValue)),
		Reason:          stringPtr("manual line item pricing adjusted"),
		AIFindingCode:   &findingCode,
	}}
}

func buildItemFloatCorrection(quoteItemID *uuid.UUID, fieldName string, aiValue, humanValue float64, description string, findingCode string) []quotePricingCorrection {
	if aiValue == humanValue {
		return nil
	}
	return []quotePricingCorrection{{
		QuoteItemID:     quoteItemID,
		FieldName:       fieldName,
		AIValue:         itemContextValue(description, aiValue),
		HumanValue:      itemContextValue(description, humanValue),
		DeltaPercentage: deltaPercentage(aiValue, humanValue),
		Reason:          stringPtr("manual line item quantity adjusted"),
		AIFindingCode:   &findingCode,
	}}
}

func buildItemIntCorrection(quoteItemID *uuid.UUID, fieldName string, aiValue, humanValue int, description string, findingCode string) []quotePricingCorrection {
	if aiValue == humanValue {
		return nil
	}
	return []quotePricingCorrection{{
		QuoteItemID:     quoteItemID,
		FieldName:       fieldName,
		AIValue:         itemContextValue(description, aiValue),
		HumanValue:      itemContextValue(description, humanValue),
		DeltaPercentage: deltaPercentage(float64(aiValue), float64(humanValue)),
		Reason:          stringPtr("manual line item tax adjusted"),
		AIFindingCode:   &findingCode,
	}}
}

func itemContextValue(description string, value any) map[string]any {
	return map[string]any{
		"description": description,
		"value":       value,
	}
}

func itemSnapshotValue(item QuoteItem) map[string]any {
	return map[string]any{
		"description":      item.Description,
		"quantity":         item.Quantity,
		"quantityNumeric":  item.QuantityNumeric,
		"unitPriceCents":   item.UnitPriceCents,
		"taxRateBps":       item.TaxRateBps,
		"isOptional":       item.IsOptional,
		"isSelected":       item.IsSelected,
		"catalogProductId": item.CatalogProductID,
	}
}

func previousItemSnapshotValue(item QuotePricingSnapshotItem) map[string]any {
	return map[string]any{
		"description":      item.Description,
		"quantity":         item.Quantity,
		"quantityNumeric":  item.QuantityNumeric,
		"unitPriceCents":   item.UnitPriceCents,
		"taxRateBps":       item.TaxRateBps,
		"isOptional":       item.IsOptional,
		"isSelected":       item.IsSelected,
		"catalogProductId": item.CatalogProductID,
	}
}

func deltaPercentage(aiValue, humanValue float64) *float64 {
	if aiValue == 0 {
		return nil
	}
	value := ((humanValue - aiValue) / aiValue) * 100
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return &value
}

func stringPtr(value string) *string {
	return &value
}
