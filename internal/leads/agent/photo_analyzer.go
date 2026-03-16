package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/orchestration"
	apptools "portal_final_backend/internal/tools"
	"portal_final_backend/platform/ai/moonshot"
)

const (
	photoAnalyzerWorkspaceName   = "photo-analyzer"
	photoAnalyzerTraceLabel      = photoAnalyzerWorkspaceName
	photoAnalyzerRetryTraceLabel = "photo-analyzer-retry"
)

// Measurement represents a single measurement extracted from photo analysis
type Measurement struct {
	Description string  `json:"description"`
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	Type        string  `json:"type"`       // dimension, area, count, volume
	Confidence  string  `json:"confidence"` // High, Medium, Low
	PhotoRef    string  `json:"photoRef,omitempty"`
}

// PhotoAnalysis represents the result of analyzing photos for a lead service
type PhotoAnalysis struct {
	ID                     uuid.UUID     `json:"id"`
	LeadID                 uuid.UUID     `json:"leadId"`
	ServiceID              uuid.UUID     `json:"serviceId"`
	Summary                string        `json:"summary"`
	Observations           []string      `json:"observations"`
	ScopeAssessment        string        `json:"scopeAssessment"`
	CostIndicators         string        `json:"costIndicators"`
	SafetyConcerns         []string      `json:"safetyConcerns,omitempty"`
	AdditionalInfo         []string      `json:"additionalInfo,omitempty"`
	Measurements           []Measurement `json:"measurements,omitempty"`
	NeedsOnsiteMeasurement []string      `json:"needsOnsiteMeasurement,omitempty"`
	Discrepancies          []string      `json:"discrepancies,omitempty"`
	ExtractedText          []string      `json:"extractedText,omitempty"`
	SuggestedSearchTerms   []string      `json:"suggestedSearchTerms,omitempty"`
	PhotoCount             int           `json:"photoCount"`
	ConfidenceLevel        string        `json:"confidenceLevel"` // High, Medium, Low
}

// PhotoAnalyzerDeps contains dependencies for the photo analyzer
type PhotoAnalyzerDeps struct {
	Repo     repository.LeadsRepository
	mu       sync.RWMutex
	tenantID *uuid.UUID
	// Result storage - set after analysis
	result                  *PhotoAnalysis
	needsOnsiteMeasurements []string // Accumulated by FlagOnsiteMeasurement tool
}

func (d *PhotoAnalyzerDeps) SetTenantID(id uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tenantID = &id
}

func (d *PhotoAnalyzerDeps) GetTenantID() (uuid.UUID, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tenantID == nil {
		return uuid.UUID{}, false
	}
	return *d.tenantID, true
}

func (d *PhotoAnalyzerDeps) SetResult(r *PhotoAnalysis) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.result = clonePhotoAnalysis(r)
}

func (d *PhotoAnalyzerDeps) GetResult() *PhotoAnalysis {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return clonePhotoAnalysis(d.result)
}

func (d *PhotoAnalyzerDeps) AddOnsiteFlag(reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.needsOnsiteMeasurements = append(d.needsOnsiteMeasurements, reason)
}

func (d *PhotoAnalyzerDeps) GetOnsiteFlags() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return cloneStringSlice(d.needsOnsiteMeasurements)
}

func (d *PhotoAnalyzerDeps) ResetAccumulators() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.result = nil
	d.needsOnsiteMeasurements = nil
}

func (d *PhotoAnalyzerDeps) NewRequestDeps() *PhotoAnalyzerDeps {
	return &PhotoAnalyzerDeps{Repo: d.Repo}
}

// PhotoAnalyzer provides AI-powered photo analysis for lead services
type PhotoAnalyzer struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
	deps           *PhotoAnalyzerDeps
}

