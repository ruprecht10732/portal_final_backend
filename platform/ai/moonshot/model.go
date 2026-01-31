package moonshot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// Config for Kimi
type Config struct {
	APIKey          string
	BaseURL         string
	Model           string
	DisableThinking bool // Disable thinking mode for kimi-k2.5 (uses temp 0.6 instead of 1.0)
}

// KimiModel adapts Moonshot to the ADK model.LLM interface
type KimiModel struct {
	config Config
	client *http.Client
}

func NewModel(cfg Config) *KimiModel {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.moonshot.ai/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "kimi-k2-turbo-preview"
	}
	return &KimiModel{
		config: cfg,
		client: &http.Client{},
	}
}

func (m *KimiModel) Name() string {
	return m.config.Model
}

// GenerateContent adapts ADK requests to Kimi's OpenAI-compatible API
func (m *KimiModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		resp, err := m.generate(ctx, req)
		yield(resp, err)
	}
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	Name       string           `json:"name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
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

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Role      string           `json:"role"`
			Content   string           `json:"content"`
			ToolCalls []openAIToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Error interface{} `json:"error"`
}

func (m *KimiModel) generate(ctx context.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	messages := m.convertMessages(req.Contents)
	tools := m.convertTools(req)

	payload := map[string]interface{}{
		"model":    m.config.Model,
		"messages": messages,
	}

	// Handle thinking mode for kimi-k2.5
	if m.config.DisableThinking {
		payload["thinking"] = map[string]string{"type": "disabled"}
		// Non-thinking mode uses fixed temperature 0.6
	} else if req.Config != nil && req.Config.Temperature != nil {
		payload["temperature"] = float64(*req.Config.Temperature)
	}

	if len(tools) > 0 {
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}

	jsonBody, _ := json.Marshal(payload)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", m.config.BaseURL+"/chat/completions", bytes.NewBuffer(jsonBody))
	httpReq.Header.Set("Authorization", "Bearer "+m.config.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode kimi response: %v", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("kimi api error: %v", result.Error)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("kimi api error: empty choices")
	}

	choice := result.Choices[0].Message
	parts := make([]*genai.Part, 0, 1+len(choice.ToolCalls))
	if strings.TrimSpace(choice.Content) != "" {
		parts = append(parts, genai.NewPartFromText(choice.Content))
	}
	for _, tc := range choice.ToolCalls {
		args := map[string]any{}
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = map[string]any{"_raw": tc.Function.Arguments}
			}
		}
		parts = append(parts, &genai.Part{
			FunctionCall: &genai.FunctionCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: args,
			},
		})
	}

	return &model.LLMResponse{
		Content: &genai.Content{
			Role:  genai.RoleModel,
			Parts: parts,
		},
	}, nil
}

func (m *KimiModel) convertMessages(contents []*genai.Content) []openAIMessage {
	messages := make([]openAIMessage, 0, len(contents))
	for _, content := range contents {
		if content == nil {
			continue
		}

		role := roleForContent(content.Role)
		text, toolCalls, toolMessages := extractContentMessages(content)
		messages = append(messages, toolMessages...)
		if text != "" || len(toolCalls) > 0 {
			messages = append(messages, openAIMessage{
				Role:      role,
				Content:   text,
				ToolCalls: toolCalls,
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

func extractContentMessages(content *genai.Content) (string, []openAIToolCall, []openAIMessage) {
	var toolCalls []openAIToolCall
	var toolMessages []openAIMessage
	var textBuilder strings.Builder

	for _, part := range content.Parts {
		if part == nil {
			continue
		}
		if msg, ok := buildToolResponseMessage(part); ok {
			toolMessages = append(toolMessages, msg)
			continue
		}
		if call, ok := buildToolCall(part); ok {
			toolCalls = append(toolCalls, call)
			continue
		}
		appendText(&textBuilder, part.Text)
	}

	return strings.TrimSpace(textBuilder.String()), toolCalls, toolMessages
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

func (m *KimiModel) convertTools(req *model.LLMRequest) []openAIToolDef {
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
