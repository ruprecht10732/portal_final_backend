package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"

	apptools "portal_final_backend/internal/tools"
)

// ToolDependencies contains the dependencies needed by tools

// applyRBACToolsets wraps the toolsets with an RBAC predicate based on the request's ToolDependencies.
func createUpdatePipelineStageTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewUpdatePipelineStageTool(func(ctx tool.Context, input UpdatePipelineStageInput) (UpdatePipelineStageOutput, error) {
		deps, err := GetDependencies(ctx)
		if err != nil {
			return UpdatePipelineStageOutput{}, err
		}
		return handleUpdatePipelineStage(ctx, deps, input)
	})
}

func createSaveAnalysisTool() (tool.Tool, error) {
	return apptools.NewSaveAnalysisTool(func(ctx tool.Context, input SaveAnalysisInput) (SaveAnalysisOutput, error) {
		deps, err := GetDependencies(ctx)
		if err != nil {
			return SaveAnalysisOutput{}, err
		}
		return handleSaveAnalysis(ctx, deps, input)
	})
}

func createUpdateLeadServiceTypeTool() (tool.Tool, error) {
	return apptools.NewUpdateLeadServiceTypeTool(func(ctx tool.Context, input UpdateLeadServiceTypeInput) (UpdateLeadServiceTypeOutput, error) {
		deps, err := GetDependencies(ctx)
		if err != nil {
			return UpdateLeadServiceTypeOutput{}, err
		}
		return handleUpdateLeadServiceType(ctx, deps, input)
	})
}

func createUpdateLeadDetailsTool(description string) (tool.Tool, error) {
	return apptools.NewUpdateLeadDetailsTool(description, func(ctx tool.Context, input UpdateLeadDetailsInput) (UpdateLeadDetailsOutput, error) {
		deps, err := GetDependencies(ctx)
		if err != nil {
			return UpdateLeadDetailsOutput{}, err
		}
		return handleUpdateLeadDetails(ctx, deps, input)
	})
}

func latestAnalysisInvariantInputs(ctx context.Context, deps *ToolDependencies, serviceID, tenantID uuid.UUID) (string, []string) {
	if analysis, err := deps.Repo.GetLatestAIAnalysis(ctx, serviceID, tenantID); err == nil {
		return analysis.RecommendedAction, analysis.MissingInformation
	}
	analysisMeta := deps.GetLastAnalysisMetadata()
	if analysisMeta == nil {
		return "", nil
	}
	recommendedAction := strings.TrimSpace(fmt.Sprint(analysisMeta["recommendedAction"]))
	return recommendedAction, parseMissingInformationMetadata(analysisMeta["missingInformation"])
}

func parseMissingInformationMetadata(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		return stringifyAnySlice(typed)
	default:
		return nil
	}
}

func stringifyAnySlice(items []any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, strings.TrimSpace(fmt.Sprint(item)))
	}
	return out
}
