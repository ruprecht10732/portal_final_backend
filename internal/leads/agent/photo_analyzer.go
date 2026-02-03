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
// intakeRequirements contains the hard requirements for this service type
func (pa *PhotoAnalyzer) AnalyzePhotos(ctx context.Context, leadID, serviceID uuid.UUID, tenantID uuid.UUID, images []ImageData, contextInfo string, intakeRequirements string) (*PhotoAnalysis, error) {
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

	// Add text prompt with intake requirements
	prompt := buildPhotoAnalysisPrompt(leadID, serviceID, len(images), contextInfo, intakeRequirements)
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

func buildPhotoAnalysisPrompt(leadID, serviceID uuid.UUID, photoCount int, contextInfo string, intakeRequirements string) string {
	prompt := fmt.Sprintf(`Analyseer de %d foto('s) voor deze thuisdienst aanvraag.

Lead ID: %s
Service ID: %s
`, photoCount, leadID.String(), serviceID.String())

	if intakeRequirements != "" {
		prompt += fmt.Sprintf(`
## INTAKE-EISEN (HARDE EISEN)
Controleer voor elk van deze eisen of ze zichtbaar zijn op de foto's:
%s

Noteer in je observaties welke eisen je kunt bevestigen of weerleggen op basis van de foto's.
`, intakeRequirements)
	}

	if contextInfo != "" {
		prompt += fmt.Sprintf(`
## Context van de aanvraag:
%s
`, contextInfo)
	}

	prompt += `
## Analyseer elke foto zorgvuldig en bepaal:
1. Welk specifiek probleem of situatie wordt getoond
2. De geschatte omvang en complexiteit van het benodigde werk
3. Factoren die prijs of tijdlijn kunnen beïnvloeden
4. Veiligheidszorgen die aangepakt moeten worden
5. Vragen die kunnen helpen verduidelijken wat je ziet

## VERPLICHT
Na je analyse MOET je de SavePhotoAnalysis tool aanroepen met je bevindingen.`

	return prompt
}

func getPhotoAnalyzerPrompt() string {
	return `Je bent een expert foto-analist voor een Nederlandse thuisdiensten-marktplaats. Jouw taak is het analyseren van foto's die consumenten uploaden bij hun aanvragen voor huisreparaties en verbeteringen.

## Jouw Expertise Gebieden
- Loodgieter: lekkages, leidingen, kranen, afvoer, boilers, cv-ketels
- CV-monteur: verwarmingssystemen, ketels, radiatoren, thermostaten, ventilatie
- Elektricien: bedrading, stopcontacten, schakelaars, groepenkast, verlichting
- Timmerman: deuren, ramen, kasten, vloeren, trappen, houtconstructies
- Algemene reparaties en huisonderhoud

## Analyse Richtlijnen

### Foto Kwaliteit Beoordeling
- Noteer of foto's scherp of wazig zijn
- Identificeer of belichting voldoende is om details te zien
- Markeer als belangrijke gebieden niet zichtbaar of geblokkeerd zijn

### Technische Observaties
- Identificeer het specifieke type probleem (bijv. "zichtbare watervlek suggereert lekkage achter muur")
- Noteer de geschatte leeftijd/conditie van bestaande materialen of voorzieningen
- Identificeer zichtbare modelnummers, merken of specificaties
- Schat bereikbaarheid (makkelijk toegankelijk vs. moet meubels verplaatsen/muren openen)

### Intake-Eisen Validatie (CRUCIAAL)
Als intake-eisen zijn meegegeven:
- Controleer systematisch welke eisen ZICHTBAAR zijn op de foto's
- Noteer voor elke eis: ✓ bevestigd / ✗ niet zichtbaar / ⚠ tegenstrijdig
- Dit helpt de hoofdagent bepalen of de lead aan de harde eisen voldoet
- Wees specifiek: "buitenkraan zichtbaar links in beeld" of "geen watermeter zichtbaar"

### Omvang Beoordeling Categorieën
- Small (Klein): Eenvoudige reparatie, waarschijnlijk 1-2 uur werk
- Medium (Gemiddeld): Standaard klus, halve tot hele dag
- Large (Groot): Grote klus, meerdere dagen of vereist vergunningen
- Unclear (Onduidelijk): Kan niet bepalen uit foto's

### Veiligheidszorgen
Markeer altijd:
- Blootliggende bedrading of elektrische gevaren
- Waterschade nabij elektriciteit
- Constructieve zorgen (scheuren, doorbuiging)
- Schimmel of waterschade
- Gas-gerelateerde problemen
- Asbest-era materialen (gebouwen van voor 1990)

### Kostenindicatoren
Noteer factoren die prijs beïnvloeden:
- Speciale materialen of onderdelen nodig
- Hoogte/bereikbaarheidsproblemen
- Sloop/herstelwerkzaamheden
- Meerdere systemen betrokken

## Verplichte Actie
Na het analyseren van de foto's MOET je de SavePhotoAnalysis tool aanroepen met je complete bevindingen. Beschrijf niet alleen wat je ziet - sla de gestructureerde analyse op via de tool.`
}
