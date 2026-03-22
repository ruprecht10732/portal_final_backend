package adapters

import (
	"context"
	"fmt"
	"strings"
	"time"

	energylabelsvc "portal_final_backend/internal/energylabel/service"
	isdesvc "portal_final_backend/internal/isde/service"
	isdetransport "portal_final_backend/internal/isde/transport"
	"portal_final_backend/internal/tasks"
	whatsappagent "portal_final_backend/internal/whatsappagent"

	"github.com/google/uuid"
)

// WhatsAppAgentTaskReaderAdapter adapts task reads for whatsapp agent tools.
type WhatsAppAgentTaskReaderAdapter struct {
	tasks *tasks.Service
}

func NewWhatsAppAgentTaskReaderAdapter(tasksSvc *tasks.Service) *WhatsAppAgentTaskReaderAdapter {
	return &WhatsAppAgentTaskReaderAdapter{tasks: tasksSvc}
}

func (a *WhatsAppAgentTaskReaderAdapter) GetLeadTasks(ctx context.Context, orgID uuid.UUID, input whatsappagent.GetLeadTasksInput) (whatsappagent.GetLeadTasksOutput, error) {
	if a.tasks == nil {
		return whatsappagent.GetLeadTasksOutput{}, fmt.Errorf("task service not configured")
	}
	leadID := strings.TrimSpace(input.LeadID)
	if leadID == "" {
		return whatsappagent.GetLeadTasksOutput{}, fmt.Errorf("lead_id is required")
	}

	req := tasks.ListTasksRequest{
		LeadID: leadID,
	}
	if leadServiceID := strings.TrimSpace(input.LeadServiceID); leadServiceID != "" {
		req.ScopeType = tasks.ScopeLeadService
		req.LeadServiceID = leadServiceID
	}
	if status := normalizeTaskStatus(input.Status); status != "" {
		req.Status = status
	}
	records, err := a.tasks.List(ctx, orgID, req)
	if err != nil {
		return whatsappagent.GetLeadTasksOutput{}, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	if len(records) > limit {
		records = records[:limit]
	}

	result := whatsappagent.GetLeadTasksOutput{Count: len(records)}
	if len(records) == 0 {
		return result, nil
	}
	result.Tasks = make([]whatsappagent.LeadTaskSummary, 0, len(records))
	for _, record := range records {
		summary := whatsappagent.LeadTaskSummary{
			TaskID:         record.ID.String(),
			Title:          strings.TrimSpace(record.Title),
			Description:    strings.TrimSpace(record.Description),
			Status:         strings.TrimSpace(record.Status),
			Priority:       strings.TrimSpace(record.Priority),
			AssignedUserID: record.AssignedUserID.String(),
			CreatedAt:      record.CreatedAt.Format(time.RFC3339),
		}
		if record.LeadID != nil {
			summary.LeadID = record.LeadID.String()
		}
		if record.LeadServiceID != nil {
			summary.LeadServiceID = record.LeadServiceID.String()
		}
		if record.DueAt != nil {
			summary.DueAt = record.DueAt.Format(time.RFC3339)
		}
		result.Tasks = append(result.Tasks, summary)
	}
	return result, nil
}

func normalizeTaskStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all", "alles":
		return ""
	case "open", "openstaand":
		return tasks.StatusOpen
	case "completed", "voltooid", "afgerond":
		return tasks.StatusCompleted
	case "cancelled", "geannuleerd", "gecancelled":
		return tasks.StatusCancelled
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

// WhatsAppAgentEnergyLabelAdapter adapts energy-label lookups for whatsapp agent tools.
type WhatsAppAgentEnergyLabelAdapter struct {
	svc *energylabelsvc.Service
}

func NewWhatsAppAgentEnergyLabelAdapter(svc *energylabelsvc.Service) *WhatsAppAgentEnergyLabelAdapter {
	return &WhatsAppAgentEnergyLabelAdapter{svc: svc}
}

func (a *WhatsAppAgentEnergyLabelAdapter) GetEnergyLabel(ctx context.Context, _ uuid.UUID, input whatsappagent.GetEnergyLabelInput) (whatsappagent.GetEnergyLabelOutput, error) {
	if a.svc == nil {
		return whatsappagent.GetEnergyLabelOutput{}, fmt.Errorf("energy label service not configured")
	}
	label, err := a.svc.GetByAddress(
		ctx,
		strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(input.Postcode), " ", "")),
		strings.TrimSpace(input.HouseNumber),
		strings.TrimSpace(input.HouseLetter),
		strings.TrimSpace(input.Addition),
		strings.TrimSpace(input.Detail),
	)
	if err != nil {
		return whatsappagent.GetEnergyLabelOutput{}, err
	}
	if label == nil {
		return whatsappagent.GetEnergyLabelOutput{Success: true, Message: "Geen energielabel gevonden", Found: false}, nil
	}
	return whatsappagent.GetEnergyLabelOutput{
		Success: true,
		Message: "Energielabel opgehaald",
		Found:   true,
		Label: &whatsappagent.EnergyLabelSummary{
			EnergyClass:        strings.TrimSpace(label.Energieklasse),
			EnergyIndex:        formatFloat(label.EnergieIndex),
			RegistrationDate:   formatDate(label.Registratiedatum),
			ValidUntil:         formatDate(label.GeldigTot),
			BuildYear:          label.Bouwjaar,
			BuildingType:       strings.TrimSpace(label.Gebouwtype),
			BuildingSubType:    strings.TrimSpace(label.Gebouwsubtype),
			AddressPostcode:    strings.TrimSpace(label.Postcode),
			AddressHouseNo:     label.Huisnummer,
			AddressHouseLetter: strings.TrimSpace(label.Huisletter),
			AddressAddition:    strings.TrimSpace(label.Huisnummertoevoeging),
		},
	}, nil
}

