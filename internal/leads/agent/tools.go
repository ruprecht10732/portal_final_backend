package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/scoring"
	"portal_final_backend/platform/ai/embeddings"
	"portal_final_backend/platform/phone"
	"portal_final_backend/platform/qdrant"
)

const (
	invalidLeadIDMessage        = "Invalid lead ID"
	invalidLeadServiceIDMessage = "Invalid lead service ID"
	missingTenantContextMessage = "Missing tenant context"
	missingLeadContextMessage   = "Missing lead context"
	missingLeadContextError     = "missing lead context"
	leadNotFoundMessage         = "Lead not found"
	leadServiceNotFoundMessage  = "Lead service not found"
	invalidFieldFormat          = "invalid %s"
)

const highConfidenceScoreThreshold = 0.45

// normalizeUrgencyLevel converts various urgency level formats to the required values: High, Medium, Low
func normalizeUrgencyLevel(level string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(level))

	switch normalized {
	case "high", "hoog", "urgent", "spoed", "spoedeisend", "critical":
		return "High", nil
	case "medium", "mid", "moderate", "matig", "gemiddeld", "normal":
		return "Medium", nil
	case "low", "laag", "non-urgent", "niet-urgent", "minor":
		return "Low", nil
	default:
		// If unrecognized, default to Medium but log it
		log.Printf("Unrecognized urgency level '%s', defaulting to Medium", level)
		return "Medium", nil
	}
}

// normalizeLeadQuality converts various lead quality formats to the required values: Junk, Low, Potential, High, Urgent
func normalizeLeadQuality(quality string) string {
	normalized := strings.ToLower(strings.TrimSpace(quality))

	switch normalized {
	case "junk", "spam", "rommel", "onzin", "fake":
		return "Junk"
	case "low", "laag":
		return "Low"
	case "potential", "potentieel", "medium", "gemiddeld", "moderate", "mid":
		return "Potential"
	case "high", "hoog", "good", "goed":
		return "High"
	case "urgent", "spoed", "critical", "kritiek":
		return "Urgent"
	default:
		log.Printf("Unrecognized lead quality '%s', defaulting to Potential", quality)
		return "Potential"
	}
}

// normalizeRecommendedAction converts various action formats to valid values: Reject, RequestInfo, ScheduleSurvey, CallImmediately
func normalizeRecommendedAction(action string) string {
	normalized := strings.ToLower(strings.TrimSpace(action))

	// Check for exact matches first
	switch normalized {
	case "reject", "afwijzen", "weigeren":
		return "Reject"
	case "requestinfo", "request_info", "request info":
		return "RequestInfo"
	case "schedulesurvey", "schedule_survey", "schedule survey", "survey", "opname", "inmeten":
		return "ScheduleSurvey"
	case "callimmediately", "call_immediately", "call immediately", "call", "bellen":
		return "CallImmediately"
	}

	// Check for partial matches (LLM often sends descriptive text)
	if strings.Contains(normalized, "reject") || strings.Contains(normalized, "spam") || strings.Contains(normalized, "junk") {
		return "Reject"
	}
	if strings.Contains(normalized, "call") || strings.Contains(normalized, "bel") || strings.Contains(normalized, "phone") {
		return "CallImmediately"
	}
	if strings.Contains(normalized, "survey") || strings.Contains(normalized, "opname") || strings.Contains(normalized, "inmeten") || strings.Contains(normalized, "schedule") {
		return "ScheduleSurvey"
	}
	// Default: anything about info, contact, nurture, clarification → RequestInfo
	if strings.Contains(normalized, "info") || strings.Contains(normalized, "contact") ||
		strings.Contains(normalized, "nurtur") || strings.Contains(normalized, "clarif") ||
		strings.Contains(normalized, "request") || strings.Contains(normalized, "more") ||
		strings.Contains(normalized, "review") {
		return "RequestInfo"
	}

	log.Printf("Unrecognized recommended action '%s', defaulting to RequestInfo", action)
	return "RequestInfo"
}

func normalizeConsumerRole(role string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(role))
	switch normalized {
	case "owner":
		return "Owner", nil
	case "tenant":
		return "Tenant", nil
	case "landlord":
		return "Landlord", nil
	default:
		return "", fmt.Errorf("invalid consumer role")
	}
}

// ToolDependencies contains the dependencies needed by tools
type ToolDependencies struct {
	Repo                 repository.LeadsRepository
	Scorer               *scoring.Service
	EventBus             events.Bus
	EmbeddingClient      *embeddings.Client
	QdrantClient         *qdrant.Client
	CatalogQdrantClient  *qdrant.Client
	CatalogReader        ports.CatalogReader // optional: hydrate search results from DB
	QuoteDrafter         ports.QuoteDrafter  // optional: draft quotes from agent
	OfferCreator         ports.PartnerOfferCreator
	mu                   sync.RWMutex
	tenantID             *uuid.UUID
	leadID               *uuid.UUID
	serviceID            *uuid.UUID
	actorType            string
	actorName            string
	existingQuoteID      *uuid.UUID              // If set, DraftQuote updates this quote instead of creating new
	lastAnalysisMetadata map[string]any          // Populated by SaveAnalysis for use in stage_change events
	saveAnalysisCalled   bool                    // Track if SaveAnalysis was called
	saveEstimationCalled bool                    // Track if SaveEstimation was called
	stageUpdateCalled    bool                    // Track if UpdatePipelineStage was called
	lastStageUpdated     string                  // Track last pipeline stage written
	draftQuoteCalled     bool                    // Track if DraftQuote was called
	offerCreated         bool                    // Track if CreatePartnerOffer was called
	lastDraftResult      *ports.DraftQuoteResult // Captured by handleDraftQuote for generate endpoint
}

func (d *ToolDependencies) SetTenantID(tenantID uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tenantID = &tenantID
}

func (d *ToolDependencies) GetTenantID() (*uuid.UUID, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tenantID == nil {
		return nil, false
	}
	return d.tenantID, true
}

func (d *ToolDependencies) SetLeadContext(leadID, serviceID uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.leadID = &leadID
	d.serviceID = &serviceID
}

func (d *ToolDependencies) GetLeadContext() (uuid.UUID, uuid.UUID, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.leadID == nil || d.serviceID == nil {
		return uuid.UUID{}, uuid.UUID{}, false
	}
	return *d.leadID, *d.serviceID, true
}

func (d *ToolDependencies) SetActor(actorType, actorName string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.actorType = actorType
	d.actorName = actorName
}

func (d *ToolDependencies) GetActor() (string, string) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.actorType == "" {
		return "AI", "Agent"
	}
	return d.actorType, d.actorName
}

// SetLastAnalysisMetadata stores the analysis metadata for inclusion in subsequent events
func (d *ToolDependencies) SetLastAnalysisMetadata(metadata map[string]any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastAnalysisMetadata = metadata
}

// GetLastAnalysisMetadata retrieves the analysis metadata saved by SaveAnalysis
func (d *ToolDependencies) GetLastAnalysisMetadata() map[string]any {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastAnalysisMetadata
}

// MarkSaveAnalysisCalled marks that SaveAnalysis tool was called
func (d *ToolDependencies) MarkSaveAnalysisCalled() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.saveAnalysisCalled = true
	log.Printf("ToolDependencies: MarkSaveAnalysisCalled() - set to true")
}

// MarkSaveEstimationCalled marks that SaveEstimation tool was called.
func (d *ToolDependencies) MarkSaveEstimationCalled() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.saveEstimationCalled = true
}

// MarkStageUpdateCalled marks that UpdatePipelineStage tool was called
func (d *ToolDependencies) MarkStageUpdateCalled(stage string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stageUpdateCalled = true
	d.lastStageUpdated = stage
}

// MarkDraftQuoteCalled marks that DraftQuote tool was called.
func (d *ToolDependencies) MarkDraftQuoteCalled() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.draftQuoteCalled = true
}

// MarkOfferCreated marks that CreatePartnerOffer tool was called.
func (d *ToolDependencies) MarkOfferCreated() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.offerCreated = true
}

// WasSaveAnalysisCalled returns whether SaveAnalysis was called
func (d *ToolDependencies) WasSaveAnalysisCalled() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.saveAnalysisCalled
}

// WasSaveEstimationCalled returns whether SaveEstimation was called.
func (d *ToolDependencies) WasSaveEstimationCalled() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.saveEstimationCalled
}

// WasStageUpdateCalled returns whether UpdatePipelineStage was called
func (d *ToolDependencies) WasStageUpdateCalled() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.stageUpdateCalled
}

// LastStageUpdated returns the last stage recorded by UpdatePipelineStage.
func (d *ToolDependencies) LastStageUpdated() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastStageUpdated
}

// WasDraftQuoteCalled returns whether DraftQuote was called.
func (d *ToolDependencies) WasDraftQuoteCalled() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.draftQuoteCalled
}

// WasOfferCreated returns whether CreatePartnerOffer was called.
func (d *ToolDependencies) WasOfferCreated() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.offerCreated
}

// SetExistingQuoteID sets the existing quote ID for update-instead-of-create behavior.
func (d *ToolDependencies) SetExistingQuoteID(id *uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.existingQuoteID = id
}

// GetExistingQuoteID returns the existing quote ID if set.
func (d *ToolDependencies) GetExistingQuoteID() *uuid.UUID {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.existingQuoteID
}

// ResetToolCallTracking resets the tool call tracking flags
func (d *ToolDependencies) ResetToolCallTracking() {
	d.mu.Lock()
	defer d.mu.Unlock()
	log.Printf("ToolDependencies: ResetToolCallTracking() - resetting flags (was saveAnalysisCalled=%v)", d.saveAnalysisCalled)
	d.saveAnalysisCalled = false
	d.saveEstimationCalled = false
	d.stageUpdateCalled = false
	d.lastStageUpdated = ""
	d.draftQuoteCalled = false
	d.offerCreated = false
	d.lastAnalysisMetadata = nil
	d.lastDraftResult = nil
	d.existingQuoteID = nil
}