// NewPhotoAnalyzer creates a new photo analyzer agent
func NewPhotoAnalyzer(apiKey string, modelName string, repo repository.LeadsRepository) (*PhotoAnalyzer, error) {
	kimi := moonshot.NewModel(newMoonshotReasoningModelConfig(apiKey, modelName))
	workspace, err := orchestration.LoadAgentWorkspace(photoAnalyzerWorkspaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to load photo analyzer workspace context: %w", err)
	}

	deps := &PhotoAnalyzerDeps{
		Repo: repo,
	}

	analyzer := &PhotoAnalyzer{
		appName: "photo_analyzer",
		deps:    deps,
	}

	tools, err := buildPhotoAnalyzerTools()
	if err != nil {
		return nil, fmt.Errorf("failed to build photo analyzer tools: %w", err)
	}
	toolsets := orchestration.BuildWorkspaceToolsets(workspace, "photo_analyzer_tools", tools)

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "PhotoAnalyzer",
		Model:       kimi,
		Description: "Expert AI agent specialized in analyzing photos of home repair and service situations",
		Instruction: workspace.Instruction,
		Toolsets:    toolsets,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create photo analyzer agent: %w", err)
	}

	sessionService := session.InMemoryService()

	r, err := runner.New(runner.Config{
		AppName:        analyzer.appName,
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create photo analyzer runner: %w", err)
	}

	analyzer.agent = adkAgent
	analyzer.runner = r
	analyzer.sessionService = sessionService

	return analyzer, nil
}

// AnalyzePhotos analyzes a set of photos for a lead service.
func (pa *PhotoAnalyzer) AnalyzePhotos(ctx context.Context, req PhotoAnalysisRequest) (*PhotoAnalysis, error) {
	if len(req.Images) == 0 {
		return nil, fmt.Errorf("no images provided")
	}

	reqDeps := pa.deps.NewRequestDeps()
	reqDeps.SetTenantID(req.TenantID)
	ctx = WithPhotoAnalyzerDeps(ctx, reqDeps)

	userContent := buildUserContent(req)
	userID, sessionID, err := pa.createSession(ctx, req.LeadID, req.ServiceID)
	if err != nil {
		return nil, err
	}
	defer pa.cleanupSession(ctx, userID, sessionID)

	output, err := pa.runAnalysis(ctx, userID, sessionID, userContent)
	if err != nil {
		return nil, err
	}
	log.Printf("Photo analysis completed for lead %s service %s. Output: %s", req.LeadID, req.ServiceID, output)

	result, err := pa.getOrRetryResult(ctx, userID, sessionID, output)
	if err != nil {
		return nil, err
	}

	pa.mergeOnsiteFlags(ctx, result)
	return result, nil
}

// PhotoAnalysisRequest contains analysis parameters for photo analysis.
// Images should be raw image data with MIME types.
type PhotoAnalysisRequest struct {
	LeadID             uuid.UUID
	ServiceID          uuid.UUID
	TenantID           uuid.UUID
	Images             []ImageData
	PreparedImages     []PreparedImage
	ContextInfo        string
	ServiceType        string
	IntakeRequirements string
}

func buildUserContent(req PhotoAnalysisRequest) *genai.Content {
	parts := make([]*genai.Part, 0, len(req.Images)+len(req.PreparedImages)+1)
	if len(req.PreparedImages) > 0 {
		for _, prepared := range req.PreparedImages {
			parts = append(parts, &genai.Part{
				InlineData: &genai.Blob{
					MIMEType: prepared.Original.MIMEType,
					Data:     prepared.Original.Data,
				},
			})
			for _, variant := range prepared.Variants {
				parts = append(parts, &genai.Part{
					InlineData: &genai.Blob{
						MIMEType: variant.MIMEType,
						Data:     variant.Data,
					},
				})
			}
		}
	} else {
		for _, img := range req.Images {
			parts = append(parts, &genai.Part{
				InlineData: &genai.Blob{
					MIMEType: img.MIMEType,
					Data:     img.Data,
				},
			})
		}
	}

	prompt := buildPhotoAnalysisPrompt(req.LeadID, req.ServiceID, len(req.Images), req.ContextInfo, req.ServiceType, req.IntakeRequirements, req.PreparedImages)
	parts = append(parts, genai.NewPartFromText(prompt))

	return &genai.Content{
		Role:  "user",
		Parts: parts,
	}
}

func (pa *PhotoAnalyzer) createSession(ctx context.Context, leadID, serviceID uuid.UUID) (string, string, error) {
	userID := fmt.Sprintf("%s-%s-%s", photoAnalyzerTraceLabel, leadID, serviceID)
	sessionID := uuid.New().String()

	_, err := pa.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   pa.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to create session: %w", err)
	}

	return userID, sessionID, nil
}

