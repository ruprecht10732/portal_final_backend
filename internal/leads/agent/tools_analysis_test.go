package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"

	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/repository"
)

const (
	analysisTestPhone          = "+31612345678"
	analysisTestServiceType    = "Kozijn vervangen"
	analysisTestResolvedFact   = "Foto ontvangen"
	analysisTestMissingMeasure = "Exacte dagmaat"
)

type fakeToolContext struct {
	context.Context
}

func (f fakeToolContext) UserContent() *genai.Content { return nil }
func (f fakeToolContext) InvocationID() string        { return "" }
func (f fakeToolContext) AgentName() string           { return "" }
func (f fakeToolContext) ReadonlyState() session.ReadonlyState {
	return nil
}
func (f fakeToolContext) UserID() string                 { return "" }
func (f fakeToolContext) AppName() string                { return "" }
func (f fakeToolContext) SessionID() string              { return "" }
func (f fakeToolContext) Branch() string                 { return "" }
func (f fakeToolContext) Artifacts() agent.Artifacts     { return nil }
func (f fakeToolContext) State() session.State           { return nil }
func (f fakeToolContext) FunctionCallID() string         { return "" }
func (f fakeToolContext) Actions() *session.EventActions { return &session.EventActions{} }
func (f fakeToolContext) SearchMemory(context.Context, string) (*memory.SearchResponse, error) {
	return nil, nil
}
func (f fakeToolContext) ToolConfirmation() *toolconfirmation.ToolConfirmation { return nil }
func (f fakeToolContext) RequestConfirmation(string, any) error                { return nil }

type analysisToolRepoStub struct {
	*repository.Repository
	lead                repository.Lead
	service             repository.LeadService
	latestAnalysis      repository.AIAnalysis
	hasLatestAnalysis   bool
	latestPhotoAnalysis repository.PhotoAnalysis
	hasPhotoAnalysis    bool
	visitReport         *repository.AppointmentVisitReport
	attachments         []repository.Attachment
	createAnalysisCalls int
	lastCreateParams    repository.CreateAIAnalysisParams
	timelineEvents      []repository.CreateTimelineEventParams
}

func (s *analysisToolRepoStub) GetByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.Lead, error) {
	return s.lead, nil
}

func (s *analysisToolRepoStub) GetLeadServiceByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.LeadService, error) {
	return s.service, nil
}

func (s *analysisToolRepoStub) GetLatestAIAnalysis(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.AIAnalysis, error) {
	if !s.hasLatestAnalysis {
		return repository.AIAnalysis{}, repository.ErrNotFound
	}
	return s.latestAnalysis, nil
}

func (s *analysisToolRepoStub) GetLatestPhotoAnalysis(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.PhotoAnalysis, error) {
	if !s.hasPhotoAnalysis {
		return repository.PhotoAnalysis{}, repository.ErrPhotoAnalysisNotFound
	}
	return s.latestPhotoAnalysis, nil
}

func (s *analysisToolRepoStub) GetLatestAppointmentVisitReportByService(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*repository.AppointmentVisitReport, error) {
	if s.visitReport == nil {
		return nil, repository.ErrNotFound
	}
	return s.visitReport, nil
}

func (s *analysisToolRepoStub) ListAttachmentsByService(_ context.Context, _ uuid.UUID, _ uuid.UUID) ([]repository.Attachment, error) {
	return s.attachments, nil
}

func (s *analysisToolRepoStub) CreateAIAnalysis(_ context.Context, params repository.CreateAIAnalysisParams) (repository.AIAnalysis, error) {
	s.createAnalysisCalls++
	s.lastCreateParams = params
	return repository.AIAnalysis{ID: uuid.New(), CreatedAt: time.Now()}, nil
}

func (s *analysisToolRepoStub) CreateTimelineEvent(_ context.Context, params repository.CreateTimelineEventParams) (repository.TimelineEvent, error) {
	s.timelineEvents = append(s.timelineEvents, params)
	return repository.TimelineEvent{}, nil
}

func newAnalysisToolDeps(repo repository.LeadsRepository, tenantID uuid.UUID) *ToolDependencies {
	deps := (&ToolDependencies{Repo: repo}).NewRequestDeps()
	deps.SetTenantID(tenantID)
	deps.SetActor(repository.ActorTypeAI, repository.ActorNameGatekeeper)
	return deps
}

func stringPtr(value string) *string {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func containsNormalizedValue(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(expected)) {
			return true
		}
	}
	return false
}