// SetLastDraftResult stores the last DraftQuoteResult for retrieval by callers.
func (d *ToolDependencies) SetLastDraftResult(result *ports.DraftQuoteResult) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastDraftResult = result
}

// GetLastDraftResult returns the last DraftQuoteResult (set by handleDraftQuote).
func (d *ToolDependencies) GetLastDraftResult() *ports.DraftQuoteResult {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastDraftResult
}

// IsProductSearchEnabled returns true if both embedding and Qdrant clients are configured.
func (d *ToolDependencies) IsProductSearchEnabled() bool {
	return d.EmbeddingClient != nil && (d.CatalogQdrantClient != nil || d.QdrantClient != nil)
}

func parseUUID(value string, invalidMessage string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return uuid.UUID{}, errors.New(invalidMessage)
	}
	return parsed, nil
}

func getTenantID(deps *ToolDependencies) (uuid.UUID, error) {
	tenantID, ok := deps.GetTenantID()
	if !ok {
		return uuid.UUID{}, fmt.Errorf("missing tenant context")
	}
	return *tenantID, nil
}

func getLeadContext(deps *ToolDependencies) (uuid.UUID, uuid.UUID, error) {
	leadID, serviceID, ok := deps.GetLeadContext()
	if !ok {
		return uuid.UUID{}, uuid.UUID{}, errors.New(missingLeadContextError)
	}
	return leadID, serviceID, nil
}

func normalizeContactChannel(channel string) (string, error) {
	clean := strings.TrimSpace(channel)
	normalized := strings.ToLower(clean)

	// WhatsApp variations
	if strings.Contains(normalized, "whatsapp") || normalized == "wa" {
		return "WhatsApp", nil
	}

	// Email variations
	if strings.Contains(normalized, "email") || strings.Contains(normalized, "e-mail") || normalized == "mail" {
		return "Email", nil
	}

	// Phone/call variations - map to WhatsApp since it's our phone-based channel
	if strings.Contains(normalized, "phone") || strings.Contains(normalized, "telefoon") ||
		strings.Contains(normalized, "call") || strings.Contains(normalized, "bel") ||
		normalized == "tel" || normalized == "sms" {
		return "WhatsApp", nil
	}

	// If unrecognized, default to Email and log
	log.Printf("Unrecognized contact channel '%s', defaulting to Email", channel)
	return "Email", nil
}

func resolvePreferredChannel(inputChannel string, lead repository.Lead) (string, error) {
	_, err := normalizeContactChannel(inputChannel)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(lead.ConsumerPhone) != "" {
		return "WhatsApp", nil
	}
	return "Email", nil
}

func parseLeadServiceID(value string) (uuid.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return uuid.UUID{}, fmt.Errorf("missing lead service ID")
	}
	return parseUUID(value, invalidLeadServiceIDMessage)
}

func handleSaveAnalysis(ctx tool.Context, deps *ToolDependencies, input SaveAnalysisInput) (SaveAnalysisOutput, error) {
	log.Printf("handleSaveAnalysis: CALLED with leadID=%s serviceID=%s urgency=%s action=%s",
		input.LeadID, input.LeadServiceID, input.UrgencyLevel, input.RecommendedAction)

	leadID, err := parseUUID(input.LeadID, invalidLeadIDMessage)
	if err != nil {
		log.Printf("handleSaveAnalysis: FAILED - invalid leadID: %s", input.LeadID)
		return SaveAnalysisOutput{Success: false, Message: invalidLeadIDMessage}, err
	}

	tenantID, err := getTenantID(deps)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: missingTenantContextMessage}, err
	}

	leadServiceID, err := parseLeadServiceID(input.LeadServiceID)
	if err != nil {
		message := err.Error()
		if err.Error() == invalidLeadServiceIDMessage {
			message = invalidLeadServiceIDMessage
		}
		return SaveAnalysisOutput{Success: false, Message: message}, err
	}

	// Terminal check: refuse to save analysis for terminal services
	svc, err := deps.Repo.GetLeadServiceByID(ctx, leadServiceID, tenantID)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: leadServiceNotFoundMessage}, err
	}
	if domain.IsTerminal(svc.Status, svc.PipelineStage) {
		log.Printf("handleSaveAnalysis: REJECTED - service %s is in terminal state (status=%s, stage=%s)", leadServiceID, svc.Status, svc.PipelineStage)
		return SaveAnalysisOutput{Success: false, Message: "Cannot save analysis for a service in terminal state"}, fmt.Errorf("service %s is terminal", leadServiceID)
	}

	urgencyLevel, err := normalizeUrgencyLevel(input.UrgencyLevel)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: err.Error()}, err
	}

	var urgencyReason *string
	if input.UrgencyReason != "" {
		urgencyReason = &input.UrgencyReason
	}

	lead, err := deps.Repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: leadNotFoundMessage}, err
	}

	channel, err := resolvePreferredChannel(input.PreferredContactChannel, lead)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: "Invalid preferred contact channel"}, err
	}

	// Normalize lead quality to valid enum value
	leadQuality := normalizeLeadQuality(input.LeadQuality)

	// Normalize recommended action to valid enum value
	recommendedAction := normalizeRecommendedAction(input.RecommendedAction)
	log.Printf("handleSaveAnalysis: normalized recommendedAction '%s' -> '%s'", input.RecommendedAction, recommendedAction)

	_, err = deps.Repo.CreateAIAnalysis(context.Background(), repository.CreateAIAnalysisParams{
		LeadID:                  leadID,
		OrganizationID:          tenantID,
		LeadServiceID:           leadServiceID,
		UrgencyLevel:            urgencyLevel,
		UrgencyReason:           urgencyReason,
		LeadQuality:             leadQuality,
		RecommendedAction:       recommendedAction,
		MissingInformation:      input.MissingInformation,
		PreferredContactChannel: channel,
		SuggestedContactMessage: input.SuggestedContactMessage,
		Summary:                 input.Summary,
	})
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: err.Error()}, err
	}

	actorType, actorName := deps.GetActor()

	// Create comprehensive analysis timeline event for frontend rendering
	analysisSummary := input.Summary
	if analysisSummary == "" {
		analysisSummary = fmt.Sprintf("AI analyse voltooid: %s urgentie, aanbevolen actie: %s", urgencyLevel, recommendedAction)
	}
	analysisMetadata := map[string]any{
		"urgencyLevel":      urgencyLevel,
		"recommendedAction": recommendedAction,
		"leadQuality":       leadQuality,
	}
	if input.SuggestedContactMessage != "" {
		analysisMetadata["suggestedContactMessage"] = input.SuggestedContactMessage
		analysisMetadata["preferredContactChannel"] = string(channel)
	}
	if len(input.MissingInformation) > 0 {
		analysisMetadata["missingInformation"] = input.MissingInformation
	}
	_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &leadServiceID,
		OrganizationID: tenantID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      "ai",
		Title:          "Gatekeeper analyse voltooid",
		Summary:        &analysisSummary,
		Metadata:       analysisMetadata,
	})

	// Store analysis metadata for use in stage_change events
	deps.SetLastAnalysisMetadata(analysisMetadata)
	log.Printf("SaveAnalysis: stored analysis metadata for lead=%s service=%s channel=%s action=%s",
		leadID, leadServiceID, channel, recommendedAction)

	recalculateAndRecordScore(ctx, deps, leadID, leadServiceID, tenantID, actorType, actorName)

	log.Printf(
		"gatekeeper SaveAnalysis: leadId=%s serviceId=%s urgency=%s quality=%s action=%s missing=%d",
		leadID,
		leadServiceID,
		urgencyLevel,
		leadQuality,
		recommendedAction,
		len(input.MissingInformation),
	)

	deps.MarkSaveAnalysisCalled()
	return SaveAnalysisOutput{Success: true, Message: "Analysis saved successfully"}, nil
}

func recalculateAndRecordScore(ctx tool.Context, deps *ToolDependencies, leadID, leadServiceID, tenantID uuid.UUID, actorType, actorName string) {
	if deps.Scorer == nil {
		return
	}
	scoreResult, scoreErr := deps.Scorer.Recalculate(ctx, leadID, &leadServiceID, tenantID, true)
	if scoreErr != nil {
		return
	}
	_ = deps.Repo.UpdateLeadScore(ctx, leadID, tenantID, repository.UpdateLeadScoreParams{
		Score:          &scoreResult.Score,
		ScorePreAI:     &scoreResult.ScorePreAI,
		ScoreFactors:   scoreResult.FactorsJSON,
		ScoreVersion:   &scoreResult.Version,
		ScoreUpdatedAt: scoreResult.UpdatedAt,
	})

	summary := buildLeadScoreSummary(scoreResult)
	_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &leadServiceID,
		OrganizationID: tenantID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      "analysis",
		Title:          "Leadscore bijgewerkt",
		Summary:        &summary,
		Metadata: map[string]any{
			"leadScore":        scoreResult.Score,
			"leadScorePreAI":   scoreResult.ScorePreAI,
			"leadScoreVersion": scoreResult.Version,
		},
	})
}

func buildLeadScoreSummary(result *scoring.Result) string {
	return fmt.Sprintf("Leadscore %d (pre-AI %d)", result.Score, result.ScorePreAI)
}