func (pa *PhotoAnalyzer) cleanupSession(ctx context.Context, userID, sessionID string) {
	if deleteErr := pa.sessionService.Delete(ctx, &session.DeleteRequest{
		AppName:   pa.appName,
		UserID:    userID,
		SessionID: sessionID,
	}); deleteErr != nil {
		log.Printf("warning: failed to delete session: %v", deleteErr)
	}
}

func (pa *PhotoAnalyzer) runAnalysis(ctx context.Context, userID, sessionID string, userContent *genai.Content) (string, error) {
	var output strings.Builder
	err := runPromptSession(ctx, promptRunRequest{
		SessionService:       pa.sessionService,
		Runner:               pa.runner,
		AppName:              pa.appName,
		UserID:               userID,
		SessionID:            sessionID,
		UserMessage:          userContent,
		CreateSessionMessage: "failed to create session",
		RunFailureMessage:    "photo analysis failed",
		TraceLabel:           photoAnalyzerTraceLabel,
	},
		func(event *session.Event) {
			output.WriteString(collectContentText(event.Content))
		},
	)
	if err != nil {
		return "", err
	}

	return output.String(), nil
}

func (pa *PhotoAnalyzer) getOrRetryResult(ctx context.Context, userID, sessionID string, output string) (*PhotoAnalysis, error) {
	reqDeps := GetPhotoAnalyzerDeps(ctx)
	result := reqDeps.GetResult()
	if result != nil {
		return result, nil
	}

	retryOutput, err := pa.retryForResult(ctx, userID, sessionID, output)
	if err != nil {
		return nil, err
	}
	_ = retryOutput

	result = reqDeps.GetResult()
	if result == nil {
		return nil, fmt.Errorf("AI did not save photo analysis")
	}

	return result, nil
}

func (pa *PhotoAnalyzer) retryForResult(ctx context.Context, userID, sessionID string, output string) (string, error) {
	retryContent := &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			genai.NewPartFromText("请选择一个工具（tool）来处理当前的问题。You MUST call the SavePhotoAnalysis tool now with your complete analysis."),
		},
	}

	err := runPromptSession(ctx, promptRunRequest{
		SessionService:       pa.sessionService,
		Runner:               pa.runner,
		AppName:              pa.appName,
		UserID:               userID,
		SessionID:            sessionID,
		UserMessage:          retryContent,
		CreateSessionMessage: "failed to create session",
		RunFailureMessage:    "photo analysis retry failed",
		TraceLabel:           photoAnalyzerRetryTraceLabel,
	},
		func(event *session.Event) {
			output += collectContentText(event.Content)
		},
	)
	if err != nil {
		return output, err
	}

	return output, nil
}

func collectContentText(content *genai.Content) string {
	if content == nil {
		return ""
	}

	var output string
	for _, part := range content.Parts {
		output += part.Text
	}

	return output
}

func (pa *PhotoAnalyzer) mergeOnsiteFlags(ctx context.Context, result *PhotoAnalysis) {
	reqDeps := GetPhotoAnalyzerDeps(ctx)
	if flags := reqDeps.GetOnsiteFlags(); len(flags) > 0 {
		result.NeedsOnsiteMeasurement = append(result.NeedsOnsiteMeasurement, flags...)
	}
}

func clonePhotoAnalysis(src *PhotoAnalysis) *PhotoAnalysis {
	if src == nil {
		return nil
	}

	clone := *src
	clone.Observations = cloneStringSlice(src.Observations)
	clone.SafetyConcerns = cloneStringSlice(src.SafetyConcerns)
	clone.AdditionalInfo = cloneStringSlice(src.AdditionalInfo)
	clone.Measurements = cloneMeasurements(src.Measurements)
	clone.NeedsOnsiteMeasurement = cloneStringSlice(src.NeedsOnsiteMeasurement)
	clone.Discrepancies = cloneStringSlice(src.Discrepancies)
	clone.ExtractedText = cloneStringSlice(src.ExtractedText)
	clone.SuggestedSearchTerms = cloneStringSlice(src.SuggestedSearchTerms)

	return &clone
}

func cloneMeasurements(src []Measurement) []Measurement {
	if len(src) == 0 {
		return nil
	}

	clone := make([]Measurement, len(src))
	copy(clone, src)
	return clone
}

func cloneStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}

	clone := make([]string, len(src))
	copy(clone, src)
	return clone
}

