package agent

import (
	"context"
	"fmt"
)

type ctxKey struct{}

// WithDependencies returns a child context carrying a request-scoped ToolDependencies.
func WithDependencies(ctx context.Context, deps *ToolDependencies) context.Context {
	return context.WithValue(ctx, ctxKey{}, deps)
}

// GetDependencies extracts the request-scoped ToolDependencies from ctx.
// Returns an error when the value is missing (indicates a programming error).
func GetDependencies(ctx context.Context) (*ToolDependencies, error) {
	deps, ok := ctx.Value(ctxKey{}).(*ToolDependencies)
	if !ok || deps == nil {
		return nil, fmt.Errorf("agent: ToolDependencies not found in context — did you forget WithDependencies?")
	}
	return deps, nil
}

type photoAnalyzerDepsKey struct{}

func WithPhotoAnalyzerDeps(ctx context.Context, deps *PhotoAnalyzerDeps) context.Context {
	return context.WithValue(ctx, photoAnalyzerDepsKey{}, deps)
}

func GetPhotoAnalyzerDeps(ctx context.Context) (*PhotoAnalyzerDeps, error) {
	deps, ok := ctx.Value(photoAnalyzerDepsKey{}).(*PhotoAnalyzerDeps)
	if !ok || deps == nil {
		return nil, fmt.Errorf("agent: PhotoAnalyzerDeps not found in context")
	}
	return deps, nil
}

type auditorDepsKey struct{}

func WithAuditorDeps(ctx context.Context, deps *AuditorToolDeps) context.Context {
	return context.WithValue(ctx, auditorDepsKey{}, deps)
}

func GetAuditorDeps(ctx context.Context) (*AuditorToolDeps, error) {
	deps, ok := ctx.Value(auditorDepsKey{}).(*AuditorToolDeps)
	if !ok || deps == nil {
		return nil, fmt.Errorf("agent: AuditorToolDeps not found in context")
	}
	return deps, nil
}

type callLoggerDepsKey struct{}

func WithCallLoggerDeps(ctx context.Context, deps *CallLoggerToolDeps) context.Context {
	return context.WithValue(ctx, callLoggerDepsKey{}, deps)
}

func GetCallLoggerDeps(ctx context.Context) (*CallLoggerToolDeps, error) {
	deps, ok := ctx.Value(callLoggerDepsKey{}).(*CallLoggerToolDeps)
	if !ok || deps == nil {
		return nil, fmt.Errorf("agent: CallLoggerToolDeps not found in context")
	}
	return deps, nil
}
