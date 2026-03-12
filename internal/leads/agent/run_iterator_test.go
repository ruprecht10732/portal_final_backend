package agent

import (
	"errors"
	"iter"
	"strings"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestConsumeRunEventsReturnsWrappedIteratorError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("moonshot unauthorized")
	err := consumeRunEvents(iter.Seq2[int, error](func(yield func(int, error) bool) {
		yield(0, sentinel)
	}), "gatekeeper run failed", nil)
	if err == nil {
		t.Fatal("expected iterator error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
	if !strings.Contains(err.Error(), "gatekeeper run failed") {
		t.Fatalf("expected wrapped error to contain context, got %q", err.Error())
	}
}

func TestConsumeRunEventsProcessesEventsUntilIteratorError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("moonshot timeout")
	handled := make([]int, 0, 2)
	err := consumeRunEvents(iter.Seq2[int, error](func(yield func(int, error) bool) {
		if !yield(1, nil) {
			return
		}
		if !yield(2, nil) {
			return
		}
		yield(0, sentinel)
	}), "dispatcher run failed", func(event int) {
		handled = append(handled, event)
	})
	if err == nil {
		t.Fatal("expected iterator error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
	if len(handled) != 2 || handled[0] != 1 || handled[1] != 2 {
		t.Fatalf("expected processed events [1 2], got %v", handled)
	}
}

func TestConsumeRunEventsAllowsNilHandler(t *testing.T) {
	t.Parallel()

	err := consumeRunEvents(iter.Seq2[int, error](func(yield func(int, error) bool) {
		if !yield(1, nil) {
			return
		}
		yield(2, nil)
	}), "auditor run failed", nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestExtractSessionToolTraceCollectsCallsAndResponses(t *testing.T) {
	t.Parallel()

	event := &session.Event{LLMResponse: model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{
		genai.NewPartFromFunctionCall("SaveAnalysis", map[string]any{"summary": "ok", "leadQuality": "hot"}),
		genai.NewPartFromFunctionResponse("SaveAnalysis", map[string]any{"output": "saved"}),
		genai.NewPartFromFunctionResponse("UpdatePipelineStage", map[string]any{"error": "blocked"}),
	}}}}

	trace := extractSessionToolTrace(event)
	if len(trace) != 3 {
		t.Fatalf("expected 3 trace items, got %d", len(trace))
	}
	if trace[0].Kind != "call" || trace[0].Name != "SaveAnalysis" {
		t.Fatalf("expected first trace item to be SaveAnalysis call, got %#v", trace[0])
	}
	if trace[1].Kind != "response" || trace[1].Name != "SaveAnalysis" || trace[1].HasError {
		t.Fatalf("expected second trace item to be successful SaveAnalysis response, got %#v", trace[1])
	}
	if trace[2].Kind != "response" || trace[2].Name != "UpdatePipelineStage" || !trace[2].HasError {
		t.Fatalf("expected third trace item to be failed UpdatePipelineStage response, got %#v", trace[2])
	}
}
