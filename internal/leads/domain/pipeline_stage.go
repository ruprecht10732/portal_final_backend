package domain

const (
	PipelineStageTriage             = "Triage"
	PipelineStageNurturing          = "Nurturing"
	PipelineStageReadyForEstimator  = "Ready_For_Estimator"
	PipelineStageQuoteDraft         = "Quote_Draft"
	PipelineStageQuoteSent          = "Quote_Sent"
	PipelineStageReadyForPartner    = "Ready_For_Partner"
	PipelineStagePartnerMatching    = "Partner_Matching"
	PipelineStagePartnerAssigned    = "Partner_Assigned"
	PipelineStageManualIntervention = "Manual_Intervention"
	PipelineStageCompleted          = "Completed"
	PipelineStageLost               = "Lost"
)

var knownPipelineStages = map[string]struct{}{
	PipelineStageTriage:             {},
	PipelineStageNurturing:          {},
	PipelineStageReadyForEstimator:  {},
	PipelineStageQuoteDraft:         {},
	PipelineStageQuoteSent:          {},
	PipelineStageReadyForPartner:    {},
	PipelineStagePartnerMatching:    {},
	PipelineStagePartnerAssigned:    {},
	PipelineStageManualIntervention: {},
	PipelineStageCompleted:          {},
	PipelineStageLost:               {},
}

func IsKnownPipelineStage(stage string) bool {
	_, ok := knownPipelineStages[stage]
	return ok
}