func handleUpdateLeadServiceType(ctx tool.Context, deps *ToolDependencies, input UpdateLeadServiceTypeInput) (UpdateLeadServiceTypeOutput, error) {
	leadID, err := parseUUID(input.LeadID, invalidLeadIDMessage)
	if err != nil {
		return UpdateLeadServiceTypeOutput{Success: false, Message: invalidLeadIDMessage}, err
	}
	leadServiceID, err := parseUUID(input.LeadServiceID, invalidLeadServiceIDMessage)
	if err != nil {
		return UpdateLeadServiceTypeOutput{Success: false, Message: invalidLeadServiceIDMessage}, err
	}
	serviceType := strings.TrimSpace(input.ServiceType)
	if serviceType == "" {
		return UpdateLeadServiceTypeOutput{Success: false, Message: "Missing service type"}, fmt.Errorf("missing service type")
	}

	tenantID, err := getTenantID(deps)
	if err != nil {
		return UpdateLeadServiceTypeOutput{Success: false, Message: missingTenantContextMessage}, err
	}

	leadService, err := deps.Repo.GetLeadServiceByID(ctx, leadServiceID, tenantID)
	if err != nil {
		return UpdateLeadServiceTypeOutput{Success: false, Message: leadServiceNotFoundMessage}, err
	}
	if leadService.LeadID != leadID {
		return UpdateLeadServiceTypeOutput{Success: false, Message: "Lead service does not belong to lead"}, fmt.Errorf("lead service mismatch")
	}

	_, err = deps.Repo.UpdateLeadServiceType(ctx, leadServiceID, tenantID, serviceType)
	if err != nil {
		if errors.Is(err, repository.ErrServiceTypeNotFound) {
			return UpdateLeadServiceTypeOutput{Success: false, Message: "Service type not found or inactive"}, nil
		}
		return UpdateLeadServiceTypeOutput{Success: false, Message: "Failed to update service type"}, err
	}

	actorType, actorName := deps.GetActor()
	reasonText := strings.TrimSpace(input.Reason)
	if reasonText == "" {
		reasonText = "Diensttype aangepast"
	}
	_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &leadServiceID,
		OrganizationID: tenantID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      "service_type_change",
		Title:          "Diensttype bijgewerkt",
		Summary:        &reasonText,
		Metadata: map[string]any{
			"oldServiceType": leadService.ServiceType,
			"newServiceType": serviceType,
			"reason":         input.Reason,
		},
	})

	log.Printf(
		"gatekeeper UpdateLeadServiceType: leadId=%s serviceId=%s from=%s to=%s",
		leadID,
		leadServiceID,
		leadService.ServiceType,
		serviceType,
	)

	return UpdateLeadServiceTypeOutput{Success: true, Message: "Service type updated"}, nil
}

// leadDetailsBuilder encapsulates field update logic for handleUpdateLeadDetails
type leadDetailsBuilder struct {
	params        repository.UpdateLeadParams
	updatedFields []string
}

func newLeadDetailsBuilder() *leadDetailsBuilder {
	return &leadDetailsBuilder{
		params:        repository.UpdateLeadParams{},
		updatedFields: make([]string, 0, 10),
	}
}

func (b *leadDetailsBuilder) setStringField(input *string, current string, fieldName string, setter func(*string)) error {
	if input == nil {
		return nil
	}
	value := strings.TrimSpace(*input)
	if value == "" {
		return fmt.Errorf(invalidFieldFormat, fieldName)
	}
	setter(&value)
	if value != current {
		b.updatedFields = append(b.updatedFields, fieldName)
	}
	return nil
}

func (b *leadDetailsBuilder) setOptionalStringField(input *string, current *string, fieldName string, setter func(*string)) error {
	if input == nil {
		return nil
	}
	value := strings.TrimSpace(*input)
	if value == "" {
		return fmt.Errorf(invalidFieldFormat, fieldName)
	}
	setter(&value)
	if current == nil || *current != value {
		b.updatedFields = append(b.updatedFields, fieldName)
	}
	return nil
}

func (b *leadDetailsBuilder) setPhoneField(input *string, current string) error {
	if input == nil {
		return nil
	}
	value := phone.NormalizeE164(strings.TrimSpace(*input))
	if value == "" {
		return fmt.Errorf("invalid phone")
	}
	b.params.ConsumerPhone = &value
	if value != current {
		b.updatedFields = append(b.updatedFields, "phone")
	}
	return nil
}

func (b *leadDetailsBuilder) setConsumerRole(input *string, current string) error {
	if input == nil {
		return nil
	}
	role, err := normalizeConsumerRole(*input)
	if err != nil {
		return fmt.Errorf("invalid consumer role")
	}
	b.params.ConsumerRole = &role
	if role != current {
		b.updatedFields = append(b.updatedFields, "consumerRole")
	}
	return nil
}

func (b *leadDetailsBuilder) setCoordinate(input *float64, current *float64, fieldName string, min, max float64, setter func(*float64)) error {
	if input == nil {
		return nil
	}
	if *input < min || *input > max {
		return fmt.Errorf(invalidFieldFormat, fieldName)
	}
	setter(input)
	if current == nil || *current != *input {
		b.updatedFields = append(b.updatedFields, fieldName)
	}
	return nil
}

func (b *leadDetailsBuilder) buildFromInput(input UpdateLeadDetailsInput, current repository.Lead) error {
	if err := b.setStringField(input.FirstName, current.ConsumerFirstName, "firstName", func(v *string) { b.params.ConsumerFirstName = v }); err != nil {
		return err
	}
	if err := b.setStringField(input.LastName, current.ConsumerLastName, "lastName", func(v *string) { b.params.ConsumerLastName = v }); err != nil {
		return err
	}
	if err := b.setPhoneField(input.Phone, current.ConsumerPhone); err != nil {
		return err
	}
	if err := b.setOptionalStringField(input.Email, current.ConsumerEmail, "email", func(v *string) { b.params.ConsumerEmail = v }); err != nil {
		return err
	}
	if err := b.setConsumerRole(input.ConsumerRole, current.ConsumerRole); err != nil {
		return err
	}
	if err := b.setStringField(input.Street, current.AddressStreet, "street", func(v *string) { b.params.AddressStreet = v }); err != nil {
		return err
	}
	if err := b.setStringField(input.HouseNumber, current.AddressHouseNumber, "houseNumber", func(v *string) { b.params.AddressHouseNumber = v }); err != nil {
		return err
	}
	if err := b.setStringField(input.ZipCode, current.AddressZipCode, "zipCode", func(v *string) { b.params.AddressZipCode = v }); err != nil {
		return err
	}
	if err := b.setStringField(input.City, current.AddressCity, "city", func(v *string) { b.params.AddressCity = v }); err != nil {
		return err
	}
	if err := b.setCoordinate(input.Latitude, current.Latitude, "latitude", -90, 90, func(v *float64) { b.params.Latitude = v }); err != nil {
		return err
	}
	if err := b.setCoordinate(input.Longitude, current.Longitude, "longitude", -180, 180, func(v *float64) { b.params.Longitude = v }); err != nil {
		return err
	}
	return nil
}

func handleUpdateLeadDetails(ctx tool.Context, deps *ToolDependencies, input UpdateLeadDetailsInput) (UpdateLeadDetailsOutput, error) {
	leadID, err := parseUUID(input.LeadID, invalidLeadIDMessage)
	if err != nil {
		return UpdateLeadDetailsOutput{Success: false, Message: invalidLeadIDMessage}, err
	}

	tenantID, err := getTenantID(deps)
	if err != nil {
		return UpdateLeadDetailsOutput{Success: false, Message: missingTenantContextMessage}, err
	}

	current, err := deps.Repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return UpdateLeadDetailsOutput{Success: false, Message: leadNotFoundMessage}, err
	}

	builder := newLeadDetailsBuilder()
	if err := builder.buildFromInput(input, current); err != nil {
		return UpdateLeadDetailsOutput{Success: false, Message: err.Error()}, err
	}

	if len(builder.updatedFields) == 0 {
		return UpdateLeadDetailsOutput{Success: true, Message: "No updates required"}, nil
	}

	_, err = deps.Repo.Update(ctx, leadID, tenantID, builder.params)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return UpdateLeadDetailsOutput{Success: false, Message: leadNotFoundMessage}, err
		}
		return UpdateLeadDetailsOutput{Success: false, Message: "Failed to update lead"}, err
	}

	recordLeadDetailsUpdate(ctx, deps, leadID, tenantID, builder.updatedFields, input.Reason, input.Confidence)
	return UpdateLeadDetailsOutput{Success: true, Message: "Lead updated", UpdatedFields: builder.updatedFields}, nil
}

func recordLeadDetailsUpdate(ctx tool.Context, deps *ToolDependencies, leadID, tenantID uuid.UUID, updatedFields []string, reason string, confidence *float64) {
	actorType, actorName := deps.GetActor()
	reasonText := strings.TrimSpace(reason)
	if reasonText == "" {
		reasonText = "Leadgegevens bijgewerkt"
	}
	metadata := map[string]any{
		"updatedFields": updatedFields,
	}
	if confidence != nil {
		metadata["confidence"] = *confidence
	}

	var serviceID *uuid.UUID
	if _, svcID, ok := deps.GetLeadContext(); ok {
		serviceID = &svcID
	}

	_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      serviceID,
		OrganizationID: tenantID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      "lead_update",
		Title:          "Leadgegevens bijgewerkt",
		Summary:        &reasonText,
		Metadata:       metadata,
	})

	log.Printf("gatekeeper UpdateLeadDetails: leadId=%s fields=%v reason=%s", leadID, updatedFields, reasonText)
}

// createSaveAnalysisTool creates the SaveAnalysis tool
func createSaveAnalysisTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "SaveAnalysis",
		Description: "Saves the gatekeeper triage analysis to the database. Call this ONCE after completing your full analysis. Include urgency, lead quality, recommended action, missing information, preferred contact channel, message, and summary.",
	}, func(ctx tool.Context, input SaveAnalysisInput) (SaveAnalysisOutput, error) {
		return handleSaveAnalysis(ctx, deps, input)
	})
}

// createUpdateLeadServiceTypeTool creates the UpdateLeadServiceType tool
func createUpdateLeadServiceTypeTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "UpdateLeadServiceType",
		Description: "Updates the service type for a lead service when there is a confident mismatch. The service type must match an active service type name or slug.",
	}, func(ctx tool.Context, input UpdateLeadServiceTypeInput) (UpdateLeadServiceTypeOutput, error) {
		return handleUpdateLeadServiceType(ctx, deps, input)
	})
}

func createUpdateLeadDetailsTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "UpdateLeadDetails",
		Description: "Updates lead contact or address details when you are highly confident the current data is wrong.",
	}, func(ctx tool.Context, input UpdateLeadDetailsInput) (UpdateLeadDetailsOutput, error) {
		return handleUpdateLeadDetails(ctx, deps, input)
	})
}

var validPipelineStages = map[string]bool{
	"Triage":              true,
	"Nurturing":           true,
	"Ready_For_Estimator": true,
	"Quote_Sent":          true,
	"Ready_For_Partner":   true,
	"Partner_Matching":    true,
	"Partner_Assigned":    true,
	"Manual_Intervention": true,
	"Completed":           true,
	"Lost":                true,
}

func handleUpdatePipelineStage(ctx tool.Context, deps *ToolDependencies, input UpdatePipelineStageInput) (UpdatePipelineStageOutput, error) {
	if !validPipelineStages[input.Stage] {
		return UpdatePipelineStageOutput{Success: false, Message: "Invalid pipeline stage"}, fmt.Errorf("invalid pipeline stage: %s", input.Stage)
	}

	tenantID, err := getTenantID(deps)
	if err != nil {
		return UpdatePipelineStageOutput{Success: false, Message: missingTenantContextMessage}, err
	}

	leadID, serviceID, err := getLeadContext(deps)
	if err != nil {
		return UpdatePipelineStageOutput{Success: false, Message: missingLeadContextMessage}, err
	}

	_, actorName := deps.GetActor()
	if actorName == "Dispatcher" && input.Stage == "Partner_Matching" && !deps.WasOfferCreated() {
		return UpdatePipelineStageOutput{Success: false, Message: "CreatePartnerOffer must be called before Partner_Matching"}, fmt.Errorf("offer not created")
	}

	svc, err := deps.Repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return UpdatePipelineStageOutput{Success: false, Message: leadServiceNotFoundMessage}, err
	}
	oldStage := svc.PipelineStage

	// Terminal check: refuse to update pipeline stage for terminal services
	if domain.IsTerminal(svc.Status, svc.PipelineStage) {
		log.Printf("handleUpdatePipelineStage: REJECTED - service %s is in terminal state (status=%s, stage=%s)", serviceID, svc.Status, svc.PipelineStage)
		return UpdatePipelineStageOutput{Success: false, Message: "Cannot update pipeline stage for a service in terminal state"}, fmt.Errorf("service %s is terminal", serviceID)
	}

	// Validate state combination
	if reason := domain.ValidateStateCombination(svc.Status, input.Stage); reason != "" {
		log.Printf("handleUpdatePipelineStage: invalid state combination: status=%s, newStage=%s - %s", svc.Status, input.Stage, reason)
		return UpdatePipelineStageOutput{Success: false, Message: reason}, fmt.Errorf("invalid state combination: %s", reason)
	}

	_, err = deps.Repo.UpdatePipelineStage(ctx, serviceID, tenantID, input.Stage)
	if err != nil {
		return UpdatePipelineStageOutput{Success: false, Message: "Failed to update pipeline stage"}, err
	}

	recordPipelineStageChange(ctx, deps, stageChangeParams{
		leadID:    leadID,
		serviceID: serviceID,
		tenantID:  tenantID,
		oldStage:  oldStage,
		newStage:  input.Stage,
		reason:    input.Reason,
	})
	deps.MarkStageUpdateCalled(input.Stage)
	return UpdatePipelineStageOutput{Success: true, Message: "Pipeline stage updated"}, nil
}

// stageChangeParams groups parameters for recording a pipeline stage change.
type stageChangeParams struct {
	leadID    uuid.UUID
	serviceID uuid.UUID
	tenantID  uuid.UUID
	oldStage  string
	newStage  string
	reason    string
}

func recordPipelineStageChange(ctx tool.Context, deps *ToolDependencies, p stageChangeParams) {
	actorType, actorName := deps.GetActor()
	reasonText := strings.TrimSpace(p.reason)
	var summary *string
	if reasonText != "" {
		summary = &reasonText
	}

	stageMetadata := map[string]any{
		"oldStage": p.oldStage,
		"newStage": p.newStage,
	}
	if analysisMeta := deps.GetLastAnalysisMetadata(); analysisMeta != nil {
		stageMetadata["analysis"] = analysisMeta
	}

	_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         p.leadID,
		ServiceID:      &p.serviceID,
		OrganizationID: p.tenantID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      "stage_change",
		Title:          "Fase bijgewerkt",
		Summary:        summary,
		Metadata:       stageMetadata,
	})

	if deps.EventBus != nil {
		deps.EventBus.Publish(ctx, events.PipelineStageChanged{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        p.leadID,
			LeadServiceID: p.serviceID,
			TenantID:      p.tenantID,
			OldStage:      p.oldStage,
			NewStage:      p.newStage,
		})
	}

	logReason := reasonText
	if logReason == "" {
		logReason = "(no reason provided)"
	}
	log.Printf("gatekeeper UpdatePipelineStage: leadId=%s serviceId=%s from=%s to=%s reason=%s",
		p.leadID, p.serviceID, p.oldStage, p.newStage, logReason)
}

func createUpdatePipelineStageTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "UpdatePipelineStage",
		Description: "Updates the pipeline stage for the lead service and records a timeline event.",
	}, func(ctx tool.Context, input UpdatePipelineStageInput) (UpdatePipelineStageOutput, error) {
		return handleUpdatePipelineStage(ctx, deps, input)
	})
}

func createFindMatchingPartnersTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "FindMatchingPartners",
		Description: "Finds partner matches by service type and distance radius. Allows excluding specific partner IDs.",
	}, func(ctx tool.Context, input FindMatchingPartnersInput) (FindMatchingPartnersOutput, error) {
		tenantID, err := getTenantID(deps)
		if err != nil {
			return FindMatchingPartnersOutput{Matches: nil}, err
		}

		excludeUUIDs := make([]uuid.UUID, 0, len(input.ExcludePartnerIDs))
		for _, idStr := range input.ExcludePartnerIDs {
			if uid, err := uuid.Parse(idStr); err == nil {
				excludeUUIDs = append(excludeUUIDs, uid)
			}
		}

		leadID, serviceID, err := getLeadContext(deps)
		if err != nil {
			return FindMatchingPartnersOutput{Matches: nil}, err
		}

		matches, err := deps.Repo.FindMatchingPartners(ctx, tenantID, input.ServiceType, input.ZipCode, input.RadiusKm, excludeUUIDs)
		if err != nil {
			return FindMatchingPartnersOutput{Matches: nil}, err
		}

		actorType, actorName := deps.GetActor()
		summary := fmt.Sprintf("Found %d partner(s)", len(matches))
		_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
			LeadID:         leadID,
			ServiceID:      &serviceID,
			OrganizationID: tenantID,
			ActorType:      actorType,
			ActorName:      actorName,
			EventType:      "partner_search",
			Title:          "Partnerzoekactie",
			Summary:        &summary,
			Metadata: map[string]any{
				"serviceType": input.ServiceType,
				"zipCode":     input.ZipCode,
				"radiusKm":    input.RadiusKm,
				"excludedIds": excludeUUIDs,
				"matches":     matches,
			},
		})

		output := make([]PartnerMatch, 0, len(matches))
		for _, match := range matches {
			output = append(output, PartnerMatch{
				PartnerID:    match.ID.String(),
				BusinessName: match.BusinessName,
				Email:        match.Email,
				DistanceKm:   match.DistanceKm,
			})
		}

		return FindMatchingPartnersOutput{Matches: output}, nil
	})
}

func resolveOfferContext(deps *ToolDependencies, partnerIDRaw string, expirationHours int) (uuid.UUID, uuid.UUID, uuid.UUID, int, string, error) {
	tenantID, err := getTenantID(deps)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, 0, missingTenantContextMessage, err
	}

	_, serviceID, err := getLeadContext(deps)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, 0, missingLeadContextMessage, err
	}

	partnerID, err := uuid.Parse(partnerIDRaw)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, 0, "Invalid partner ID", err
	}

	hours := expirationHours
	if hours <= 0 {
		hours = 48
	}
	if hours > 168 {
		hours = 168
	}

	return tenantID, serviceID, partnerID, hours, "", nil
}

func resolveOfferPricing(ctx tool.Context, deps *ToolDependencies, serviceID, tenantID uuid.UUID) (string, int64, error) {
	totalCents, err := deps.Repo.GetLatestQuoteTotal(ctx, serviceID, tenantID)
	if err != nil {
		return "", 0, err
	}

	pricingSource := "quote"
	if totalCents <= 0 {
		pricingSource = "estimate"
	}

	return pricingSource, totalCents, nil
}

func createCreatePartnerOfferTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "CreatePartnerOffer",
		Description: "Creates a formal job offer for a specific partner. This generates the unique link they use to accept the job.",
	}, func(ctx tool.Context, input CreatePartnerOfferInput) (CreatePartnerOfferOutput, error) {
		if deps.OfferCreator == nil {
			return CreatePartnerOfferOutput{Success: false, Message: "Offer creation not configured"}, fmt.Errorf("offer creator not configured")
		}

		tenantID, serviceID, partnerID, hours, contextMessage, err := resolveOfferContext(deps, input.PartnerID, input.ExpirationHours)
		if err != nil {
			return CreatePartnerOfferOutput{Success: false, Message: contextMessage}, err
		}

		pricingSource, totalCents, err := resolveOfferPricing(ctx, deps, serviceID, tenantID)
		if err != nil {
			return CreatePartnerOfferOutput{Success: false, Message: "Quote not found for service"}, err
		}

		summary := truncateRunes(strings.TrimSpace(input.JobSummaryShort), 200)
		result, err := deps.OfferCreator.CreateOffer(ctx, tenantID, ports.CreateOfferParams{
			PartnerID:          partnerID,
			LeadServiceID:      serviceID,
			PricingSource:      pricingSource,
			CustomerPriceCents: totalCents,
			ExpiresInHours:     hours,
			JobSummaryShort:    summary,
		})
		if err != nil {
			return CreatePartnerOfferOutput{Success: false, Message: err.Error()}, err
		}

		deps.MarkOfferCreated()

		return CreatePartnerOfferOutput{
			Success:     true,
			Message:     "Offer created",
			OfferID:     result.OfferID.String(),
			PublicToken: result.PublicToken,
		}, nil
	})
}

func createSaveEstimationTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "SaveEstimation",
		Description: "Saves estimation metadata (scope and price range) to the lead timeline.",
	}, func(ctx tool.Context, input SaveEstimationInput) (SaveEstimationOutput, error) {
		tenantID, err := getTenantID(deps)
		if err != nil {
			return SaveEstimationOutput{Success: false, Message: missingTenantContextMessage}, err
		}

		leadID, serviceID, err := getLeadContext(deps)
		if err != nil {
			return SaveEstimationOutput{Success: false, Message: missingLeadContextMessage}, err
		}

		actorType, actorName := deps.GetActor()
		summary := strings.TrimSpace(input.Summary)
		var summaryPtr *string
		if summary != "" {
			summaryPtr = &summary
		}

		_, err = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
			LeadID:         leadID,
			ServiceID:      &serviceID,
			OrganizationID: tenantID,
			ActorType:      actorType,
			ActorName:      actorName,
			EventType:      "analysis",
			Title:          "Schatting opgeslagen",
			Summary:        summaryPtr,
			Metadata: map[string]any{
				"scope":      input.Scope,
				"priceRange": input.PriceRange,
				"notes":      input.Notes,
			},
		})
		if err != nil {
			return SaveEstimationOutput{Success: false, Message: "Failed to save estimation"}, err
		}

		deps.MarkSaveEstimationCalled()

		return SaveEstimationOutput{Success: true, Message: "Estimation saved"}, nil
	})
}

// handleCalculator evaluates a single arithmetic operation deterministically.
// The LLM MUST call this for ANY math instead of doing it in its head.
func handleCalculator(_ tool.Context, input CalculatorInput) (CalculatorOutput, error) {
	var result float64
	var expr string

	switch strings.ToLower(strings.TrimSpace(input.Operation)) {
	case "add":
		result = input.A + input.B
		expr = fmt.Sprintf("%g + %g = %g", input.A, input.B, result)
	case "subtract":
		result = input.A - input.B
		expr = fmt.Sprintf("%g - %g = %g", input.A, input.B, result)
	case "multiply":
		result = input.A * input.B
		expr = fmt.Sprintf("%g × %g = %g", input.A, input.B, result)
	case "divide":
		if input.B == 0 {
			return CalculatorOutput{}, fmt.Errorf("division by zero")
		}
		result = input.A / input.B
		expr = fmt.Sprintf("%g ÷ %g = %g", input.A, input.B, result)
	case "ceil_divide":
		if input.B == 0 {
			return CalculatorOutput{}, fmt.Errorf("division by zero")
		}
		result = math.Ceil(input.A / input.B)
		expr = fmt.Sprintf("⌈%g ÷ %g⌉ = %g", input.A, input.B, result)
	case "ceil":
		result = math.Ceil(input.A)
		expr = fmt.Sprintf("⌈%g⌉ = %g", input.A, result)
	case "floor":
		result = math.Floor(input.A)
		expr = fmt.Sprintf("⌊%g⌋ = %g", input.A, result)
	case "round":
		places := int(input.B)
		if places < 0 {
			places = 0
		}
		if places > 10 {
			places = 10
		}
		factor := math.Pow(10, float64(places))
		result = math.Round(input.A*factor) / factor
		expr = fmt.Sprintf("round(%g, %d) = %g", input.A, places, result)
	case "percentage":
		result = input.A * input.B / 100
		expr = fmt.Sprintf("%g × %g%% = %g", input.A, input.B, result)
	default:
		return CalculatorOutput{}, fmt.Errorf("unknown operation %q; use add, subtract, multiply, divide, ceil_divide, ceil, floor, round, percentage", input.Operation)
	}

	return CalculatorOutput{Result: result, Expression: expr}, nil
}

func createCalculatorTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "Calculator",
		Description: `Performs exact arithmetic. You MUST use this for ANY calculation — never do math yourself.
Supported operations:
  "add"         → a + b
  "subtract"    → a - b
  "multiply"    → a × b
  "divide"      → a ÷ b
  "ceil_divide" → ⌈a ÷ b⌉  (divide then round UP — use for quantity-needed calculations)
  "ceil"        → ⌈a⌉      (round a up to nearest integer)
  "floor"       → ⌊a⌋      (round a down to nearest integer)
  "round"       → round a to b decimal places
  "percentage"  → a × b / 100  (e.g., tax amount)
Examples:
  Window area 2m × 1.5m: Calculator(operation="multiply", a=2, b=1.5) → 3
  Sheets needed: 4 m² ÷ 2.5 m²/sheet, round up: Calculator(operation="ceil_divide", a=4, b=2.5) → 2
  Price total: Calculator(operation="multiply", a=15.99, b=3) → 47.97`,
	}, handleCalculator)
}

func createCalculateEstimateTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "CalculateEstimate",
		Description: "Calculates material subtotal, labor subtotal range, and total range from structured inputs.",
	}, func(ctx tool.Context, input CalculateEstimateInput) (CalculateEstimateOutput, error) {
		_ = ctx
		materialSubtotal := 0.0
		for _, item := range input.MaterialItems {
			if item.UnitPrice <= 0 || item.Quantity <= 0 {
				continue
			}
			materialSubtotal += item.UnitPrice * item.Quantity
		}

		laborLow := clampNonNegative(input.LaborHoursLow) * clampNonNegative(input.HourlyRateLow)
		laborHigh := clampNonNegative(input.LaborHoursHigh) * clampNonNegative(input.HourlyRateHigh)
		if laborHigh < laborLow {
			laborLow, laborHigh = laborHigh, laborLow
		}

		extra := clampNonNegative(input.ExtraCosts)

		return CalculateEstimateOutput{
			MaterialSubtotal:  round2(materialSubtotal),
			LaborSubtotalLow:  round2(laborLow),
			LaborSubtotalHigh: round2(laborHigh),
			TotalLow:          round2(materialSubtotal + laborLow + extra),
			TotalHigh:         round2(materialSubtotal + laborHigh + extra),
			AppliedExtraCosts: round2(extra),
		}, nil
	})
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func clampNonNegative(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}

// defaultSearchScoreThreshold is the minimum cosine similarity score for
// BGE-M3 embeddings. It controls recall (what enters candidate set).
const defaultSearchScoreThreshold = 0.35
const maxCatalogRewordRetries = 2

// noMatchMessage builds the "no relevant products" message for a query.
func noMatchMessage(query string) string {
	return fmt.Sprintf("No relevant products found for query '%s'. Try different search terms (synonyms, broader/narrower terms, Dutch and English). If no match exists, you may add an ad-hoc item.", query)
}

// resolveSearchParams extracts and normalises the search parameters from input.
func resolveSearchParams(input SearchProductMaterialsInput) (query string, limit int, useCatalog bool, scoreThreshold float64, err error) {
	query = strings.TrimSpace(input.Query)
	if query == "" {
		return "", 0, false, 0, fmt.Errorf("empty query")
	}
	limit = normalizeLimit(input.Limit, 5, 20)
	useCatalog = true
	if input.UseCatalog != nil {
		useCatalog = *input.UseCatalog
	}
	scoreThreshold = defaultSearchScoreThreshold
	if input.MinScore != nil && *input.MinScore > 0 && *input.MinScore < 1 {
		scoreThreshold = *input.MinScore
	}
	return query, limit, useCatalog, scoreThreshold, nil
}

// searchCatalogCollection searches the catalog Qdrant collection and hydrates results.
// Returns nil products (not an error) when nothing relevant is found or the search fails.
func searchCatalogCollection(ctx tool.Context, deps *ToolDependencies, vector []float32, limit int, scoreThreshold float64, query string) []ProductResult {
	results, err := deps.CatalogQdrantClient.SearchWithThreshold(ctx, vector, limit, scoreThreshold)
	if err != nil {
		log.Printf("SearchProductMaterials: catalog search failed: %v", err)
		return nil
	}
	products := convertSearchResults(results)
	if len(products) == 0 {
		log.Printf("SearchProductMaterials: catalog query=%q found 0 products above threshold %.2f, falling back", query, scoreThreshold)
		return nil
	}
	products = hydrateProductResults(ctx, deps, products)
	products = rerankCatalogProducts(query, products)
	markHighConfidence(products)
	logCatalogSelectionAudit(query, products)
	log.Printf("SearchProductMaterials: catalog query=%q found %d products (threshold=%.2f, scores: %s)",
		query, len(products), scoreThreshold, formatScores(products))
	return products
}

