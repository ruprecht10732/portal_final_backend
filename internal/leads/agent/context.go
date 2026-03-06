package agent

import "context"

type ctxKey struct{}

// WithDependencies returns a child context carrying a request-scoped ToolDependencies.
func WithDependencies(ctx context.Context, deps *ToolDependencies) context.Context {
	return context.WithValue(ctx, ctxKey{}, deps)
}

// GetDependencies extracts the request-scoped ToolDependencies from ctx.
// Panics when the value is missing (indicates a programming error).
func GetDependencies(ctx context.Context) *ToolDependencies {
	deps, ok := ctx.Value(ctxKey{}).(*ToolDependencies)
	if !ok || deps == nil {
		panic("agent: ToolDependencies not found in context — did you forget WithDependencies?")
	}
	return deps
}

type photoAnalyzerDepsKey struct{}

func WithPhotoAnalyzerDeps(ctx context.Context, deps *PhotoAnalyzerDeps) context.Context {
	return context.WithValue(ctx, photoAnalyzerDepsKey{}, deps)
}

func GetPhotoAnalyzerDeps(ctx context.Context) *PhotoAnalyzerDeps {
	deps, ok := ctx.Value(photoAnalyzerDepsKey{}).(*PhotoAnalyzerDeps)
	if !ok || deps == nil {
		panic("agent: PhotoAnalyzerDeps not found in context")
	}
	return deps
}

type auditorDepsKey struct{}

func WithAuditorDeps(ctx context.Context, deps *AuditorToolDeps) context.Context {
	return context.WithValue(ctx, auditorDepsKey{}, deps)
}

func GetAuditorDeps(ctx context.Context) *AuditorToolDeps {
	deps, ok := ctx.Value(auditorDepsKey{}).(*AuditorToolDeps)
	if !ok || deps == nil {
		panic("agent: AuditorToolDeps not found in context")
	}
	return deps
}

type callLoggerDepsKey struct{}

func WithCallLoggerDeps(ctx context.Context, deps *CallLoggerToolDeps) context.Context {
	return context.WithValue(ctx, callLoggerDepsKey{}, deps)
}

func GetCallLoggerDeps(ctx context.Context) *CallLoggerToolDeps {
	deps, ok := ctx.Value(callLoggerDepsKey{}).(*CallLoggerToolDeps)
	if !ok || deps == nil {
		panic("agent: CallLoggerToolDeps not found in context")
	}
	return deps
}
