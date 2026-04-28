package agent

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/google/uuid"
)

func ValidatePromptTemplates() error {
	validations := []struct {
		name string
		tmpl *template.Template
		data any
	}{
		{
			name: "gatekeeper",
			tmpl: gatekeeperPromptTemplate,
			data: gatekeeperPromptTemplateData{
				ExecutionContract:         "execution",
				CommunicationContract:     "communication",
				PreferredChannel:          "WhatsApp",
				RecoveryModeSection:       "recovery",
				CycleAwarenessSection:     "cycle awareness",
				LeadID:                    uuid.New(),
				ServiceID:                 uuid.New(),
				ServiceType:               "Test service",
				PipelineStage:             "Triage",
				CreatedAt:                 "2026-03-11T10:00:00Z",
				ConsumerSummary:           "consumer",
				LocationSummary:           "location",
				ServiceNoteSummary:        "service note",
				NotesSection:              "notes",
				VisitReportSummary:        "visit report",
				PreferencesSummary:        "preferences",
				PreviousEstimatorBlockers: "blockers",
				KnownFacts:                "known facts",
				AttachmentAwareness:       "attachments",
				LeadContext:               "lead context",
				IntakeContextSummary:      "intake",
				EstimationContextSummary:  "estimation",
			},
		},
		{
			name: "scope-analyzer",
			tmpl: scopeAnalyzerPromptTemplate,
			data: struct {
				ExecutionContract            string
				SharedIntakeCompletenessGate string
				LeadID                       uuid.UUID
				ServiceID                    uuid.UUID
				ServiceType                  string
				PipelineStage                string
				ServiceNoteSummary           string
				NotesSection                 string
				PreferencesSummary           string
			}{"execution", "intake gate", uuid.New(), uuid.New(), "service", "Estimation", "note", "notes", "prefs"},
		},
		{
			name: "quote-builder",
			tmpl: quoteBuilderPromptTemplate,
			data: struct {
				ExecutionContract            string
				ScopeSummary                 string
				SharedProductSelectionRules  string
				SharedMathExamples           string
				SharedIntakeCompletenessGate string
				LeadID                       uuid.UUID
				ServiceID                    uuid.UUID
				ServiceType                  string
				PipelineStage                string
				CreatedAt                    string
				ConsumerSummary              string
				LocationSummary              string
				ServiceNoteSummary           string
				NotesSection                 string
				PreferencesSummary           string
				EstimationContextSummary     string
			}{"execution", "scope", "product rules", "math examples", "intake gate", uuid.New(), uuid.New(), "service", "Estimation", "2026-03-11T10:00:00Z", "consumer", "location", "note", "notes", "prefs", "guidelines"},
		},
		{
			name: "investigative",
			tmpl: investigativePromptTemplate,
			data: struct {
				ExecutionContract        string
				CommunicationContract    string
				Missing                  string
				PreferredChannel         string
				LeadID                   uuid.UUID
				ServiceID                uuid.UUID
				ServiceType              string
				ServiceNoteSummary       string
				NotesSection             string
				PreferencesSummary       string
				HouseContextSummary      string
				EstimationContextSummary string
			}{"execution", "communication", "- missing", "WhatsApp", uuid.New(), uuid.New(), "service", "note", "notes", "prefs", "house", "estimation"},
		},
		{
			name: "dispatcher",
			tmpl: dispatcherPromptTemplate,
			data: struct {
				ExecutionContract string
				ReferenceData     string
				ServiceType       string
				ZipCode           string
				RadiusKm          int
			}{"execution", "reference", "service", "1234AB", 25},
		},
		{
			name: "quote-generate",
			tmpl: quoteGeneratePromptTemplate,
			data: struct {
				ExecutionContract           string
				SharedProductSelectionRules string
				SharedMathExamples          string
				ReferenceData               string
			}{"execution", "product rules", "math examples", "reference"},
		},
		{
			name: "quote-critic",
			tmpl: quoteCriticPromptTemplate,
			data: struct {
				ExecutionContract        string
				LeadID                   uuid.UUID
				ServiceID                uuid.UUID
				QuoteID                  uuid.UUID
				QuoteNumber              string
				ServiceType              string
				ConsumerSummary          string
				LocationSummary          string
				ServiceNoteSummary       string
				NotesSection             string
				PreferencesSummary       string
				ScopeSummary             string
				EstimationContextSummary string
				DraftJSON                string
			}{"execution", uuid.New(), uuid.New(), uuid.New(), "Q-1", "service", "consumer", "location", "note", "notes", "prefs", "scope", "estimation", "{}"},
		},
		{
			name: "quote-repair",
			tmpl: quoteRepairPromptTemplate,
			data: struct {
				ExecutionContract        string
				LeadID                   uuid.UUID
				ServiceID                uuid.UUID
				ServiceType              string
				Attempt                  int
				ConsumerSummary          string
				LocationSummary          string
				ServiceNoteSummary       string
				NotesSection             string
				PreferencesSummary       string
				ScopeSummary             string
				EstimationContextSummary string
				DraftJSON                string
				CritiqueJSON             string
			}{"execution", uuid.New(), uuid.New(), "service", 1, "consumer", "location", "note", "notes", "prefs", "scope", "estimation", "{}", "{}"},
		},

	}

	for _, validation := range validations {
		var buf bytes.Buffer
		if err := validation.tmpl.Execute(&buf, validation.data); err != nil {
			return fmt.Errorf("validate prompt template %s: %w", validation.name, err)
		}
	}

	return nil
}