func handleSearchProductMaterials(ctx tool.Context, deps *ToolDependencies, input SearchProductMaterialsInput) (SearchProductMaterialsOutput, error) {
	if !deps.IsProductSearchEnabled() {
		return SearchProductMaterialsOutput{Products: nil, Message: "Product search is not configured"}, nil
	}

	query, limit, useCatalog, scoreThreshold, err := resolveSearchParams(input)
	if err != nil {
		return SearchProductMaterialsOutput{Products: nil, Message: "Query cannot be empty"}, err
	}

	vector, err := deps.EmbeddingClient.Embed(ctx, query)
	if err != nil {
		log.Printf("SearchProductMaterials: embedding failed: %v", err)
		return SearchProductMaterialsOutput{Products: nil, Message: "Failed to generate embedding for query"}, err
	}

	// Try catalog collection first.
	if useCatalog && deps.CatalogQdrantClient != nil {
		if products := searchCatalogCollection(ctx, deps, vector, limit, scoreThreshold, query); len(products) > 0 {
			if hasHighConfidenceMatch(products) {
				return SearchProductMaterialsOutput{
					Products: products,
					Message:  fmt.Sprintf("Found %d high-confidence matching products from catalog (min relevance %.0f%%)", len(products), scoreThreshold*100),
				}, nil
			}

			bestProducts := products
			rewordedQueries := buildCatalogRewordedQueries(query)
			if len(rewordedQueries) > maxCatalogRewordRetries {
				rewordedQueries = rewordedQueries[:maxCatalogRewordRetries]
			}

			for _, retryQuery := range rewordedQueries {
				retryVector, retryErr := deps.EmbeddingClient.Embed(ctx, retryQuery)
				if retryErr != nil {
					log.Printf("SearchProductMaterials: catalog retry embedding failed query=%q: %v", retryQuery, retryErr)
					continue
				}

				retryProducts := searchCatalogCollection(ctx, deps, retryVector, limit, scoreThreshold, retryQuery)
				if len(retryProducts) == 0 {
					continue
				}

				if shouldPreferCandidateSet(retryProducts, bestProducts) {
					bestProducts = retryProducts
				}

				if hasHighConfidenceMatch(retryProducts) {
					log.Printf("SearchProductMaterials: catalog retry improved confidence query=%q -> retry_query=%q", query, retryQuery)
					return SearchProductMaterialsOutput{
						Products: retryProducts,
						Message:  fmt.Sprintf("Found %d high-confidence matching products from catalog after query rewording (min relevance %.0f%%)", len(retryProducts), scoreThreshold*100),
					}, nil
				}
			}

			return SearchProductMaterialsOutput{
				Products: bestProducts,
				Message:  fmt.Sprintf("Found %d matching products from catalog (lower confidence; verify variant/unit, min relevance %.0f%%)", len(bestProducts), scoreThreshold*100),
			}, nil
		}
	}

	// If primary catalog search had no candidates, try catalog rewording retries before falling back.
	if useCatalog && deps.CatalogQdrantClient != nil {
		rewordedQueries := buildCatalogRewordedQueries(query)
		if len(rewordedQueries) > maxCatalogRewordRetries {
			rewordedQueries = rewordedQueries[:maxCatalogRewordRetries]
		}

		var bestRetryProducts []ProductResult
		for _, retryQuery := range rewordedQueries {
			retryVector, retryErr := deps.EmbeddingClient.Embed(ctx, retryQuery)
			if retryErr != nil {
				log.Printf("SearchProductMaterials: catalog retry embedding failed query=%q: %v", retryQuery, retryErr)
				continue
			}
			retryProducts := searchCatalogCollection(ctx, deps, retryVector, limit, scoreThreshold, retryQuery)
			if len(retryProducts) == 0 {
				continue
			}
			if shouldPreferCandidateSet(retryProducts, bestRetryProducts) {
				bestRetryProducts = retryProducts
			}
			if hasHighConfidenceMatch(retryProducts) {
				log.Printf("SearchProductMaterials: catalog retry recovered results query=%q -> retry_query=%q", query, retryQuery)
				return SearchProductMaterialsOutput{
					Products: retryProducts,
					Message:  fmt.Sprintf("Found %d high-confidence matching products from catalog after query rewording (min relevance %.0f%%)", len(retryProducts), scoreThreshold*100),
				}, nil
			}
		}

		if len(bestRetryProducts) > 0 {
			return SearchProductMaterialsOutput{
				Products: bestRetryProducts,
				Message:  fmt.Sprintf("Found %d matching products from catalog after query rewording (lower confidence; verify variant/unit, min relevance %.0f%%)", len(bestRetryProducts), scoreThreshold*100),
			}, nil
		}
	}

	// Fallback to general collection.
	if deps.QdrantClient == nil {
		return SearchProductMaterialsOutput{Products: nil, Message: noMatchMessage(query)}, nil
	}

	results, err := deps.QdrantClient.SearchWithThreshold(ctx, vector, limit, scoreThreshold)
	if err != nil {
		log.Printf("SearchProductMaterials: fallback search failed: %v", err)
		return SearchProductMaterialsOutput{Products: nil, Message: "Failed to search product catalog"}, err
	}

	products := convertSearchResults(results)
	markHighConfidence(products)
	if len(products) == 0 {
		log.Printf("SearchProductMaterials: fallback query=%q found 0 products above threshold %.2f", query, scoreThreshold)
		return SearchProductMaterialsOutput{Products: nil, Message: noMatchMessage(query)}, nil
	}

	// Fallback results are scraped reference data — strip IDs so the AI
	// treats them as ad-hoc line items (no catalogProductId, no auto-attachments).
	stripProductIDs(products)

	log.Printf("SearchProductMaterials: fallback query=%q found %d reference products (threshold=%.2f, scores: %s)",
		query, len(products), scoreThreshold, formatScores(products))

	return SearchProductMaterialsOutput{
		Products: products,
		Message:  fmt.Sprintf("Found %d reference products (not from your catalog — use as ad-hoc line items without catalogProductId, min relevance %.0f%%)", len(products), scoreThreshold*100),
	}, nil
}

func hasHighConfidenceMatch(products []ProductResult) bool {
	for _, product := range products {
		if product.HighConfidence {
			return true
		}
	}
	return false
}

func shouldPreferCandidateSet(candidate []ProductResult, current []ProductResult) bool {
	if len(candidate) == 0 {
		return false
	}
	if len(current) == 0 {
		return true
	}
	candidateHigh := hasHighConfidenceMatch(candidate)
	currentHigh := hasHighConfidenceMatch(current)
	if candidateHigh != currentHigh {
		return candidateHigh
	}
	return candidate[0].Score > current[0].Score
}

func buildCatalogRewordedQueries(query string) []string {
	base := strings.TrimSpace(strings.ToLower(query))
	if base == "" {
		return nil
	}

	queries := make([]string, 0, 4)
	appendUniqueQuery := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range queries {
			if existing == value {
				return
			}
		}
		queries = append(queries, value)
	}

	synonymExpansions := map[string]string{
		"kantstuk":      "dagkantafwerking deurlijst chambranle aftimmerlat afdeklat kozijnplint sponninglat",
		"kantstukken":   "dagkantafwerking deurlijst chambranle aftimmerlat afdeklat kozijnplint sponninglat",
		"zweeds rabat":  "potdekselplank gevelbekleding rabatdeel",
		"grondverf":     "primer hout grondlaag",
		"randsealer":    "kanten sealer randafdichting",
		"paal":          "staander tuinpaal",
		"angelim":       "hardhout paal tropisch",
		"geimpregneerd": "druk geimpregneerd buitenhout",
	}

	for key, expansion := range synonymExpansions {
		if strings.Contains(base, key) {
			appendUniqueQuery(base + " " + expansion)
		}
	}

	appendUniqueQuery(strings.ReplaceAll(base, " inclusief ", " "))
	appendUniqueQuery(base + " catalog product variant maat unit")

	return queries
}

// stripProductIDs clears the ID field on all products so the AI treats
// them as ad-hoc items (no catalogProductId on the draft quote).
// Also sets a default VAT rate of 21% for fallback products that lack one.
func stripProductIDs(products []ProductResult) {
	for i := range products {
		products[i].ID = ""
		if products[i].VatRateBps == 0 {
			products[i].VatRateBps = 2100 // 21% BTW default
		}
	}
}

// formatScores returns a compact summary of product scores for logging.
func formatScores(products []ProductResult) string {
	if len(products) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(products))
	for _, p := range products {
		parts = append(parts, fmt.Sprintf("%.3f", p.Score))
	}
	return strings.Join(parts, ", ")
}

func normalizeLimit(limit, defaultVal, maxVal int) int {
	if limit <= 0 {
		return defaultVal
	}
	if limit > maxVal {
		return maxVal
	}
	return limit
}

func markHighConfidence(products []ProductResult) {
	for i := range products {
		products[i].HighConfidence = products[i].Score >= highConfidenceScoreThreshold
	}
}

func truncateRunes(value string, max int) string {
	if max <= 0 || value == "" {
		return ""
	}
	if len(value) <= max {
		return value
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func convertSearchResults(results []qdrant.SearchResult) []ProductResult {
	products := make([]ProductResult, 0, len(results))
	for _, r := range results {
		product := extractProductFromPayload(r.Payload, r.Score)
		if product.Name != "" {
			products = append(products, product)
		}
	}

	// Default ordering: strongest semantic matches first.
	sort.SliceStable(products, func(i, j int) bool {
		if products[i].Score == products[j].Score {
			return products[i].PriceEuros < products[j].PriceEuros
		}
		return products[i].Score > products[j].Score
	})
	return products
}

func rerankCatalogProducts(query string, products []ProductResult) []ProductResult {
	if len(products) <= 1 {
		return products
	}

	queryTokens := tokenizeForMatch(query)
	queryDims := extractDimensionTokens(query)
	queryUnits := extractUnitTokens(query)

	type rankedProduct struct {
		product    ProductResult
		rankScore  float64
		overlap    float64
		dimMatches int
		unitMatch  bool
	}

	ranked := make([]rankedProduct, 0, len(products))
	for _, product := range products {
		text := strings.ToLower(strings.Join([]string{product.Name, product.Description, product.Unit, product.Category}, " "))
		textTokens := tokenizeForMatch(text)
		overlap := tokenOverlapRatio(queryTokens, textTokens)
		dimMatches := countSetIntersection(queryDims, extractDimensionTokens(text))
		unitMatch := hasAnyUnitToken(text, queryUnits)

		rank := product.Score*1000 + overlap*120 + float64(dimMatches)*30
		if unitMatch {
			rank += 20
		}

		ranked = append(ranked, rankedProduct{
			product:    product,
			rankScore:  rank,
			overlap:    overlap,
			dimMatches: dimMatches,
			unitMatch:  unitMatch,
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].rankScore == ranked[j].rankScore {
			return ranked[i].product.Score > ranked[j].product.Score
		}
		return ranked[i].rankScore > ranked[j].rankScore
	})

	for i := range products {
		products[i] = ranked[i].product
	}

	return products
}

func logCatalogSelectionAudit(query string, products []ProductResult) {
	if len(products) == 0 {
		return
	}

	highConfidenceCount := 0
	for _, product := range products {
		if product.HighConfidence {
			highConfidenceCount++
		}
	}

	top := products[0]
	log.Printf(
		"SearchProductMaterials: catalog selection audit query=%q top_id=%q top_name=%q top_score=%.3f top_price_cents=%d top_unit=%q high_confidence_count=%d total_candidates=%d",
		query,
		top.ID,
		top.Name,
		top.Score,
		top.PriceCents,
		top.Unit,
		highConfidenceCount,
		len(products),
	)

	if highConfidenceCount == 0 {
		log.Printf("SearchProductMaterials: catalog query=%q has no high-confidence candidates; verify selected variants before drafting", query)
	}
}

func tokenizeForMatch(value string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !(r == '-' || r == '+' || r == '.' || r == '/' || r == 'x' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z'))
	}) {
		token = strings.TrimSpace(token)
		if len(token) < 2 {
			continue
		}
		set[token] = struct{}{}
	}
	return set
}

func extractDimensionTokens(value string) map[string]struct{} {
	tokens := make(map[string]struct{})
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !(r == '-' || r == 'x' || r == '/' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z'))
	}) {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		hasDigit := false
		hasSeparator := false
		for _, r := range token {
			if r >= '0' && r <= '9' {
				hasDigit = true
			}
			if r == 'x' || r == '-' {
				hasSeparator = true
			}
		}
		if hasDigit && hasSeparator {
			tokens[token] = struct{}{}
		}
	}
	return tokens
}

