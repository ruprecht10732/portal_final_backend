package agent

import (
	"fmt"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"

	"portal_final_backend/internal/orchestration"
	"portal_final_backend/platform/ai/openaicompat"
)

// AgentKit holds the reusable ADK components for an eagerly-built agent.
type AgentKit struct {
	Agent       agent.Agent
	Runner      *runner.Runner
	Workspace   orchestration.Workspace
	Instruction string
}

// BuildAgentKit creates the ADK agent and runner from a workspace definition.
// This is the canonical path for agents that are fully constructed at init time.
func BuildAgentKit(name, description, workspace, appName string, llm model.LLM, sessionService session.Service, tools []tool.Tool, extraPrompts ...string) (*AgentKit, error) {
	ws, err := orchestration.LoadAgentWorkspace(workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s workspace: %w", workspace, err)
	}

	instruction, err := orchestration.BuildAgentInstruction(workspace, extraPrompts...)
	if err != nil {
		return nil, fmt.Errorf("failed to build %s instruction: %w", workspace, err)
	}

	var toolsets []tool.Toolset
	if len(tools) > 0 {
		toolsets = orchestration.BuildWorkspaceToolsets(ws, appName+"_tools", tools)
		toolsets = applyRBACToolsets(toolsets)
	}

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        name,
		Model:       llm,
		Description: description,
		Instruction: ws.Instruction,
		Toolsets:    toolsets,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create %s agent: %w", name, err)
	}

	r, err := runner.New(runner.Config{
		AppName:        appName,
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create %s runner: %w", name, err)
	}

	return &AgentKit{
		Agent:       adkAgent,
		Runner:      r,
		Workspace:   ws,
		Instruction: instruction,
	}, nil
}

// LazyRunnerBuilder holds configuration for agents that build a fresh runner
// on every request (e.g. reply agents, re-engagement agents).
type LazyRunnerBuilder struct {
	Name         string
	Description  string
	Workspace    string
	AppName      string
	ModelConfig  openaicompat.Config
	SessionSvc   session.Service
	ExtraPrompts []string

	instruction string
	ready       bool
}

// NewLazyRunnerBuilder creates a lazy builder that loads workspace metadata
// immediately but defers LLM and runner construction until the first request.
func NewLazyRunnerBuilder(name, description, workspace, appName string, modelConfig openaicompat.Config, sessionService session.Service, extraPrompts ...string) (*LazyRunnerBuilder, error) {
	instruction, err := orchestration.BuildAgentInstruction(workspace, extraPrompts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s workspace: %w", workspace, err)
	}

	return &LazyRunnerBuilder{
		Name:         name,
		Description:  description,
		Workspace:    workspace,
		AppName:      appName,
		ModelConfig:  modelConfig,
		SessionSvc:   sessionService,
		ExtraPrompts: extraPrompts,
		instruction:  instruction,
		ready:        true,
	}, nil
}

// Runner creates a fresh ADK agent and runner for a single request.
func (b *LazyRunnerBuilder) Runner() (*runner.Runner, error) {
	if !b.ready {
		return nil, fmt.Errorf("lazy runner builder for %s not initialized", b.Name)
	}

	llm := openaicompat.NewModel(b.ModelConfig)
	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        b.Name,
		Model:       llm,
		Description: b.Description,
		Instruction: b.instruction,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create %s agent: %w", b.Name, err)
	}

	r, err := runner.New(runner.Config{
		AppName:        b.AppName,
		Agent:          adkAgent,
		SessionService: b.SessionSvc,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create %s runner: %w", b.Name, err)
	}

	return r, nil
}

// Instruction returns the compiled workspace instruction.
func (b *LazyRunnerBuilder) Instruction() string {
	return b.instruction
}

// buildToolsFrom runs a list of tool creators and collects the results.
// It stops at the first error, returning nil tools so callers fail fast.
func buildToolsFrom(creators ...func() (tool.Tool, error)) ([]tool.Tool, error) {
	tools := make([]tool.Tool, 0, len(creators))
	for _, create := range creators {
		t, err := create()
		if err != nil {
			return nil, err
		}
		tools = append(tools, t)
	}
	return tools, nil
}
