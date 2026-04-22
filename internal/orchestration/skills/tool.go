package skills

import (
	"fmt"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// LoadSkillResourceArgs is the input schema for the load_skill_resource tool.
type LoadSkillResourceArgs struct {
	ResourcePath string `json:"resource_path" jsonschema:"The relative path of the resource to load (e.g., references/compliance-rules.md)"`
}

// LoadSkillResourceResult is the output of the load_skill_resource tool.
type LoadSkillResourceResult struct {
	Content string `json:"content"`
	Found   bool   `json:"found"`
}

// NewLoadSkillResourceTool creates a tool that dynamically loads skill resources
// on demand, implementing progressive context disclosure (L3 resources).
func NewLoadSkillResourceTool(loader *ResourceLoader) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "load_skill_resource",
		Description: "Loads a specific skill resource (documentation, template, or workflow) by its relative path. Use this when the current task requires deep context that is not in the base instructions. Example: load_skill_resource with resource_path=references/api-schema.md",
	}, func(ctx tool.Context, args LoadSkillResourceArgs) (LoadSkillResourceResult, error) {
		if args.ResourcePath == "" {
			return LoadSkillResourceResult{Found: false}, fmt.Errorf("resource_path is required")
		}
		content, err := loader.LoadResource(args.ResourcePath)
		if err != nil {
			return LoadSkillResourceResult{Found: false, Content: ""}, nil
		}
		return LoadSkillResourceResult{Found: true, Content: content}, nil
	})
}

// NewListSkillResourcesTool creates a tool that lists all available resources.
func NewListSkillResourcesTool(loader *ResourceLoader) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "list_skill_resources",
		Description: "Lists all available skill resources that can be loaded on demand. Use this to discover what deep-context documentation, templates, and workflows are available.",
	}, func(ctx tool.Context, _ struct{}) ([]ResourceDescriptor, error) {
		return loader.ListAvailableResources()
	})
}