func extractUnitTokens(value string) map[string]struct{} {
	units := map[string]struct{}{}
	lookup := []string{"m1", "m2", "m3", "stuk", "stuks", "liter", "l", "cm", "mm", "meter", "per"}
	lower := strings.ToLower(value)
	for _, unit := range lookup {
		if strings.Contains(lower, unit) {
			units[unit] = struct{}{}
		}
	}
	return units
}

func tokenOverlapRatio(queryTokens map[string]struct{}, textTokens map[string]struct{}) float64 {
	if len(queryTokens) == 0 {
		return 0
	}
	intersection := 0
	for token := range queryTokens {
		if _, ok := textTokens[token]; ok {
			intersection++
		}
	}
	return float64(intersection) / float64(len(queryTokens))
}

func countSetIntersection(a map[string]struct{}, b map[string]struct{}) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	count := 0
	for key := range a {
		if _, ok := b[key]; ok {
			count++
		}
	}
	return count
}

func hasAnyUnitToken(text string, units map[string]struct{}) bool {
	if len(units) == 0 {
		return false
	}
	for unit := range units {
		if strings.Contains(text, unit) {
			return true
		}
	}
	return false
}

func extractProductFromPayload(payload map[string]any, score float64) ProductResult {
	product := ProductResult{Score: score}
	product.ID = payloadStr(payload, "id")
	product.Name = payloadStr(payload, "name")
	product.Description = payloadStr(payload, "description")
	product.Type = resolveProductType(payload)
	product.PriceEuros = payloadFloat(payload, "price")
	product.Unit = resolveUnit(payload)
	product.LaborTime = strings.TrimSpace(payloadStr(payload, "labor_time_text"))
	product.Category = payloadStr(payload, "category")
	product.SourceURL = payloadStr(payload, "source_url")

	if product.PriceEuros <= 0 {
		product.PriceEuros = payloadFloat(payload, "unit_price")
	}

	applyBrandPrefix(&product, payloadStr(payload, "brand"))
	extractSpecsMaterial(&product, payload)

	product.PriceCents = eurosToCents(product.PriceEuros)
	return product
}

// payloadStr safely extracts a string value from the payload map.
func payloadStr(payload map[string]any, key string) string {
	v, _ := payload[key].(string)
	return v
}

// payloadFloat safely extracts a float64 value from the payload map.
func payloadFloat(payload map[string]any, key string) float64 {
	v, _ := payload[key].(float64)
	return v
}

// resolveUnit determines the unit label from the payload, preferring
// unit_label > unit > parsed from price_raw.
func resolveUnit(payload map[string]any) string {
	if u := payloadStr(payload, "unit_label"); u != "" {
		return u
	}
	if u := payloadStr(payload, "unit"); u != "" {
		return u
	}
	return parseUnitFromPriceRaw(payload)
}

// resolveProductType returns the product type from the payload.
// Catalog products have a "type" field (service, digital_service, product, material).
// Fallback/scraped products default to "material".
func resolveProductType(payload map[string]any) string {
	if t := payloadStr(payload, "type"); t != "" {
		return t
	}
	return "material"
}

// applyBrandPrefix prepends the brand to the product description if present.
func applyBrandPrefix(product *ProductResult, brand string) {
	if brand == "" {
		return
	}
	if product.Description != "" {
		product.Description = brand + " — " + product.Description
	} else {
		product.Description = brand
	}
}

// parseUnitFromPriceRaw extracts a unit string from the scraped price_raw field.
// e.g. "€1,21/m1" → "per m1", "€3,50/stuk" → "per stuk".
func parseUnitFromPriceRaw(payload map[string]any) string {
	raw, ok := payload["price_raw"].(string)
	if !ok || raw == "" {
		return ""
	}
	idx := strings.LastIndex(raw, "/")
	if idx < 0 || idx >= len(raw)-1 {
		return ""
	}
	unit := strings.TrimSpace(raw[idx+1:])
	if unit == "" {
		return ""
	}
	return "per " + unit
}

// extractSpecsMaterial reads specs.raw.Materiaal from the payload and populates
// the product's Materials slice if it's empty.
func extractSpecsMaterial(product *ProductResult, payload map[string]any) {
	if len(product.Materials) > 0 {
		return
	}
	specs, ok := payload["specs"].(map[string]any)
	if !ok {
		return
	}
	raw, ok := specs["raw"].(map[string]any)
	if !ok {
		return
	}
	if mat, ok := raw["Materiaal"].(string); ok && mat != "" {
		product.Materials = []string{mat}
	}
}

// eurosToCents converts a euro amount to integer cents, rounding to nearest.
func eurosToCents(euros float64) int64 {
	return int64(math.Round(euros * 100))
}

// hydrateProductResults enriches vector-search ProductResults with DB-accurate
// pricing, VAT rates, and materials via the CatalogReader port. Products whose
// IDs cannot be resolved are returned unchanged.
func hydrateProductResults(ctx context.Context, deps *ToolDependencies, products []ProductResult) []ProductResult {
	if deps.CatalogReader == nil {
		return products
	}
	tenantID, ok := deps.GetTenantID()
	if !ok || tenantID == nil {
		return products
	}

	ids := collectProductUUIDs(products)
	if len(ids) == 0 {
		return products
	}

	details, err := deps.CatalogReader.GetProductDetails(ctx, *tenantID, ids)
	if err != nil {
		log.Printf("hydrateProductResults: catalog reader failed: %v", err)
		return products
	}

	return applyProductDetails(products, details)
}

// collectProductUUIDs extracts unique, parseable UUIDs from product results.
func collectProductUUIDs(products []ProductResult) []uuid.UUID {
	seen := make(map[string]struct{}, len(products))
	ids := make([]uuid.UUID, 0, len(products))
	for _, p := range products {
		if p.ID == "" {
			continue
		}
		if _, dup := seen[p.ID]; dup {
			continue
		}
		uid, err := uuid.Parse(p.ID)
		if err != nil {
			continue
		}
		seen[p.ID] = struct{}{}
		ids = append(ids, uid)
	}
	return ids
}

// applyProductDetails merges DB-accurate catalog details back into product results.
func applyProductDetails(products []ProductResult, details []ports.CatalogProductDetails) []ProductResult {
	detailMap := make(map[string]ports.CatalogProductDetails, len(details))
	for _, d := range details {
		detailMap[d.ID.String()] = d
	}

	for i, p := range products {
		d, ok := detailMap[p.ID]
		if !ok {
			continue
		}
		products[i].PriceEuros = float64(d.UnitPriceCents) / 100
		products[i].PriceCents = d.UnitPriceCents
		products[i].VatRateBps = d.VatRateBps
		products[i].Materials = d.Materials
		mergeOptionalString(&products[i].Unit, d.UnitLabel)
		mergeOptionalString(&products[i].LaborTime, d.LaborTimeText)
		mergeOptionalString(&products[i].Description, d.Description)
	}
	return products
}

// mergeOptionalString overwrites dst when src is non-empty.
func mergeOptionalString(dst *string, src string) {
	if src != "" {
		*dst = src
	}
}

func createSearchProductMaterialsTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "SearchProductMaterials",
		Description: `Searches the product catalog for materials and their prices via semantic (vector) search.
The query is embedded into a vector, so use descriptive, varied language for best recall.
Only products with a relevance score >= 35% are returned. If no results are returned, it means the
catalog does not contain a matching product — try a different query or add an ad-hoc item.

Tips for effective queries:
- Use generic product category names (e.g. "scharnier deur" instead of just "RVS scharnieren").
- Include synonyms and alternative terms (e.g. "deurhanger deurbeslag scharnier").
- Mix Dutch and English terms if the catalog may contain either.
- Translate consumer wording into trade and DIY/store terms.
- Example query expansion for "kantstukken": "dagkantafwerking", "deurlijst/chambranle", "aftimmerlat/afdeklat", "kozijnplint", "sponninglat".
- Search for broader categories first, then refine with specific queries.
- Call this tool multiple times with different queries to cover all needed materials.

Each result includes a "score" field (0-1) indicating match quality.
Products with score >= 0.45 are high-confidence matches and include highConfidence=true.
For high-confidence matches, use the product price directly (no markup).
Products with score 0.35-0.45 are lower-confidence candidates — verify variant/unit before using.

Result fields:
- name: product name
- description: product description (may include brand)
- type: product type — "service" or "digital_service" means price INCLUDES labor; "product" or "material" means price is material only.
- priceEuros: price in euros (e.g. 7.93 = EUR 7.93). Use for CalculateEstimate unitPrice.
- priceCents: price in euro-cents (e.g. 793). Use this directly as unitPriceCents in DraftQuote.
- unit: how the product is sold (e.g. "per m1", "per stuk", "per m2"). Use to compute correct quantities.
- vatRateBps: VAT rate in basis points (2100 = 21%). Defaults to 2100 for reference products.
- materials: included materials (e.g. ["Verzinkt staal"])
- category: product category path (reference products only)
- sourceUrl: reference URL (reference products only)
- laborTime: estimated labor time text (if available)
- score: similarity score (0-1)
- highConfidence: true when score >= 0.45 (use found price without markup)
- id: catalog product UUID (only for catalog items — use as catalogProductId in DraftQuote)`,
	}, func(ctx tool.Context, input SearchProductMaterialsInput) (SearchProductMaterialsOutput, error) {
		return handleSearchProductMaterials(ctx, deps, input)
	})
}

// handleDraftQuote creates a draft quote via the QuoteDrafter port.
func handleDraftQuote(ctx tool.Context, deps *ToolDependencies, input DraftQuoteInput) (DraftQuoteOutput, error) {
	if deps.QuoteDrafter == nil {
		return DraftQuoteOutput{Success: false, Message: "Quote drafting is not configured"}, nil
	}

	tenantID, ok := deps.GetTenantID()
	if !ok || tenantID == nil {
		return DraftQuoteOutput{Success: false, Message: "Organization context not available"}, fmt.Errorf("missing tenant context")
	}

	leadID, serviceID, ok := deps.GetLeadContext()
	if !ok {
		return DraftQuoteOutput{Success: false, Message: "Lead context not available"}, errors.New(missingLeadContextError)
	}

	if len(input.Items) == 0 {
		return DraftQuoteOutput{Success: false, Message: "At least one item is required"}, fmt.Errorf("empty items")
	}

	portItems := convertDraftItems(input.Items)
	portItems = enforceCatalogUnitPrices(ctx, deps, *tenantID, portItems)
	portAttachments, portURLs := collectCatalogAssetsForDraft(ctx, deps, tenantID, portItems)

	result, err := deps.QuoteDrafter.DraftQuote(ctx, ports.DraftQuoteParams{
		QuoteID:        deps.GetExistingQuoteID(),
		LeadID:         leadID,
		LeadServiceID:  serviceID,
		OrganizationID: *tenantID,
		CreatedByID:    uuid.Nil,
		Notes:          input.Notes,
		Items:          portItems,
		Attachments:    portAttachments,
		URLs:           portURLs,
	})
	if err != nil {
		log.Printf("DraftQuote: failed: %v", err)
		return DraftQuoteOutput{Success: false, Message: fmt.Sprintf("Failed to draft quote: %v", err)}, err
	}

	log.Printf("DraftQuote: created %s with %d items for lead %s", result.QuoteNumber, result.ItemCount, leadID)
	deps.SetLastDraftResult(result)
	deps.MarkDraftQuoteCalled()

	return DraftQuoteOutput{
		Success:     true,
		Message:     fmt.Sprintf("Draft quote %s created with %d items", result.QuoteNumber, result.ItemCount),
		QuoteID:     result.QuoteID.String(),
		QuoteNumber: result.QuoteNumber,
		ItemCount:   result.ItemCount,
	}, nil
}

// enforceCatalogUnitPrices ensures catalog-linked quote items use authoritative
// catalog pricing metadata (unit price + VAT). Ad-hoc items (without
// catalogProductId) are left unchanged so they can be estimated.
func enforceCatalogUnitPrices(ctx context.Context, deps *ToolDependencies, tenantID uuid.UUID, items []ports.DraftQuoteItem) []ports.DraftQuoteItem {
	if deps.CatalogReader == nil || len(items) == 0 {
		return items
	}

	catalogIDs := collectCatalogProductIDs(items)
	if len(catalogIDs) == 0 {
		return items
	}

	details, err := deps.CatalogReader.GetProductDetails(ctx, tenantID, catalogIDs)
	if err != nil {
		log.Printf("DraftQuote: catalog price normalization skipped, details fetch failed: %v", err)
		return items
	}

	detailByID := make(map[uuid.UUID]ports.CatalogProductDetails, len(details))
	for _, d := range details {
		detailByID[d.ID] = d
	}

	priceAdjusted := 0
	vatAdjusted := 0
	unresolvedCatalogIDs := 0
	for i := range items {
		if items[i].CatalogProductID == nil {
			continue
		}
		d, ok := detailByID[*items[i].CatalogProductID]
		if !ok {
			unresolvedCatalogIDs++
			continue
		}
		if items[i].UnitPriceCents != d.UnitPriceCents {
			items[i].UnitPriceCents = d.UnitPriceCents
			priceAdjusted++
		}
		if d.VatRateBps > 0 && items[i].TaxRateBps != d.VatRateBps {
			items[i].TaxRateBps = d.VatRateBps
			vatAdjusted++
		}
	}

	if priceAdjusted > 0 || vatAdjusted > 0 {
		log.Printf("DraftQuote: normalized catalog-linked metadata (prices=%d vat=%d)", priceAdjusted, vatAdjusted)
	}
	if unresolvedCatalogIDs > 0 {
		log.Printf("DraftQuote: %d catalog-linked item(s) could not be resolved; kept input values to avoid breaking flow", unresolvedCatalogIDs)
	}

	return items
}

// convertDraftItems converts tool-level DraftQuoteItems to port-level items.
func convertDraftItems(items []DraftQuoteItem) []ports.DraftQuoteItem {
	portItems := make([]ports.DraftQuoteItem, len(items))
	for i, it := range items {
		portItems[i] = ports.DraftQuoteItem{
			Description:    it.Description,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
			TaxRateBps:     it.TaxRateBps,
			IsOptional:     it.IsOptional,
		}
		if it.CatalogProductID != nil && *it.CatalogProductID != "" {
			uid, err := uuid.Parse(*it.CatalogProductID)
			if err == nil {
				portItems[i].CatalogProductID = &uid
			}
		}
	}
	return portItems
}

// collectCatalogAssetsForDraft auto-collects catalog product attachments and URLs.
func collectCatalogAssetsForDraft(ctx context.Context, deps *ToolDependencies, tenantID *uuid.UUID, items []ports.DraftQuoteItem) ([]ports.DraftQuoteAttachment, []ports.DraftQuoteURL) {
	if deps.CatalogReader == nil {
		return nil, nil
	}
	catalogIDs := collectCatalogProductIDs(items)
	if len(catalogIDs) == 0 {
		return nil, nil
	}
	details, err := deps.CatalogReader.GetProductDetails(ctx, *tenantID, catalogIDs)
	if err != nil {
		log.Printf("DraftQuote: catalog details fetch failed (non-fatal): %v", err)
		return nil, nil
	}
	return collectCatalogAssets(details)
}

func createDraftQuoteTool(deps *ToolDependencies) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "DraftQuote",
		Description: `Creates a draft quote for the current lead based on estimation results.
Use this AFTER searching the catalog and calculating estimates.
For each item, provide description, quantity, unitPriceCents (in euro-cents), taxRateBps.
IMPORTANT: If a high-confidence product is found, set unitPriceCents exactly to the product's "priceCents" value from SearchProductMaterials (already in cents), without markup.
Only estimate unitPriceCents when no suitable high-confidence product was found.
If the item came from SearchProductMaterials, include its catalogProductId.
When catalogProductId is present, backend catalog metadata is authoritative: unitPriceCents and taxRateBps are normalized to catalog values.
Ad-hoc items (not found in catalog) should omit catalogProductId.`,
	}, func(ctx tool.Context, input DraftQuoteInput) (DraftQuoteOutput, error) {
		return handleDraftQuote(ctx, deps, input)
	})
}

// collectCatalogProductIDs extracts unique, non-nil catalog product UUIDs from draft items.
func collectCatalogProductIDs(items []ports.DraftQuoteItem) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(items))
	ids := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		if it.CatalogProductID == nil {
			continue
		}
		if _, dup := seen[*it.CatalogProductID]; dup {
			continue
		}
		seen[*it.CatalogProductID] = struct{}{}
		ids = append(ids, *it.CatalogProductID)
	}
	return ids
}

// collectCatalogAssets de-duplicates document attachments and URLs across all
// catalog product details and returns them as port-level types.
func collectCatalogAssets(details []ports.CatalogProductDetails) ([]ports.DraftQuoteAttachment, []ports.DraftQuoteURL) {
	seenFileKeys := make(map[string]struct{})
	seenHrefs := make(map[string]struct{})

	var attachments []ports.DraftQuoteAttachment
	var urls []ports.DraftQuoteURL

	for _, d := range details {
		pid := d.ID
		for _, doc := range d.Documents {
			if _, dup := seenFileKeys[doc.FileKey]; dup {
				continue
			}
			seenFileKeys[doc.FileKey] = struct{}{}
			attachments = append(attachments, ports.DraftQuoteAttachment{
				Filename:         doc.Filename,
				FileKey:          doc.FileKey,
				Source:           "catalog",
				CatalogProductID: &pid,
			})
		}
		for _, u := range d.URLs {
			if _, dup := seenHrefs[u.Href]; dup {
				continue
			}
			seenHrefs[u.Href] = struct{}{}
			urls = append(urls, ports.DraftQuoteURL{
				Label:            u.Label,
				Href:             u.Href,
				CatalogProductID: &pid,
			})
		}
	}

	return attachments, urls
}
