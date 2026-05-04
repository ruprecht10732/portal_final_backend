package agent

import (
	"github.com/google/uuid"
)

// AgentTaskPayload is the unified task payload for all agent runs.
// It replaces GatekeeperRunPayload, EstimatorRunPayload, DispatcherRunPayload,
// AuditVisitReportPayload, and AuditCallLogPayload.
type AgentTaskPayload struct {
	Workspace     string    `json:"workspace"`               // e.g. "gatekeeper", "calculator", "matchmaker", "auditor"
	Mode          string    `json:"mode,omitempty"`          // e.g. "estimator", "quote-generator" (for calculator)
	LeadID        uuid.UUID `json:"leadId"`
	ServiceID     uuid.UUID `json:"serviceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	Force         bool      `json:"force,omitempty"`
	AppointmentID uuid.UUID `json:"appointmentId,omitempty"` // for auditor visit-report audits
	Fingerprint   string    `json:"fingerprint,omitempty"`   // semantic dedup fingerprint (stripped before queueing gatekeeper)
}

// TaskName returns the scheduler task name for all agent runs.
const TaskAgentRun = "agent:run"
