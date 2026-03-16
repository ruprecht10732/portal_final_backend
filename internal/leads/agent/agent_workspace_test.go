package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"portal_final_backend/internal/orchestration"
)

const offerSummaryPromptBase = "offer summary prompt base"
const errLoadAgentWorkspace = "LoadAgentWorkspace returned error: %v"
const errUnexpectedAllowedTools = "unexpected allowed tools: got %v want %v"
const gatekeeperContextText = "gatekeeper context"

func TestLoadAgentContextSuccess(t *testing.T) {
	rootDir := createTestAgentWorkspace(t)
	t.Setenv("AGENT_WORKSPACE_ROOT", rootDir)

	workspace, err := orchestration.LoadAgentWorkspace("gatekeeper")
	if err != nil {
		t.Fatalf(errLoadAgentWorkspace, err)
	}
	context := workspace.Instruction

	checks := []string{
		"root instructions",
		"Shared Governance",
		"shared identity",
		"Gatekeeper",
		gatekeeperContextText,
		"save analysis skill",
		"Workspace Persistence",
		"Active workspace agent: gatekeeper",
	}
	for _, check := range checks {
		if !strings.Contains(context, check) {
			t.Fatalf("expected context to contain %q", check)
		}
	}

	if strings.Index(context, "a shared file") > strings.Index(context, "z shared file") {
		t.Fatalf("expected shared markdown files to load in alphabetical order")
	}
	if strings.Index(context, "a skill file") > strings.Index(context, "z skill file") {
		t.Fatalf("expected skill markdown files to load in alphabetical order")
	}
}

func TestLoadAgentWorkspaceParsesAllowedTools(t *testing.T) {
	rootDir := createTestAgentWorkspace(t)
	t.Setenv("AGENT_WORKSPACE_ROOT", rootDir)

	workspace, err := orchestration.LoadAgentWorkspace("gatekeeper")
	if err != nil {
		t.Fatalf(errLoadAgentWorkspace, err)
	}

	if workspace.Name != "gatekeeper" {
		t.Fatalf("expected skill name gatekeeper, got %q", workspace.Name)
	}
	if workspace.Description == "" {
		t.Fatal("expected skill description to be populated")
	}

	wantTools := []string{"SaveAnalysis", "UpdateLeadDetails", "UpdateLeadServiceType", "UpdatePipelineStage"}
	if strings.Join(workspace.AllowedTools, ",") != strings.Join(wantTools, ",") {
		t.Fatalf(errUnexpectedAllowedTools, workspace.AllowedTools, wantTools)
	}
}

func TestLoadWhatsAppAgentWorkspaceParsesAllowedTools(t *testing.T) {
	rootDir := createTestAgentWorkspace(t)
	t.Setenv("AGENT_WORKSPACE_ROOT", rootDir)

	workspace, err := orchestration.LoadAgentWorkspace("whatsapp-agent")
	if err != nil {
		t.Fatalf(errLoadAgentWorkspace, err)
	}

	wantTools := []string{
		"SearchLeads",
		"GetLeadDetails",
		"CreateLead",
		"SearchProductMaterials",
		"AttachCurrentWhatsAppPhoto",
		"GetAvailableVisitSlots",
		"GetNavigationLink",
		"GetQuotes",
		"DraftQuote",
		"GenerateQuote",
		"SendQuotePDF",
		"GetAppointments",
		"UpdateLeadDetails",
		"AskCustomerClarification",
		"SaveNote",
		"UpdateStatus",
		"ScheduleVisit",
		"RescheduleVisit",
		"CancelVisit",
	}
	if strings.Join(workspace.AllowedTools, ",") != strings.Join(wantTools, ",") {
		t.Fatalf(errUnexpectedAllowedTools, workspace.AllowedTools, wantTools)
	}
	if !strings.Contains(workspace.Instruction, "WhatsApp Agent") {
		t.Fatalf("expected WhatsApp agent instruction to include skill body")
	}
}

