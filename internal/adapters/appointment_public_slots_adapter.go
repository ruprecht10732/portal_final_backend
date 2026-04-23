package adapters

import (
	"context"
	"fmt"
	"sort"
	"time"

	"portal_final_backend/internal/appointments/service"
	"portal_final_backend/internal/appointments/transport"
	"portal_final_backend/internal/leads/ports"

	"github.com/google/uuid"
)

type AppointmentSlotAdapter struct {
	svc *service.Service
}

func NewAppointmentSlotAdapter(svc *service.Service) *AppointmentSlotAdapter {
	return &AppointmentSlotAdapter{svc: svc}
}

func (a *AppointmentSlotAdapter) HasAvailabilityRules(ctx context.Context, organizationID uuid.UUID) (bool, error) {
	userIDs, err := a.svc.ListAvailabilityRuleUserIDs(ctx, organizationID)
	if err != nil {
		return false, err
	}
	return len(userIDs) > 0, nil
}

func (a *AppointmentSlotAdapter) GetAvailableSlots(ctx context.Context, organizationID uuid.UUID, startDate, endDate string, slotDuration int) (*ports.PublicAvailableSlotsResponse, error) {
	userIDs, err := a.svc.ListAvailabilityRuleUserIDs(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	if len(userIDs) == 0 {
		return &ports.PublicAvailableSlotsResponse{Days: []ports.PublicDaySlots{}}, nil
	}

	dayMap, err := a.collectAvailableSlots(ctx, organizationID, userIDs, startDate, endDate, slotDuration)
	if err != nil {
		return nil, err
	}

	return &ports.PublicAvailableSlotsResponse{Days: a.buildPublicDaySlots(dayMap)}, nil
}

func (a *AppointmentSlotAdapter) CreateRequestedAppointment(ctx context.Context, userID, organizationID, leadID, leadServiceID uuid.UUID, startTime, endTime time.Time) (*ports.PublicAppointmentSummary, error) {
	allowed, err := a.isAllowedToBook(ctx, organizationID, userID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		// Fixed: Reverted to fmt.Errorf to resolve the "undefined: ports.ErrUserNotAvailable" error
		return nil, fmt.Errorf("user not available for booking")
	}

	sendEmail := false
	appt, err := a.svc.Create(ctx, userID, true, organizationID, transport.CreateAppointmentRequest{
		LeadID:                &leadID,
		LeadServiceID:         &leadServiceID,
		Type:                  transport.AppointmentTypeLeadVisit,
		Title:                 "Offerte inspectie",
		StartTime:             startTime,
		EndTime:               endTime,
		AllDay:                false,
		SendConfirmationEmail: &sendEmail,
		InitialStatus:         transport.AppointmentStatusRequested,
	})
	if err != nil {
		return nil, err
	}

	return a.toPublicAppointmentSummary(appt), nil
}

func (a *AppointmentSlotAdapter) isAllowedToBook(ctx context.Context, organizationID, userID uuid.UUID) (bool, error) {
	userIDs, err := a.svc.ListAvailabilityRuleUserIDs(ctx, organizationID)
	if err != nil {
		return false, err
	}
	for _, id := range userIDs {
		if id == userID {
			return true, nil
		}
	}
	return false, nil
}

// Internal helper for deduplication without string allocation
type slotKey struct {
	date  string
	start int64
	end   int64
}

func (a *AppointmentSlotAdapter) collectAvailableSlots(
	ctx context.Context,
	organizationID uuid.UUID,
	userIDs []uuid.UUID,
	startDate, endDate string,
	slotDuration int,
) (map[string][]ports.PublicTimeSlot, error) {
	dayMap := make(map[string][]ports.PublicTimeSlot)
	seen := make(map[slotKey]struct{})

	for _, userID := range userIDs {
		resp, err := a.svc.GetAvailableSlots(ctx, userID, true, organizationID, transport.GetAvailableSlotsRequest{
			StartDate:    startDate,
			EndDate:      endDate,
			SlotDuration: slotDuration,
		})
		if err != nil {
			return nil, err
		}

		for _, day := range resp.Days {
			for _, slot := range day.Slots {
				key := slotKey{
					date:  day.Date,
					start: slot.StartTime.Unix(),
					end:   slot.EndTime.Unix(),
				}

				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}

				dayMap[day.Date] = append(dayMap[day.Date], ports.PublicTimeSlot{
					UserID:    userID,
					StartTime: slot.StartTime,
					EndTime:   slot.EndTime,
				})
			}
		}
	}

	return dayMap, nil
}

func (a *AppointmentSlotAdapter) buildPublicDaySlots(dayMap map[string][]ports.PublicTimeSlot) []ports.PublicDaySlots {
	keys := make([]string, 0, len(dayMap))
	for date := range dayMap {
		keys = append(keys, date)
	}
	sort.Strings(keys)

	result := make([]ports.PublicDaySlots, 0, len(keys))
	for _, date := range keys {
		slots := dayMap[date]
		sort.Slice(slots, func(i, j int) bool {
			return slots[i].StartTime.Before(slots[j].StartTime)
		})
		result = append(result, ports.PublicDaySlots{
			Date:  date,
			Slots: slots,
		})
	}

	return result
}

func (a *AppointmentSlotAdapter) toPublicAppointmentSummary(appt *transport.AppointmentResponse) *ports.PublicAppointmentSummary {
	if appt == nil {
		return nil
	}
	return &ports.PublicAppointmentSummary{
		ID:        appt.ID,
		StartTime: appt.StartTime,
		EndTime:   appt.EndTime,
		Title:     appt.Title,
		Status:    string(appt.Status),
	}
}

var _ ports.AppointmentSlotProvider = (*AppointmentSlotAdapter)(nil)
