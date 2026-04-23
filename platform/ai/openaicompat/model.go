package openaicompat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"net/http"
	"strings"
	"time"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

const (
	// httpRequestTimeout is the per-request timeout for LLM API calls.
	// Reasoning models (deepseek-reasoner, kimi-k2.5) typically complete
	// gatekeeper calls in <30s, but during peak times or complex reasoning
	// they can exceed 50s.
	httpRequestTimeout = 120 * time.Second
)

// Config for an OpenAI-compatible LLM provider (Kimi, DeepSeek, etc.).
type Config struct {
	APIKey          string
	BaseURL         string
	Model           string
	Provider        string // "kimi" or "deepseek" — controls thinking-mode payload
	DisableThinking bool   // For Kimi: toggles thinking payload. For DeepSeek: ignored (reasoning via model name).
	SupportsVision  bool   // Whether this provider accepts image_url content parts.
}

// Model adapts an OpenAI-compatible provider to the ADK model.LLM interface.
type Model struct {
	config Config
	client *http.Client
}

func NewModel(cfg Config) *Model {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.moonshot.ai/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "kimi-k2-turbo-preview"
	}
	if cfg.Provider == "" {
		cfg.Provider = "kimi"
	}
	return &Model{
		config: cfg,
		client: &http.Client{Timeout: httpRequestTimeout},
	}
}

func (m *Model) Name() string {
	return m.config.Model
}

func (m *Model) ProviderName() string {
	return m.config.Provider
}

// GenerateContent adapts ADK requests to an OpenAI-compatible chat completions API.
func (m *Model) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		resp, err := m.generate(ctx, req)
		yield(resp, err)
	}
}

// openAIMessage represents a message in OpenAI/Kimi API format
// Content can be either a string (text-only) or array of content parts (multimodal)
type openAIMessage struct {
	Role             string           `json:"role"`
	Content          interface{}      `json:"content,omitempty"` // string or []contentPart for multimodal
	ToolCalls        []openAIToolCall `json:"tool_calls,omitempty"`
	Name             string           `json:"name,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
}

// contentPart represents a part of multimodal content (text or image)
type contentPart struct {
	Type     string    `json:"type"`                // "text" or "image_url"
	Text     string    `json:"text,omitempty"`      // for type="text"
	ImageURL *imageURL `json:"image_url,omitempty"` // for type="image_url"
}

// imageURL contains the URL for an image (base64 data URL for Kimi)
type imageURL struct {
	URL string `json:"url"` // "data:image/png;base64,..." format
}

type openAIToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function openAIToolCallDetail `json:"function"`
}

type openAIToolCallDetail struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIToolDef struct {
	Type     string            `json:"type"`
	Function openAIToolDefFunc `json:"function"`
}

type openAIToolDefFunc struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type openAIUsage struct {
	PromptTokens     int32 `json:"prompt_tokens"`
	CompletionTokens int32 `json:"completion_tokens"`
	TotalTokens      int32 `json:"total_tokens"`
}

type openAIResponse struct {
	Choices []struct {
		Message openAIChoiceMessage `json:"message"`
	} `json:"choices"`
	Usage *openAIUsage  `json:"usage,omitempty"`
	Error interface{}   `json:"error"`
}

type openAIChoiceMessage struct {
	Role             string           `json:"role"`
	Content          string           `json:"content"`
	ToolCalls        []openAIToolCall `json:"tool_calls"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
}

func (m *Model) generate(ctx context.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	payload := m.buildPayload(req)

	result, err := m.doRequest(ctx, payload)
	if err != nil {
		return nil, err
	}

	choice := result.Choices[0].Message
	parts := buildResponseParts(choice)

	if len(parts) == 0 {
		return nil, fmt.Errorf("llm api error (%s): model returned empty response (no content, no tool calls)", m.config.Provider)
	}

	resp := &model.LLMResponse{
		Content: &genai.Content{
			Role:  genai.RoleModel,
			Parts: parts,
		},
	}
	if result.Usage != nil {
		resp.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     result.Usage.PromptTokens,
			CandidatesTokenCount: result.Usage.CompletionTokens,
			TotalTokenCount:      result.Usage.TotalTokens,
		}
	}
	return resp, nil
}

func (m *Model) buildPayload(req *model.LLMRequest) map[string]interface{} {
	messages := m.convertMessages(req.Contents)
	if len(messages) == 0 {
		log.Printf("llm api warning (%s): messages array is empty after conversion; req.Contents had %d items", m.config.Provider, len(req.Contents))
	}

	// Prepend system instruction if present. The ADK stores agent instructions
	// in req.Config.SystemInstruction, but OpenAI-compatible APIs expect them
	// as a "system" message.
	if req.Config != nil && req.Config.SystemInstruction != nil {
		sysMsg := m.convertSystemInstruction(req.Config.SystemInstruction)
		if sysMsg != nil {
			messages = append([]openAIMessage{*sysMsg}, messages...)
		}
	}

	tools := m.convertTools(req)

	payload := map[string]interface{}{
		"model":    m.config.Model,
		"messages": messages,
	}

	// Thinking mode is provider-specific:
	// - Kimi: uses a "thinking" payload field to toggle reasoning on the same model.
	// - DeepSeek: reasoning is selected via model name (deepseek-reasoner), no payload field needed.
	if m.config.Provider == "kimi" {
		if m.config.DisableThinking {
			payload["thinking"] = map[string]string{"type": "disabled"}
		} else if req.Config != nil && req.Config.Temperature != nil {
			payload["temperature"] = float64(*req.Config.Temperature)
		}
	}

	if len(tools) > 0 {
		payload["tools"] = tools
		// Enforce strict tool sandboxing: the model may only call the provided
		// tools (equivalent to FunctionCallingConfigModeAny / VALIDATED).
		payload["tool_choice"] = "auto"
	}

	return payload
}

