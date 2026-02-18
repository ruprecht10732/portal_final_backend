package domain

const (
	// StageUnchanged is a sentinel indicating that a derivation function
	// intentionally does not prescribe a pipeline stage. The caller must
	// substitute the current stage of the service.
	StageUnchanged = ""

	PipelineStageTriage             = "Triage"
	PipelineStageNurturing          = "Nurturing"
	PipelineStageEstimation         = "Estimation"
	PipelineStageProposal           = "Proposal"
	PipelineStageFulfillment        = "Fulfillment"
	PipelineStageManualIntervention = "Manual_Intervention"
	PipelineStageCompleted          = "Completed"
	PipelineStageLost               = "Lost"
)

var knownPipelineStages = map[string]struct{}{
	PipelineStageTriage:             {},
	PipelineStageNurturing:          {},
	PipelineStageEstimation:         {},
	PipelineStageProposal:           {},
	PipelineStageFulfillment:        {},
	PipelineStageManualIntervention: {},
	PipelineStageCompleted:          {},
	PipelineStageLost:               {},
}

func IsKnownPipelineStage(stage string) bool {
	_, ok := knownPipelineStages[stage]
	return ok
}