func TestNormalizeAnalysisInputAutoPopulatesTrustedFacts(t *testing.T) {
	tenantID := uuid.New()
	leadID := uuid.New()
	serviceID := uuid.New()
	prefs, err := json.Marshal(map[string]string{
		"budget":       "€5.000",
		"timeframe":    "Binnen 2 maanden",
		"availability": "Vrijdagen",
		"extraNotes":   "Graag stil werken",
	})
	if err != nil {
		t.Fatalf(expectedNoErrorMessage, err)
	}
	repo := &analysisToolRepoStub{
		lead: repository.Lead{
			ID:               leadID,
			ConsumerPhone:    analysisTestPhone,
			EnergyClass:      stringPtr("A"),
			EnergyBouwjaar:   intPtr(1998),
			EnergyGebouwtype: stringPtr("woning"),
			CreatedAt:        time.Now(),
		},
		service: repository.LeadService{
			ID:                  serviceID,
			LeadID:              leadID,
			OrganizationID:      tenantID,
			Status:              domain.LeadStatusNew,
			PipelineStage:       domain.PipelineStageTriage,
			ServiceType:         analysisTestServiceType,
			ConsumerNote:        stringPtr("Voordeur vervangen inclusief glas"),
			CustomerPreferences: prefs,
		},
		hasLatestAnalysis: true,
		latestAnalysis: repository.AIAnalysis{
			ResolvedInformation: []string{analysisTestResolvedFact},
			ExtractedFacts:      map[string]string{"existing_fact": "bestaand"},
			CreatedAt:           time.Now(),
		},
		visitReport: &repository.AppointmentVisitReport{
			Measurements: stringPtr("Breedte 100 cm"),
		},
		hasPhotoAnalysis: true,
		latestPhotoAnalysis: repository.PhotoAnalysis{
			Summary:                "Kozijn deels rot",
			ExtractedText:          []string{"100 cm"},
			NeedsOnsiteMeasurement: []string{"diepte opening"},
			Measurements:           []repository.Measurement{{Description: "breedte", Value: 100, Unit: "cm"}},
		},
		attachments: []repository.Attachment{{FileName: "plattegrond.pdf"}},
	}
	deps := newAnalysisToolDeps(repo, tenantID)

	normalized, err := normalizeAnalysisInput(fakeToolContext{Context: context.Background()}, deps, SaveAnalysisInput{
		LeadID:                  leadID.String(),
		LeadServiceID:           serviceID.String(),
		UrgencyLevel:            "Medium",
		LeadQuality:             "Potential",
		RecommendedAction:       "RequestInfo",
		MissingInformation:      []string{"Exacte diepte opening"},
		PreferredContactChannel: "WhatsApp",
	}, repo.lead, tenantID, serviceID)
	if err != nil {
		t.Fatalf(expectedNoErrorMessage, err)
	}

	for _, expected := range []string{
		analysisTestResolvedFact,
		"Budget gedeeld: €5.000",
		"Gewenste termijn: Binnen 2 maanden",
		"Beschikbaarheid gedeeld: Vrijdagen",
		"Ingemeten tijdens afspraak: Breedte 100 cm",
		"Klant heeft document(en) geüpload voor handmatige controle",
	} {
		if !containsNormalizedValue(normalized.ResolvedInformation, expected) {
			t.Fatalf("expected resolvedInformation to contain %q, got %#v", expected, normalized.ResolvedInformation)
		}
	}

	checks := map[string]string{
		"existing_fact":             "bestaand",
		"service_type":              analysisTestServiceType,
		"consumer_note":             "Voordeur vervangen inclusief glas",
		"budget":                    "€5.000",
		"timeframe":                 "Binnen 2 maanden",
		"availability":              "Vrijdagen",
		"preference_notes":          "Graag stil werken",
		"visit_report_measurements": "Breedte 100 cm",
		"photo_summary":             "Kozijn deels rot",
		"photo_ocr_text":            "100 cm",
		"document_review_required":  "true",
		"attachment_documents":      "plattegrond.pdf",
		"energy_class":              "A",
		"build_year":                "1998",
		"building_type":             "woning",
	}
	for key, expected := range checks {
		if got := normalized.ExtractedFacts[key]; got != expected {
			t.Fatalf("expected extractedFacts[%q] = %q, got %q", key, expected, got)
		}
	}
}

