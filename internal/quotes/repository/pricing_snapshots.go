package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	quotesdb "portal_final_backend/internal/quotes/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type QuotePricingSnapshot struct {
	ID                     uuid.UUID
	QuoteID                uuid.UUID
	OrganizationID         uuid.UUID
	LeadID                 uuid.UUID
	LeadServiceID          *uuid.UUID
	ServiceType            *string
	PostcodeRaw            *string
	PostcodePrefixZIP4     *string
	SourceType             string
	QuoteRevision          int
	PricingMode            string
	DiscountType           string
	DiscountValue          int64
	MaterialSubtotalCents  *int64
	LaborSubtotalLowCents  *int64
	LaborSubtotalHighCents *int64
	ExtraCostsCents        *int64
	SubtotalCents          int64
	DiscountAmountCents    int64
	TaxTotalCents          int64
	TotalCents             int64
	ItemCount              int
	CatalogItemCount       int
	AdHocItemCount         int
	Notes                  *string
	PriceRangeText         *string
	ScopeText              *string
	EstimatorRunID         *string
	ModelName              *string
	CreatedByActor         string
	CreatedByUserID        *uuid.UUID
	CreatedAt              time.Time
}

type QuotePricingSnapshotItem struct {
	Description      string     `json:"description"`
	Quantity         string     `json:"quantity"`
	QuantityNumeric  float64    `json:"quantityNumeric"`
	UnitPriceCents   int64      `json:"unitPriceCents"`
	TaxRateBps       int        `json:"taxRateBps"`
	IsOptional       bool       `json:"isOptional"`
	IsSelected       bool       `json:"isSelected"`
	CatalogProductID *uuid.UUID `json:"catalogProductId,omitempty"`
}

func (r *Repository) insertPricingSnapshot(ctx context.Context, qtx *quotesdb.Queries, quote *Quote, items []QuoteItem, snapshot *QuotePricingSnapshot) error {
	if snapshot == nil {
		return nil
	}
	if err := r.hydratePricingSnapshotContext(ctx, qtx, quote, snapshot); err != nil {
		return err
	}

	revision, err := qtx.GetLatestQuotePricingSnapshotRevision(ctx, quotesdb.GetLatestQuotePricingSnapshotRevisionParams{
		QuoteID:        toPgUUID(quote.ID),
		OrganizationID: toPgUUID(quote.OrganizationID),
	})
	if err != nil && err != pgx.ErrNoRows {
		return fmt.Errorf("load quote pricing snapshot revision: %w", err)
	}

	itemPayload := buildQuotePricingSnapshotItems(items)
	itemCount := len(itemPayload)
	catalogItemCount := 0
	for _, item := range itemPayload {
		if item.CatalogProductID != nil {
			catalogItemCount++
		}
	}

	createdAt := quote.UpdatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	createdByActor := strings.TrimSpace(snapshot.CreatedByActor)
	if createdByActor == "" {
		createdByActor = "system"
	}

	row, err := qtx.CreateQuotePricingSnapshot(ctx, quotesdb.CreateQuotePricingSnapshotParams{
		ID:                     toPgUUID(uuid.New()),
		QuoteID:                toPgUUID(quote.ID),
		OrganizationID:         toPgUUID(quote.OrganizationID),
		LeadID:                 toPgUUID(quote.LeadID),
		LeadServiceID:          toPgUUIDPtr(quote.LeadServiceID),
		ServiceType:            toPgTextPtr(snapshot.ServiceType),
		PostcodeRaw:            toPgTextPtr(snapshot.PostcodeRaw),
		PostcodePrefixZip4:     toPgTextPtr(snapshot.PostcodePrefixZIP4),
		SourceType:             snapshot.SourceType,
		QuoteRevision:          revision + 1,
		PricingMode:            quote.PricingMode,
		DiscountType:           quote.DiscountType,
		DiscountValue:          quote.DiscountValue,
		MaterialSubtotalCents:  toPgInt8Ptr(snapshot.MaterialSubtotalCents),
		LaborSubtotalLowCents:  toPgInt8Ptr(snapshot.LaborSubtotalLowCents),
		LaborSubtotalHighCents: toPgInt8Ptr(snapshot.LaborSubtotalHighCents),
		ExtraCostsCents:        toPgInt8Ptr(snapshot.ExtraCostsCents),
		SubtotalCents:          quote.SubtotalCents,
		DiscountAmountCents:    quote.DiscountAmountCents,
		TaxTotalCents:          quote.TaxTotalCents,
		TotalCents:             quote.TotalCents,
		ItemCount:              int32(itemCount),
		CatalogItemCount:       int32(catalogItemCount),
		AdHocItemCount:         int32(itemCount - catalogItemCount),
		StructuredItems:        marshalJSON(itemPayload),
		Notes:                  toPgTextPtr(quote.Notes),
		PriceRangeText:         toPgTextPtr(snapshot.PriceRangeText),
		ScopeText:              toPgTextPtr(snapshot.ScopeText),
		EstimatorRunID:         toPgTextPtr(snapshot.EstimatorRunID),
		ModelName:              toPgTextPtr(snapshot.ModelName),
		CreatedByActor:         createdByActor,
		CreatedByUserID:        toPgUUIDPtr(snapshot.CreatedByUserID),
		CreatedAt:              toPgTimestamp(createdAt),
	})
	if err != nil {
		return fmt.Errorf("create quote pricing snapshot: %w", err)
	}

	snapshot.ID = uuid.UUID(row.ID.Bytes)
	snapshot.QuoteRevision = int(row.QuoteRevision)
	snapshot.CreatedAt = row.CreatedAt.Time
	return nil
}