// ImageData represents an image to analyze
type ImageData struct {
	MIMEType string // e.g., "image/jpeg", "image/png"
	Data     []byte // Raw image bytes
	Filename string // Original filename (optional)
}

// SavePhotoAnalysisInput contains the input parameters for the SavePhotoAnalysis tool
type SavePhotoAnalysisInput struct {
	LeadID               string             `json:"leadId" description:"The UUID of the lead"`
	ServiceID            string             `json:"serviceId" description:"The UUID of the lead service"`
	Summary              string             `json:"summary" description:"A concise 2-3 sentence summary of what the photos show"`
	Observations         []string           `json:"observations" description:"List of specific observations from the photos"`
	ScopeAssessment      string             `json:"scopeAssessment" description:"Assessment of work scope: Small, Medium, Large, or Unclear"`
	CostIndicators       string             `json:"costIndicators" description:"Factors visible that may affect pricing"`
	SafetyConcerns       []string           `json:"safetyConcerns" description:"Any safety issues visible in the photos"`
	AdditionalInfo       []string           `json:"additionalInfo" description:"Additional info or questions to ask the consumer"`
	ConfidenceLevel      string             `json:"confidenceLevel" description:"Analysis confidence: High, Medium, or Low"`
	Measurements         []MeasurementInput `json:"measurements,omitempty" description:"Measurements extracted or estimated from the photos"`
	Discrepancies        []string           `json:"discrepancies,omitempty" description:"Discrepancies between consumer claims and visible evidence"`
	ExtractedText        []string           `json:"extractedText,omitempty" description:"Text, labels, model numbers, or serial numbers read from photos"`
	SuggestedSearchTerms []string           `json:"suggestedSearchTerms,omitempty" description:"Product/material search terms for the Estimator to look up"`
}

// MeasurementInput represents a single measurement from the AI tool
type MeasurementInput struct {
	Description string  `json:"description" description:"What was measured, e.g. 'window width'"`
	Value       float64 `json:"value" description:"Numeric value of the measurement"`
	Unit        string  `json:"unit" description:"Unit of measurement: m, m2, m3, cm, mm, stuks"`
	Type        string  `json:"type" description:"Measurement type: dimension, area, count, or volume"`
	Confidence  string  `json:"confidence" description:"Confidence in measurement: High, Medium, or Low"`
	PhotoRef    string  `json:"photoRef,omitempty" description:"Which photo this was measured from, e.g. 'photo 1'"`
}

// FlagOnsiteMeasurementInput is the input for the FlagOnsiteMeasurement tool
type FlagOnsiteMeasurementInput struct {
	Reason string `json:"reason" description:"Why an on-site measurement is needed, e.g. 'ceiling height not visible from photo angle'"`
}

// SavePhotoAnalysisOutput is the result of saving the photo analysis
type SavePhotoAnalysisOutput struct {
	Success bool   `json:"success"`
	ID      string `json:"id"`
	Message string `json:"message"`
}

