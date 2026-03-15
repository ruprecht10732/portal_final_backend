package transport

import (
	appointmentstransport "portal_final_backend/internal/appointments/transport"
	"portal_final_backend/internal/leads/repository"
	quotestransport "portal_final_backend/internal/quotes/transport"
)

type LeadDetailAnalysisContext struct {
	Analysis  *AIAnalysisResponse `json:"analysis,omitempty"`
	IsDefault bool                `json:"isDefault"`
}

type LeadDetailContextResponse struct {
	Lead                        LeadResponse                                `json:"lead"`
	Notes                       []LeadNoteResponse                          `json:"notes"`
	Appointments                []appointmentstransport.AppointmentResponse `json:"appointments"`
	Quotes                      []quotestransport.QuoteResponse             `json:"quotes"`
	Communications              LeadInboxCommunicationsResponse             `json:"communications"`
	CurrentServiceAnalysis      *LeadDetailAnalysisContext                  `json:"currentServiceAnalysis,omitempty"`
	CurrentServicePhotoAnalysis *repository.PhotoAnalysis                   `json:"currentServicePhotoAnalysis,omitempty"`
}