func formatDate(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02")
}

func formatFloat(value *float64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *value)
}

// WhatsAppAgentISDEAdapter adapts ISDE calculations for whatsapp agent tools.
type WhatsAppAgentISDEAdapter struct {
	svc *isdesvc.Service
}

func NewWhatsAppAgentISDEAdapter(svc *isdesvc.Service) *WhatsAppAgentISDEAdapter {
	return &WhatsAppAgentISDEAdapter{svc: svc}
}

func (a *WhatsAppAgentISDEAdapter) GetISDE(ctx context.Context, orgID uuid.UUID, input whatsappagent.GetISDEInput) (whatsappagent.GetISDEOutput, error) {
	if a.svc == nil {
		return whatsappagent.GetISDEOutput{}, fmt.Errorf("isde service not configured")
	}

	req := isdetransport.ISDECalculationRequest{
		ExecutionYear:                   input.ExecutionYear,
		PreviousSubsidiesWithin24Months: input.PreviousSubsidiesWithin24Months,
		HasExistingWarmtenetConnection:  input.HasExistingWarmtenetConnection,
		HasReceivedWarmtenetSubsidy:     input.HasReceivedWarmtenetSubsidy,
	}
	if len(input.Measures) > 0 {
		req.Measures = make([]isdetransport.RequestedMeasure, 0, len(input.Measures))
		for _, measure := range input.Measures {
			req.Measures = append(req.Measures, isdetransport.RequestedMeasure{
				MeasureID:                measure.MeasureID,
				AreaM2:                   measure.AreaM2,
				PerformanceValue:         measure.PerformanceValue,
				FramePerformanceValue:    measure.FramePerformanceValue,
				HasMKIBonus:              measure.HasMKIBonus,
				FrameReplaced:            measure.FrameReplaced,
				StackedWithPairedMeasure: measure.StackedWithPairedMeasure,
			})
		}
	}
	if len(input.Installations) > 0 {
		req.Installations = make([]isdetransport.RequestedInstallation, 0, len(input.Installations))
		for _, installation := range input.Installations {
			req.Installations = append(req.Installations, isdetransport.RequestedInstallation{
				Kind:                installation.Kind,
				Meldcode:            installation.Meldcode,
				HeatPumpType:        installation.HeatPumpType,
				HeatPumpEnergyLabel: installation.HeatPumpEnergyLabel,
				ThermalPowerKW:      installation.ThermalPowerKW,
				IsAdditionalUnit:    installation.IsAdditionalUnit,
				IsSplitSystem:       installation.IsSplitSystem,
				RefrigerantChargeKg: installation.RefrigerantChargeKg,
				RefrigerantGWP:      installation.RefrigerantGWP,
			})
		}
	}

	resp, err := a.svc.Calculate(ctx, orgID, req)
	if err != nil {
		return whatsappagent.GetISDEOutput{}, err
	}

	result := whatsappagent.GetISDEOutput{
		TotalAmountCents:     resp.TotalAmountCents,
		IsDoubled:            resp.IsDoubled,
		EligibleMeasureCount: resp.EligibleMeasureCount,
		ValidationMessages:   resp.ValidationMessages,
		UnknownMeasureIDs:    resp.UnknownMeasureIDs,
		UnknownMeldcodes:     resp.UnknownMeldcodes,
	}
	if len(resp.InsulationBreakdown) > 0 {
		result.InsulationBreakdown = make([]whatsappagent.ISDELineItem, 0, len(resp.InsulationBreakdown))
		for _, item := range resp.InsulationBreakdown {
			result.InsulationBreakdown = append(result.InsulationBreakdown, whatsappagent.ISDELineItem(item))
		}
	}
	if len(resp.GlassBreakdown) > 0 {
		result.GlassBreakdown = make([]whatsappagent.ISDELineItem, 0, len(resp.GlassBreakdown))
		for _, item := range resp.GlassBreakdown {
			result.GlassBreakdown = append(result.GlassBreakdown, whatsappagent.ISDELineItem(item))
		}
	}
	if len(resp.Installations) > 0 {
		result.Installations = make([]whatsappagent.ISDELineItem, 0, len(resp.Installations))
		for _, item := range resp.Installations {
			result.Installations = append(result.Installations, whatsappagent.ISDELineItem(item))
		}
	}
	return result, nil
}

var _ whatsappagent.TaskReader = (*WhatsAppAgentTaskReaderAdapter)(nil)
var _ whatsappagent.EnergyLabelReader = (*WhatsAppAgentEnergyLabelAdapter)(nil)
var _ whatsappagent.ISDECalculator = (*WhatsAppAgentISDEAdapter)(nil)
