package agent

import (
	"bytes"
	"fmt"
	"text/template"

	"portal_final_backend/internal/orchestration"
)

func mustLoadWorkspaceMarkdownText(relativePath string) string {
	return orchestration.MustLoadWorkspaceMarkdownText(relativePath)
}

func mustParsePromptTemplate(name, body string) *template.Template {
	return template.Must(template.New(name).Option("missingkey=error").Parse(body))
}

func renderPromptTemplate(tmpl *template.Template, data any) string {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("render %s prompt template: %v", tmpl.Name(), err))
	}
	return buf.String()
}

var scopeAnalyzerPromptTemplate = mustParsePromptTemplate("scope-analyzer", mustLoadWorkspaceMarkdownText("agents/calculator/prompts/scope_analyzer.md"))

var quoteBuilderPromptTemplate = mustParsePromptTemplate("quote-builder", mustLoadWorkspaceMarkdownText("agents/calculator/prompts/quote_builder.md"))

var investigativePromptTemplate = mustParsePromptTemplate("investigative", mustLoadWorkspaceMarkdownText("agents/calculator/prompts/investigative.md"))

var dispatcherPromptTemplate = mustParsePromptTemplate("dispatcher", mustLoadWorkspaceMarkdownText("agents/matchmaker/prompts/base.md"))

var quoteGeneratePromptTemplate = mustParsePromptTemplate("quote-generate", mustLoadWorkspaceMarkdownText("agents/calculator/prompts/quote_generate.md"))

var quoteCriticPromptTemplate = mustParsePromptTemplate("quote-critic", mustLoadWorkspaceMarkdownText("agents/calculator/prompts/quote_critic.md"))

var quoteRepairPromptTemplate = mustParsePromptTemplate("quote-repair", mustLoadWorkspaceMarkdownText("agents/calculator/prompts/quote_repair.md"))

var photoAnalysisPromptTemplate = mustParsePromptTemplate("photo-analysis", mustLoadWorkspaceMarkdownText("agents/support/photo_analyzer/prompts/request.md"))