func TestLoadWhatsAppPartnerAgentWorkspaceParsesAllowedTools(t *testing.T) {
	rootDir := createTestAgentWorkspace(t)
	t.Setenv("AGENT_WORKSPACE_ROOT", rootDir)

	workspace, err := orchestration.LoadAgentWorkspace("whatsapp_partner_agent")
	if err != nil {
		t.Fatalf(errLoadAgentWorkspace, err)
	}

	wantTools := []string{
		"GetMyJobs",
		"GetPartnerJobDetails",
		"GetNavigationLink",
		"GetAppointments",
		"AttachCurrentWhatsAppPhoto",
		"SaveMeasurement",
		"UpdateAppointmentStatus",
		"RescheduleVisit",
		"CancelVisit",
	}
	if strings.Join(workspace.AllowedTools, ",") != strings.Join(wantTools, ",") {
		t.Fatalf(errUnexpectedAllowedTools, workspace.AllowedTools, wantTools)
	}
	if !strings.Contains(workspace.Instruction, "WhatsApp Partner Agent") {
		t.Fatalf("expected WhatsApp partner agent instruction to include skill body")
	}

	legacyWorkspace, err := orchestration.LoadAgentWorkspace("whatsapp-partner-agent")
	if err != nil {
		t.Fatalf(errLoadAgentWorkspace, err)
	}
	if strings.Join(legacyWorkspace.AllowedTools, ",") != strings.Join(wantTools, ",") {
		t.Fatalf(errUnexpectedAllowedTools, legacyWorkspace.AllowedTools, wantTools)
	}
}

func TestLoadAgentContextUnknownAgent(t *testing.T) {
	rootDir := createTestAgentWorkspace(t)
	t.Setenv("AGENT_WORKSPACE_ROOT", rootDir)

	_, err := orchestration.LoadAgentWorkspace("unknown-agent")
	if err == nil || !strings.Contains(err.Error(), "unsupported agent workspace") {
		t.Fatalf("expected unsupported agent workspace error, got %v", err)
	}
}

func TestLoadAgentContextMissingFile(t *testing.T) {
	rootDir := createTestAgentWorkspace(t)
	t.Setenv("AGENT_WORKSPACE_ROOT", rootDir)

	if err := os.Remove(filepath.Join(rootDir, "agents", "gatekeeper", "SKILL.md")); err != nil {
		t.Fatalf("remove entry skill: %v", err)
	}

	_, err := orchestration.LoadAgentWorkspace("gatekeeper")
	if err == nil || !strings.Contains(err.Error(), "agents/gatekeeper/SKILL.md") {
		t.Fatalf("expected missing SKILL.md error, got %v", err)
	}
}

func TestLoadAgentContextInvalidRootOverride(t *testing.T) {
	t.Setenv("AGENT_WORKSPACE_ROOT", t.TempDir())

	_, err := orchestration.LoadAgentWorkspace("gatekeeper")
	if err == nil || !strings.Contains(err.Error(), "missing AGENTS.md") {
		t.Fatalf("expected missing root instruction file error, got %v", err)
	}
}

func TestLoadAgentContextMissingSkillsDirectory(t *testing.T) {
	rootDir := createTestAgentWorkspace(t)
	t.Setenv("AGENT_WORKSPACE_ROOT", rootDir)

	skillsDir := filepath.Join(rootDir, "agents", "gatekeeper", "skills")
	if err := os.RemoveAll(skillsDir); err != nil {
		t.Fatalf("remove skills dir: %v", err)
	}

	_, err := orchestration.LoadAgentWorkspace("gatekeeper")
	if err == nil || !strings.Contains(err.Error(), "agents/gatekeeper/skills") {
		t.Fatalf("expected missing skills directory error, got %v", err)
	}
}

func TestBuildAgentInstructionAppendsExtraSections(t *testing.T) {
	rootDir := createTestAgentWorkspace(t)
	t.Setenv("AGENT_WORKSPACE_ROOT", rootDir)

	instruction, err := orchestration.BuildAgentInstruction("offer-summary", "extra section one", "", "extra section two")
	if err != nil {
		t.Fatalf("BuildAgentInstruction returned error: %v", err)
	}

	if !strings.Contains(instruction, "offer summary context") {
		t.Fatalf("expected base workspace context to be included")
	}
	if !strings.Contains(instruction, offerSummaryPromptBase) {
		t.Fatalf("expected prompt file discovered by convention to be included")
	}
	if !strings.Contains(instruction, "extra section one") || !strings.Contains(instruction, "extra section two") {
		t.Fatalf("expected extra sections to be appended")
	}
}

func TestLoadAgentContextUsesConventionOrder(t *testing.T) {
	rootDir := createTestAgentWorkspace(t)
	t.Setenv("AGENT_WORKSPACE_ROOT", rootDir)

	workspace, err := orchestration.LoadAgentWorkspace("offer-summary")
	if err != nil {
		t.Fatalf(errLoadAgentWorkspace, err)
	}
	context := workspace.Instruction
	if strings.Index(context, offerSummaryPromptBase) > strings.Index(context, "offer summary skill") {
		t.Fatalf("expected prompt files to load before skills according to workspace conventions")
	}
}

