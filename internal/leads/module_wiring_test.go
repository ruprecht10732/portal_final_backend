package leads

import (
	"context"
	"strings"
	"testing"
	"time"

	leadagent "portal_final_backend/internal/leads/agent"
	leadhandler "portal_final_backend/internal/leads/handler"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/internal/scheduler"

	"github.com/google/uuid"
)

type testAppointmentBooker struct{}

func (testAppointmentBooker) BookLeadVisit(context.Context, ports.BookVisitParams) error { return nil }
func (testAppointmentBooker) GetLeadVisitByService(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*ports.LeadVisitSummary, error) {
	return nil, nil
}
func (testAppointmentBooker) RescheduleLeadVisit(context.Context, ports.RescheduleVisitParams) error {
	return nil
}
func (testAppointmentBooker) CancelLeadVisit(context.Context, ports.CancelVisitParams) error {
	return nil
}

type testLeadUpdater struct{}

func (testLeadUpdater) Update(context.Context, uuid.UUID, transport.UpdateLeadRequest, uuid.UUID, uuid.UUID, []string) (transport.LeadResponse, error) {
	return transport.LeadResponse{}, nil
}

type testAutomationScheduler struct{}

func (testAutomationScheduler) EnqueueGatekeeperRun(context.Context, scheduler.GatekeeperRunPayload) error {
	return nil
}
func (testAutomationScheduler) EnqueueEstimatorRun(context.Context, scheduler.EstimatorRunPayload) error {
	return nil
}
func (testAutomationScheduler) EnqueueDispatcherRun(context.Context, scheduler.DispatcherRunPayload) error {
	return nil
}
func (testAutomationScheduler) EnqueuePhotoAnalysis(context.Context, scheduler.PhotoAnalysisPayload) error {
	return nil
}
func (testAutomationScheduler) EnqueuePhotoAnalysisIn(context.Context, scheduler.PhotoAnalysisPayload, time.Duration) error {
	return nil
}
func (testAutomationScheduler) EnqueueAuditVisitReport(context.Context, scheduler.AuditVisitReportPayload) error {
	return nil
}
func (testAutomationScheduler) EnqueueAuditCallLog(context.Context, scheduler.AuditCallLogPayload) error {
	return nil
}

func TestVerifyWiringFailsWhenAppointmentBookerMissing(t *testing.T) {
	callLogger := &leadagent.CallLogger{}
	callLogger.SetLeadUpdater(testLeadUpdater{})

	module := &Module{
		callLogger:           callLogger,
		automationQueue:      testAutomationScheduler{},
		handler:              &leadhandler.Handler{},
		photoAnalysisHandler: &leadhandler.PhotoAnalysisHandler{},
		orchestrator:         &Orchestrator{},
	}

	err := module.VerifyWiring()
	if err == nil {
		t.Fatal("expected VerifyWiring to fail when appointment booker is missing")
	}
	if !strings.Contains(err.Error(), "appointment booker") {
		t.Fatalf("expected appointment booker error, got %v", err)
	}
}

func TestVerifyWiringSucceedsWhenRequiredDependenciesPresent(t *testing.T) {
	callLogger := &leadagent.CallLogger{}
	callLogger.SetLeadUpdater(testLeadUpdater{})
	callLogger.SetAppointmentBooker(testAppointmentBooker{})

	module := &Module{
		callLogger:           callLogger,
		automationQueue:      testAutomationScheduler{},
		handler:              &leadhandler.Handler{},
		photoAnalysisHandler: &leadhandler.PhotoAnalysisHandler{},
		orchestrator:         &Orchestrator{},
	}

	if err := module.VerifyWiring(); err != nil {
		t.Fatalf("expected VerifyWiring to succeed, got %v", err)
	}
}
