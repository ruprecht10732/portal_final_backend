package agent

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/ai/moonshot"
)

// PhotoAnalysis represents the result of analyzing photos for a lead service
type PhotoAnalysis struct {
	ID              uuid.UUID `json:"id"`
	LeadID          uuid.UUID `json:"leadId"`
	ServiceID       uuid.UUID `json:"serviceId"`
	Summary         string    `json:"summary"`
	Observations    []string  `json:"observations"`
	ScopeAssessment string    `json:"scopeAssessment"`
	CostIndicators  string    `json:"costIndicators"`
	SafetyConcerns  []string  `json:"safetyConcerns,omitempty"`
	AdditionalInfo  []string  `json:"additionalInfo,omitempty"`
	PhotoCount      int       `json:"photoCount"`
	ConfidenceLevel string    `json:"confidenceLevel"` // High, Medium, Low
}

// PhotoAnalyzerDeps contains dependencies for the photo analyzer
type PhotoAnalyzerDeps struct {
	Repo     repository.LeadsRepository
	mu       sync.RWMutex
	tenantID *uuid.UUID
	// Result storage - set after analysis
	result *PhotoAnalysis
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
	d.result = r
}

func (d *PhotoAnalyzerDeps) GetResult() *PhotoAnalysis {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.result
}

// PhotoAnalyzer provides AI-powered photo analysis for lead services
type PhotoAnalyzer struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
	deps           *PhotoAnalyzerDeps
	runMu          sync.Mutex
}