func createTestAgentWorkspace(t *testing.T) string {
	t.Helper()
	rootDir := t.TempDir()
	files := map[string]string{
		"AGENTS.md":                                                        "root instructions",
		"agents/shared/SKILL.md":                                           "---\nname: shared\ndescription: Use when any backend lead-processing agent needs shared governance.\nmetadata:\n  allowed-tools: []\n---\n\n# Shared Governance",
		"agents/shared/a-shared.md":                                        "a shared file",
		"agents/shared/execution-contract.md":                              "shared execution contract",
		"agents/shared/identity.md":                                        "shared identity",
		"agents/shared/pipeline-invariants.md":                             "shared pipeline invariants",
		"agents/shared/status-governance.md":                               "shared status governance",
		"agents/shared/tool-catalog.md":                                    "shared tool catalog",
		"agents/shared/product-selection.md":                               "shared product selection",
		"agents/shared/communication-rules.md":                             "shared communication rules",
		"agents/shared/z-shared.md":                                        "z shared file",
		"agents/gatekeeper/SKILL.md":                                       "---\nname: gatekeeper\ndescription: Use when a lead or service needs intake validation before pipeline progression.\nmetadata:\n  allowed-tools:\n    - SaveAnalysis\n    - UpdateLeadDetails\n    - UpdateLeadServiceType\n    - UpdatePipelineStage\n---\n\n# Gatekeeper",
		"agents/gatekeeper/context.md":                                     gatekeeperContextText,
		"agents/gatekeeper/skills/a-skill.md":                              "a skill file",
		"agents/gatekeeper/skills/save_analysis.md":                        "save analysis skill",
		"agents/gatekeeper/skills/update_lead_details.md":                  "update lead details skill",
		"agents/gatekeeper/skills/update_lead_service_type.md":             "update lead service type skill",
		"agents/gatekeeper/skills/update_pipeline_stage.md":                "update pipeline stage skill",
		"agents/gatekeeper/skills/z-skill.md":                              "z skill file",
		"agents/qualifier/SKILL.md":                                        "---\nname: qualifier\ndescription: Use when intake needs a clarification request.\nmetadata:\n  allowed-tools:\n    - AskCustomerClarification\n    - SaveAnalysis\n---\n\n# Qualifier",
		"agents/qualifier/context.md":                                      "qualifier context",
		"agents/qualifier/skills/ask_customer_clarification.md":            "ask customer clarification skill",
		"agents/qualifier/skills/save_analysis.md":                         "qualifier save analysis skill",
		"agents/calculator/SKILL.md":                                       "---\nname: calculator\ndescription: Use when a service must be scoped, estimated, or quoted.\nmetadata:\n  allowed-tools:\n    - Calculator\n    - CalculateEstimate\n    - SearchProductMaterials\n    - ListCatalogGaps\n    - DraftQuote\n    - SaveEstimation\n    - SubmitQuoteCritique\n    - CommitScopeArtifact\n    - AskCustomerClarification\n    - UpdatePipelineStage\n---\n\n# Calculator",
		"agents/calculator/context.md":                                     "calculator context",
		"agents/calculator/skills/calculator.md":                           "calculator skill",
		"agents/calculator/skills/calculate_estimate.md":                   "calculate estimate skill",
		"agents/calculator/skills/search_product_materials.md":             "search product materials skill",
		"agents/calculator/skills/list_catalog_gaps.md":                    "list catalog gaps skill",
		"agents/calculator/skills/draft_quote.md":                          "draft quote skill",
		"agents/calculator/skills/save_estimation.md":                      "save estimation skill",
		"agents/calculator/skills/submit_quote_critique.md":                "submit quote critique skill",
		"agents/calculator/skills/commit_scope_artifact.md":                "commit scope artifact skill",
		"agents/matchmaker/SKILL.md":                                       "---\nname: matchmaker\ndescription: Use when a fulfillment-ready service needs partner routing.\nmetadata:\n  allowed-tools:\n    - FindMatchingPartners\n    - CreatePartnerOffer\n    - UpdatePipelineStage\n---\n\n# Matchmaker",
		"agents/matchmaker/context.md":                                     "matchmaker context",
		"agents/matchmaker/skills/find_matching_partners.md":               "find matching partners skill",
		"agents/matchmaker/skills/create_partner_offer.md":                 "create partner offer skill",
		"agents/matchmaker/skills/update_pipeline_stage.md":                "matchmaker update stage skill",
		"agents/support/auditor/SKILL.md":                                  "---\nname: auditor\ndescription: Use when operational evidence must be audited.\nmetadata:\n  allowed-tools:\n    - SubmitAuditResult\n---\n\n# Auditor",
		"agents/support/auditor/context.md":                                "auditor context",
		"agents/support/auditor/skills/submit_audit_result.md":             "submit audit result skill",
		"agents/support/photo_analyzer/SKILL.md":                           "---\nname: photo_analyzer\ndescription: Use when images need structured visual analysis.\nmetadata:\n  allowed-tools:\n    - SavePhotoAnalysis\n    - Calculator\n    - FlagOnsiteMeasurement\n---\n\n# Photo Analyzer",
		"agents/support/photo_analyzer/context.md":                         "photo analyzer context",
		"agents/support/photo_analyzer/skills/analyze_photos.md":           "photo analyzer skill",
		"agents/support/call_logger/SKILL.md":                              "---\nname: call_logger\ndescription: Use when a rough call summary must become structured lead updates.\nmetadata:\n  allowed-tools:\n    - SaveNote\n    - UpdateLeadDetails\n    - SetCallOutcome\n    - UpdateStatus\n    - UpdatePipelineStage\n    - ScheduleVisit\n    - RescheduleVisit\n    - CancelVisit\n---\n\n# Call Logger",
		"agents/support/call_logger/context.md":                            "call logger context",
		"agents/support/call_logger/prompts/base.md":                       "call logger prompt base",
		"agents/support/call_logger/skills/log_call.md":                    "call logger skill",
		"agents/support/offer_summary/SKILL.md":                            "---\nname: offer_summary\ndescription: Use when a concise partner-offer summary is requested.\nmetadata:\n  allowed-tools: []\n---\n\n# Offer Summary",
		"agents/support/offer_summary/context.md":                          "offer summary context",
		"agents/support/offer_summary/prompts/base.md":                     offerSummaryPromptBase,
		"agents/support/offer_summary/skills/generate_summary.md":          "offer summary skill",
		"agents/support/whatsapp_reply/SKILL.md":                           "---\nname: whatsapp_reply\ndescription: Use when a grounded WhatsApp reply draft is requested.\nmetadata:\n  allowed-tools: []\n---\n\n# WhatsApp Reply",
		"agents/support/whatsapp_reply/context.md":                         "whatsapp reply context",
		"agents/support/whatsapp_reply/prompts/base.md":                    "whatsapp reply prompt base",
		"agents/support/whatsapp_reply/skills/reply_generation.md":         "whatsapp reply skill",
		"agents/support/whatsapp_agent/SKILL.md":                           "---\nname: whatsapp_agent\ndescription: Use when an incoming WhatsApp message from an authenticated external user must be answered autonomously using function-calling tools.\nmetadata:\n  allowed-tools:\n    - SearchLeads\n    - GetLeadDetails\n    - CreateLead\n    - SearchProductMaterials\n    - AttachCurrentWhatsAppPhoto\n    - GetAvailableVisitSlots\n    - GetNavigationLink\n    - GetQuotes\n    - DraftQuote\n    - GenerateQuote\n    - SendQuotePDF\n    - GetAppointments\n    - UpdateLeadDetails\n    - AskCustomerClarification\n    - SaveNote\n    - UpdateStatus\n    - ScheduleVisit\n    - RescheduleVisit\n    - CancelVisit\n---\n\n# WhatsApp Agent",
		"agents/support/whatsapp_agent/context.md":                         "whatsapp agent context",
		"agents/support/whatsapp_agent/prompts/base.md":                    "whatsapp agent prompt base",
		"agents/support/whatsapp_agent/skills/reply_generation.md":         "whatsapp agent skill",
		"agents/support/whatsapp_partner_agent/SKILL.md":                   "---\nname: whatsapp_partner_agent\ndescription: Use when an incoming WhatsApp message from a registered partner or vakman must be answered autonomously with partner-scoped tools only.\nmetadata:\n  allowed-tools:\n    - GetMyJobs\n    - GetPartnerJobDetails\n    - GetNavigationLink\n    - GetAppointments\n    - AttachCurrentWhatsAppPhoto\n    - SaveMeasurement\n    - UpdateAppointmentStatus\n    - RescheduleVisit\n    - CancelVisit\n---\n\n# WhatsApp Partner Agent",
		"agents/support/whatsapp_partner_agent/context.md":                 "whatsapp partner agent context",
		"agents/support/whatsapp_partner_agent/prompts/base.md":            "whatsapp partner agent prompt base",
		"agents/support/whatsapp_partner_agent/skills/reply_generation.md": "whatsapp partner agent skill",
		"agents/support/email_reply/SKILL.md":                              "---\nname: email_reply\ndescription: Use when a grounded email reply draft is requested.\nmetadata:\n  allowed-tools: []\n---\n\n# Email Reply",
		"agents/support/email_reply/context.md":                            "email reply context",
		"agents/support/email_reply/prompts/base.md":                       "email reply prompt base",
		"agents/support/email_reply/skills/reply_generation.md":            "email reply skill",
	}

	for relativePath, content := range files {
		fullPath := filepath.Join(rootDir, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", relativePath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", relativePath, err)
		}
	}

	return rootDir
}