func (r *Repository) hydratePricingSnapshotContext(ctx context.Context, qtx *quotesdb.Queries, quote *Quote, snapshot *QuotePricingSnapshot) error {
	snapshot.ServiceType = normalizeBlankStringPtr(snapshot.ServiceType)
	snapshot.PostcodeRaw = normalizeBlankStringPtr(snapshot.PostcodeRaw)
	snapshot.PostcodePrefixZIP4 = normalizeBlankStringPtr(snapshot.PostcodePrefixZIP4)

	if snapshot.ServiceType == nil || snapshot.PostcodeRaw == nil {
		row, err := qtx.GetQuotePricingSnapshotContext(ctx, quotesdb.GetQuotePricingSnapshotContextParams{
			LeadID:         toPgUUID(quote.LeadID),
			LeadServiceID:  toPgUUIDPtr(quote.LeadServiceID),
			OrganizationID: toPgUUID(quote.OrganizationID),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("load quote pricing snapshot context: %w", err)
		}
		if snapshot.ServiceType == nil {
			snapshot.ServiceType = normalizeBlankStringPtr(optionalString(row.ServiceType))
		}
		if snapshot.PostcodeRaw == nil {
			snapshot.PostcodeRaw = normalizeBlankStringPtr(&row.PostcodeRaw)
		}
	}

	if snapshot.PostcodePrefixZIP4 == nil {
		snapshot.PostcodePrefixZIP4 = derivePostcodePrefixZIP4(snapshot.PostcodeRaw)
	}

	return nil
}

func (r *Repository) getLatestPricingSnapshot(ctx context.Context, qtx *quotesdb.Queries, quoteID, organizationID uuid.UUID) (*quotesdb.RacQuotePricingSnapshot, error) {
	row, err := qtx.GetLatestQuotePricingSnapshotByQuote(ctx, quotesdb.GetLatestQuotePricingSnapshotByQuoteParams{
		QuoteID:        toPgUUID(quoteID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load latest quote pricing snapshot: %w", err)
	}
	return &row, nil
}

func buildQuotePricingSnapshotItems(items []QuoteItem) []QuotePricingSnapshotItem {
	out := make([]QuotePricingSnapshotItem, 0, len(items))
	for _, item := range items {
		out = append(out, QuotePricingSnapshotItem{
			Description:      item.Description,
			Quantity:         item.Quantity,
			QuantityNumeric:  item.QuantityNumeric,
			UnitPriceCents:   item.UnitPriceCents,
			TaxRateBps:       item.TaxRateBps,
			IsOptional:       item.IsOptional,
			IsSelected:       item.IsSelected,
			CatalogProductID: item.CatalogProductID,
		})
	}
	return out
}

func derivePostcodePrefixZIP4(raw *string) *string {
	if raw == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil
	}
	digits := make([]rune, 0, 4)
	for _, r := range trimmed {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
			if len(digits) == 4 {
				prefix := string(digits)
				return &prefix
			}
		}
	}
	return nil
}

func normalizeBlankStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