func buildPhotoAnalyzerTools() ([]tool.Tool, error) {
	savePhotoAnalysis, err := apptools.NewSavePhotoAnalysisTool(func(ctx tool.Context, args SavePhotoAnalysisInput) (SavePhotoAnalysisOutput, error) {
		deps := GetPhotoAnalyzerDeps(ctx)
		leadID, err := uuid.Parse(args.LeadID)
		if err != nil {
			return SavePhotoAnalysisOutput{Success: false, Message: "Invalid leadId"}, err
		}

		serviceID, err := uuid.Parse(args.ServiceID)
		if err != nil {
			return SavePhotoAnalysisOutput{Success: false, Message: "Invalid serviceId"}, err
		}

		// Normalize confidence level
		confidence := normalizeConfidenceLevel(args.ConfidenceLevel)
		// Normalize scope assessment
		scope := normalizeScopeAssessment(args.ScopeAssessment)

		// Convert measurement inputs to measurements
		measurements := make([]Measurement, 0, len(args.Measurements))
		for _, m := range args.Measurements {
			measurements = append(measurements, Measurement{
				Description: m.Description,
				Value:       m.Value,
				Unit:        m.Unit,
				Type:        normalizeMeasurementType(m.Type),
				Confidence:  normalizeConfidenceLevel(m.Confidence),
				PhotoRef:    m.PhotoRef,
			})
		}

		result := &PhotoAnalysis{
			ID:                   uuid.New(),
			LeadID:               leadID,
			ServiceID:            serviceID,
			Summary:              args.Summary,
			Observations:         args.Observations,
			ScopeAssessment:      scope,
			CostIndicators:       args.CostIndicators,
			SafetyConcerns:       args.SafetyConcerns,
			AdditionalInfo:       args.AdditionalInfo,
			ConfidenceLevel:      confidence,
			Measurements:         measurements,
			Discrepancies:        args.Discrepancies,
			ExtractedText:        args.ExtractedText,
			SuggestedSearchTerms: args.SuggestedSearchTerms,
		}

		deps.SetResult(result)

		log.Printf("Photo analysis saved for lead %s service %s (measurements=%d, discrepancies=%d, extractedText=%d, searchTerms=%d)",
			leadID, serviceID, len(measurements), len(args.Discrepancies), len(args.ExtractedText), len(args.SuggestedSearchTerms))

		return SavePhotoAnalysisOutput{
			Success: true,
			ID:      result.ID.String(),
			Message: "Photo analysis saved successfully",
		}, nil
	})
	if err != nil {
		return nil, err
	}

	// Calculator for exact arithmetic (reuse the shared handler from tools.go)
	calculator, err := createCalculatorTool()
	if err != nil {
		return nil, err
	}

	// FlagOnsiteMeasurement accumulates reasons why on-site measurement is needed
	flagOnsite, err := apptools.NewFlagOnsiteMeasurementTool(func(ctx tool.Context, args FlagOnsiteMeasurementInput) (map[string]any, error) {
		deps := GetPhotoAnalyzerDeps(ctx)
		if args.Reason == "" {
			return map[string]any{"success": false, "message": "reason is required"}, nil
		}
		deps.AddOnsiteFlag(args.Reason)
		log.Printf("Flagged on-site measurement needed: %s", args.Reason)
		return map[string]any{"success": true, "message": "On-site flag recorded"}, nil
	})
	if err != nil {
		return nil, err
	}

	return []tool.Tool{savePhotoAnalysis, calculator, flagOnsite}, nil
}

func normalizeConfidenceLevel(level string) string {
	switch level {
	case "High", "HIGH", "high", "Hoog", "hoog":
		return "High"
	case "Low", "LOW", "low", "Laag", "laag":
		return "Low"
	default:
		return "Medium"
	}
}

func normalizeScopeAssessment(scope string) string {
	// Normalize various AI responses to allowed values
	switch scope {
	case "Small", "SMALL", "small", "Klein", "klein", "Minor", "minor":
		return "Small"
	case "Medium", "MEDIUM", "medium", "Gemiddeld", "gemiddeld", "Moderate", "moderate":
		return "Medium"
	case "Large", "LARGE", "large", "Groot", "groot", "Major", "major", "Extensive", "extensive":
		return "Large"
	case "Unclear", "UNCLEAR", "unclear", "Onduidelijk", "onduidelijk", "Unknown", "unknown":
		return "Unclear"
	default:
		// If not recognized, default to Unclear
		return "Unclear"
	}
}

func normalizeMeasurementType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "dimension", "length", "width", "height", "depth", "lengte", "breedte", "hoogte":
		return "dimension"
	case "area", "oppervlakte", "m2":
		return "area"
	case "count", "aantal", "stuks", "quantity":
		return "count"
	case "volume", "inhoud", "m3":
		return "volume"
	default:
		return "dimension"
	}
}

