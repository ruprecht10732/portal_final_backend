package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	quotesdb "portal_final_backend/internal/quotes/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type quotePricingOutcomeParams struct {
	OutcomeType        string
	RejectionReason    *string
	AcceptedTotalCents *int64
	FinalTotalCents    *int64
	OutcomeAt          time.Time
	Metadata           map[string]any
}

func (r *Repository) insertPricingOutcome(ctx context.Context, qtx *quotesdb.Queries, quote *Quote, params quotePricingOutcomeParams) error {
	var snapshotID *uuid.UUID
	latestSnapshot, err := qtx.GetLatestQuotePricingSnapshotByQuote(ctx, quotesdb.GetLatestQuotePricingSnapshotByQuoteParams{
		QuoteID:        toPgUUID(quote.ID),
		OrganizationID: toPgUUID(quote.OrganizationID),
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("load latest quote pricing snapshot: %w", err)
		}
	} else {
		resolvedSnapshotID := uuid.UUID(latestSnapshot.ID.Bytes)
		snapshotID = &resolvedSnapshotID
	}

	_, err = qtx.CreateQuotePricingOutcome(ctx, quotesdb.CreateQuotePricingOutcomeParams{
		ID:                 toPgUUID(uuid.New()),
		QuoteID:            toPgUUID(quote.ID),
		SnapshotID:         toPgUUIDPtr(snapshotID),
		OrganizationID:     toPgUUID(quote.OrganizationID),
		LeadID:             toPgUUID(quote.LeadID),
		LeadServiceID:      toPgUUIDPtr(quote.LeadServiceID),
		OutcomeType:        params.OutcomeType,
		RejectionReason:    toPgTextPtr(params.RejectionReason),
		AcceptedTotalCents: toPgInt8Ptr(params.AcceptedTotalCents),
		FinalTotalCents:    toPgInt8Ptr(params.FinalTotalCents),
		EstimatorRunID:     latestSnapshot.EstimatorRunID,
		OutcomeAt:          toPgTimestamp(params.OutcomeAt),
		Metadata:           marshalJSON(params.Metadata),
		CreatedAt:          toPgTimestamp(params.OutcomeAt),
	})
	if err != nil {
		return fmt.Errorf("create quote pricing outcome: %w", err)
	}

	return nil
}