func (m *Model) doRequest(ctx context.Context, payload map[string]interface{}) (*openAIResponse, error) {
	jsonBody, _ := json.Marshal(payload)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", m.config.BaseURL+"/chat/completions", bytes.NewBuffer(jsonBody))
	httpReq.Header.Set("Authorization", "Bearer "+m.config.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	result, err := m.decodeResponse(resp)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (m *Model) decodeResponse(resp *http.Response) (*openAIResponse, error) {
	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("llm api error (%s): failed to decode response: %v", m.config.Provider, err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("llm api error (%s): %v", m.config.Provider, result.Error)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("llm api error (%s): empty choices", m.config.Provider)
	}
	return &result, nil
}

func buildResponseParts(choice openAIChoiceMessage) []*genai.Part {
	parts := make([]*genai.Part, 0, 2+len(choice.ToolCalls))
	if strings.TrimSpace(choice.ReasoningContent) != "" {
		parts = append(parts, &genai.Part{Text: choice.ReasoningContent, Thought: true})
	}
	if strings.TrimSpace(choice.Content) != "" {
		parts = append(parts, genai.NewPartFromText(choice.Content))
	}
	for _, tc := range choice.ToolCalls {
		if strings.TrimSpace(tc.Function.Name) == "" {
			log.Printf("llm: skipping tool call with empty name (id=%s)", tc.ID)
			continue
		}
		parts = append(parts, buildToolCallPart(tc))
	}
	return parts
}

func buildToolCallPart(tc openAIToolCall) *genai.Part {
	args := map[string]any{}
	if tc.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			log.Printf("llm: tool call %q has unparseable arguments, falling back to raw string (id=%s)", tc.Function.Name, tc.ID)
			args = map[string]any{"_raw": tc.Function.Arguments}
		}
	}
	return &genai.Part{
		FunctionCall: &genai.FunctionCall{
			ID:   tc.ID,
			Name: tc.Function.Name,
			Args: args,
		},
	}
}

func (m *Model) convertMessages(contents []*genai.Content) []openAIMessage {
	messages := make([]openAIMessage, 0, len(contents))
	for _, content := range contents {
		if content == nil {
			continue
		}

		role := roleForContent(content.Role)
		contentBody, reasoningContent, toolCalls, toolMessages := m.extractContentMessages(content)
		messages = append(messages, toolMessages...)

		// Check if we have content to add
		hasMessageBody := strings.TrimSpace(reasoningContent) != ""
		switch v := contentBody.(type) {
		case string:
			hasMessageBody = hasMessageBody || v != ""
		case []contentPart:
			hasMessageBody = hasMessageBody || len(v) > 0
		}

		if hasMessageBody || len(toolCalls) > 0 {
			messages = append(messages, openAIMessage{
				Role:             role,
				Content:          contentBody,
				ToolCalls:        toolCalls,
				ReasoningContent: reasoningContent,
			})
		}
	}
	return messages
}

func roleForContent(role string) string {
	if role == "model" {
		return "assistant"
	}
	return "user"
}

// convertSystemInstruction converts a genai system instruction content to an
// OpenAI-compatible system message.
func (m *Model) convertSystemInstruction(content *genai.Content) *openAIMessage {
	if content == nil || len(content.Parts) == 0 {
		return nil
	}
	var sb strings.Builder
	for _, part := range content.Parts {
		if part == nil {
			continue
		}
		if strings.TrimSpace(part.Text) != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(part.Text)
		}
	}
	text := strings.TrimSpace(sb.String())
	if text == "" {
		return nil
	}
	return &openAIMessage{
		Role:    "system",
		Content: text,
	}
}

// extractor holds state for extracting content messages from genai parts.
type extractor struct {
	toolCalls        []openAIToolCall
	toolMessages     []openAIMessage
	textBuilder      strings.Builder
	reasoningBuilder strings.Builder
	imageParts       []contentPart
	hasImages        bool
	model            *Model
}

// processToolResponsePart handles function response parts.
func (e *extractor) processToolResponsePart(part *genai.Part) bool {
	if part.FunctionResponse == nil {
		return false
	}
	msg := openAIMessage{
		Role:       "tool",
		ToolCallID: part.FunctionResponse.ID,
		Name:       part.FunctionResponse.Name,
	}
	if part.FunctionResponse.Response != nil {
		payload, _ := json.Marshal(part.FunctionResponse.Response)
		msg.Content = string(payload)
	}
	e.toolMessages = append(e.toolMessages, msg)
	return true
}