func buildPhotoAnalysisPrompt(leadID, serviceID uuid.UUID, photoCount int, contextInfo string, serviceType string, intakeRequirements string, preparedImages []PreparedImage) string {
	preprocessingSection := ""
	if preprocessingSummary := buildPreprocessingPromptSection(preparedImages); preprocessingSummary != "" {
		preprocessingSection = wrapReferenceBlock(preprocessingSummary)
	}

	serviceTypeSection := ""
	if serviceType != "" {
		serviceTypeSection = fmt.Sprintf(`## DIENSTTYPE: %s
Pas je analyse aan voor dit specifieke vakgebied. Gebruik je vakkennis over '%s' om:
- Specifieke materialen, componenten en systemen te herkennen
- Relevante Nederlandse bouwstandaarden en -normen toe te passen (NEN, KOMO, BRL, etc.)
- Standaard componenten en configuraties te herkennen zonder speculatieve absolute maatinschattingen
- Productzoektermen te suggereren die de Schatter kan gebruiken om materialen te vinden
- Controleer eerst of de foto's inhoudelijk matchen met dit diensttype. Bij mismatch: zet confidence op Low, benoem de mismatch expliciet in summary én discrepancies, en vermijd speculatieve aannames.`, serviceType, serviceType)
	}

	intakeRequirementsSection := ""
	if intakeRequirements != "" {
		intakeRequirementsSection = fmt.Sprintf(`## INTAKE-EISEN (HARDE EISEN)
Controleer voor elk van deze eisen of ze zichtbaar zijn op de foto's:
%s

Noteer in je observaties welke eisen je kunt bevestigen of weerleggen op basis van de foto's.
Voeg tegenstrijdigheden toe aan discrepancies als claims niet overeenkomen met wat je ziet.`, wrapReferenceBlock(intakeRequirements))
	}

	contextInfoSection := ""
	if contextInfo != "" {
		contextInfoSection = fmt.Sprintf(`## Context van de aanvraag (CLAIMS VAN CONSUMENT):
%s

BELANGRIJK: Vergelijk deze claims kritisch met wat je daadwerkelijk op de foto's ziet.
Als een claim niet klopt met de visuele bewijzen, voeg het toe aan discrepancies.`, wrapReferenceBlock(contextInfo))
	}

	return renderPromptTemplate(photoAnalysisPromptTemplate, struct {
		PhotoCount                int
		LeadID                    string
		ServiceID                 string
		PreprocessingSection      string
		ServiceTypeSection        string
		IntakeRequirementsSection string
		ContextInfoSection        string
	}{
		PhotoCount:                photoCount,
		LeadID:                    leadID.String(),
		ServiceID:                 serviceID.String(),
		PreprocessingSection:      preprocessingSection,
		ServiceTypeSection:        serviceTypeSection,
		IntakeRequirementsSection: intakeRequirementsSection,
		ContextInfoSection:        contextInfoSection,
	})
}

func buildPreprocessingPromptSection(preparedImages []PreparedImage) string {
	if len(preparedImages) == 0 {
		return ""
	}

	var sb strings.Builder
	for index, prepared := range preparedImages {
		appendPreparedImagePromptSummary(&sb, index, prepared)
	}
	return strings.TrimSpace(sb.String())
}

func appendPreparedImagePromptSummary(sb *strings.Builder, index int, prepared PreparedImage) {
	appendPreparedImageMetadataLine(sb, index, prepared.Metadata)
	appendPreparedImageOCRCandidates(sb, prepared.OCRCandidates)
}

func appendPreparedImageMetadataLine(sb *strings.Builder, index int, metadata PreprocessingMetadata) {
	filename := metadata.Filename
	if strings.TrimSpace(filename) == "" {
		filename = fmt.Sprintf("photo-%d", index+1)
	}
	_, _ = fmt.Fprintf(sb, "- Foto %d (%s): %dx%d", index+1, filename, metadata.Width, metadata.Height)
	if metadata.CameraMake != "" || metadata.CameraModel != "" {
		_, _ = fmt.Fprintf(sb, ", camera=%s %s", metadata.CameraMake, metadata.CameraModel)
	}
	if metadata.FocalLengthMM != "" {
		_, _ = fmt.Fprintf(sb, ", focal=%s", metadata.FocalLengthMM)
	}
	if len(metadata.AppliedTransforms) > 0 {
		_, _ = fmt.Fprintf(sb, ", transforms=%s", strings.Join(metadata.AppliedTransforms, ", "))
	}
	if len(metadata.SkippedTransforms) > 0 {
		_, _ = fmt.Fprintf(sb, ", skipped=%s", strings.Join(metadata.SkippedTransforms, ", "))
	}
	sb.WriteString("\n")
}

func appendPreparedImageOCRCandidates(sb *strings.Builder, candidates []OCRCandidate) {
	for _, candidate := range candidates {
		_, _ = fmt.Fprintf(sb, "  OCR assist candidate: %s", candidate.Text)
		if candidate.Source != "" {
			_, _ = fmt.Fprintf(sb, " [source=%s]", candidate.Source)
		}
		sb.WriteString("\n")
	}
}
