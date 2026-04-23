package agent

import (
	"bytes"
	"fmt"
	"text/template"

	"portal_final_backend/agents"
)

func mustReadPromptFile(path string) string {
	body, err := agents.ReadPromptFile(path)
	if err != nil {
		panic(fmt.Sprintf("load prompt file %s: %v", path, err))
	}
	return body
}

func mustLoadPromptTemplate(name, path string) *template.Template {
	body, err := agents.ReadPromptFile(path)
	if err != nil {
		panic(fmt.Sprintf("load prompt template %s: %v", path, err))
	}
	return template.Must(template.New(name).Option("missingkey=error").Parse(body))
}

func mustLoadPromptTemplateWithPreamble(name, path string) *template.Template {
	body, err := agents.ReadPromptFile(path)
	if err != nil {
		panic(fmt.Sprintf("load prompt template %s: %v", path, err))
	}
	// Prepend global preamble to ensure universal constraints are applied first
	fullBody := sharedGlobalPreamble + "\n\n" + body
	return template.Must(template.New(name).Option("missingkey=error").Parse(fullBody))
}

func renderPromptTemplate(tmpl *template.Template, data any) string {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("render %s prompt template: %v", tmpl.Name(), err))
	}
	return buf.String()
}

var scopeAnalyzerPromptTemplate = mustLoadPromptTemplate("scope-analyzer", "calculator/prompts/scope_analyzer.md")

var quoteBuilderPromptTemplate = mustLoadPromptTemplate("quote-builder", "calculator/prompts/quote_builder.md")

var investigativePromptTemplate = mustLoadPromptTemplate("investigative", "calculator/prompts/investigative.md")

var dispatcherPromptTemplate = mustLoadPromptTemplate("dispatcher", "matchmaker/prompts/base.md")

var quoteGeneratePromptTemplate = mustLoadPromptTemplate("quote-generate", "calculator/prompts/quote_generate.md")

var quoteCriticPromptTemplate = mustLoadPromptTemplate("quote-critic", "calculator/prompts/quote_critic.md")

var quoteRepairPromptTemplate = mustLoadPromptTemplate("quote-repair", "calculator/prompts/quote_repair.md")

var photoAnalysisPromptTemplate = mustLoadPromptTemplate("photo-analysis", "support/photo_analyzer/prompts/request.md")
