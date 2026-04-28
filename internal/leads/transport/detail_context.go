package transport

import (
	appointmentstransport "portal_final_backend/internal/appointments/transport"
	quotestransport "portal_final_backend/internal/quotes/transport"
)

type LeadDetailAnalysisContext struct {
	Analysis  *AIAnalysisResponse `json:"analysis,omitempty"`
	IsDefault bool                `json:"isDefault"`
}

type LeadDetailWorkflowOverrideContext struct {
	WorkflowID   *string `json:"workflowId,omitempty"`
	OverrideMode string  `json:"overrideMode"`
}

type LeadDetailWorkflowResolutionContext struct {
	WorkflowID       *string `json:"workflowId,omitempty"`
	WorkflowName     *string `json:"workflowName,omitempty"`
	ResolutionSource string  `json:"resolutionSource"`
	OverrideMode     *string `json:"overrideMode,omitempty"`
	MatchedRuleID    *string `json:"matchedRuleId,omitempty"`
}

type LeadDetailWorkflowContext struct {
	Override *LeadDetailWorkflowOverrideContext   `json:"override,omitempty"`
	Resolved *LeadDetailWorkflowResolutionContext `json:"resolved,omitempty"`
}

type LeadDetailContextResponse struct {
	Lead                        LeadResponse                                `json:"lead"`
	Notes                       []LeadNoteResponse                          `json:"notes"`
	Appointments                []appointmentstransport.AppointmentResponse `json:"appointments"`
	Quotes                      []quotestransport.QuoteResponse             `json:"quotes"`
	Communications              LeadInboxCommunicationsResponse             `json:"communications"`
	Workflow                    *LeadDetailWorkflowContext                  `json:"workflow,omitempty"`
	CurrentServiceAnalysis      *LeadDetailAnalysisContext                  `json:"currentServiceAnalysis,omitempty"`
}