func TestShouldSkipEquivalentRecentAnalysisWhenAutoPopulatedFactsMatch(t *testing.T) {
	tenantID := uuid.New()
	leadID := uuid.New()
	serviceID := uuid.New()
	repo := &analysisToolRepoStub{
		lead: repository.Lead{
			ID:            leadID,
			ConsumerPhone: analysisTestPhone,
			CreatedAt:     time.Now(),
		},
		service: repository.LeadService{
			ID:             serviceID,
			LeadID:         leadID,
			OrganizationID: tenantID,
			Status:         domain.LeadStatusNew,
			PipelineStage:  domain.PipelineStageTriage,
			ServiceType:    analysisTestServiceType,
		},
		hasLatestAnalysis: true,
		latestAnalysis: repository.AIAnalysis{
			CreatedAt:               time.Now(),
			UrgencyLevel:            "Medium",
			LeadQuality:             "Potential",
			RecommendedAction:       "RequestInfo",
			MissingInformation:      []string{analysisTestMissingMeasure},
			ResolvedInformation:     []string{analysisTestResolvedFact},
			ExtractedFacts:          map[string]string{"service_type": analysisTestServiceType, "existing_fact": "bestaand"},
			PreferredContactChannel: "WhatsApp",
		},
	}
	deps := newAnalysisToolDeps(repo, tenantID)
	normalized, err := normalizeAnalysisInput(fakeToolContext{Context: context.Background()}, deps, SaveAnalysisInput{
		LeadID:                  leadID.String(),
		LeadServiceID:           serviceID.String(),
		UrgencyLevel:            "Medium",
		LeadQuality:             "Potential",
		RecommendedAction:       "RequestInfo",
		MissingInformation:      []string{analysisTestMissingMeasure},
		PreferredContactChannel: "WhatsApp",
	}, repo.lead, tenantID, serviceID)
	if err != nil {
		t.Fatalf(expectedNoErrorMessage, err)
	}
	if !shouldSkipEquivalentRecentAnalysis(fakeToolContext{Context: context.Background()}, deps, serviceID, tenantID, normalized) {
		t.Fatalf("expected duplicate-equivalent analysis to be skipped; normalized facts=%#v resolved=%#v latest=%#v", normalized.ExtractedFacts, normalized.ResolvedInformation, repo.latestAnalysis)
	}
}

func TestHandleSaveAnalysisDoesNotSkipDuplicateWhenTrustedFactsChange(t *testing.T) {
	tenantID := uuid.New()
	leadID := uuid.New()
	serviceID := uuid.New()
	repo := &analysisToolRepoStub{
		lead: repository.Lead{
			ID:            leadID,
			ConsumerPhone: analysisTestPhone,
			CreatedAt:     time.Now(),
		},
		service: repository.LeadService{
			ID:             serviceID,
			LeadID:         leadID,
			OrganizationID: tenantID,
			Status:         domain.LeadStatusNew,
			PipelineStage:  domain.PipelineStageTriage,
			ServiceType:    "Nieuwe service",
		},
		hasLatestAnalysis: true,
		latestAnalysis: repository.AIAnalysis{
			CreatedAt:               time.Now(),
			UrgencyLevel:            "Medium",
			LeadQuality:             "Potential",
			RecommendedAction:       "RequestInfo",
			MissingInformation:      []string{analysisTestMissingMeasure},
			ResolvedInformation:     []string{analysisTestResolvedFact},
			ExtractedFacts:          map[string]string{"service_type": "Oude service"},
			PreferredContactChannel: "WhatsApp",
		},
	}
	deps := newAnalysisToolDeps(repo, tenantID)
	ctx := fakeToolContext{Context: WithDependencies(context.Background(), deps)}

	out, err := handleSaveAnalysis(ctx, deps, SaveAnalysisInput{
		LeadID:                  leadID.String(),
		LeadServiceID:           serviceID.String(),
		UrgencyLevel:            "Medium",
		LeadQuality:             "Potential",
		RecommendedAction:       "RequestInfo",
		MissingInformation:      []string{analysisTestMissingMeasure},
		PreferredContactChannel: "WhatsApp",
	})
	if err != nil {
		t.Fatalf(expectedNoErrorMessage, err)
	}
	if !out.Success {
		t.Fatalf(expectedSuccessMessage, out)
	}
	if repo.createAnalysisCalls != 1 {
		t.Fatalf("expected CreateAIAnalysis to be called once when trusted facts changed, got %d", repo.createAnalysisCalls)
	}
	if repo.lastCreateParams.ExtractedFacts["service_type"] != "Nieuwe service" {
		t.Fatalf("expected current service_type to override stale prior fact, got %#v", repo.lastCreateParams.ExtractedFacts)
	}
}
