package orchestration

import (
	"sync"

	"google.golang.org/adk/tool"
	"portal_final_backend/internal/orchestration/skills"
)

var (
	skillLoader     *skills.ResourceLoader
	skillLoaderOnce sync.Once
)

// InitSkillLoader initializes the global skill resource loader from the
// workspace root directory. Safe to call multiple times; only the first call
// has effect.
func InitSkillLoader() error {
	var err error
	skillLoaderOnce.Do(func() {
		rootDir, e := resolveAgentWorkspaceRoot()
		if e != nil {
			err = e
			return
		}
		skillLoader = skills.NewResourceLoader(rootDir)
	})
	return err
}

// MustInitSkillLoader panics if the skill loader cannot be initialized.
func MustInitSkillLoader() {
	if err := InitSkillLoader(); err != nil {
		panic("failed to initialize skill loader: " + err.Error())
	}
}

// skillTools returns the on-demand skill loading tools if the loader is ready.
func skillTools() []tool.Tool {
	if skillLoader == nil {
		return nil
	}
	loadTool, err := skills.NewLoadSkillResourceTool(skillLoader)
	if err != nil {
		return nil
	}
	listTool, err := skills.NewListSkillResourcesTool(skillLoader)
	if err != nil {
		return nil
	}
	return []tool.Tool{loadTool, listTool}
}
