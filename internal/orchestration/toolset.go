package orchestration

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/tool"
)

type staticToolset struct {
	name  string
	tools []tool.Tool
}

func newStaticToolset(name string, tools []tool.Tool) tool.Toolset {
	cloned := make([]tool.Tool, len(tools))
	copy(cloned, tools)
	return &staticToolset{name: name, tools: cloned}
}

func (s *staticToolset) Name() string {
	return s.name
}

func (s *staticToolset) Tools(agent.ReadonlyContext) ([]tool.Tool, error) {
	cloned := make([]tool.Tool, len(s.tools))
	copy(cloned, s.tools)
	return cloned, nil
}

func BuildWorkspaceToolsets(workspace Workspace, name string, tools []tool.Tool) []tool.Toolset {
	// Always inject on-demand skill loading tools for progressive context disclosure.
	if st := skillTools(); len(st) > 0 {
		tools = append(tools, st...)
	}
	if len(tools) == 0 {
		return nil
	}
	toolset := newStaticToolset(name, tools)
	if len(workspace.AllowedTools) > 0 {
		toolset = tool.FilterToolset(toolset, tool.StringPredicate(workspace.AllowedTools))
	}
	return []tool.Toolset{toolset}
}
