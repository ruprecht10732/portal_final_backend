package agents

import (
	"embed"
	"fmt"
)

//go:embed calculator/prompts/*.md
//go:embed gatekeeper/prompts/*.md
//go:embed matchmaker/prompts/*.md
//go:embed shared/prompts/*.md
//go:embed subsidy-analyzer/prompts/*.md
//go:embed support/call_logger/prompts/*.md
//go:embed support/email_reply/prompts/*.md
//go:embed support/offer_summary/prompts/*.md
//go:embed support/photo_analyzer/prompts/*.md
//go:embed support/whatsapp_agent/prompts/*.md
//go:embed support/whatsapp_partner_agent/prompts/*.md
//go:embed support/whatsapp_reply/prompts/*.md
var promptFS embed.FS

// ReadPromptFile returns the contents of an embedded prompt markdown file.
// The path is relative to the agents directory (e.g. "calculator/prompts/scope_analyzer.md").
func ReadPromptFile(path string) (string, error) {
	content, err := promptFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read embedded prompt %s: %w", path, err)
	}
	return string(content), nil
}

// MustReadPromptFile panics if the embedded prompt file cannot be read.
// This is safe for compile-time-known paths.
func MustReadPromptFile(path string) string {
	content, err := ReadPromptFile(path)
	if err != nil {
		panic(err)
	}
	return string(content)
}
