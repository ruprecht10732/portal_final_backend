// Package otel provides OpenTelemetry instrumentation for agentic workflows.
package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// InstrumentationName is the name of the OpenTelemetry instrumentation library.
	InstrumentationName = "portal_final_backend/agentic"
	// InstrumentationVersion is the version of the instrumentation library.
	InstrumentationVersion = "1.0.0"
)

var (
	tracer = otel.Tracer(InstrumentationName, trace.WithInstrumentationVersion(InstrumentationVersion))
)

// AgentRunOptions configures the AgentRun span.
type AgentRunOptions struct {
	AgentName      string
	LeadID         string
	ServiceID      string
	TenantID       string
	RunID          string
	ModelUsed      string
	ReasoningMode  string
	SessionLabel   string
}

// StartAgentRun starts a new span for an agent run.
func StartAgentRun(ctx context.Context, opts AgentRunOptions) (context.Context, trace.Span) {
	ctx, span := tracer.Start(ctx, fmt.Sprintf("agent.run/%s", opts.AgentName),
		trace.WithAttributes(
			attribute.String("agent.name", opts.AgentName),
			attribute.String("agent.lead_id", opts.LeadID),
			attribute.String("agent.service_id", opts.ServiceID),
			attribute.String("agent.tenant_id", opts.TenantID),
			attribute.String("agent.run_id", opts.RunID),
			attribute.String("agent.model", opts.ModelUsed),
			attribute.String("agent.reasoning_mode", opts.ReasoningMode),
			attribute.String("agent.session_label", opts.SessionLabel),
		),
	)
	return ctx, span
}

// ToolCallOptions configures the ToolCall span.
type ToolCallOptions struct {
	ToolName   string
	RunID      string
	SequenceNum int
}

// StartToolCall starts a new span for a tool call.
func StartToolCall(ctx context.Context, opts ToolCallOptions) (context.Context, trace.Span) {
	ctx, span := tracer.Start(ctx, fmt.Sprintf("tool.call/%s", opts.ToolName),
		trace.WithAttributes(
			attribute.String("tool.name", opts.ToolName),
			attribute.String("tool.run_id", opts.RunID),
			attribute.Int("tool.sequence_num", opts.SequenceNum),
		),
	)
	return ctx, span
}

// LLMCallOptions configures the LLMCall span.
type LLMCallOptions struct {
	Model       string
	Provider    string
	PromptTokens  int32
	ResponseTokens int32
}

// StartLLMCall starts a new span for an LLM generation call.
func StartLLMCall(ctx context.Context, opts LLMCallOptions) (context.Context, trace.Span) {
	ctx, span := tracer.Start(ctx, "llm.generate",
		trace.WithAttributes(
			attribute.String("llm.model", opts.Model),
			attribute.String("llm.provider", opts.Provider),
		),
	)
	return ctx, span
}

// RecordLLMTokens records token usage on an LLM span.
func RecordLLMTokens(span trace.Span, promptTokens, responseTokens int32) {
	span.SetAttributes(
		attribute.Int64("llm.tokens.prompt", int64(promptTokens)),
		attribute.Int64("llm.tokens.response", int64(responseTokens)),
		attribute.Int64("llm.tokens.total", int64(promptTokens+responseTokens)),
	)
}

// RecordToolResult records the outcome of a tool call on its span.
func RecordToolResult(span trace.Span, result any, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "success")
		if result != nil {
			span.SetAttributes(attribute.String("tool.result_type", fmt.Sprintf("%T", result)))
		}
	}
}

// RecordAgentRunResult records the outcome of an agent run.
func RecordAgentRunResult(span trace.Span, outcome string, toolCallCount int, durationMs int) {
	span.SetAttributes(
		attribute.String("agent.outcome", outcome),
		attribute.Int("agent.tool_call_count", toolCallCount),
		attribute.Int("agent.duration_ms", durationMs),
	)
}

// PropagateTracer ensures that sub-agents inherit the parent tracer.
// Call this when spawning a sub-runner so that its internal spans are
// children of the parent agent run span.
func PropagateTracer(ctx context.Context) context.Context {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return ctx
	}
	return trace.ContextWithSpan(ctx, span)
}
