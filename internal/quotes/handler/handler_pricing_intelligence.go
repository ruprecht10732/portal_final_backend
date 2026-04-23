package handler

import (
	"net/http"
	"strings"

	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
)

const defaultPricingIntelligenceServiceType = ""

func (h *Handler) GetPricingIntelligenceSummary(c *gin.Context) {
	tenantID, ok := httpkit.RequireTenant(c)
	if !ok {
		return
	}

	serviceType := strings.TrimSpace(c.Query("serviceType"))
	if serviceType == defaultPricingIntelligenceServiceType {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, "serviceType is required")
		return
	}

	report, err := h.svc.GetPricingIntelligenceReport(c.Request.Context(), tenantID, serviceType, strings.TrimSpace(c.Query("postcodePrefix")))
	if httpkit.HandleError(c, err) {
		return
	}

	response := transport.PricingIntelligenceSummaryResponse{
		ServiceType:  report.ServiceType,
		RegionPrefix: report.RegionPrefix,
		Aggregates:   make([]transport.PricingIntelligenceAggregateResponse, 0, len(report.Aggregates)),
	}
	for _, aggregate := range report.Aggregates {
		response.Aggregates = append(response.Aggregates, transport.PricingIntelligenceAggregateResponse{
			RegionPrefix:        aggregate.RegionPrefix,
			PriceBand:           aggregate.PriceBand,
			SampleCount:         aggregate.SampleCount,
			AcceptedCount:       aggregate.AcceptedCount,
			RejectedCount:       aggregate.RejectedCount,
			ConversionRate:      aggregate.ConversionRate,
			AverageQuotedCents:  aggregate.AverageQuotedCents,
			AverageOutcomeCents: aggregate.AverageOutcomeCents,
		})
	}

	httpkit.OK(c, response)
}

func (h *Handler) GetPricingIntelligenceRecords(c *gin.Context) {
	tenantID, ok := httpkit.RequireTenant(c)
	if !ok {
		return
	}

	serviceType := strings.TrimSpace(c.Query("serviceType"))
	if serviceType == defaultPricingIntelligenceServiceType {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, "serviceType is required")
		return
	}

	report, err := h.svc.GetPricingIntelligenceReport(c.Request.Context(), tenantID, serviceType, strings.TrimSpace(c.Query("postcodePrefix")))
	if httpkit.HandleError(c, err) {
		return
	}

	response := transport.PricingIntelligenceRecordsResponse{
		ServiceType:  report.ServiceType,
		RegionPrefix: report.RegionPrefix,
		Snapshots:    make([]transport.PricingIntelligenceSnapshotResponse, 0, len(report.RecentSnapshots)),
		Outcomes:     make([]transport.PricingIntelligenceOutcomeResponse, 0, len(report.RecentOutcomes)),
		Corrections:  make([]transport.PricingIntelligenceCorrectionResponse, 0, len(report.RecentCorrections)),
	}
	for _, snapshot := range report.RecentSnapshots {
		response.Snapshots = append(response.Snapshots, transport.PricingIntelligenceSnapshotResponse{
			QuoteID:       snapshot.QuoteID,
			RegionPrefix:  snapshot.RegionPrefix,
			PriceBand:     snapshot.PriceBand,
			SourceType:    snapshot.SourceType,
			QuoteRevision: snapshot.QuoteRevision,
			TotalCents:    snapshot.TotalCents,
			CreatedAt:     snapshot.CreatedAt,
		})
	}
	for _, outcome := range report.RecentOutcomes {
		response.Outcomes = append(response.Outcomes, transport.PricingIntelligenceOutcomeResponse{
			QuoteID:         outcome.QuoteID,
			RegionPrefix:    outcome.RegionPrefix,
			PriceBand:       outcome.PriceBand,
			OutcomeType:     outcome.OutcomeType,
			FinalTotalCents: outcome.FinalTotalCents,
			EstimatorRunID:  outcome.EstimatorRunID,
			Reason:          outcome.Reason,
			CreatedAt:       outcome.CreatedAt,
		})
	}
	for _, correction := range report.RecentCorrections {
		response.Corrections = append(response.Corrections, transport.PricingIntelligenceCorrectionResponse{
			QuoteID:         correction.QuoteID,
			RegionPrefix:    correction.RegionPrefix,
			PriceBand:       correction.PriceBand,
			FieldName:       correction.FieldName,
			DeltaCents:      correction.DeltaCents,
			DeltaPercentage: correction.DeltaPercentage,
			Reason:          correction.Reason,
			AIFindingCode:   correction.AIFindingCode,
			EstimatorRunID:  correction.EstimatorRunID,
			CreatedAt:       correction.CreatedAt,
		})
	}

	httpkit.OK(c, response)
}
