package moonshot

import (
	"encoding/json"
	"testing"

	"google.golang.org/genai"
)

func TestBuildResponsePartsPreservesReasoningContentForToolCalls(t *testing.T) {
	t.Parallel()

	parts := buildResponseParts(openAIChoiceMessage{
		Role:             "assistant",
		ReasoningContent: "Need to inspect the lead record before choosing the next action.",
		Content:          "I'll check the lead details now.",
		ToolCalls: []openAIToolCall{{
			ID:   "call_1",
			Type: "function",
			Function: openAIToolCallDetail{
				Name:      "LookupLead",
				Arguments: `{"lead_id":"123"}`,
			},
		}},
	})

	if len(parts) != 3 {
		t.Fatalf("expected reasoning, content, and tool call parts, got %d", len(parts))
	}
	if !parts[0].Thought {
		t.Fatal("expected first part to be marked as thought")
	}
	if parts[0].Text != "Need to inspect the lead record before choosing the next action." {
		t.Fatalf("unexpected reasoning part text: %q", parts[0].Text)
	}
	if parts[1].Thought {
		t.Fatal("expected content part to remain non-thought text")
	}
	if parts[1].Text != "I'll check the lead details now." {
		t.Fatalf("unexpected content part text: %q", parts[1].Text)
	}
	if parts[2].FunctionCall == nil {
		t.Fatal("expected final part to contain a tool call")
	}
	if parts[2].FunctionCall.Name != "LookupLead" {
		t.Fatalf("unexpected tool call name: %q", parts[2].FunctionCall.Name)
	}
}

func TestConvertMessagesIncludesReasoningContentForAssistantToolCalls(t *testing.T) {
	t.Parallel()

	model := NewModel(Config{APIKey: "test-key", Model: "kimi-k2.5"})
	messages := model.convertMessages([]*genai.Content{{
		Role: genai.RoleModel,
		Parts: []*genai.Part{
			{Text: "I should validate the address first.", Thought: true},
			{Text: "Checking the address now."},
			{FunctionCall: &genai.FunctionCall{ID: "call_2", Name: "ValidateAddress", Args: map[string]any{"postal_code": "1234AB"}}},
		},
	}})

	if len(messages) != 1 {
		t.Fatalf("expected a single assistant message, got %d", len(messages))
	}
	msg := messages[0]
	if msg.Role != "assistant" {
		t.Fatalf("expected assistant role, got %q", msg.Role)
	}
	if msg.ReasoningContent != "I should validate the address first." {
		t.Fatalf("unexpected reasoning content: %q", msg.ReasoningContent)
	}
	content, ok := msg.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", msg.Content)
	}
	if content != "Checking the address now." {
		t.Fatalf("unexpected content: %q", content)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].ID != "call_2" {
		t.Fatalf("unexpected tool call id: %q", msg.ToolCalls[0].ID)
	}
	if msg.ToolCalls[0].Function.Arguments != `{"postal_code":"1234AB"}` {
		t.Fatalf("unexpected tool call arguments: %s", msg.ToolCalls[0].Function.Arguments)
	}
}

func TestConvertMessagesPreservesToolResponsesAndMultimodalInputs(t *testing.T) {
	t.Parallel()

	model := NewModel(Config{APIKey: "test-key", Model: "kimi-k2.5"})
	messages := model.convertMessages([]*genai.Content{{
		Role: genai.RoleUser,
		Parts: []*genai.Part{
			{
				FunctionResponse: &genai.FunctionResponse{
					ID:       "call_3",
					Name:     "LookupLead",
					Response: map[string]any{"status": "ok"},
				},
			},
			{
				InlineData: &genai.Blob{
					MIMEType: "image/png",
					Data:     []byte{0x01, 0x02, 0x03},
				},
			},
			{Text: "Please analyze this photo."},
		},
	}})

	if len(messages) != 2 {
		t.Fatalf("expected tool message and multimodal user message, got %d", len(messages))
	}
	if messages[0].Role != "tool" {
		t.Fatalf("expected first message to be a tool response, got %q", messages[0].Role)
	}
	if messages[0].ToolCallID != "call_3" {
		t.Fatalf("unexpected tool call id: %q", messages[0].ToolCallID)
	}
	toolPayload, ok := messages[0].Content.(string)
	if !ok {
		t.Fatalf("expected serialized tool payload, got %T", messages[0].Content)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(toolPayload), &decoded); err != nil {
		t.Fatalf("failed to decode tool payload: %v", err)
	}
	if decoded["status"] != "ok" {
		t.Fatalf("unexpected tool payload: %#v", decoded)
	}
	if messages[1].Role != "user" {
		t.Fatalf("expected second message to be a user message, got %q", messages[1].Role)
	}
	parts, ok := messages[1].Content.([]contentPart)
	if !ok {
		t.Fatalf("expected multimodal content parts, got %T", messages[1].Content)
	}
	if len(parts) != 2 {
		t.Fatalf("expected image and text parts, got %d", len(parts))
	}
	if parts[0].Type != "image_url" || parts[0].ImageURL == nil {
		t.Fatalf("expected first part to be image_url, got %#v", parts[0])
	}
	if parts[1].Type != "text" || parts[1].Text != "Please analyze this photo." {
		t.Fatalf("unexpected text part: %#v", parts[1])
	}
	if messages[1].ReasoningContent != "" {
		t.Fatalf("expected user message to have no reasoning content, got %q", messages[1].ReasoningContent)
	}
	if len(messages[1].ToolCalls) != 0 {
		t.Fatalf("expected user multimodal message to have no tool calls, got %d", len(messages[1].ToolCalls))
	}
}
