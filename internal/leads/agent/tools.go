package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"

	apptools "portal_final_backend/internal/tools"
)

// withDeps wraps a handler that needs ToolDependencies so the wrapper
// automatically resolves dependencies from the tool context.
func withDeps[In any, Out any](fn func(tool.Context, *ToolDependencies, In) (Out, error)) func(tool.Context, In) (Out, error) {
	return func(ctx tool.Context, input In) (Out, error) {
		deps, err := GetDependencies(ctx)
		if err != nil {
			var zero Out
			return zero, err
		}
		return fn(ctx, deps, input)
	}
}

func createUpdatePipelineStageTool() (tool.Tool, error) {
	return apptools.NewUpdatePipelineStageTool(withDeps(handleUpdatePipelineStage))
}

func createSaveAnalysisTool() (tool.Tool, error) {
	return apptools.NewSaveAnalysisTool(withDeps(handleSaveAnalysis))
}

func createUpdateLeadServiceTypeTool() (tool.Tool, error) {
	return apptools.NewUpdateLeadServiceTypeTool(withDeps(handleUpdateLeadServiceType))
}

func createUpdateLeadDetailsTool(description string) (tool.Tool, error) {
	return apptools.NewUpdateLeadDetailsTool(description, withDeps(handleUpdateLeadDetails))
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