// processToolCallPart handles function call parts.
func (e *extractor) processToolCallPart(part *genai.Part) bool {
	if part.FunctionCall == nil {
		return false
	}
	args, _ := json.Marshal(part.FunctionCall.Args)
	e.toolCalls = append(e.toolCalls, openAIToolCall{
		ID:   part.FunctionCall.ID,
		Type: "function",
		Function: openAIToolCallDetail{
			Name:      part.FunctionCall.Name,
			Arguments: string(args),
		},
	})
	return true
}

// processThoughtPart handles thought/reasoning parts.
func (e *extractor) processThoughtPart(part *genai.Part) bool {
	if !part.Thought {
		return false
	}
	appendText(&e.reasoningBuilder, part.Text)
	return true
}

// processImagePart handles inline image data parts.
func (e *extractor) processImagePart(part *genai.Part) bool {
	if part.InlineData == nil || !strings.HasPrefix(part.InlineData.MIMEType, "image/") {
		return false
	}
	// Safety guard: skip image parts for providers that don't
	// support multimodal input to avoid deserialization errors.
	if !e.model.config.SupportsVision {
		return true
	}
	e.hasImages = true
	dataURL := fmt.Sprintf("data:%s;base64,%s",
		part.InlineData.MIMEType,
		base64.StdEncoding.EncodeToString(part.InlineData.Data))
	e.imageParts = append(e.imageParts, contentPart{
		Type:     "image_url",
		ImageURL: &imageURL{URL: dataURL},
	})
	return true
}

// processTextPart handles plain text parts.
func (e *extractor) processTextPart(part *genai.Part) {
	appendText(&e.textBuilder, part.Text)
}

// result builds and returns the extraction result.
func (e *extractor) result() (interface{}, string, []openAIToolCall, []openAIMessage) {
	text := strings.TrimSpace(e.textBuilder.String())
	reasoning := strings.TrimSpace(e.reasoningBuilder.String())

	if e.hasImages {
		return e.buildMultimodalResult(text), reasoning, e.toolCalls, e.toolMessages
	}
	return text, reasoning, e.toolCalls, e.toolMessages
}

// buildMultimodalResult constructs the content part array for multimodal output.
func (e *extractor) buildMultimodalResult(text string) []contentPart {
	parts := make([]contentPart, 0, len(e.imageParts)+1)
	parts = append(parts, e.imageParts...)
	if text != "" {
		parts = append(parts, contentPart{Type: "text", Text: text})
	}
	return parts
}

func (m *Model) extractContentMessages(content *genai.Content) (interface{}, string, []openAIToolCall, []openAIMessage) {
	e := &extractor{model: m}

	for _, part := range content.Parts {
		if part == nil {
			continue
		}
		if e.processToolResponsePart(part) {
			continue
		}
		if e.processToolCallPart(part) {
			continue
		}
		if e.processThoughtPart(part) {
			continue
		}
		if e.processImagePart(part) {
			continue
		}
		e.processTextPart(part)
	}

	return e.result()
}

func buildToolResponseMessage(part *genai.Part) (openAIMessage, bool) {
	if part.FunctionResponse == nil {
		return openAIMessage{}, false
	}
	payload, _ := json.Marshal(part.FunctionResponse.Response)
	return openAIMessage{
		Role:       "tool",
		ToolCallID: part.FunctionResponse.ID,
		Content:    string(payload),
		Name:       part.FunctionResponse.Name,
	}, true
}

func buildToolCall(part *genai.Part) (openAIToolCall, bool) {
	if part.FunctionCall == nil {
		return openAIToolCall{}, false
	}
	args, _ := json.Marshal(part.FunctionCall.Args)
	return openAIToolCall{
		ID:   part.FunctionCall.ID,
		Type: "function",
		Function: openAIToolCallDetail{
			Name:      part.FunctionCall.Name,
			Arguments: string(args),
		},
	}, true
}

func appendText(builder *strings.Builder, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	if builder.Len() > 0 {
		builder.WriteString("\n")
	}
	builder.WriteString(text)
}

func (m *Model) convertTools(req *model.LLMRequest) []openAIToolDef {
	if req == nil || req.Config == nil || len(req.Config.Tools) == 0 {
		return nil
	}

	var tools []openAIToolDef
	for _, gt := range req.Config.Tools {
		if gt == nil || gt.FunctionDeclarations == nil {
			continue
		}
		for _, decl := range gt.FunctionDeclarations {
			if decl == nil || decl.Name == "" {
				continue
			}
			var params interface{}
			switch {
			case decl.ParametersJsonSchema != nil:
				params = decl.ParametersJsonSchema
			case decl.Parameters != nil:
				params = decl.Parameters
			}
			tools = append(tools, openAIToolDef{
				Type: "function",
				Function: openAIToolDefFunc{
					Name:        decl.Name,
					Description: decl.Description,
					Parameters:  params,
				},
			})
		}
	}

	return tools
}