// NewPhotoAnalyzer creates a new photo analyzer agent
func NewPhotoAnalyzer(apiKey string, repo repository.LeadsRepository) (*PhotoAnalyzer, error) {
	// Use kimi-k2.5 with thinking disabled for multimodal analysis
	kimi := moonshot.NewModel(moonshot.Config{
		APIKey:          apiKey,
		Model:           "kimi-k2.5",
		DisableThinking: true,
	})

	deps := &PhotoAnalyzerDeps{
		Repo: repo,
	}

	analyzer := &PhotoAnalyzer{
		appName: "photo_analyzer",
		deps:    deps,
	}

	tools, err := buildPhotoAnalyzerTools(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build photo analyzer tools: %w", err)
	}

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "PhotoAnalyzer",
		Model:       kimi,
		Description: "Expert AI agent specialized in analyzing photos of home repair and service situations",
		Instruction: getPhotoAnalyzerPrompt(),
		Tools:       tools,
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

// AnalyzePhotos analyzes a set of photos for a lead service
// images should be base64-encoded image data with MIME types
func (pa *PhotoAnalyzer) AnalyzePhotos(ctx context.Context, leadID, serviceID uuid.UUID, tenantID uuid.UUID, images []ImageData, contextInfo string) (*PhotoAnalysis, error) {
	pa.runMu.Lock()
	defer pa.runMu.Unlock()

	if len(images) == 0 {
		return nil, fmt.Errorf("no images provided")
	}

	pa.deps.SetTenantID(tenantID)
	pa.deps.SetResult(nil) // Clear previous result

	// Build multimodal content with images and text
	parts := make([]*genai.Part, 0, len(images)+1)

	// Add images first
	for _, img := range images {
		parts = append(parts, &genai.Part{
			InlineData: &genai.Blob{
				MIMEType: img.MIMEType,
				Data:     img.Data,
			},
		})
	}

	// Add text prompt
	prompt := buildPhotoAnalysisPrompt(leadID, serviceID, len(images), contextInfo)
	parts = append(parts, genai.NewPartFromText(prompt))

	userContent := &genai.Content{
		Role:  "user",
		Parts: parts,
	}

	// Create session
	userID := fmt.Sprintf("photo-analyzer-%s-%s", leadID, serviceID)
	sessionID := uuid.New().String()

	_, err := pa.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   pa.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer func() {
		if deleteErr := pa.sessionService.Delete(ctx, &session.DeleteRequest{
			AppName:   pa.appName,
			UserID:    userID,
			SessionID: sessionID,
		}); deleteErr != nil {
			log.Printf("warning: failed to delete session: %v", deleteErr)
		}
	}()

	// Run analysis
	var output string
	runConfig := agent.RunConfig{
		StreamingMode: agent.StreamingModeNone,
	}

	for event, err := range pa.runner.Run(ctx, userID, sessionID, userContent, runConfig) {
		if err != nil {
			return nil, fmt.Errorf("photo analysis failed: %w", err)
		}
		if event.Content != nil {
			for _, part := range event.Content.Parts {
				output += part.Text
			}
		}
	}

	log.Printf("Photo analysis completed for lead %s service %s. Output: %s", leadID, serviceID, output)

	// Get result from tool call
	result := pa.deps.GetResult()
	if result == nil {
		// Try to force tool call
		retryContent := &genai.Content{
			Role: "user",
			Parts: []*genai.Part{
				genai.NewPartFromText("请选择一个工具（tool）来处理当前的问题。You MUST call the SavePhotoAnalysis tool now with your complete analysis."),
			},
		}

		for event, err := range pa.runner.Run(ctx, userID, sessionID, retryContent, runConfig) {
			if err != nil {
				return nil, fmt.Errorf("photo analysis retry failed: %w", err)
			}
			if event.Content != nil {
				for _, part := range event.Content.Parts {
					output += part.Text
				}
			}
		}

		result = pa.deps.GetResult()
		if result == nil {
			return nil, fmt.Errorf("AI did not save photo analysis")
		}
	}

	return result, nil
}

// ImageData represents an image to analyze
type ImageData struct {
	MIMEType string // e.g., "image/jpeg", "image/png"
	Data     []byte // Raw image bytes
	Filename string // Original filename (optional)
}

// SavePhotoAnalysisInput contains the input parameters for the SavePhotoAnalysis tool
type SavePhotoAnalysisInput struct {
	LeadID          string   `json:"leadId" description:"The UUID of the lead"`
	ServiceID       string   `json:"serviceId" description:"The UUID of the lead service"`
	Summary         string   `json:"summary" description:"A concise 2-3 sentence summary of what the photos show"`
	Observations    []string `json:"observations" description:"List of specific observations from the photos"`
	ScopeAssessment string   `json:"scopeAssessment" description:"Assessment of work scope: Small, Medium, Large, or Unclear"`
	CostIndicators  string   `json:"costIndicators" description:"Factors visible that may affect pricing"`
	SafetyConcerns  []string `json:"safetyConcerns" description:"Any safety issues visible in the photos"`
	AdditionalInfo  []string `json:"additionalInfo" description:"Additional info or questions to ask the consumer"`
	ConfidenceLevel string   `json:"confidenceLevel" description:"Analysis confidence: High, Medium, or Low"`
}

// SavePhotoAnalysisOutput is the result of saving the photo analysis
type SavePhotoAnalysisOutput struct {
	Success bool   `json:"success"`
	ID      string `json:"id"`
	Message string `json:"message"`
}

func buildPhotoAnalyzerTools(deps *PhotoAnalyzerDeps) ([]tool.Tool, error) {
	savePhotoAnalysis, err := functiontool.New(functiontool.Config{
		Name:        "SavePhotoAnalysis",
		Description: "Save the analysis of photos for a lead service. Call this after analyzing all photos.",
	}, func(ctx tool.Context, args SavePhotoAnalysisInput) (SavePhotoAnalysisOutput, error) {
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

		result := &PhotoAnalysis{
			ID:              uuid.New(),
			LeadID:          leadID,
			ServiceID:       serviceID,
			Summary:         args.Summary,
			Observations:    args.Observations,
			ScopeAssessment: scope,
			CostIndicators:  args.CostIndicators,
			SafetyConcerns:  args.SafetyConcerns,
			AdditionalInfo:  args.AdditionalInfo,
			ConfidenceLevel: confidence,
		}

		deps.SetResult(result)

		log.Printf("Photo analysis saved for lead %s service %s", leadID, serviceID)

		return SavePhotoAnalysisOutput{
			Success: true,
			ID:      result.ID.String(),
			Message: "Photo analysis saved successfully",
		}, nil
	})
	if err != nil {
		return nil, err
	}

	return []tool.Tool{savePhotoAnalysis}, nil
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

func buildPhotoAnalysisPrompt(leadID, serviceID uuid.UUID, photoCount int, contextInfo string) string {
	prompt := fmt.Sprintf(`Analyze the %d photo(s) provided for this home service lead.

Lead ID: %s
Service ID: %s
`, photoCount, leadID.String(), serviceID.String())

	if contextInfo != "" {
		prompt += fmt.Sprintf(`
Context from lead:
%s
`, contextInfo)
	}

	prompt += `
Examine each photo carefully and provide:
1. What specific issue or situation is shown
2. The apparent scope and complexity of the work needed
3. Any factors that may affect pricing or timeline
4. Any safety concerns that should be addressed
5. Questions that might help clarify what you see

After your analysis, you MUST call the SavePhotoAnalysis tool with your findings.`

	return prompt
}

func getPhotoAnalyzerPrompt() string {
	return `You are an expert photo analyst for a Dutch home services marketplace. Your job is to analyze photos submitted by consumers requesting home repairs and improvements.

## Your Expertise Areas
- Plumbing (loodgieter): leaks, pipes, fixtures, drainage, water heaters
- HVAC (cv-monteur): heating systems, boilers, radiators, thermostats, ventilation
- Electrical (elektricien): wiring, outlets, switches, panels, lighting
- Carpentry (timmerman): doors, windows, cabinets, flooring, stairs, structural wood
- General repairs and home maintenance

## Analysis Guidelines

### Photo Quality Assessment
- Note if photos are clear or blurry
- Identify if lighting is adequate to see details
- Flag if important areas are not visible or obstructed

### Technical Observations
- Identify the specific type of issue shown (e.g., "visible water stain suggesting leak behind wall")
- Note the apparent age/condition of existing materials or fixtures
- Identify any visible model numbers, brands, or specifications
- Estimate accessibility (easy access vs. need to move furniture/cut walls)

### Scope Assessment Categories
- Small: Simple repair, likely 1-2 hours work
- Medium: Standard job, half day to full day
- Large: Major work, multiple days or requires permits
- Unclear: Cannot determine from photos

### Safety Concerns
Always flag:
- Exposed wiring or electrical hazards
- Water damage near electrical
- Structural concerns (cracks, sagging)
- Mold or water damage
- Gas-related issues
- Asbestos-era materials (pre-1990 buildings)

### Cost Indicators
Note factors affecting price:
- Special materials or parts needed
- Height/accessibility issues
- Demolition/restoration needs
- Multiple system involvement

## Language
Provide your analysis in Dutch, as the service operates in the Netherlands.

## Required Action
After analyzing the photos, you MUST call the SavePhotoAnalysis tool with your complete findings. Do not just describe what you see - save the structured analysis using the tool.`
}
