package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/scoring"
	apptools "portal_final_backend/internal/tools"
	"portal_final_backend/platform/ai/embeddings"
	"portal_final_backend/platform/phone"
	"portal_final_backend/platform/qdrant"
)

const (
	invalidLeadIDMessage             = "Invalid lead ID"
	invalidLeadServiceIDMessage      = "Invalid lead service ID"
	missingTenantContextMessage      = "Missing tenant context"
	missingTenantContextError        = "missing tenant context"
	missingLeadContextMessage        = "Missing lead context"
	missingLeadContextError          = "missing lead context"
	leadNotFoundMessage              = "Lead not found"
	leadServiceNotFoundMessage       = "Lead service not found"
	invalidFieldFormat               = "invalid %s"
	recentEquivalentAnalysisTTL      = 2 * time.Minute
	divisionByZeroMessage            = "division by zero"
	gatekeeperNurturingLoopThreshold = 3
)

const highConfidenceScoreThreshold = 0.45
const catalogEarlyReturnScoreThreshold = 0.55
const (
	defaultHouthandelCollection   = "houthandel_products"
	defaultBouwmaatCollection     = "bouwmaat_products"
	gatekeeperLoopDetectedTrigger = "ai_loop_detected"
	gatekeeperLoopReasonCode      = "nurturing_loop_threshold"
	gatekeeperLoopDetectedSummary = "Systeem: AI zat in een lus. Menselijke controle vereist."
)

// normalizeUrgencyLevel converts various urgency level formats to the required values: High, Medium, Low
func normalizeUrgencyLevel(level string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(level))

	switch normalized {
	case "high", "hoog", "urgent", "spoed", "spoedeisend", "critical":
		return "High", nil
	case "medium", "mid", "moderate", "matig", "gemiddeld", "normal", "standard", "standaard":
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
	case "high", "hoog", "good", "goed", "qualified", "gekwalificeerd":
		return "High"
	case "urgent", "spoed", "critical", "kritiek":
		return "Urgent"
	default:
		log.Printf("Unrecognized lead quality '%s', defaulting to Potential", quality)
		return "Potential"
	}
}

// normalizeRecommendedAction converts various action formats to valid values: Reject, RequestInfo, ScheduleSurvey, CallImmediately.
// Estimation-ready aliases map to ScheduleSurvey because that is the existing
// canonical non-RequestInfo action stored in the analysis table.
func normalizeRecommendedAction(action string) string {
	normalized := strings.ToLower(strings.TrimSpace(action))

	// Check for exact matches first
	switch normalized {
	case "reject", "afwijzen", "weigeren":
		return "Reject"
	case "requestinfo", "request_info", "request info":
		return "RequestInfo"
	case "movetoestimation", "move_to_estimation", "move to estimation",
		"proceedtoestimation", "proceed_to_estimation", "proceed to estimation",
		"estimate", "estimateready", "estimate_ready", "estimate ready":
		return "ScheduleSurvey"
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
	if strings.Contains(normalized, "estimat") || strings.Contains(normalized, "proceed") || strings.Contains(normalized, "move to estimation") {
		return "ScheduleSurvey"
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
	Repo                   repository.LeadsRepository
	Scorer                 *scoring.Service
	EventBus               events.Bus
	EmbeddingClient        *embeddings.Client
	QdrantClient           *qdrant.Client
	BouwmaatQdrantClient   *qdrant.Client
	CatalogQdrantClient    *qdrant.Client
	CatalogReader          ports.CatalogReader // optional: hydrate search results from DB
	QuoteDrafter           ports.QuoteDrafter  // optional: draft quotes from agent
	PricingIntelligence    ports.PricingIntelligenceReader
	OfferCreator           ports.PartnerOfferCreator
	CouncilService         MultiAgentCouncil
	OrgSettingsReader      ports.OrganizationAISettingsReader
	mu                     sync.RWMutex
	tenantID               *uuid.UUID
	leadID                 *uuid.UUID
	serviceID              *uuid.UUID
	actorType              string
	actorName              string
	orgSettings            *ports.OrganizationAISettings
	existingQuoteID        *uuid.UUID     // If set, DraftQuote updates this quote instead of creating new
	lastAnalysisMetadata   map[string]any // Populated by SaveAnalysis for use in stage_change events
	lastEstimationMetadata map[string]any // Populated by SaveEstimation for use in stage_change events
	lastEstimateSnapshot   *EstimateComputationSnapshot
	lastCouncilMetadata    map[string]any          // Populated by council evaluation for stage/quote governance events
	saveAnalysisCalled     bool                    // Track if SaveAnalysis was called
	saveEstimationCalled   bool                    // Track if SaveEstimation was called
	stageUpdateCalled      bool                    // Track if UpdatePipelineStage was called
	lastStageUpdated       string                  // Track last pipeline stage written
	draftQuoteCalled       bool                    // Track if DraftQuote was called
	offerCreated           bool                    // Track if CreatePartnerOffer was called
	lastDraftResult        *ports.DraftQuoteResult // Captured by handleDraftQuote for generate endpoint
	lastDraftInput         *DraftQuoteInput        // Snapshot of the latest drafted line items for downstream review
	lastQuoteReviewResult  *ports.QuoteAIReviewResult
	lastQuoteCritiqueInput *SubmitQuoteCritiqueInput
	quoteCriticAttempt     int
	scopeArtifact          *ScopeArtifact // Produced by Scope Analyzer and consumed by Quote Builder
	clarificationAsked     bool           // Track if AskCustomerClarification was called in investigative mode
	runID                  string         // Correlates all tool calls within one agent run
	forceDraftQuote        bool           // Allows manual runs to bypass draft governance (intake + council)
	searchCache            map[string]SearchProductMaterialsOutput
	emittedAlertKeys       map[string]struct{} // Dedupe identical alerts within a single agent run
}

// NewRequestDeps creates a request-scoped ToolDependencies that shares the
// immutable service references from the receiver but has its own mutable state.
// This enables concurrent agent runs without a global mutex.
func (d *ToolDependencies) NewRequestDeps() *ToolDependencies {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return &ToolDependencies{
		Repo:                 d.Repo,
		Scorer:               d.Scorer,
		EventBus:             d.EventBus,
		EmbeddingClient:      d.EmbeddingClient,
		QdrantClient:         d.QdrantClient,
		BouwmaatQdrantClient: d.BouwmaatQdrantClient,
		CatalogQdrantClient:  d.CatalogQdrantClient,
		CatalogReader:        d.CatalogReader,
		QuoteDrafter:         d.QuoteDrafter,
		PricingIntelligence:  d.PricingIntelligence,
		OfferCreator:         d.OfferCreator,
		CouncilService:       d.CouncilService,
		OrgSettingsReader:    d.OrgSettingsReader,
	}
}

func (d *ToolDependencies) SetTenantID(tenantID uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tenantID = &tenantID
}

func (d *ToolDependencies) SetOrganizationAISettingsReader(reader ports.OrganizationAISettingsReader) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.OrgSettingsReader = reader
}

// LoadOrganizationAISettings fetches organization AI settings (if a reader is configured)
// and stores them on the ToolDependencies for later tool calls.
//
// Returns the loaded settings. If loading fails, returns an error.
func (d *ToolDependencies) LoadOrganizationAISettings(ctx context.Context) (ports.OrganizationAISettings, error) {
	tenantID, ok := d.GetTenantID()
	if !ok || tenantID == nil {
		return ports.OrganizationAISettings{}, errors.New(strings.ToLower(missingTenantContextMessage[:1]) + missingTenantContextMessage[1:])
	}

	d.mu.RLock()
	reader := d.OrgSettingsReader
	d.mu.RUnlock()
	if reader == nil {
		settings := ports.DefaultOrganizationAISettings()
		d.mu.Lock()
		d.orgSettings = &settings
		d.mu.Unlock()
		return settings, nil
	}

	settings, err := reader(ctx, *tenantID)
	if err != nil {
		return ports.OrganizationAISettings{}, err
	}

	d.mu.Lock()
	d.orgSettings = &settings
	d.mu.Unlock()
	return settings, nil
}

func (d *ToolDependencies) GetOrganizationAISettings() (ports.OrganizationAISettings, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.orgSettings == nil {
		return ports.OrganizationAISettings{}, false
	}
	return *d.orgSettings, true
}

func (d *ToolDependencies) GetOrganizationAISettingsOrDefault() ports.OrganizationAISettings {
	if s, ok := d.GetOrganizationAISettings(); ok {
		return s
	}
	return ports.DefaultOrganizationAISettings()
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

func (d *ToolDependencies) SetCouncilMetadata(metadata map[string]any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if metadata == nil {
		d.lastCouncilMetadata = nil
		return
	}
	d.lastCouncilMetadata = metadata
}

func (d *ToolDependencies) GetLastCouncilMetadata() map[string]any {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.lastCouncilMetadata == nil {
		return nil
	}
	dup := make(map[string]any, len(d.lastCouncilMetadata))
	for k, v := range d.lastCouncilMetadata {
		dup[k] = v
	}
	return dup
}

func (d *ToolDependencies) GetActor() (string, string) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.actorType == "" {
		return "AI", "Agent"
	}
	return d.actorType, d.actorName
}

func buildAlertDedupKey(category, reasonCode, summary string) string {
	parts := []string{strings.TrimSpace(category), strings.TrimSpace(reasonCode), strings.TrimSpace(summary)}
	return strings.Join(parts, "|")
}

func (d *ToolDependencies) MarkAlertEmitted(category, reasonCode, summary string) bool {
	key := buildAlertDedupKey(category, reasonCode, summary)

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.emittedAlertKeys == nil {
		d.emittedAlertKeys = make(map[string]struct{})
	}
	if _, exists := d.emittedAlertKeys[key]; exists {
		return false
	}
	defer func() {
		d.emittedAlertKeys[key] = struct{}{}
	}()
	return true
}

// GetRunID returns the correlation ID for the current agent run.
func (d *ToolDependencies) GetRunID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.runID
}

// SetLastAnalysisMetadata stores the analysis metadata for inclusion in subsequent events
func (d *ToolDependencies) SetLastAnalysisMetadata(metadata map[string]any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastAnalysisMetadata = metadata
}

func (d *ToolDependencies) SetLastEstimationMetadata(metadata map[string]any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastEstimationMetadata = metadata
}

func (d *ToolDependencies) SetLastEstimateSnapshot(snapshot EstimateComputationSnapshot) {
	d.mu.Lock()
	defer d.mu.Unlock()
	copySnapshot := snapshot
	d.lastEstimateSnapshot = &copySnapshot
}

// GetLastAnalysisMetadata retrieves a shallow copy of the analysis metadata saved by SaveAnalysis.
func (d *ToolDependencies) GetLastAnalysisMetadata() map[string]any {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.lastAnalysisMetadata == nil {
		return nil
	}
	cp := make(map[string]any, len(d.lastAnalysisMetadata))
	for k, v := range d.lastAnalysisMetadata {
		cp[k] = v
	}
	return cp
}

func (d *ToolDependencies) GetLastEstimationMetadata() map[string]any {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.lastEstimationMetadata == nil {
		return nil
	}
	cp := make(map[string]any, len(d.lastEstimationMetadata))
	for k, v := range d.lastEstimationMetadata {
		cp[k] = v
	}
	return cp
}

func (d *ToolDependencies) GetLastEstimateSnapshot() (*EstimateComputationSnapshot, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.lastEstimateSnapshot == nil {
		return nil, false
	}
	copySnapshot := *d.lastEstimateSnapshot
	return &copySnapshot, true
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

// SetForceDraftQuote controls whether draft governance is bypassed for manual quote generation.
func (d *ToolDependencies) SetForceDraftQuote(force bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.forceDraftQuote = force
}

// ShouldForceDraftQuote returns true when manual quote generation should bypass draft governance.
func (d *ToolDependencies) ShouldForceDraftQuote() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.forceDraftQuote
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
	if d.serviceID != nil {
		d.runID = d.serviceID.String() + ":" + uuid.NewString()
	} else {
		d.runID = uuid.NewString()
	}
	d.saveAnalysisCalled = false
	d.saveEstimationCalled = false
	d.stageUpdateCalled = false
	d.lastStageUpdated = ""
	d.draftQuoteCalled = false
	d.offerCreated = false
	d.lastAnalysisMetadata = nil
	d.lastEstimationMetadata = nil
	d.lastEstimateSnapshot = nil
	d.lastCouncilMetadata = nil
	d.lastDraftResult = nil
	d.lastDraftInput = nil
	d.lastQuoteReviewResult = nil
	d.lastQuoteCritiqueInput = nil
	d.quoteCriticAttempt = 0
	d.scopeArtifact = nil
	d.clarificationAsked = false
	d.existingQuoteID = nil
	d.forceDraftQuote = false
	d.searchCache = nil
	d.emittedAlertKeys = nil
}

func (d *ToolDependencies) getSearchCache(key string) (SearchProductMaterialsOutput, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.searchCache == nil {
		return SearchProductMaterialsOutput{}, false
	}
	output, ok := d.searchCache[key]
	return output, ok
}

func (d *ToolDependencies) setSearchCache(key string, output SearchProductMaterialsOutput) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.searchCache == nil {
		d.searchCache = make(map[string]SearchProductMaterialsOutput)
	}
	d.searchCache[key] = output
}

// SetLastDraftResult stores the last DraftQuoteResult for retrieval by callers.
func (d *ToolDependencies) SetLastDraftResult(result *ports.DraftQuoteResult) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastDraftResult = result
}

func (d *ToolDependencies) SetLastDraftInput(input DraftQuoteInput) {
	d.mu.Lock()
	defer d.mu.Unlock()
	copyInput := DraftQuoteInput{
		Notes: input.Notes,
		Items: append([]DraftQuoteItem(nil), input.Items...),
	}
	d.lastDraftInput = &copyInput
}

func (d *ToolDependencies) GetLastDraftInput() (*DraftQuoteInput, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.lastDraftInput == nil {
		return nil, false
	}
	copyInput := DraftQuoteInput{
		Notes: d.lastDraftInput.Notes,
		Items: append([]DraftQuoteItem(nil), d.lastDraftInput.Items...),
	}
	return &copyInput, true
}

func (d *ToolDependencies) SetLastQuoteReviewResult(result *ports.QuoteAIReviewResult) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastQuoteReviewResult = result
}

func (d *ToolDependencies) GetLastQuoteReviewResult() *ports.QuoteAIReviewResult {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastQuoteReviewResult
}

func (d *ToolDependencies) SetLastQuoteCritiqueInput(input SubmitQuoteCritiqueInput) {
	d.mu.Lock()
	defer d.mu.Unlock()
	copyInput := SubmitQuoteCritiqueInput{
		Approved: input.Approved,
		Summary:  input.Summary,
		Findings: append([]QuoteCritiqueFinding(nil), input.Findings...),
		Signals:  append([]string(nil), input.Signals...),
	}
	d.lastQuoteCritiqueInput = &copyInput
}

func (d *ToolDependencies) GetLastQuoteCritiqueInput() (*SubmitQuoteCritiqueInput, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.lastQuoteCritiqueInput == nil {
		return nil, false
	}
	copyInput := SubmitQuoteCritiqueInput{
		Approved: d.lastQuoteCritiqueInput.Approved,
		Summary:  d.lastQuoteCritiqueInput.Summary,
		Findings: append([]QuoteCritiqueFinding(nil), d.lastQuoteCritiqueInput.Findings...),
		Signals:  append([]string(nil), d.lastQuoteCritiqueInput.Signals...),
	}
	return &copyInput, true
}

func (d *ToolDependencies) SetQuoteCriticAttempt(attempt int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.quoteCriticAttempt = attempt
}

func (d *ToolDependencies) GetQuoteCriticAttempt() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.quoteCriticAttempt
}

// SetScopeArtifact stores the scope analyzer artifact for downstream quote building.
func (d *ToolDependencies) SetScopeArtifact(artifact ScopeArtifact) {
	d.mu.Lock()
	defer d.mu.Unlock()
	copyArtifact := artifact
	d.scopeArtifact = &copyArtifact
}

// GetScopeArtifact returns the latest scope artifact if available.
func (d *ToolDependencies) GetScopeArtifact() (*ScopeArtifact, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.scopeArtifact == nil {
		return nil, false
	}
	copyArtifact := *d.scopeArtifact
	return &copyArtifact, true
}

// MarkClarificationAsked marks that investigative mode asked the customer for more intake details.
func (d *ToolDependencies) MarkClarificationAsked() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.clarificationAsked = true
}

// WasClarificationAsked reports whether AskCustomerClarification was called.
func (d *ToolDependencies) WasClarificationAsked() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.clarificationAsked
}

// GetLastDraftResult returns the last DraftQuoteResult (set by handleDraftQuote).
func (d *ToolDependencies) GetLastDraftResult() *ports.DraftQuoteResult {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastDraftResult
}

// IsProductSearchEnabled returns true if both embedding and Qdrant clients are configured.
func (d *ToolDependencies) IsProductSearchEnabled() bool {
	return d.EmbeddingClient != nil && (d.CatalogQdrantClient != nil || d.QdrantClient != nil || d.BouwmaatQdrantClient != nil)
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
		return uuid.UUID{}, errors.New(missingTenantContextError)
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

func normalizeMissingInformation(items []string) []string {
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	sort.Strings(normalized)
	return normalized
}

func normalizeExtractedFacts(facts map[string]string) map[string]string {
	if len(facts) == 0 {
		return map[string]string{}
	}
	normalized := make(map[string]string, len(facts))
	for key, value := range facts {
		trimmedKey := strings.TrimSpace(key)
		trimmedValue := strings.TrimSpace(value)
		if trimmedKey == "" || trimmedValue == "" {
			continue
		}
		normalized[trimmedKey] = trimmedValue
	}
	if len(normalized) == 0 {
		return map[string]string{}
	}
	return normalized
}

// contactMessageReplacer is compiled once and reused for all contact message normalisations.
var contactMessageReplacer = strings.NewReplacer(
	"\r\n", "\n",
	"\\n", "\n",
	"\\t", " ",
	"\r", "",
	"nodie", "nodig",
	"nodien", "nodig",
)

func normalizeSuggestedContactMessage(message string) string {
	if strings.TrimSpace(message) == "" {
		return ""
	}

	normalized := contactMessageReplacer.Replace(message)

	lines := strings.Split(normalized, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	normalized = strings.Join(lines, "\n")
	return strings.TrimSpace(normalized)
}

func isEquivalentRecentAnalysis(current repository.AIAnalysis, candidate normalizedAnalysisInput) bool {
	if time.Since(current.CreatedAt) > recentEquivalentAnalysisTTL {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(current.UrgencyLevel), strings.TrimSpace(candidate.UrgencyLevel)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(current.LeadQuality), strings.TrimSpace(candidate.LeadQuality)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(current.RecommendedAction), strings.TrimSpace(candidate.RecommendedAction)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(current.PreferredContactChannel), strings.TrimSpace(candidate.Channel)) {
		return false
	}
	if strings.TrimSpace(normalizeSuggestedContactMessage(current.SuggestedContactMessage)) != strings.TrimSpace(normalizeSuggestedContactMessage(candidate.SuggestedMessage)) {
		return false
	}
	return equalNormalizedStringSlices(current.MissingInformation, candidate.MissingInformation) &&
		equalNormalizedStringSlices(current.ResolvedInformation, candidate.ResolvedInformation) &&
		equalNormalizedFactMaps(current.ExtractedFacts, candidate.ExtractedFacts)
}

func equalNormalizedStringSlices(current []string, candidate []string) bool {
	currentNormalized := normalizeMissingInformation(current)
	candidateNormalized := normalizeMissingInformation(candidate)
	if len(currentNormalized) != len(candidateNormalized) {
		return false
	}
	for i := range currentNormalized {
		if !strings.EqualFold(currentNormalized[i], candidateNormalized[i]) {
			return false
		}
	}
	return true
}

func equalNormalizedFactMaps(current map[string]string, candidate map[string]string) bool {
	currentNormalized := normalizeExtractedFacts(current)
	candidateNormalized := normalizeExtractedFacts(candidate)
	if len(currentNormalized) != len(candidateNormalized) {
		return false
	}
	for key, value := range currentNormalized {
		if !strings.EqualFold(strings.TrimSpace(candidateNormalized[key]), strings.TrimSpace(value)) {
			return false
		}
	}
	return true
}

func parseLeadServiceID(value string) (uuid.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return uuid.UUID{}, fmt.Errorf("missing lead service ID")
	}
	return parseUUID(value, invalidLeadServiceIDMessage)
}

func resolveSaveAnalysisContext(ctx tool.Context, deps *ToolDependencies, input SaveAnalysisInput) (uuid.UUID, uuid.UUID, uuid.UUID, SaveAnalysisOutput, error) {
	leadID, err := parseUUID(input.LeadID, invalidLeadIDMessage)
	if err != nil {
		log.Printf("handleSaveAnalysis: FAILED - invalid leadID: %s", input.LeadID)
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, SaveAnalysisOutput{Success: false, Message: invalidLeadIDMessage}, err
	}

	tenantID, err := getTenantID(deps)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, SaveAnalysisOutput{Success: false, Message: missingTenantContextMessage}, err
	}

	leadServiceID, err := parseLeadServiceID(input.LeadServiceID)
	if err != nil {
		message := err.Error()
		if err.Error() == invalidLeadServiceIDMessage {
			message = invalidLeadServiceIDMessage
		}
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, SaveAnalysisOutput{Success: false, Message: message}, err
	}

	if out, err := rejectTerminalAnalysisService(ctx, deps, tenantID, leadServiceID); err != nil {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, out, err
	}

	return leadID, tenantID, leadServiceID, SaveAnalysisOutput{}, nil
}

func rejectTerminalAnalysisService(ctx tool.Context, deps *ToolDependencies, tenantID, leadServiceID uuid.UUID) (SaveAnalysisOutput, error) {
	// Terminal check: refuse to save analysis for terminal services.
	svc, err := deps.Repo.GetLeadServiceByID(ctx, leadServiceID, tenantID)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: leadServiceNotFoundMessage}, err
	}
	if domain.IsTerminal(svc.Status, svc.PipelineStage) {
		log.Printf("handleSaveAnalysis: REJECTED - service %s is in terminal state (status=%s, stage=%s)", leadServiceID, svc.Status, svc.PipelineStage)
		return SaveAnalysisOutput{Success: false, Message: "Cannot save analysis for a service in terminal state"}, fmt.Errorf("service %s is terminal", leadServiceID)
	}
	return SaveAnalysisOutput{}, nil
}

type normalizedAnalysisInput struct {
	UrgencyLevel        string
	UrgencyReason       *string
	LeadQuality         string
	RecommendedAction   string
	Channel             string
	MissingInformation  []string
	ResolvedInformation []string
	ExtractedFacts      map[string]string
	SuggestedMessage    string
	Summary             string
	CompositeConfidence float64
	ConfidenceBreakdown map[string]float64
	RiskFlags           []string
	Metadata            map[string]any
}

type trustedAnalysisContext struct {
	service       *repository.LeadService
	priorAnalysis *repository.AIAnalysis
	visitReport   *repository.AppointmentVisitReport
	photoAnalysis *repository.PhotoAnalysis
	attachments   []repository.Attachment
}

type analysisFactCollector struct {
	resolved map[string]string
	facts    map[string]string
}

type analysisPreferences struct {
	Budget       string `json:"budget"`
	Timeframe    string `json:"timeframe"`
	Availability string `json:"availability"`
	ExtraNotes   string `json:"extraNotes"`
}

func loadTrustedAnalysisContext(ctx context.Context, deps *ToolDependencies, tenantID, leadServiceID uuid.UUID) trustedAnalysisContext {
	trusted := trustedAnalysisContext{}
	if service, err := deps.Repo.GetLeadServiceByID(ctx, leadServiceID, tenantID); err == nil {
		trusted.service = &service
	}
	if analysis, err := deps.Repo.GetLatestAIAnalysis(ctx, leadServiceID, tenantID); err == nil {
		trusted.priorAnalysis = &analysis
	}
	if report, err := deps.Repo.GetLatestAppointmentVisitReportByService(ctx, leadServiceID, tenantID); err == nil {
		trusted.visitReport = report
	}
	if photoAnalysis, err := deps.Repo.GetLatestPhotoAnalysis(ctx, leadServiceID, tenantID); err == nil {
		trusted.photoAnalysis = &photoAnalysis
	}
	if attachments, err := deps.Repo.ListAttachmentsByService(ctx, leadServiceID, tenantID); err == nil {
		trusted.attachments = attachments
	}
	return trusted
}

func populateAnalysisFacts(ctx context.Context, deps *ToolDependencies, lead repository.Lead, tenantID, leadServiceID uuid.UUID, resolvedInformation []string, extractedFacts map[string]string) ([]string, map[string]string) {
	trusted := loadTrustedAnalysisContext(ctx, deps, tenantID, leadServiceID)
	collector := newAnalysisFactCollector(resolvedInformation, extractedFacts)
	mergeLeadFacts(&collector, lead)
	mergeServiceFacts(&collector, trusted.service)
	mergeVisitReportFacts(&collector, trusted.visitReport)
	mergePhotoAnalysisFacts(&collector, trusted.photoAnalysis)
	mergeAttachmentFacts(&collector, trusted.attachments)
	mergePriorAnalysisFacts(&collector, trusted.priorAnalysis)
	return collector.resolvedValues(), collector.extractedFactMap()
}

func newAnalysisFactCollector(resolvedInformation []string, extractedFacts map[string]string) analysisFactCollector {
	collector := analysisFactCollector{
		resolved: map[string]string{},
		facts:    map[string]string{},
	}
	for _, value := range normalizeMissingInformation(resolvedInformation) {
		collector.addResolved(value)
	}
	for key, value := range normalizeExtractedFacts(extractedFacts) {
		collector.addFact(key, value)
	}
	return collector
}

func (c *analysisFactCollector) addResolved(value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}
	normalized := strings.ToLower(trimmed)
	if _, exists := c.resolved[normalized]; exists {
		return
	}
	c.resolved[normalized] = trimmed
}

func (c *analysisFactCollector) addFact(key string, value string) {
	trimmedKey := strings.TrimSpace(key)
	trimmedValue := strings.TrimSpace(value)
	if trimmedKey == "" || trimmedValue == "" {
		return
	}
	if _, exists := c.facts[trimmedKey]; exists {
		return
	}
	c.facts[trimmedKey] = trimmedValue
}

func (c *analysisFactCollector) resolvedValues() []string {
	values := make([]string, 0, len(c.resolved))
	for _, value := range c.resolved {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		return strings.ToLower(values[i]) < strings.ToLower(values[j])
	})
	return values
}

func (c *analysisFactCollector) extractedFactMap() map[string]string {
	result := make(map[string]string, len(c.facts))
	for key, value := range c.facts {
		result[key] = value
	}
	return result
}

func mergePriorAnalysisFacts(collector *analysisFactCollector, priorAnalysis *repository.AIAnalysis) {
	if priorAnalysis == nil {
		return
	}
	for _, value := range priorAnalysis.ResolvedInformation {
		collector.addResolved(value)
	}
	for key, value := range priorAnalysis.ExtractedFacts {
		collector.addFact(key, value)
	}
}

func mergeLeadFacts(collector *analysisFactCollector, lead repository.Lead) {
	if lead.EnergyClass != nil {
		collector.addFact("energy_class", *lead.EnergyClass)
	}
	if lead.EnergyBouwjaar != nil {
		collector.addFact("build_year", strconv.Itoa(*lead.EnergyBouwjaar))
	}
	if lead.EnergyGebouwtype != nil {
		collector.addFact("building_type", *lead.EnergyGebouwtype)
	}
}

func mergeServiceFacts(collector *analysisFactCollector, service *repository.LeadService) {
	if service == nil {
		return
	}
	collector.addFact("service_type", service.ServiceType)
	collector.addFact("consumer_note", trustedOptionalString(service.ConsumerNote))
	prefs := parseAnalysisPreferences(service.CustomerPreferences)
	addPreferenceFact(collector, "budget", prefs.Budget, "Budget gedeeld: %s")
	addPreferenceFact(collector, "timeframe", prefs.Timeframe, "Gewenste termijn: %s")
	addPreferenceFact(collector, "availability", prefs.Availability, "Beschikbaarheid gedeeld: %s")
	collector.addFact("preference_notes", prefs.ExtraNotes)
}

func parseAnalysisPreferences(raw json.RawMessage) analysisPreferences {
	if len(raw) == 0 {
		return analysisPreferences{}
	}
	var prefs analysisPreferences
	if err := json.Unmarshal(raw, &prefs); err != nil {
		return analysisPreferences{}
	}
	prefs.Budget = strings.TrimSpace(prefs.Budget)
	prefs.Timeframe = strings.TrimSpace(prefs.Timeframe)
	prefs.Availability = strings.TrimSpace(prefs.Availability)
	prefs.ExtraNotes = strings.TrimSpace(prefs.ExtraNotes)
	return prefs
}

func addPreferenceFact(collector *analysisFactCollector, key string, value string, resolvedTemplate string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	collector.addFact(key, value)
	collector.addResolved(fmt.Sprintf(resolvedTemplate, value))
}

func trustedOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" || strings.EqualFold(trimmed, valueNotProvided) {
		return ""
	}
	return trimmed
}

func mergeVisitReportFacts(collector *analysisFactCollector, visitReport *repository.AppointmentVisitReport) {
	if visitReport == nil {
		return
	}
	if measurements := trustedOptionalString(visitReport.Measurements); measurements != "" {
		collector.addFact("visit_report_measurements", measurements)
		collector.addResolved("Ingemeten tijdens afspraak: " + measurements)
	}
	collector.addFact("visit_report_access_difficulty", trustedOptionalString(visitReport.AccessDifficulty))
	collector.addFact("visit_report_notes", trustedOptionalString(visitReport.Notes))
}

func mergePhotoAnalysisFacts(collector *analysisFactCollector, photoAnalysis *repository.PhotoAnalysis) {
	if photoAnalysis == nil {
		return
	}
	collector.addFact("photo_summary", strings.TrimSpace(photoAnalysis.Summary))
	collector.addFact("photo_scope_assessment", strings.TrimSpace(photoAnalysis.ScopeAssessment))
	collector.addFact("photo_measurements", summarizePhotoMeasurements(photoAnalysis.Measurements))
	collector.addFact("photo_ocr_text", strings.Join(compactPromptList(photoAnalysis.ExtractedText), "; "))
	collector.addFact("photo_needs_onsite_measurement", strings.Join(compactPromptList(photoAnalysis.NeedsOnsiteMeasurement), "; "))
}

func summarizePhotoMeasurements(measurements []repository.Measurement) string {
	if len(measurements) == 0 {
		return ""
	}
	parts := make([]string, 0, len(measurements))
	for _, measurement := range measurements {
		description := strings.TrimSpace(measurement.Description)
		unit := strings.TrimSpace(measurement.Unit)
		if description == "" {
			description = "maat"
		}
		value := strconv.FormatFloat(measurement.Value, 'f', -1, 64)
		segment := description + ": " + value
		if unit != "" {
			segment += " " + unit
		}
		parts = append(parts, segment)
	}
	return strings.Join(parts, "; ")
}

func mergeAttachmentFacts(collector *analysisFactCollector, attachments []repository.Attachment) {
	if len(attachments) == 0 {
		return
	}
	documentNames := make([]string, 0, len(attachments))
	requiresDocumentReview := false
	for _, attachment := range attachments {
		_, isNonImageDocument, requiresReview := classifyAttachment(attachment)
		if isNonImageDocument {
			documentNames = append(documentNames, strings.TrimSpace(attachment.FileName))
		}
		if requiresReview {
			requiresDocumentReview = true
		}
	}
	if len(documentNames) > 0 {
		sort.Strings(documentNames)
		collector.addFact("attachment_documents", strings.Join(documentNames, ", "))
	}
	if requiresDocumentReview {
		collector.addFact("document_review_required", "true")
		collector.addResolved("Klant heeft document(en) geüpload voor handmatige controle")
	}
}

func normalizeAnalysisInput(ctx tool.Context, deps *ToolDependencies, input SaveAnalysisInput, lead repository.Lead, tenantID, leadServiceID uuid.UUID) (normalizedAnalysisInput, error) {
	urgencyLevel, err := normalizeUrgencyLevel(input.UrgencyLevel)
	if err != nil {
		return normalizedAnalysisInput{}, err
	}

	var urgencyReason *string
	trimmedUrgencyReason := strings.TrimSpace(input.UrgencyReason)
	if trimmedUrgencyReason != "" {
		urgencyReason = &trimmedUrgencyReason
	}

	channel, err := resolvePreferredChannel(input.PreferredContactChannel, lead)
	if err != nil {
		return normalizedAnalysisInput{}, fmt.Errorf("invalid preferred contact channel")
	}

	leadQuality := normalizeLeadQuality(input.LeadQuality)
	recommendedAction := normalizeRecommendedAction(input.RecommendedAction)
	log.Printf("handleSaveAnalysis: normalized recommendedAction '%s' -> '%s'", input.RecommendedAction, recommendedAction)

	missingInformation := normalizeMissingInformation(input.MissingInformation)

	// Guard: LLMs sometimes produce a contradiction where missing fields are listed
	// but the recommended action is not "RequestInfo". Force the correct action so
	// the domain invariant check (ValidateAnalysisStageTransition) is never bypassed.
	if len(missingInformation) > 0 && recommendedAction != "RequestInfo" {
		log.Printf("normalizeAnalysisInput: auto-correcting contradictory recommendedAction '%s' -> 'RequestInfo' (missingInformation non-empty, count=%d)", recommendedAction, len(missingInformation))
		recommendedAction = "RequestInfo"
	}

	resolvedInformation := normalizeMissingInformation(input.ResolvedInformation)
	extractedFacts := normalizeExtractedFacts(input.ExtractedFacts)
	resolvedInformation, extractedFacts = populateAnalysisFacts(ctx, deps, lead, tenantID, leadServiceID, resolvedInformation, extractedFacts)
	normalizedMessage := normalizeSuggestedContactMessage(input.SuggestedContactMessage)
	analysisSummary := strings.TrimSpace(input.Summary)
	if analysisSummary == "" {
		analysisSummary = fmt.Sprintf("AI analyse voltooid: %s urgentie, aanbevolen actie: %s", urgencyLevel, recommendedAction)
	}

	meta := repository.AIAnalysisMetadata{
		RunID:             deps.GetRunID(),
		UrgencyLevel:      urgencyLevel,
		RecommendedAction: recommendedAction,
		LeadQuality:       leadQuality,
	}
	if normalizedMessage != "" {
		meta.SuggestedContactMessage = normalizedMessage
		meta.PreferredContactChannel = string(channel)
	}
	if len(missingInformation) > 0 {
		meta.MissingInformation = missingInformation
	}
	if len(resolvedInformation) > 0 {
		meta.ResolvedInformation = resolvedInformation
	}
	if len(extractedFacts) > 0 {
		meta.ExtractedFacts = extractedFacts
	}

	var photoAnalysis *repository.PhotoAnalysis
	if pa, paErr := deps.Repo.GetLatestPhotoAnalysis(ctx, leadServiceID, tenantID); paErr == nil {
		photoAnalysis = &pa
	}

	confidence := calculateAnalysisConfidence(lead, leadQuality, recommendedAction, missingInformation, photoAnalysis)
	meta.CompositeConfidence = confidence.Score
	meta.ConfidenceBreakdown = confidence.Breakdown
	meta.RiskFlags = confidence.RiskFlags

	return normalizedAnalysisInput{
		UrgencyLevel:        urgencyLevel,
		UrgencyReason:       urgencyReason,
		LeadQuality:         leadQuality,
		RecommendedAction:   recommendedAction,
		Channel:             channel,
		MissingInformation:  missingInformation,
		ResolvedInformation: resolvedInformation,
		ExtractedFacts:      extractedFacts,
		SuggestedMessage:    normalizedMessage,
		Summary:             analysisSummary,
		CompositeConfidence: confidence.Score,
		ConfidenceBreakdown: confidence.Breakdown,
		RiskFlags:           confidence.RiskFlags,
		Metadata:            meta.ToMap(),
	}, nil
}

func shouldSkipEquivalentRecentAnalysis(ctx tool.Context, deps *ToolDependencies, leadServiceID, tenantID uuid.UUID, normalized normalizedAnalysisInput) bool {
	latest, latestErr := deps.Repo.GetLatestAIAnalysis(ctx, leadServiceID, tenantID)
	if latestErr != nil {
		return false
	}
	return isEquivalentRecentAnalysis(latest, normalized)
}

func handleSaveAnalysis(ctx tool.Context, deps *ToolDependencies, input SaveAnalysisInput) (SaveAnalysisOutput, error) {
	log.Printf("handleSaveAnalysis: CALLED with leadID=%s serviceID=%s urgency=%s action=%s",
		input.LeadID, input.LeadServiceID, input.UrgencyLevel, input.RecommendedAction)

	leadID, tenantID, leadServiceID, contextOut, err := resolveSaveAnalysisContext(ctx, deps, input)
	if err != nil {
		return contextOut, err
	}

	lead, err := deps.Repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: leadNotFoundMessage}, err
	}

	normalized, err := normalizeAnalysisInput(ctx, deps, input, lead, tenantID, leadServiceID)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: err.Error()}, err
	}

	if shouldSkipEquivalentRecentAnalysis(ctx, deps, leadServiceID, tenantID, normalized) {
		deps.SetLastAnalysisMetadata(normalized.Metadata)
		deps.MarkSaveAnalysisCalled()
		log.Printf("handleSaveAnalysis: skipped duplicate-equivalent analysis for lead=%s service=%s", leadID, leadServiceID)
		return SaveAnalysisOutput{Success: true, Message: "Analysis unchanged; duplicate save skipped"}, nil
	}

	_, err = deps.Repo.CreateAIAnalysis(context.Background(), repository.CreateAIAnalysisParams{
		LeadID:                  leadID,
		OrganizationID:          tenantID,
		LeadServiceID:           leadServiceID,
		UrgencyLevel:            normalized.UrgencyLevel,
		UrgencyReason:           normalized.UrgencyReason,
		LeadQuality:             normalized.LeadQuality,
		RecommendedAction:       normalized.RecommendedAction,
		MissingInformation:      normalized.MissingInformation,
		ResolvedInformation:     normalized.ResolvedInformation,
		ExtractedFacts:          normalized.ExtractedFacts,
		CompositeConfidence:     &normalized.CompositeConfidence,
		ConfidenceBreakdown:     normalized.ConfidenceBreakdown,
		RiskFlags:               normalized.RiskFlags,
		PreferredContactChannel: normalized.Channel,
		SuggestedContactMessage: normalized.SuggestedMessage,
		Summary:                 normalized.Summary,
	})
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: err.Error()}, err
	}

	actorType, actorName := deps.GetActor()

	// Create comprehensive analysis timeline event for frontend rendering.
	_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &leadServiceID,
		OrganizationID: tenantID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      repository.EventTypeAI,
		Title:          repository.EventTitleGatekeeperAnalysis,
		Summary:        &normalized.Summary,
		Metadata:       normalized.Metadata,
	})

	// Store analysis metadata for use in stage_change events
	deps.SetLastAnalysisMetadata(normalized.Metadata)
	log.Printf("SaveAnalysis: stored analysis metadata for lead=%s service=%s channel=%s action=%s",
		leadID, leadServiceID, normalized.Channel, normalized.RecommendedAction)

	recalculateAndRecordScore(ctx, deps, leadID, leadServiceID, tenantID, actorType, actorName)

	log.Printf(
		"gatekeeper SaveAnalysis: run=%s leadId=%s serviceId=%s urgency=%s quality=%s action=%s missing=%d confidence=%.2f risk_flags=%d",
		deps.GetRunID(),
		leadID,
		leadServiceID,
		normalized.UrgencyLevel,
		normalized.LeadQuality,
		normalized.RecommendedAction,
		len(input.MissingInformation),
		normalized.CompositeConfidence,
		len(normalized.RiskFlags),
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
		EventType:      repository.EventTypeAnalysis,
		Title:          repository.EventTitleLeadScoreUpdated,
		Summary:        &summary,
		Metadata: repository.LeadScoreMetadata{
			RunID:            deps.GetRunID(),
			LeadScore:        scoreResult.Score,
			LeadScorePreAI:   scoreResult.ScorePreAI,
			LeadScoreVersion: scoreResult.Version,
		}.ToMap(),
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

	// Stability guard: service type changes are only allowed during initial triage.
	// Gatekeeper re-runs on many changes (notes/attachments); without this guard the
	// LLM can "flip-flop" service type on ambiguous new info.
	if leadService.PipelineStage != domain.PipelineStageTriage {
		return UpdateLeadServiceTypeOutput{Success: false, Message: "Service type is locked after Triage"}, nil
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
		EventType:      repository.EventTypeServiceTypeChange,
		Title:          repository.EventTitleServiceTypeUpdated,
		Summary:        &reasonText,
		Metadata: repository.ServiceTypeChangeMetadata{
			OldServiceType: leadService.ServiceType,
			NewServiceType: serviceType,
			Reason:         input.Reason,
		}.ToMap(),
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

func (b *leadDetailsBuilder) setAssignee(input *string, current *uuid.UUID) error {
	if input == nil {
		return nil
	}
	value := strings.TrimSpace(*input)
	if value == "" {
		return fmt.Errorf("invalid assigneeId")
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return fmt.Errorf("invalid assigneeId")
	}
	b.params.AssignedAgentID = &parsed
	b.params.AssignedAgentIDSet = true
	if current == nil || *current != parsed {
		b.updatedFields = append(b.updatedFields, "assigneeId")
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

func (b *leadDetailsBuilder) setWhatsAppOptedIn(input *bool, current bool) {
	if input == nil {
		return
	}
	b.params.WhatsAppOptedIn = input
	b.params.WhatsAppOptedInSet = true
	if *input != current {
		b.updatedFields = append(b.updatedFields, "whatsAppOptedIn")
	}
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
	if err := b.setAssignee(input.AssigneeID, current.AssignedAgentID); err != nil {
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
	b.setWhatsAppOptedIn(input.WhatsAppOptedIn, current.WhatsAppOptedIn)
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
		EventType:      repository.EventTypeLeadUpdate,
		Title:          repository.EventTitleLeadDetailsUpdated,
		Summary:        &reasonText,
		Metadata: repository.LeadUpdateMetadata{
			UpdatedFields: updatedFields,
			Confidence:    confidence,
		}.ToMap(),
	})

	log.Printf("gatekeeper UpdateLeadDetails: leadId=%s fields=%v reason=%s", leadID, updatedFields, reasonText)
}
func validateProposalQuoteGuard(ctx context.Context, deps *ToolDependencies, stage string, serviceID, tenantID uuid.UUID) (UpdatePipelineStageOutput, error) {
	if stage != domain.PipelineStageProposal {
		return UpdatePipelineStageOutput{}, nil
	}

	hasNonDraftQuote, checkErr := deps.Repo.HasNonDraftQuote(ctx, serviceID, tenantID)
	if checkErr != nil {
		return UpdatePipelineStageOutput{Success: false, Message: "Failed to validate quote state"}, checkErr
	}
	if !hasNonDraftQuote {
		return UpdatePipelineStageOutput{Success: false, Message: "Cannot move to Proposal while quote is still draft"}, fmt.Errorf("quote state guard blocked Proposal for service %s", serviceID)
	}

	return UpdatePipelineStageOutput{}, nil
}

func validateActorSequence(stage string, serviceID uuid.UUID, deps *ToolDependencies, actorType, actorName string) (UpdatePipelineStageOutput, error) {
	if actorType == repository.ActorTypeAI && actorName == repository.ActorNameGatekeeper && !deps.WasSaveAnalysisCalled() {
		return UpdatePipelineStageOutput{Success: false, Message: "SaveAnalysis is required before stage update"}, fmt.Errorf("gatekeeper sequence violation: SaveAnalysis missing before UpdatePipelineStage for service %s", serviceID)
	}

	if actorType == repository.ActorTypeAI && actorName == repository.ActorNameEstimator {
		if !deps.WasSaveEstimationCalled() {
			return UpdatePipelineStageOutput{Success: false, Message: "SaveEstimation is required before stage update"}, fmt.Errorf("estimator sequence violation: SaveEstimation missing before UpdatePipelineStage for service %s", serviceID)
		}
		if stage == domain.PipelineStageEstimation && !deps.WasDraftQuoteCalled() {
			return UpdatePipelineStageOutput{Success: false, Message: "DraftQuote is required before moving to Estimation"}, fmt.Errorf("estimator sequence violation: DraftQuote missing before Estimation stage update for service %s", serviceID)
		}
	}

	if actorType == repository.ActorTypeAI && actorName == repository.ActorNameDispatcher && stage == domain.PipelineStageFulfillment && !deps.WasOfferCreated() {
		return UpdatePipelineStageOutput{Success: false, Message: "CreatePartnerOffer is required before moving to Fulfillment"}, fmt.Errorf("dispatcher sequence violation: CreatePartnerOffer missing before Fulfillment stage update for service %s", serviceID)
	}

	return UpdatePipelineStageOutput{}, nil
}

func validateEstimationInvariant(ctx context.Context, deps *ToolDependencies, stage string, serviceID, tenantID uuid.UUID) (UpdatePipelineStageOutput, error) {
	if stage != domain.PipelineStageEstimation {
		return UpdatePipelineStageOutput{}, nil
	}

	recommendedAction, missingInformation := latestAnalysisInvariantInputs(ctx, deps, serviceID, tenantID)
	if reason := domain.ValidateAnalysisStageTransition(recommendedAction, missingInformation, stage); reason != "" {
		log.Printf("stage_blocked=true stage=%s service=%s block_reason=%s recommended_action=%s missing_count=%d",
			stage, serviceID, reason, recommendedAction, len(missingInformation))
		return UpdatePipelineStageOutput{Success: false, Message: "Cannot move to Estimation while intake is incomplete"}, fmt.Errorf("analysis-stage invariant blocked Estimation for service %s: %s", serviceID, reason)
	}

	return UpdatePipelineStageOutput{}, nil
}

func evaluateCouncilForStageUpdate(ctx context.Context, deps *ToolDependencies, leadID, serviceID, tenantID uuid.UUID, targetStage string) (CouncilEvaluation, error) {
	settings := deps.GetOrganizationAISettingsOrDefault()
	if !settings.AICouncilMode || deps.CouncilService == nil {
		deps.SetCouncilMetadata(nil)
		return CouncilEvaluation{Decision: CouncilDecisionAllow, ReasonCode: "council_disabled", Summary: "Council uitgeschakeld."}, nil
	}

	evaluation, err := deps.CouncilService.Evaluate(ctx, CouncilEvaluationInput{
		Action:      CouncilActionStageUpdate,
		LeadID:      leadID,
		ServiceID:   serviceID,
		TenantID:    tenantID,
		Mode:        settings.AICouncilConsensusMode,
		TargetStage: targetStage,
	})
	if err != nil {
		return CouncilEvaluation{}, err
	}

	deps.SetCouncilMetadata(repository.CouncilAdviceMetadata{
		Decision:         evaluation.Decision,
		ReasonCode:       evaluation.ReasonCode,
		Summary:          evaluation.Summary,
		EstimatorSignals: evaluation.EstimatorSignals,
		RiskSignals:      evaluation.RiskSignals,
		ReadinessSignals: evaluation.ReadinessSignals,
	}.ToMap())

	return evaluation, nil
}

func handleUpdatePipelineStage(ctx tool.Context, deps *ToolDependencies, input UpdatePipelineStageInput) (UpdatePipelineStageOutput, error) {
	return applyPipelineStageUpdate(ctx, deps, input)
}

func applyPipelineStageUpdate(ctx context.Context, deps *ToolDependencies, input UpdatePipelineStageInput) (UpdatePipelineStageOutput, error) {
	state, loopResult, out, done, err := prepareStageUpdate(ctx, deps, &input)
	if done || err != nil {
		return out, err
	}

	councilEval, councilErr := evaluateCouncilForStageUpdate(ctx, deps, state.leadID, state.serviceID, state.tenantID, input.Stage)
	if councilErr != nil {
		return UpdatePipelineStageOutput{Success: false, Message: "Council evaluatie mislukt"}, councilErr
	}

	out, err = applyCouncilDecision(ctx, deps, councilEval, state, &input)
	if err != nil || out.Message != "" {
		return out, err
	}

	_, err = deps.Repo.UpdatePipelineStage(ctx, state.serviceID, state.tenantID, input.Stage)
	if err != nil {
		return UpdatePipelineStageOutput{Success: false, Message: "Failed to update pipeline stage"}, err
	}
	if shouldResetGatekeeperNurturingLoopState(state.service, input.Stage, loopResult.Trigger) {
		if resetErr := deps.Repo.ResetGatekeeperNurturingLoopState(ctx, state.serviceID, state.tenantID); resetErr != nil {
			log.Printf("gatekeeper nurturing loop reset failed for service=%s tenant=%s: %v", state.serviceID, state.tenantID, resetErr)
		}
	}

	recordPipelineStageChange(ctx, deps, stageChangeParams{
		leadID:             state.leadID,
		serviceID:          state.serviceID,
		tenantID:           state.tenantID,
		oldStage:           state.oldStage,
		newStage:           input.Stage,
		reason:             input.Reason,
		trigger:            loopResult.Trigger,
		reasonCode:         loopResult.ReasonCode,
		loopAttemptCount:   loopResult.AttemptCount,
		blockerFingerprint: loopResult.BlockerFingerprint,
		missingInformation: loopResult.MissingInformation,
	})
	deps.MarkStageUpdateCalled(input.Stage)
	log.Printf("agent stage transition committed (run=%s actor=%s/%s lead=%s service=%s from=%s to=%s)",
		state.runID, state.actorType, state.actorName, state.leadID, state.serviceID, state.oldStage, input.Stage)
	return UpdatePipelineStageOutput{Success: true, Message: "Pipeline stage updated"}, nil
}

type stageUpdateState struct {
	tenantID  uuid.UUID
	leadID    uuid.UUID
	serviceID uuid.UUID
	oldStage  string
	service   repository.LeadService
	actorType string
	actorName string
	runID     string
}

type gatekeeperNurturingLoopResult struct {
	Trigger            string
	ReasonCode         string
	AttemptCount       int
	BlockerFingerprint string
	MissingInformation []string
}

func prepareStageUpdate(ctx context.Context, deps *ToolDependencies, input *UpdatePipelineStageInput) (stageUpdateState, gatekeeperNurturingLoopResult, UpdatePipelineStageOutput, bool, error) {
	if !domain.IsKnownPipelineStage(input.Stage) {
		return stageUpdateState{}, gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{Success: false, Message: "Invalid pipeline stage"}, true, fmt.Errorf("invalid pipeline stage: %s", input.Stage)
	}

	tenantID, err := getTenantID(deps)
	if err != nil {
		return stageUpdateState{}, gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{Success: false, Message: missingTenantContextMessage}, true, err
	}
	leadID, serviceID, err := getLeadContext(deps)
	if err != nil {
		return stageUpdateState{}, gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{Success: false, Message: missingLeadContextMessage}, true, err
	}

	svc, err := deps.Repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return stageUpdateState{}, gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{Success: false, Message: leadServiceNotFoundMessage}, true, err
	}
	actorType, actorName := deps.GetActor()
	state := stageUpdateState{
		tenantID:  tenantID,
		leadID:    leadID,
		serviceID: serviceID,
		oldStage:  svc.PipelineStage,
		service:   svc,
		actorType: actorType,
		actorName: actorName,
		runID:     deps.GetRunID(),
	}

	if domain.IsTerminal(svc.Status, svc.PipelineStage) {
		log.Printf("handleUpdatePipelineStage: REJECTED - service %s is in terminal state (status=%s, stage=%s)", state.serviceID, svc.Status, svc.PipelineStage)
		return state, gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{Success: false, Message: "Cannot update pipeline stage for a service in terminal state"}, true, fmt.Errorf("service %s is terminal", state.serviceID)
	}
	if out, err := validateProposalQuoteGuard(ctx, deps, input.Stage, state.serviceID, state.tenantID); err != nil {
		return state, gatekeeperNurturingLoopResult{}, out, true, err
	}
	if reason := domain.ValidateStateCombination(svc.Status, input.Stage); reason != "" {
		log.Printf("handleUpdatePipelineStage: invalid state combination: status=%s, newStage=%s - %s", svc.Status, input.Stage, reason)
		return state, gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{Success: false, Message: reason}, true, fmt.Errorf("invalid state combination: %s", reason)
	}
	if out, err := validateActorSequence(input.Stage, state.serviceID, deps, state.actorType, state.actorName); err != nil {
		return state, gatekeeperNurturingLoopResult{}, out, true, err
	}
	if out, err := validateEstimationInvariant(ctx, deps, input.Stage, state.serviceID, state.tenantID); err != nil {
		return state, gatekeeperNurturingLoopResult{}, out, true, err
	}

	loopResult, out, done, err := applyGatekeeperNurturingLoopPolicy(ctx, deps, state, input)
	if done || err != nil {
		return state, loopResult, out, done, err
	}

	if state.oldStage == input.Stage {
		deps.MarkStageUpdateCalled(input.Stage)
		log.Printf("agent stage transition skipped (run=%s actor=%s/%s lead=%s service=%s stage=%s): no change",
			state.runID, state.actorType, state.actorName, state.leadID, state.serviceID, input.Stage)
		return state, loopResult, UpdatePipelineStageOutput{Success: true, Message: "Pipeline stage unchanged"}, true, nil
	}

	return state, loopResult, UpdatePipelineStageOutput{}, false, nil
}

func applyGatekeeperNurturingLoopPolicy(ctx context.Context, deps *ToolDependencies, state stageUpdateState, input *UpdatePipelineStageInput) (gatekeeperNurturingLoopResult, UpdatePipelineStageOutput, bool, error) {
	if !shouldTrackGatekeeperNurturingLoop(state.actorName, input.Stage) {
		return gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{}, false, nil
	}

	fingerprint, missingInformation := resolveGatekeeperLoopFingerprint(ctx, deps, state.serviceID, state.tenantID, input.Reason)
	if fingerprint == "" {
		return gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{}, false, nil
	}

	attemptCount := 1
	if state.service.GatekeeperNurturingLoopFingerprint != nil && *state.service.GatekeeperNurturingLoopFingerprint == fingerprint {
		attemptCount = state.service.GatekeeperNurturingLoopCount + 1
	}
	if attemptCount < 1 {
		attemptCount = 1
	}
	if err := deps.Repo.SetGatekeeperNurturingLoopState(ctx, state.serviceID, state.tenantID, attemptCount, fingerprint); err != nil {
		return gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{Success: false, Message: "Failed to update nurturing loop state"}, true, err
	}

	result := gatekeeperNurturingLoopResult{
		AttemptCount:       attemptCount,
		BlockerFingerprint: fingerprint,
		MissingInformation: missingInformation,
	}
	if attemptCount >= gatekeeperNurturingLoopThreshold {
		input.Stage = domain.PipelineStageManualIntervention
		input.Reason = gatekeeperLoopDetectedSummary
		result.Trigger = gatekeeperLoopDetectedTrigger
		result.ReasonCode = gatekeeperLoopReasonCode
	}

	return result, UpdatePipelineStageOutput{}, false, nil
}

func shouldTrackGatekeeperNurturingLoop(actorName, targetStage string) bool {
	return actorName == repository.ActorNameGatekeeper && targetStage == domain.PipelineStageNurturing
}

func shouldResetGatekeeperNurturingLoopState(service repository.LeadService, targetStage, trigger string) bool {
	if service.GatekeeperNurturingLoopCount == 0 && service.GatekeeperNurturingLoopFingerprint == nil {
		return false
	}
	if targetStage == domain.PipelineStageNurturing {
		return false
	}
	if targetStage == domain.PipelineStageManualIntervention && trigger == gatekeeperLoopDetectedTrigger {
		return false
	}
	return true
}

func resolveGatekeeperLoopFingerprint(ctx context.Context, deps *ToolDependencies, serviceID, tenantID uuid.UUID, fallbackReason string) (string, []string) {
	_, missingInformation := latestAnalysisInvariantInputs(ctx, deps, serviceID, tenantID)
	normalizedMissingInformation := normalizeGatekeeperLoopItems(missingInformation)
	if len(normalizedMissingInformation) > 0 {
		return strings.Join(normalizedMissingInformation, " | "), normalizedMissingInformation
	}

	fallbackParts := normalizeGatekeeperLoopItems([]string{fallbackReason})
	if len(fallbackParts) > 0 {
		return "reason:" + strings.Join(fallbackParts, " | "), nil
	}

	return "", nil
}

func normalizeGatekeeperLoopItems(values []string) []string {
	set := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		if _, exists := set[trimmed]; exists {
			continue
		}
		set[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	sort.Strings(normalized)
	return normalized
}

func applyCouncilDecision(ctx context.Context, deps *ToolDependencies, eval CouncilEvaluation, state stageUpdateState, input *UpdatePipelineStageInput) (UpdatePipelineStageOutput, error) {
	if eval.Decision == CouncilDecisionRequireManualReview {
		summary := strings.TrimSpace(eval.Summary)
		if summary == "" {
			summary = "Council vraagt handmatige beoordeling voordat de stage kan wijzigen."
		}
		if deps.MarkAlertEmitted("council_stage_update", eval.ReasonCode, summary) {
			_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
				LeadID:         state.leadID,
				ServiceID:      &state.serviceID,
				OrganizationID: state.tenantID,
				ActorType:      repository.ActorTypeSystem,
				ActorName:      "Council",
				EventType:      repository.EventTypeAlert,
				Title:          repository.EventTitleManualIntervention,
				Summary:        &summary,
				Metadata: repository.CouncilAdviceMetadata{
					Decision:         eval.Decision,
					ReasonCode:       eval.ReasonCode,
					Summary:          eval.Summary,
					EstimatorSignals: eval.EstimatorSignals,
					RiskSignals:      eval.RiskSignals,
					ReadinessSignals: eval.ReadinessSignals,
				}.ToMap(),
			})
		}
		return UpdatePipelineStageOutput{Success: false, Message: summary}, fmt.Errorf("council blocked stage update: %s", eval.ReasonCode)
	}

	if eval.Decision == CouncilDecisionDowngradeToNurture && input.Stage == domain.PipelineStageEstimation {
		input.Stage = domain.PipelineStageNurturing
		if strings.TrimSpace(input.Reason) == "" {
			if strings.TrimSpace(eval.Summary) != "" {
				input.Reason = eval.Summary
			} else {
				input.Reason = "Council verlaagt stage: aanvullende intake nodig."
			}
		}
	}

	return UpdatePipelineStageOutput{}, nil
}

// stageChangeParams groups parameters for recording a pipeline stage change.
type stageChangeParams struct {
	leadID             uuid.UUID
	serviceID          uuid.UUID
	tenantID           uuid.UUID
	oldStage           string
	newStage           string
	reason             string
	trigger            string
	reasonCode         string
	loopAttemptCount   int
	blockerFingerprint string
	missingInformation []string
}

func recordPipelineStageChange(ctx context.Context, deps *ToolDependencies, p stageChangeParams) {
	actorType, actorName := deps.GetActor()
	reasonText := strings.TrimSpace(p.reason)
	var summary *string
	if reasonText != "" {
		summary = &reasonText
	}

	stageMetadata := repository.StageChangeMetadata{
		OldStage: p.oldStage,
		NewStage: p.newStage,
		RunID:    deps.GetRunID(),
	}.ToMap()
	if analysisMeta := deps.GetLastAnalysisMetadata(); analysisMeta != nil {
		stageMetadata["analysis"] = analysisMeta
	}
	if estimationMeta := deps.GetLastEstimationMetadata(); estimationMeta != nil {
		stageMetadata["estimation"] = estimationMeta
	}
	if councilMeta := deps.GetLastCouncilMetadata(); councilMeta != nil {
		stageMetadata["council"] = councilMeta
	}
	if draftResult := deps.GetLastDraftResult(); draftResult != nil {
		stageMetadata["draftQuote"] = map[string]any{
			"quoteId":     draftResult.QuoteID,
			"quoteNumber": draftResult.QuoteNumber,
			"itemCount":   draftResult.ItemCount,
		}
	}
	if p.trigger == gatekeeperLoopDetectedTrigger {
		stageMetadata["loopDetected"] = repository.LoopDetectedMetadata{
			Trigger:            p.trigger,
			ReasonCode:         p.reasonCode,
			AttemptCount:       p.loopAttemptCount,
			Threshold:          gatekeeperNurturingLoopThreshold,
			BlockerFingerprint: p.blockerFingerprint,
			MissingInformation: p.missingInformation,
		}.ToMap()
	}

	_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         p.leadID,
		ServiceID:      &p.serviceID,
		OrganizationID: p.tenantID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      repository.EventTypeStageChange,
		Title:          repository.EventTitleStageUpdated,
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
			Reason:        reasonText,
			ReasonCode:    p.reasonCode,
			Trigger:       p.trigger,
			ActorType:     actorType,
			ActorName:     actorName,
			RunID:         deps.GetRunID(),
		})
	}

	logReason := reasonText
	if logReason == "" {
		logReason = "(no reason provided)"
	}
	log.Printf("gatekeeper UpdatePipelineStage: leadId=%s serviceId=%s from=%s to=%s reason=%s",
		p.leadID, p.serviceID, p.oldStage, p.newStage, logReason)
}

func createUpdatePipelineStageTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewUpdatePipelineStageTool(func(ctx tool.Context, input UpdatePipelineStageInput) (UpdatePipelineStageOutput, error) {
		return handleUpdatePipelineStage(ctx, GetDependencies(ctx), input)
	})
}

func createSaveAnalysisTool() (tool.Tool, error) {
	return apptools.NewSaveAnalysisTool(func(ctx tool.Context, input SaveAnalysisInput) (SaveAnalysisOutput, error) {
		return handleSaveAnalysis(ctx, GetDependencies(ctx), input)
	})
}

func createUpdateLeadServiceTypeTool() (tool.Tool, error) {
	return apptools.NewUpdateLeadServiceTypeTool(func(ctx tool.Context, input UpdateLeadServiceTypeInput) (UpdateLeadServiceTypeOutput, error) {
		return handleUpdateLeadServiceType(ctx, GetDependencies(ctx), input)
	})
}

func createUpdateLeadDetailsTool(description string) (tool.Tool, error) {
	return apptools.NewUpdateLeadDetailsTool(description, func(ctx tool.Context, input UpdateLeadDetailsInput) (UpdateLeadDetailsOutput, error) {
		return handleUpdateLeadDetails(ctx, GetDependencies(ctx), input)
	})
}

func latestAnalysisInvariantInputs(ctx context.Context, deps *ToolDependencies, serviceID, tenantID uuid.UUID) (string, []string) {
	if analysis, err := deps.Repo.GetLatestAIAnalysis(ctx, serviceID, tenantID); err == nil {
		return analysis.RecommendedAction, analysis.MissingInformation
	}
	analysisMeta := deps.GetLastAnalysisMetadata()
	if analysisMeta == nil {
		return "", nil
	}
	recommendedAction := strings.TrimSpace(fmt.Sprint(analysisMeta["recommendedAction"]))
	return recommendedAction, parseMissingInformationMetadata(analysisMeta["missingInformation"])
}

func parseMissingInformationMetadata(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		return stringifyAnySlice(typed)
	default:
		return nil
	}
}

func stringifyAnySlice(items []any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, strings.TrimSpace(fmt.Sprint(item)))
	}
	return out
}

func createFindMatchingPartnersTool() (tool.Tool, error) {
	return apptools.NewFindMatchingPartnersTool(func(ctx tool.Context, input FindMatchingPartnersInput) (FindMatchingPartnersOutput, error) {
		return handleFindMatchingPartners(ctx, GetDependencies(ctx), input)
	})
}

func createCreatePartnerOfferTool() (tool.Tool, error) {
	return apptools.NewCreatePartnerOfferTool(func(ctx tool.Context, input CreatePartnerOfferInput) (CreatePartnerOfferOutput, error) {
		deps := GetDependencies(ctx)
		if deps.OfferCreator == nil {
			return CreatePartnerOfferOutput{Success: false, Message: "Offer creation not configured"}, fmt.Errorf("offer creator not configured")
		}

		tenantID, serviceID, partnerID, hours, contextMessage, err := resolveOfferContext(deps, input.PartnerID, input.ExpirationHours)
		if err != nil {
			return CreatePartnerOfferOutput{Success: false, Message: contextMessage}, err
		}

		quoteID, err := deps.Repo.GetLatestAcceptedQuoteIDForService(ctx, serviceID, tenantID)
		if err != nil {
			return CreatePartnerOfferOutput{Success: false, Message: "Accepted quote not found for service"}, err
		}

		summary := truncateRunes(strings.TrimSpace(input.JobSummaryShort), 200)
		result, err := deps.OfferCreator.CreateOfferFromQuote(ctx, tenantID, ports.CreateOfferFromQuoteParams{
			PartnerID:         partnerID,
			QuoteID:           quoteID,
			ExpiresInHours:    hours,
			JobSummaryShort:   summary,
			MarginBasisPoints: input.MarginBasisPoints,
			VakmanPriceCents:  input.VakmanPriceCents,
		})
		if err != nil {
			return CreatePartnerOfferOutput{Success: false, Message: err.Error()}, err
		}

		deps.MarkOfferCreated()
		return CreatePartnerOfferOutput{Success: true, Message: "Offer created", OfferID: result.OfferID.String(), PublicToken: result.PublicToken}, nil
	})
}

func handleFindMatchingPartners(ctx tool.Context, deps *ToolDependencies, input FindMatchingPartnersInput) (FindMatchingPartnersOutput, error) {
	tenantID, err := getTenantID(deps)
	if err != nil {
		return FindMatchingPartnersOutput{Matches: nil}, err
	}

	excludeUUIDs := parsePartnerExclusions(input.ExcludePartnerIDs)

	leadID, serviceID, err := getLeadContext(deps)
	if err != nil {
		return FindMatchingPartnersOutput{Matches: nil}, err
	}

	matches, err := deps.Repo.FindMatchingPartners(ctx, tenantID, leadID, input.ServiceType, input.ZipCode, input.RadiusKm, excludeUUIDs)
	if err != nil {
		return FindMatchingPartnersOutput{Matches: nil}, err
	}

	statsByPartner := lookupPartnerOfferStats(ctx, deps, tenantID, matches)
	recordPartnerSearchTimelineEvent(ctx, deps, tenantID, leadID, serviceID, input, len(matches))
	log.Printf("dispatcher FindMatchingPartners: run=%s lead=%s service=%s matches=%d", deps.GetRunID(), leadID, serviceID, len(matches))

	return FindMatchingPartnersOutput{Matches: buildPartnerMatchOutput(matches, statsByPartner)}, nil
}

func parsePartnerExclusions(rawIDs []string) []uuid.UUID {
	excludeUUIDs := make([]uuid.UUID, 0, len(rawIDs))
	for _, idStr := range rawIDs {
		if uid, err := uuid.Parse(idStr); err == nil {
			excludeUUIDs = append(excludeUUIDs, uid)
		}
	}
	return excludeUUIDs
}

func lookupPartnerOfferStats(ctx tool.Context, deps *ToolDependencies, tenantID uuid.UUID, matches []repository.PartnerMatch) map[uuid.UUID]repository.PartnerOfferStats {
	partnerIDs := make([]uuid.UUID, 0, len(matches))
	for _, m := range matches {
		partnerIDs = append(partnerIDs, m.ID)
	}
	if len(partnerIDs) == 0 {
		return map[uuid.UUID]repository.PartnerOfferStats{}
	}

	since := time.Now().AddDate(0, 0, -30)
	statsByPartner, statsErr := deps.Repo.GetPartnerOfferStatsSince(ctx, tenantID, partnerIDs, since)
	if statsErr != nil {
		// Non-fatal: if stats query fails, fall back to distance-only selection.
		log.Printf("FindMatchingPartners: offer stats lookup failed: %v", statsErr)
		return map[uuid.UUID]repository.PartnerOfferStats{}
	}
	return statsByPartner
}

func recordPartnerSearchTimelineEvent(ctx tool.Context, deps *ToolDependencies, tenantID, leadID, serviceID uuid.UUID, input FindMatchingPartnersInput, matchCount int) {
	actorType, actorName := deps.GetActor()
	summary := fmt.Sprintf("Found %d partner(s)", matchCount)
	_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: tenantID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      repository.EventTypePartnerSearch,
		Title:          repository.EventTitlePartnerSearch,
		Summary:        &summary,
		Metadata: repository.PartnerSearchMetadata{
			ServiceType: input.ServiceType,
			ZipCode:     input.ZipCode,
			RadiusKm:    input.RadiusKm,
			MatchCount:  matchCount,
		}.ToMap(),
	})
}

func buildPartnerMatchOutput(matches []repository.PartnerMatch, statsByPartner map[uuid.UUID]repository.PartnerOfferStats) []PartnerMatch {
	output := make([]PartnerMatch, 0, len(matches))
	for _, match := range matches {
		stats := statsByPartner[match.ID]
		output = append(output, PartnerMatch{
			PartnerID:         match.ID.String(),
			BusinessName:      match.BusinessName,
			Email:             match.Email,
			DistanceKm:        match.DistanceKm,
			RejectedOffers30d: stats.Rejected,
			AcceptedOffers30d: stats.Accepted,
			OpenOffers30d:     stats.Open,
		})
	}
	return output
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
		hours = 12
	}
	if hours > 72 {
		hours = 72
	}

	return tenantID, serviceID, partnerID, hours, "", nil
}
func createSaveEstimationTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewSaveEstimationTool(func(ctx tool.Context, input SaveEstimationInput) (SaveEstimationOutput, error) {
		deps := GetDependencies(ctx)
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
			EventType:      repository.EventTypeAnalysis,
			Title:          repository.EventTitleEstimationSaved,
			Summary:        summaryPtr,
			Metadata: repository.EstimationMetadata{
				Scope:      input.Scope,
				PriceRange: input.PriceRange,
				Notes:      input.Notes,
			}.ToMap(),
		})
		if err != nil {
			return SaveEstimationOutput{Success: false, Message: "Failed to save estimation"}, err
		}

		deps.SetLastEstimationMetadata(repository.EstimationMetadata{
			Scope:      input.Scope,
			PriceRange: input.PriceRange,
			Notes:      input.Notes,
		}.ToMap())
		deps.MarkSaveEstimationCalled()

		return SaveEstimationOutput{Success: true, Message: "Estimation saved"}, nil
	})
}

func createCommitScopeArtifactTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewCommitScopeArtifactTool(func(ctx tool.Context, input CommitScopeArtifactInput) (CommitScopeArtifactOutput, error) {
		deps := GetDependencies(ctx)
		artifact := input.Artifact
		if len(artifact.MissingDimensions) > 0 {
			artifact.IsComplete = false
		}
		if artifact.WorkItems == nil {
			artifact.WorkItems = make([]ScopeWorkItem, 0)
		}
		deps.SetScopeArtifact(artifact)
		return CommitScopeArtifactOutput{Success: true, Message: "Scope artifact opgeslagen"}, nil
	})
}

func createAskCustomerClarificationTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewAskCustomerClarificationTool(func(ctx tool.Context, input AskCustomerClarificationInput) (AskCustomerClarificationOutput, error) {
		deps := GetDependencies(ctx)
		tenantID, err := getTenantID(deps)
		if err != nil {
			return AskCustomerClarificationOutput{Success: false, Message: missingTenantContextMessage}, err
		}

		leadID, serviceID, err := getLeadContext(deps)
		if err != nil {
			return AskCustomerClarificationOutput{Success: false, Message: missingLeadContextMessage}, err
		}

		message := strings.TrimSpace(input.Message)
		if message == "" {
			return AskCustomerClarificationOutput{Success: false, Message: "Bericht is verplicht"}, fmt.Errorf("empty clarification message")
		}

		if len([]rune(message)) > 1200 {
			message = truncateRunes(message, 1200)
		}

		actorType, actorName := deps.GetActor()
		_, err = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
			LeadID:         leadID,
			ServiceID:      &serviceID,
			OrganizationID: tenantID,
			ActorType:      actorType,
			ActorName:      actorName,
			EventType:      repository.EventTypeNote,
			Title:          repository.EventTitleNoteAdded,
			Summary:        &message,
			Metadata: map[string]any{
				"noteType":          "ai_clarification_request",
				"missingDimensions": input.MissingDimensions,
			},
		})
		if err != nil {
			return AskCustomerClarificationOutput{Success: false, Message: "Kon verduidelijkingsvraag niet opslaan"}, err
		}

		deps.MarkClarificationAsked()
		log.Printf("estimator AskCustomerClarification: run=%s lead=%s service=%s missing=%d", deps.GetRunID(), leadID, serviceID, len(input.MissingDimensions))
		return AskCustomerClarificationOutput{Success: true, Message: "Verduidelijkingsvraag opgeslagen"}, nil
	})
}

// handleCalculator evaluates a single arithmetic operation deterministically.
// The LLM MUST call this for ANY math instead of doing it in its head.
func handleCalculator(_ tool.Context, input CalculatorInput) (CalculatorOutput, error) {
	expression := strings.TrimSpace(input.Expression)
	if expression != "" {
		result, err := evaluateCalculatorExpression(expression)
		if err != nil {
			return CalculatorOutput{}, err
		}
		return CalculatorOutput{
			Result:     result,
			Expression: fmt.Sprintf("%s = %g", expression, result),
		}, nil
	}

	return evaluateLegacyCalculatorOperation(input)
}

func evaluateLegacyCalculatorOperation(input CalculatorInput) (CalculatorOutput, error) {
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
			return CalculatorOutput{}, errors.New(divisionByZeroMessage)
		}
		result = input.A / input.B
		expr = fmt.Sprintf("%g ÷ %g = %g", input.A, input.B, result)
	case "ceil_divide":
		if input.B == 0 {
			return CalculatorOutput{}, errors.New(divisionByZeroMessage)
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

func evaluateCalculatorExpression(raw string) (float64, error) {
	expr, err := parser.ParseExpr(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid expression: %w", err)
	}

	result, err := evaluateCalculatorAST(expr)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return 0, fmt.Errorf("expression result must be finite")
	}
	return result, nil
}

func evaluateCalculatorAST(expr ast.Expr) (float64, error) {
	switch node := expr.(type) {
	case *ast.BasicLit:
		return parseCalculatorLiteral(node)
	case *ast.ParenExpr:
		return evaluateCalculatorAST(node.X)
	case *ast.UnaryExpr:
		return evaluateCalculatorUnary(node)
	case *ast.BinaryExpr:
		return evaluateCalculatorBinary(node)
	case *ast.CallExpr:
		return evaluateCalculatorCall(node)
	default:
		return 0, fmt.Errorf("unsupported expression element %T", expr)
	}
}

func parseCalculatorLiteral(node *ast.BasicLit) (float64, error) {
	if node.Kind != token.INT && node.Kind != token.FLOAT {
		return 0, fmt.Errorf("unsupported literal %q", node.Value)
	}
	value, err := strconv.ParseFloat(node.Value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric literal %q", node.Value)
	}
	return value, nil
}

func evaluateCalculatorUnary(node *ast.UnaryExpr) (float64, error) {
	value, err := evaluateCalculatorAST(node.X)
	if err != nil {
		return 0, err
	}
	if node.Op == token.ADD {
		return value, nil
	}
	if node.Op == token.SUB {
		return -value, nil
	}
	return 0, fmt.Errorf("unsupported unary operator %q", node.Op)
}

func evaluateCalculatorBinary(node *ast.BinaryExpr) (float64, error) {
	left, err := evaluateCalculatorAST(node.X)
	if err != nil {
		return 0, err
	}
	right, err := evaluateCalculatorAST(node.Y)
	if err != nil {
		return 0, err
	}

	switch node.Op {
	case token.ADD:
		return left + right, nil
	case token.SUB:
		return left - right, nil
	case token.MUL:
		return left * right, nil
	case token.QUO:
		if right == 0 {
			return 0, errors.New(divisionByZeroMessage)
		}
		return left / right, nil
	default:
		return 0, fmt.Errorf("unsupported operator %q", node.Op)
	}
}

func evaluateCalculatorCall(node *ast.CallExpr) (float64, error) {
	name, ok := node.Fun.(*ast.Ident)
	if !ok {
		return 0, fmt.Errorf("unsupported function call")
	}

	args, err := evaluateCalculatorArgs(node.Args)
	if err != nil {
		return 0, err
	}

	return evaluateCalculatorFunction(name.Name, args)
}

func evaluateCalculatorArgs(expressions []ast.Expr) ([]float64, error) {
	args := make([]float64, 0, len(expressions))
	for _, expression := range expressions {
		value, err := evaluateCalculatorAST(expression)
		if err != nil {
			return nil, err
		}
		args = append(args, value)
	}
	return args, nil
}

func evaluateCalculatorFunction(name string, args []float64) (float64, error) {
	handler, ok := map[string]func([]float64) (float64, error){
		"ceil":        calculatorCeil,
		"floor":       calculatorFloor,
		"round":       calculatorRound,
		"percentage":  calculatorPercentage,
		"ceil_divide": calculatorCeilDivide,
	}[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return 0, fmt.Errorf("unsupported function %q; use ceil, floor, round, percentage, ceil_divide", name)
	}
	return handler(args)
}

func calculatorCeil(args []float64) (float64, error) {
	if err := requireCalculatorArgCount("ceil", args, 1); err != nil {
		return 0, err
	}
	return math.Ceil(args[0]), nil
}

func calculatorFloor(args []float64) (float64, error) {
	if err := requireCalculatorArgCount("floor", args, 1); err != nil {
		return 0, err
	}
	return math.Floor(args[0]), nil
}

func calculatorRound(args []float64) (float64, error) {
	if len(args) == 1 {
		return math.Round(args[0]), nil
	}
	if err := requireCalculatorArgCountRange("round", args, 1, 2); err != nil {
		return 0, err
	}
	places := args[1]
	if places != math.Trunc(places) {
		return 0, fmt.Errorf("round decimal places must be an integer")
	}
	if places < 0 {
		places = 0
	}
	if places > 10 {
		places = 10
	}
	factor := math.Pow(10, places)
	return math.Round(args[0]*factor) / factor, nil
}

func calculatorPercentage(args []float64) (float64, error) {
	if err := requireCalculatorArgCount("percentage", args, 2); err != nil {
		return 0, err
	}
	return args[0] * args[1] / 100, nil
}

func calculatorCeilDivide(args []float64) (float64, error) {
	if err := requireCalculatorArgCount("ceil_divide", args, 2); err != nil {
		return 0, err
	}
	if args[1] == 0 {
		return 0, errors.New(divisionByZeroMessage)
	}
	return math.Ceil(args[0] / args[1]), nil
}

func requireCalculatorArgCount(name string, args []float64, want int) error {
	if len(args) != want {
		return fmt.Errorf("%s expects %d argument", name, want)
	}
	return nil
}

func requireCalculatorArgCountRange(name string, args []float64, min int, max int) error {
	if len(args) < min || len(args) > max {
		return fmt.Errorf("%s expects %d or %d arguments", name, min, max)
	}
	return nil
}

func createCalculatorTool() (tool.Tool, error) {
	return apptools.NewCalculatorTool(`Performs exact arithmetic. You MUST use this for ANY calculation and never do math in your head.
Preferred input:
	expression      -> one full arithmetic expression using +, -, *, /, parentheses,
										and helper functions ceil(...), floor(...), round(...), percentage(...), ceil_divide(...)
Legacy input remains supported:
	operation + a/b -> add, subtract, multiply, divide, ceil_divide, ceil, floor, round, percentage
Examples:
	Window area 2m x 1.5m: Calculator(expression="2 * 1.5") -> 3
	Sheets needed: Calculator(expression="ceil_divide(4, 2.5)") -> 2
	Material subtotal plus VAT: Calculator(expression="((15.99 * 3) + (12.50 * 2)) * 1.21") -> 88.2937
	Material subtotal plus VAT plus 10%% markup: Calculator(expression="(((15.99 * 3) + (12.50 * 2)) * 1.21) * 1.10") -> 97.12307`, handleCalculator)
}

// MaxSafeUnitPrice is the ceiling for a single material item (€5M).
const MaxSafeUnitPrice = 5_000_000.00

func createCalculateEstimateTool() (tool.Tool, error) {
	return apptools.NewCalculateEstimateTool(func(ctx tool.Context, input CalculateEstimateInput) (CalculateEstimateOutput, error) {
		deps := GetDependencies(ctx)
		if err := validateCalculateEstimateInput(input); err != nil {
			return CalculateEstimateOutput{}, err
		}

		materialCents := calculateMaterialSubtotalCents(input)
		laborLowCents, laborHighCents := calculateLaborSubtotalRangeCents(input)
		extraCents := int64(math.Round(input.ExtraCosts * 100))
		deps.SetLastEstimateSnapshot(EstimateComputationSnapshot{
			MaterialSubtotalCents:  materialCents,
			LaborSubtotalLowCents:  laborLowCents,
			LaborSubtotalHighCents: laborHighCents,
			TotalLowCents:          materialCents + laborLowCents + extraCents,
			TotalHighCents:         materialCents + laborHighCents + extraCents,
			ExtraCostsCents:        extraCents,
		})

		return buildCalculateEstimateOutput(materialCents, laborLowCents, laborHighCents, extraCents), nil
	})
}

func validateCalculateEstimateInput(input CalculateEstimateInput) error {
	// Reject negative financial inputs (AI hallucination guard).
	if input.LaborHoursLow < 0 || input.LaborHoursHigh < 0 ||
		input.HourlyRateLow < 0 || input.HourlyRateHigh < 0 ||
		input.ExtraCosts < 0 {
		return fmt.Errorf("financial inputs cannot be negative")
	}

	for _, item := range input.MaterialItems {
		if item.UnitPrice < 0 || item.Quantity < 0 {
			return fmt.Errorf("material item price and quantity cannot be negative")
		}
		if item.UnitPrice > MaxSafeUnitPrice {
			return fmt.Errorf("unitPrice %.2f exceeds safety limit of %.2f", item.UnitPrice, MaxSafeUnitPrice)
		}
	}

	return nil
}

func calculateMaterialSubtotalCents(input CalculateEstimateInput) int64 {
	// Calculate using integer cents to avoid IEEE 754 precision loss.
	var materialCents int64
	for _, item := range input.MaterialItems {
		if item.UnitPrice <= 0 || item.Quantity <= 0 {
			continue
		}
		unitCents := int64(math.Round(item.UnitPrice * 100))
		lineCents := int64(math.Round(float64(unitCents) * item.Quantity))
		materialCents += lineCents
	}
	return materialCents
}

func calculateLaborSubtotalRangeCents(input CalculateEstimateInput) (int64, int64) {
	laborLowCents := int64(math.Round(input.LaborHoursLow * input.HourlyRateLow * 100))
	laborHighCents := int64(math.Round(input.LaborHoursHigh * input.HourlyRateHigh * 100))
	if laborHighCents < laborLowCents {
		laborLowCents, laborHighCents = laborHighCents, laborLowCents
	}
	return laborLowCents, laborHighCents
}

func buildCalculateEstimateOutput(materialCents, laborLowCents, laborHighCents, extraCents int64) CalculateEstimateOutput {
	return CalculateEstimateOutput{
		MaterialSubtotal:  centsToEuro(materialCents),
		LaborSubtotalLow:  centsToEuro(laborLowCents),
		LaborSubtotalHigh: centsToEuro(laborHighCents),
		TotalLow:          centsToEuro(materialCents + laborLowCents + extraCents),
		TotalHigh:         centsToEuro(materialCents + laborHighCents + extraCents),
		AppliedExtraCosts: centsToEuro(extraCents),
	}
}

func centsToEuro(cents int64) float64 {
	return math.Round(float64(cents)) / 100.0
}

// defaultSearchScoreThreshold is the minimum cosine similarity score for
// BGE-M3 embeddings. It controls recall (what enters candidate set).
const defaultSearchScoreThreshold = 0.35
const maxCatalogRewordRetries = 2

// noMatchMessage builds the "no relevant products" message for a query.
func noMatchMessage(query string) string {
	return fmt.Sprintf("No relevant products found for query '%s'. Try different search terms (synonyms, broader/narrower terms, Dutch and English). If no match exists, you may add an ad-hoc item.", query)
}

func recordCatalogSearch(ctx context.Context, deps *ToolDependencies, query string, collection string, resultCount int, topScore *float64) {
	tenantID, ok := deps.GetTenantID()
	if !ok || tenantID == nil {
		return
	}

	_, serviceID, hasLeadCtx := deps.GetLeadContext()
	var servicePtr *uuid.UUID
	if hasLeadCtx {
		sid := serviceID
		servicePtr = &sid
	}

	if deps.Repo == nil {
		return
	}
	runID := strings.TrimSpace(deps.GetRunID())
	var runIDPtr *string
	if runID != "" {
		runIDPtr = &runID
	}
	_, actorName := deps.GetActor()
	actorName = strings.TrimSpace(actorName)
	var agentNamePtr *string
	if actorName != "" {
		agentNamePtr = &actorName
	}
	toolName := "SearchProductMaterials"
	if err := deps.Repo.CreateCatalogSearchLog(ctx, repository.CreateCatalogSearchLogParams{
		OrganizationID: *tenantID,
		LeadServiceID:  servicePtr,
		RunID:          runIDPtr,
		ToolName:       &toolName,
		AgentName:      agentNamePtr,
		Query:          query,
		Collection:     collection,
		ResultCount:    resultCount,
		TopScore:       topScore,
	}); err != nil {
		log.Printf("SearchProductMaterials: failed to write catalog search log: %v", err)
	}
}

type ListCatalogGapsInput struct {
	// LookbackDays defaults to organization setting catalogGapLookbackDays.
	LookbackDays *int `json:"lookbackDays,omitempty"`
	// MinCount defaults to organization setting catalogGapThreshold.
	MinCount *int `json:"minCount,omitempty"`
	// Limit defaults to 25.
	Limit *int `json:"limit,omitempty"`
}

type CatalogSearchMissSummaryDTO struct {
	Query       string    `json:"query"`
	SearchCount int       `json:"searchCount"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
	Collections []string  `json:"collections"`
}

type AdHocQuoteItemSummaryDTO struct {
	Description string    `json:"description"`
	UseCount    int       `json:"useCount"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
}

type ListCatalogGapsOutput struct {
	LookbackDays    int                           `json:"lookbackDays"`
	MinCount        int                           `json:"minCount"`
	SearchMisses    []CatalogSearchMissSummaryDTO `json:"searchMisses"`
	AdHocQuoteItems []AdHocQuoteItemSummaryDTO    `json:"adHocQuoteItems"`
	Message         string                        `json:"message,omitempty"`
}

type listCatalogGapsParams struct {
	lookbackDays int
	minCount     int
	limit        int
}

func resolveOptionalIntWithin(defaultVal int, override *int, minVal int, maxVal int) int {
	val := defaultVal
	if override != nil {
		val = *override
	}
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

func resolveListCatalogGapsParams(settings ports.OrganizationAISettings, input ListCatalogGapsInput) listCatalogGapsParams {
	lookbackDays := resolveOptionalIntWithin(settings.CatalogGapLookbackDays, input.LookbackDays, 1, 365)
	minCount := resolveOptionalIntWithin(settings.CatalogGapThreshold, input.MinCount, 1, 1000)

	limit := 25
	if input.Limit != nil {
		limit = normalizeLimit(*input.Limit, 25, 100)
	}

	return listCatalogGapsParams{lookbackDays: lookbackDays, minCount: minCount, limit: limit}
}

func mapCatalogSearchMissSummaries(misses []repository.CatalogSearchMissSummary) []CatalogSearchMissSummaryDTO {
	out := make([]CatalogSearchMissSummaryDTO, 0, len(misses))
	for _, m := range misses {
		out = append(out, CatalogSearchMissSummaryDTO{
			Query:       m.Query,
			SearchCount: m.SearchCount,
			LastSeenAt:  m.LastSeenAt,
			Collections: m.Collections,
		})
	}
	return out
}

func mapAdHocQuoteItemSummaries(items []repository.AdHocQuoteItemSummary) []AdHocQuoteItemSummaryDTO {
	out := make([]AdHocQuoteItemSummaryDTO, 0, len(items))
	for _, it := range items {
		out = append(out, AdHocQuoteItemSummaryDTO{
			Description: it.Description,
			UseCount:    it.UseCount,
			LastSeenAt:  it.LastSeenAt,
		})
	}
	return out
}

func buildListCatalogGapsOutput(params listCatalogGapsParams, misses []repository.CatalogSearchMissSummary, adHoc []repository.AdHocQuoteItemSummary) ListCatalogGapsOutput {
	out := ListCatalogGapsOutput{
		LookbackDays:    params.lookbackDays,
		MinCount:        params.minCount,
		SearchMisses:    mapCatalogSearchMissSummaries(misses),
		AdHocQuoteItems: mapAdHocQuoteItemSummaries(adHoc),
	}

	if len(out.SearchMisses) == 0 && len(out.AdHocQuoteItems) == 0 {
		out.Message = "No frequent catalog gaps detected in the selected lookback window."
	}

	return out
}

func listCatalogGapsErrorOutput(params listCatalogGapsParams, message string) ListCatalogGapsOutput {
	return ListCatalogGapsOutput{LookbackDays: params.lookbackDays, MinCount: params.minCount, Message: message}
}

func handleListCatalogGaps(ctx tool.Context, deps *ToolDependencies, input ListCatalogGapsInput) (ListCatalogGapsOutput, error) {
	tenantID, ok := deps.GetTenantID()
	if !ok || tenantID == nil {
		return ListCatalogGapsOutput{Message: missingTenantContextMessage}, nil
	}
	if deps.Repo == nil {
		return ListCatalogGapsOutput{Message: "Repository not configured"}, nil
	}

	params := resolveListCatalogGapsParams(deps.GetOrganizationAISettingsOrDefault(), input)

	misses, err := deps.Repo.ListFrequentCatalogSearchMisses(ctx, *tenantID, params.lookbackDays, params.minCount, params.limit)
	if err != nil {
		log.Printf("ListCatalogGaps: failed to list catalog search misses: %v", err)
		return listCatalogGapsErrorOutput(params, "Failed to load catalog search misses"), nil
	}

	adHoc, err := deps.Repo.ListFrequentAdHocQuoteItems(ctx, *tenantID, params.lookbackDays, params.minCount, params.limit)
	if err != nil {
		log.Printf("ListCatalogGaps: failed to list ad-hoc quote items: %v", err)
		return listCatalogGapsErrorOutput(params, "Failed to load ad-hoc quote items"), nil
	}

	return buildListCatalogGapsOutput(params, misses, adHoc), nil
}

func createListCatalogGapsTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewListCatalogGapsTool(func(ctx tool.Context, input ListCatalogGapsInput) (ListCatalogGapsOutput, error) {
		return handleListCatalogGaps(ctx, GetDependencies(ctx), input)
	})
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
	tenantID, tenantOk := deps.GetTenantID()
	var filter *qdrant.Filter
	if tenantOk && tenantID != nil {
		filter = qdrant.NewOrganizationFilter(tenantID.String())
		log.Printf("SearchProductMaterials: catalog search with tenant filter organization_id=%s", tenantID.String())
	} else {
		log.Printf("SearchProductMaterials: catalog search without tenant filter (missing tenant context)")
	}

	results, err := deps.CatalogQdrantClient.SearchWithFilter(ctx, vector, limit, scoreThreshold, filter)
	if err != nil {
		log.Printf("SearchProductMaterials: catalog search failed: %v", err)
		recordCatalogSearch(ctx, deps, query, "catalog", 0, nil)
		return nil
	}
	var topScore *float64
	if len(results) > 0 {
		s := results[0].Score
		topScore = &s
	}
	products := convertSearchResults(results)
	recordCatalogSearch(ctx, deps, query, "catalog", len(products), topScore)
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

	cacheKey := fmt.Sprintf("%s|%d|%t|%.4f", strings.ToLower(strings.TrimSpace(query)), limit, useCatalog, scoreThreshold)
	if cached, ok := deps.getSearchCache(cacheKey); ok {
		log.Printf("SearchProductMaterials: cache hit query=%q limit=%d useCatalog=%t minScore=%.2f", query, limit, useCatalog, scoreThreshold)
		return cached, nil
	}

	vector, err := deps.EmbeddingClient.Embed(ctx, query)
	if err != nil {
		log.Printf("SearchProductMaterials: embedding failed: %v", err)
		return SearchProductMaterialsOutput{Products: nil, Message: "Failed to generate embedding for query"}, err
	}

	catalogOutput, foundInCatalog := tryCatalogSearchFlow(ctx, deps, query, limit, scoreThreshold, useCatalog, vector)
	if foundInCatalog && hasStrongCatalogMatch(catalogOutput.Products) {
		deps.setSearchCache(cacheKey, catalogOutput)
		return catalogOutput, nil
	}

	fallbackOutput, fallbackErr := searchFallbackReferenceCollections(ctx, deps, query, vector, limit, scoreThreshold)
	if fallbackErr != nil {
		if foundInCatalog && len(catalogOutput.Products) > 0 {
			log.Printf("SearchProductMaterials: fallback search failed, returning catalog-only low-confidence results: %v", fallbackErr)
			deps.setSearchCache(cacheKey, catalogOutput)
			return catalogOutput, nil
		}
		return fallbackOutput, fallbackErr
	}

	if foundInCatalog && len(catalogOutput.Products) > 0 {
		if len(fallbackOutput.Products) == 0 {
			deps.setSearchCache(cacheKey, catalogOutput)
			return catalogOutput, nil
		}

		log.Printf("SearchProductMaterials: catalog had no high-confidence matches, adding fallback collections")
		combinedOutput := combineCatalogAndFallbackResults(catalogOutput, fallbackOutput, query, scoreThreshold, limit)
		deps.setSearchCache(cacheKey, combinedOutput)
		return combinedOutput, nil
	}

	deps.setSearchCache(cacheKey, fallbackOutput)
	return fallbackOutput, nil
}

func searchFallbackReferenceCollections(ctx tool.Context, deps *ToolDependencies, query string, vector []float32, limit int, scoreThreshold float64) (SearchProductMaterialsOutput, error) {

	// Fallback to reference collections.
	if deps.QdrantClient == nil && deps.BouwmaatQdrantClient == nil {
		return SearchProductMaterialsOutput{Products: nil, Message: noMatchMessage(query)}, nil
	}

	batchClient := resolveFallbackBatchClient(deps)
	batchRequests, requestCollections := buildFallbackBatchRequests(deps, vector, limit, scoreThreshold)

	batchResults, err := batchClient.BatchSearch(ctx, batchRequests)
	if err != nil {
		log.Printf("SearchProductMaterials: fallback batch search failed: %v", err)
		return SearchProductMaterialsOutput{Products: nil, Message: "Failed to search product catalog"}, err
	}

	products := flattenFallbackBatchResults(ctx, deps, query, batchResults, requestCollections, limit)
	return buildFallbackSearchOutput(query, products, requestCollections, scoreThreshold), nil
}

func resolveFallbackBatchClient(deps *ToolDependencies) *qdrant.Client {
	if deps.QdrantClient != nil {
		return deps.QdrantClient
	}
	return deps.BouwmaatQdrantClient
}

func buildFallbackBatchRequests(deps *ToolDependencies, vector []float32, limit int, scoreThreshold float64) ([]qdrant.SearchRequest, []string) {
	batchRequests := make([]qdrant.SearchRequest, 0, 2)
	requestCollections := make([]string, 0, 2)

	if deps.QdrantClient != nil {
		houthandelCollection := deps.QdrantClient.CollectionName()
		if houthandelCollection == "" {
			houthandelCollection = defaultHouthandelCollection
		}
		batchRequests = append(batchRequests, newFallbackBatchRequest(houthandelCollection, vector, limit, scoreThreshold))
		requestCollections = append(requestCollections, houthandelCollection)
	}

	if deps.BouwmaatQdrantClient != nil {
		bouwmaatCollection := deps.BouwmaatQdrantClient.CollectionName()
		if bouwmaatCollection == "" {
			bouwmaatCollection = defaultBouwmaatCollection
		}
		batchRequests = append(batchRequests, newFallbackBatchRequest(bouwmaatCollection, vector, limit, scoreThreshold))
		requestCollections = append(requestCollections, bouwmaatCollection)
	}

	return batchRequests, requestCollections
}

func newFallbackBatchRequest(collectionName string, vector []float32, limit int, scoreThreshold float64) qdrant.SearchRequest {
	return qdrant.SearchRequest{
		CollectionName: collectionName,
		Vector:         vector,
		Limit:          limit,
		WithPayload:    true,
		ScoreThreshold: &scoreThreshold,
	}
}

func flattenFallbackBatchResults(ctx tool.Context, deps *ToolDependencies, query string, batchResults [][]qdrant.SearchResult, requestCollections []string, limit int) []ProductResult {
	products := make([]ProductResult, 0, limit*len(batchResults))
	for idx, results := range batchResults {
		collectionName := "unknown"
		if idx < len(requestCollections) {
			collectionName = requestCollections[idx]
		}
		var topScore *float64
		if len(results) > 0 {
			s := results[0].Score
			topScore = &s
		}
		collectionProducts := convertSearchResults(results)
		recordCatalogSearch(ctx, deps, query, collectionName, len(collectionProducts), topScore)
		for i := range collectionProducts {
			collectionProducts[i].SourceCollection = collectionName
		}
		products = append(products, collectionProducts...)
		log.Printf("SearchProductMaterials: fallback batch query=%q collection=%s results=%d", query, collectionName, len(collectionProducts))
	}

	sort.SliceStable(products, func(i, j int) bool {
		if products[i].Score == products[j].Score {
			return products[i].PriceEuros < products[j].PriceEuros
		}
		return products[i].Score > products[j].Score
	})

	return products
}

func buildFallbackSearchOutput(query string, products []ProductResult, requestCollections []string, scoreThreshold float64) SearchProductMaterialsOutput {
	markHighConfidence(products)
	if len(products) == 0 {
		log.Printf("SearchProductMaterials: fallback batch query=%q found 0 products above threshold %.2f", query, scoreThreshold)
		return SearchProductMaterialsOutput{Products: nil, Message: noMatchMessage(query)}
	}

	// Fallback results are scraped reference data — strip IDs so the AI
	// treats them as ad-hoc line items (no catalogProductId, no auto-attachments).
	stripProductIDs(products)

	log.Printf("SearchProductMaterials: fallback batch query=%q found %d reference products across %d collections (threshold=%.2f, scores: %s)",
		query, len(products), len(requestCollections), scoreThreshold, formatScores(products))

	log.Printf("SearchProductMaterials: fallback collections=%s", strings.Join(requestCollections, ","))

	return SearchProductMaterialsOutput{
		Products: products,
		Message:  fmt.Sprintf("Found %d reference products (not from your catalog — use as ad-hoc line items without catalogProductId, min relevance %.0f%%)", len(products), scoreThreshold*100),
	}
}

func combineCatalogAndFallbackResults(catalogOutput SearchProductMaterialsOutput, fallbackOutput SearchProductMaterialsOutput, query string, scoreThreshold float64, limit int) SearchProductMaterialsOutput {
	products := make([]ProductResult, 0, len(catalogOutput.Products)+len(fallbackOutput.Products))
	products = append(products, catalogOutput.Products...)
	products = append(products, fallbackOutput.Products...)

	sort.SliceStable(products, func(i, j int) bool {
		if products[i].Score == products[j].Score {
			return products[i].PriceEuros < products[j].PriceEuros
		}
		return products[i].Score > products[j].Score
	})

	if len(products) > limit {
		products = products[:limit]
	}

	catalogCount := len(catalogOutput.Products)
	fallbackCount := len(fallbackOutput.Products)

	log.Printf("SearchProductMaterials: combined query=%q catalog=%d fallback=%d total=%d (threshold=%.2f)",
		query, catalogCount, fallbackCount, len(products), scoreThreshold)

	return SearchProductMaterialsOutput{
		Products: products,
		Message: fmt.Sprintf(
			"Found %d products: %d catalog + %d fallback references (catalog is lower confidence; verify variant/unit before drafting, min relevance %.0f%%)",
			len(products),
			catalogCount,
			fallbackCount,
			scoreThreshold*100,
		),
	}
}

func tryCatalogSearchFlow(ctx tool.Context, deps *ToolDependencies, query string, limit int, scoreThreshold float64, useCatalog bool, initialVector []float32) (SearchProductMaterialsOutput, bool) {
	if !useCatalog || deps.CatalogQdrantClient == nil {
		return SearchProductMaterialsOutput{}, false
	}

	initialProducts := searchCatalogCollection(ctx, deps, initialVector, limit, scoreThreshold, query)
	if len(initialProducts) > 0 {
		if hasHighConfidenceMatch(initialProducts) {
			// Original query produced a genuine high-confidence match — authoritative.
			return catalogSearchOutput(initialProducts, scoreThreshold, false, true), true
		}

		bestProducts, highConfidenceProducts, _ := runCatalogRetries(ctx, deps, query, limit, scoreThreshold, initialProducts)
		if len(highConfidenceProducts) > 0 {
			// Retry improved confidence but the original query did NOT have high
			// confidence. Return products but mark highConfidence=false so the
			// caller still tries fallback reference collections.
			return catalogSearchOutput(highConfidenceProducts, scoreThreshold, true, false), true
		}
		return catalogSearchOutput(bestProducts, scoreThreshold, false, false), true
	}

	bestRetryProducts, highConfidenceProducts, _ := runCatalogRetries(ctx, deps, query, limit, scoreThreshold, nil)
	if len(highConfidenceProducts) > 0 {
		// Only retries found something — not authoritative enough to skip fallback.
		return catalogSearchOutput(highConfidenceProducts, scoreThreshold, true, false), true
	}
	if len(bestRetryProducts) > 0 {
		return catalogSearchOutput(bestRetryProducts, scoreThreshold, true, false), true
	}

	return SearchProductMaterialsOutput{}, false
}

func catalogSearchOutput(products []ProductResult, scoreThreshold float64, reworded bool, highConfidence bool) SearchProductMaterialsOutput {
	if highConfidence {
		if reworded {
			return SearchProductMaterialsOutput{
				Products: products,
				Message:  fmt.Sprintf("Found %d high-confidence matching products from catalog after query rewording (min relevance %.0f%%)", len(products), scoreThreshold*100),
			}
		}
		return SearchProductMaterialsOutput{
			Products: products,
			Message:  fmt.Sprintf("Found %d high-confidence matching products from catalog (min relevance %.0f%%)", len(products), scoreThreshold*100),
		}
	}

	if reworded {
		return SearchProductMaterialsOutput{
			Products: products,
			Message:  fmt.Sprintf("Found %d matching products from catalog after query rewording (lower confidence; verify variant/unit, min relevance %.0f%%)", len(products), scoreThreshold*100),
		}
	}

	return SearchProductMaterialsOutput{
		Products: products,
		Message:  fmt.Sprintf("Found %d matching products from catalog (lower confidence; verify variant/unit, min relevance %.0f%%)", len(products), scoreThreshold*100),
	}
}

func limitedCatalogRewordedQueries(query string) []string {
	rewordedQueries := buildCatalogRewordedQueries(query)
	if len(rewordedQueries) > maxCatalogRewordRetries {
		return rewordedQueries[:maxCatalogRewordRetries]
	}
	return rewordedQueries
}

func runCatalogRetries(ctx tool.Context, deps *ToolDependencies, query string, limit int, scoreThreshold float64, currentBest []ProductResult) (bestProducts []ProductResult, highConfidenceProducts []ProductResult, usedRewording bool) {
	bestProducts = currentBest
	for _, retryQuery := range limitedCatalogRewordedQueries(query) {
		retryProducts := searchCatalogRetryQuery(ctx, deps, retryQuery, limit, scoreThreshold)
		if len(retryProducts) == 0 {
			continue
		}

		usedRewording = true
		if shouldPreferCandidateSet(retryProducts, bestProducts) {
			bestProducts = retryProducts
		}

		if hasHighConfidenceMatch(retryProducts) {
			log.Printf("SearchProductMaterials: catalog retry improved confidence query=%q -> retry_query=%q", query, retryQuery)
			return bestProducts, retryProducts, true
		}
	}

	return bestProducts, nil, usedRewording
}

func searchCatalogRetryQuery(ctx tool.Context, deps *ToolDependencies, retryQuery string, limit int, scoreThreshold float64) []ProductResult {
	retryVector, retryErr := deps.EmbeddingClient.Embed(ctx, retryQuery)
	if retryErr != nil {
		log.Printf("SearchProductMaterials: catalog retry embedding failed query=%q: %v", retryQuery, retryErr)
		return nil
	}
	return searchCatalogCollection(ctx, deps, retryVector, limit, scoreThreshold, retryQuery)
}

func hasHighConfidenceMatch(products []ProductResult) bool {
	for _, product := range products {
		if product.HighConfidence {
			return true
		}
	}
	return false
}

func hasStrongCatalogMatch(products []ProductResult) bool {
	for _, product := range products {
		if product.Score >= catalogEarlyReturnScoreThreshold {
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

	without := strings.ReplaceAll(base, " inclusief ", " ")
	if without != base {
		appendUniqueQuery(without)
	}

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
		return r != '-' && r != '+' && r != '.' && r != '/' && r != 'x' && (r < '0' || r > '9') && (r < 'a' || r > 'z')
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
		return r != '-' && r != 'x' && r != '/' && r != '.' && (r < '0' || r > '9') && (r < 'a' || r > 'z')
	}) {
		token = strings.TrimSpace(token)
		if isDimensionToken(token) {
			tokens[token] = struct{}{}
		}
	}
	return tokens
}

func isDimensionToken(token string) bool {
	if token == "" {
		return false
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

	return hasDigit && hasSeparator
}

// unitLookup is the set of recognised unit tokens for exact matching.
var unitLookup = map[string]bool{
	"m1": true, "m2": true, "m3": true,
	"stuk": true, "stuks": true,
	"liter": true, "l": true,
	"cm": true, "mm": true,
	"meter": true, "per": true,
}

func extractUnitTokens(value string) map[string]struct{} {
	units := map[string]struct{}{}
	tokens := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	for _, token := range tokens {
		if unitLookup[token] {
			units[token] = struct{}{}
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
	tokens := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	for _, token := range tokens {
		if _, ok := units[token]; ok {
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

	// Safety: only keep results that resolve to a non-draft catalog product.
	// The CatalogReader adapter omits unknown IDs and draft products.
	resolved := make(map[string]ports.CatalogProductDetails, len(details))
	for _, d := range details {
		resolved[d.ID.String()] = d
	}
	if len(resolved) == 0 {
		return nil
	}

	filtered := make([]ProductResult, 0, len(products))
	for _, p := range products {
		if p.ID == "" {
			continue
		}
		if _, ok := resolved[p.ID]; !ok {
			continue
		}
		filtered = append(filtered, p)
	}

	return applyProductDetails(filtered, details)
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

func createSearchProductMaterialsTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewSearchProductMaterialsTool(func(ctx tool.Context, input SearchProductMaterialsInput) (SearchProductMaterialsOutput, error) {
		return handleSearchProductMaterials(ctx, GetDependencies(ctx), input)
	})
}

// handleDraftQuote creates a draft quote via the QuoteDrafter port.
func handleDraftQuote(ctx tool.Context, deps *ToolDependencies, input DraftQuoteInput) (DraftQuoteOutput, error) {
	if deps.QuoteDrafter == nil {
		return DraftQuoteOutput{Success: false, Message: "Quote drafting is not configured"}, nil
	}

	tenantID, ok := deps.GetTenantID()
	if !ok || tenantID == nil {
		return DraftQuoteOutput{Success: false, Message: "Organization context not available"}, errors.New(missingTenantContextError)
	}

	leadID, serviceID, ok := deps.GetLeadContext()
	if !ok {
		return DraftQuoteOutput{Success: false, Message: "Lead context not available"}, errors.New(missingLeadContextError)
	}

	if !deps.ShouldForceDraftQuote() {
		if blocked, reason := shouldBlockDraftQuoteForInsufficientIntake(ctx, deps, serviceID, *tenantID); blocked {
			log.Printf("DraftQuote: blocked run=%s service=%s reason=%s", deps.GetRunID(), serviceID, reason)
			return DraftQuoteOutput{Success: false, Message: "Onvoldoende intakegegevens voor een betrouwbare conceptofferte"}, fmt.Errorf("draft quote blocked: %s", reason)
		}
	} else {
		log.Printf("DraftQuote: intake guard bypass enabled run=%s service=%s", deps.GetRunID(), serviceID)
	}

	if len(input.Items) == 0 {
		return DraftQuoteOutput{Success: false, Message: "At least one item is required"}, fmt.Errorf("empty items")
	}

	normalizedInput, quantityCorrections := normalizeDraftQuoteInput(input)
	for _, correction := range quantityCorrections {
		log.Printf("DraftQuote: defaulted missing quantity to 1 run=%s service=%s itemIndex=%d description=%q", deps.GetRunID(), serviceID, correction.Index, correction.Description)
	}
	if invalidQuantity, invalid := findInvalidDraftQuoteQuantity(normalizedInput.Items); invalid {
		log.Printf("DraftQuote: rejected vague quantity run=%s service=%s itemIndex=%d quantity=%q description=%q", deps.GetRunID(), serviceID, invalidQuantity.Index, invalidQuantity.Quantity, invalidQuantity.Description)
		return DraftQuoteOutput{Success: false, Message: "Conceptofferte vereist concrete hoeveelheden per regel"}, fmt.Errorf("draft quote invalid quantity at item %d: %q", invalidQuantity.Index, invalidQuantity.Quantity)
	}

	deps.SetLastDraftInput(normalizedInput)

	if blockedOutput, blockedErr := validateDraftQuoteGovernance(ctx, deps, leadID, serviceID, *tenantID, len(normalizedInput.Items)); blockedErr != nil {
		return blockedOutput, blockedErr
	}

	portItems := convertDraftItems(normalizedInput.Items)
	portItems, err := enforceCatalogUnitPrices(ctx, deps, *tenantID, portItems)
	if err != nil {
		return DraftQuoteOutput{Success: false, Message: err.Error()}, err
	}
	portAttachments, portURLs := collectCatalogAssetsForDraft(ctx, deps, tenantID, portItems)
	pricingSnapshot, pricingSnapshotErr := buildDraftPricingSnapshot(ctx, deps, *tenantID, leadID, serviceID)
	if pricingSnapshotErr != nil {
		log.Printf("DraftQuote: pricing snapshot context unavailable: %v", pricingSnapshotErr)
	}

	result, err := deps.QuoteDrafter.DraftQuote(ctx, ports.DraftQuoteParams{
		QuoteID:         deps.GetExistingQuoteID(),
		LeadID:          leadID,
		LeadServiceID:   serviceID,
		OrganizationID:  *tenantID,
		CreatedByID:     uuid.Nil,
		Notes:           normalizedInput.Notes,
		Items:           portItems,
		Attachments:     portAttachments,
		URLs:            portURLs,
		PricingSnapshot: pricingSnapshot,
	})
	if err != nil {
		log.Printf("DraftQuote: failed: %v", err)
		return DraftQuoteOutput{Success: false, Message: fmt.Sprintf("Failed to draft quote: %v", err)}, err
	}

	log.Printf("DraftQuote: created run=%s quote=%s items=%d lead=%s service=%s", deps.GetRunID(), result.QuoteNumber, result.ItemCount, leadID, serviceID)
	deps.SetLastDraftResult(result)
	deps.SetExistingQuoteID(&result.QuoteID)
	deps.MarkDraftQuoteCalled()

	return DraftQuoteOutput{
		Success:     true,
		Message:     fmt.Sprintf("Draft quote %s created with %d items", result.QuoteNumber, result.ItemCount),
		QuoteID:     result.QuoteID.String(),
		QuoteNumber: result.QuoteNumber,
		ItemCount:   result.ItemCount,
	}, nil
}

func buildDraftPricingSnapshot(ctx context.Context, deps *ToolDependencies, tenantID, leadID, serviceID uuid.UUID) (*ports.QuotePricingSnapshot, error) {
	lead, err := deps.Repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("load lead for pricing snapshot: %w", err)
	}

	service, err := deps.Repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("load service for pricing snapshot: %w", err)
	}

	var materialSubtotalCents *int64
	var laborSubtotalLowCents *int64
	var laborSubtotalHighCents *int64
	var extraCostsCents *int64
	if snapshot, ok := deps.GetLastEstimateSnapshot(); ok {
		materialSubtotalCents = &snapshot.MaterialSubtotalCents
		laborSubtotalLowCents = &snapshot.LaborSubtotalLowCents
		laborSubtotalHighCents = &snapshot.LaborSubtotalHighCents
		extraCostsCents = &snapshot.ExtraCostsCents
	}

	runID := deps.GetRunID()
	postcodeRaw := strings.TrimSpace(lead.AddressZipCode)

	return &ports.QuotePricingSnapshot{
		ServiceType:            service.ServiceType,
		PostcodeRaw:            postcodeRaw,
		PostcodePrefixZIP4:     derivePostcodePrefixZIP4(postcodeRaw),
		SourceType:             "ai_draft",
		MaterialSubtotalCents:  materialSubtotalCents,
		LaborSubtotalLowCents:  laborSubtotalLowCents,
		LaborSubtotalHighCents: laborSubtotalHighCents,
		ExtraCostsCents:        extraCostsCents,
		EstimatorRunID:         nilIfEmptyString(runID),
		CreatedByActor:         repository.ActorNameEstimator,
	}, nil
}

func derivePostcodePrefixZIP4(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	digits := make([]rune, 0, 4)
	for _, r := range trimmed {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
			if len(digits) == 4 {
				return string(digits)
			}
		}
	}
	return ""
}

func nilIfEmptyString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func createSubmitQuoteCritiqueTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewSubmitQuoteCritiqueTool(func(ctx tool.Context, input SubmitQuoteCritiqueInput) (SubmitQuoteCritiqueOutput, error) {
		return handleSubmitQuoteCritique(ctx, GetDependencies(ctx), input)
	})
}

func handleSubmitQuoteCritique(ctx tool.Context, deps *ToolDependencies, input SubmitQuoteCritiqueInput) (SubmitQuoteCritiqueOutput, error) {
	if deps.QuoteDrafter == nil {
		return SubmitQuoteCritiqueOutput{Success: false, Message: "Quote drafting is not configured"}, nil
	}

	tenantID, ok := deps.GetTenantID()
	if !ok || tenantID == nil {
		return SubmitQuoteCritiqueOutput{Success: false, Message: "Organization context not available"}, errors.New(missingTenantContextError)
	}

	draftResult := deps.GetLastDraftResult()
	if draftResult == nil {
		return SubmitQuoteCritiqueOutput{Success: false, Message: "No draft quote available for review"}, fmt.Errorf("missing draft quote context")
	}

	decision := "needs_repair"
	if input.Approved {
		decision = "approved"
	}

	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		if input.Approved {
			summary = "AI-review akkoord: conceptofferte is klaar voor menselijke controle."
		} else {
			summary = "AI-review afgekeurd: conceptofferte heeft nog herstel nodig."
		}
	}

	findings := make([]ports.QuoteAIReviewFinding, 0, len(input.Findings))
	for _, finding := range input.Findings {
		message := strings.TrimSpace(finding.Message)
		if message == "" {
			continue
		}
		findings = append(findings, ports.QuoteAIReviewFinding{
			Code:      strings.TrimSpace(finding.Code),
			Message:   message,
			Severity:  strings.TrimSpace(finding.Severity),
			ItemIndex: finding.ItemIndex,
		})
	}
	deps.SetLastQuoteCritiqueInput(input)

	runID := deps.GetRunID()
	reviewerName := "QuoteCritic"
	modelName := "moonshot"
	attemptCount := deps.GetQuoteCriticAttempt()
	if attemptCount <= 0 {
		attemptCount = 1
	}
	reviewResult, err := deps.QuoteDrafter.RecordQuoteAIReview(ctx, ports.RecordQuoteAIReviewParams{
		QuoteID:        draftResult.QuoteID,
		OrganizationID: *tenantID,
		Decision:       decision,
		Summary:        summary,
		Findings:       findings,
		Signals:        normalizeMissingInformation(input.Signals),
		AttemptCount:   attemptCount,
		RunID:          &runID,
		ReviewerName:   &reviewerName,
		ModelName:      &modelName,
	})
	if err != nil {
		return SubmitQuoteCritiqueOutput{Success: false, Message: err.Error()}, err
	}

	deps.SetLastQuoteReviewResult(reviewResult)
	return SubmitQuoteCritiqueOutput{
		Success:  true,
		Message:  summary,
		Decision: reviewResult.Decision,
		ReviewID: reviewResult.ReviewID.String(),
	}, nil
}

func validateDraftQuoteGovernance(ctx tool.Context, deps *ToolDependencies, leadID, serviceID, tenantID uuid.UUID, itemCount int) (DraftQuoteOutput, error) {
	if deps.ShouldForceDraftQuote() {
		log.Printf("DraftQuote: manual governance bypass enabled run=%s service=%s", deps.GetRunID(), serviceID)
		return DraftQuoteOutput{}, nil
	}

	if blocked, reason := shouldBlockDraftQuoteForInsufficientIntake(ctx, deps, serviceID, tenantID); blocked {
		log.Printf("DraftQuote: blocked run=%s service=%s reason=%s", deps.GetRunID(), serviceID, reason)
		return DraftQuoteOutput{Success: false, Message: "Onvoldoende intakegegevens voor een betrouwbare conceptofferte"}, fmt.Errorf("draft quote blocked: %s", reason)
	}

	councilEval, councilErr := evaluateCouncilForDraftQuote(ctx, deps, leadID, serviceID, tenantID, itemCount)
	if councilErr != nil {
		return DraftQuoteOutput{Success: false, Message: "Council evaluatie mislukt"}, councilErr
	}
	if councilEval.Decision == CouncilDecisionAllow {
		return DraftQuoteOutput{}, nil
	}

	summary := strings.TrimSpace(councilEval.Summary)
	if summary == "" {
		summary = "Council blokkeert conceptofferte: handmatige beoordeling vereist."
	}
	if deps.MarkAlertEmitted("council_draft_quote", councilEval.ReasonCode, summary) {
		_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
			LeadID:         leadID,
			ServiceID:      &serviceID,
			OrganizationID: tenantID,
			ActorType:      repository.ActorTypeSystem,
			ActorName:      "Council",
			EventType:      repository.EventTypeAlert,
			Title:          repository.EventTitleManualIntervention,
			Summary:        &summary,
			Metadata: repository.CouncilAdviceMetadata{
				Decision:         councilEval.Decision,
				ReasonCode:       councilEval.ReasonCode,
				Summary:          councilEval.Summary,
				EstimatorSignals: councilEval.EstimatorSignals,
				RiskSignals:      councilEval.RiskSignals,
				ReadinessSignals: councilEval.ReadinessSignals,
			}.ToMap(),
		})
	}

	return DraftQuoteOutput{Success: false, Message: summary}, fmt.Errorf("council blocked draft quote: %s", councilEval.ReasonCode)
}

func evaluateCouncilForDraftQuote(ctx tool.Context, deps *ToolDependencies, leadID, serviceID, tenantID uuid.UUID, itemCount int) (CouncilEvaluation, error) {
	settings := deps.GetOrganizationAISettingsOrDefault()
	if !settings.AICouncilMode || deps.CouncilService == nil {
		deps.SetCouncilMetadata(nil)
		return CouncilEvaluation{Decision: CouncilDecisionAllow, ReasonCode: "council_disabled", Summary: "Council uitgeschakeld."}, nil
	}

	evaluation, err := deps.CouncilService.Evaluate(ctx, CouncilEvaluationInput{
		Action:    CouncilActionDraftQuote,
		LeadID:    leadID,
		ServiceID: serviceID,
		TenantID:  tenantID,
		Mode:      settings.AICouncilConsensusMode,
		ItemCount: itemCount,
	})
	if err != nil {
		return CouncilEvaluation{}, err
	}

	deps.SetCouncilMetadata(repository.CouncilAdviceMetadata{
		Decision:         evaluation.Decision,
		ReasonCode:       evaluation.ReasonCode,
		Summary:          evaluation.Summary,
		EstimatorSignals: evaluation.EstimatorSignals,
		RiskSignals:      evaluation.RiskSignals,
		ReadinessSignals: evaluation.ReadinessSignals,
	}.ToMap())

	return evaluation, nil
}

func shouldBlockDraftQuoteForInsufficientIntake(ctx context.Context, deps *ToolDependencies, serviceID, tenantID uuid.UUID) (bool, string) {
	analysis, err := deps.Repo.GetLatestAIAnalysis(ctx, serviceID, tenantID)
	if err != nil {
		return true, "latest analysis unavailable"
	}
	if reason := domain.ValidateAnalysisStageTransition(analysis.RecommendedAction, analysis.MissingInformation, domain.PipelineStageEstimation); reason != "" {
		return true, reason
	}
	return false, ""
}

// enforceCatalogUnitPrices ensures catalog-linked quote items use authoritative
// catalog pricing metadata (unit price + VAT). Ad-hoc items (without
// catalogProductId) are left unchanged so they can be estimated.
func enforceCatalogUnitPrices(ctx context.Context, deps *ToolDependencies, tenantID uuid.UUID, items []ports.DraftQuoteItem) ([]ports.DraftQuoteItem, error) {
	if deps.CatalogReader == nil || len(items) == 0 {
		return items, nil
	}

	catalogIDs := collectCatalogProductIDs(items)
	if len(catalogIDs) == 0 {
		return items, nil
	}

	details, err := deps.CatalogReader.GetProductDetails(ctx, tenantID, catalogIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to validate catalog-linked quote items: %w", err)
	}

	detailByID := mapCatalogDetailsByID(details)

	priceAdjusted, vatAdjusted, unresolvedCatalogIDs := normalizeCatalogLinkedItems(items, detailByID)
	if unresolvedCatalogIDs > 0 {
		return nil, fmt.Errorf("failed to resolve %d catalog-linked quote item(s)", unresolvedCatalogIDs)
	}

	logCatalogNormalizationSummary(priceAdjusted, vatAdjusted, unresolvedCatalogIDs)
	return items, nil
}

func mapCatalogDetailsByID(details []ports.CatalogProductDetails) map[uuid.UUID]ports.CatalogProductDetails {
	detailByID := make(map[uuid.UUID]ports.CatalogProductDetails, len(details))
	for _, d := range details {
		detailByID[d.ID] = d
	}
	return detailByID
}

func normalizeCatalogLinkedItems(items []ports.DraftQuoteItem, detailByID map[uuid.UUID]ports.CatalogProductDetails) (priceAdjusted int, vatAdjusted int, unresolvedCatalogIDs int) {
	for i := range items {
		if items[i].CatalogProductID == nil {
			continue
		}

		priceChanged, vatChanged, resolved := applyCatalogDetailToDraftItem(&items[i], detailByID)
		if !resolved {
			unresolvedCatalogIDs++
			continue
		}
		if priceChanged {
			priceAdjusted++
		}
		if vatChanged {
			vatAdjusted++
		}
	}
	return priceAdjusted, vatAdjusted, unresolvedCatalogIDs
}

func applyCatalogDetailToDraftItem(item *ports.DraftQuoteItem, detailByID map[uuid.UUID]ports.CatalogProductDetails) (priceChanged bool, vatChanged bool, resolved bool) {
	d, ok := detailByID[*item.CatalogProductID]
	if !ok {
		return false, false, false
	}

	if item.UnitPriceCents != d.UnitPriceCents {
		item.UnitPriceCents = d.UnitPriceCents
		priceChanged = true
	}
	if d.VatRateBps > 0 && item.TaxRateBps != d.VatRateBps {
		item.TaxRateBps = d.VatRateBps
		vatChanged = true
	}

	return priceChanged, vatChanged, true
}

func logCatalogNormalizationSummary(priceAdjusted int, vatAdjusted int, unresolvedCatalogIDs int) {
	if priceAdjusted > 0 || vatAdjusted > 0 {
		log.Printf("DraftQuote: normalized catalog-linked metadata (prices=%d vat=%d)", priceAdjusted, vatAdjusted)
	}
	if unresolvedCatalogIDs > 0 {
		log.Printf("DraftQuote: %d catalog-linked item(s) could not be resolved; kept input values to avoid breaking flow", unresolvedCatalogIDs)
	}
}

type draftQuoteQuantityCorrection struct {
	Index       int
	Description string
}

type invalidDraftQuoteQuantity struct {
	Index       int
	Description string
	Quantity    string
}

func normalizeDraftQuoteInput(input DraftQuoteInput) (DraftQuoteInput, []draftQuoteQuantityCorrection) {
	normalized := input
	normalized.Notes = strings.TrimSpace(input.Notes)
	normalized.Items = make([]DraftQuoteItem, len(input.Items))
	corrections := make([]draftQuoteQuantityCorrection, 0)
	for i, item := range input.Items {
		normalizedItem, corrected := normalizeDraftQuoteItem(item)
		normalized.Items[i] = normalizedItem
		if corrected {
			corrections = append(corrections, draftQuoteQuantityCorrection{
				Index:       i,
				Description: normalizedItem.Description,
			})
		}
	}
	return normalized, corrections
}

func normalizeDraftQuoteItem(item DraftQuoteItem) (DraftQuoteItem, bool) {
	item.Description = strings.TrimSpace(item.Description)
	item.Quantity = strings.TrimSpace(item.Quantity)
	if item.Quantity != "" {
		return item, false
	}
	item.Quantity = "1"
	return item, true
}

func findInvalidDraftQuoteQuantity(items []DraftQuoteItem) (invalidDraftQuoteQuantity, bool) {
	for i, item := range items {
		if isVagueDraftQuoteQuantity(item.Quantity) {
			return invalidDraftQuoteQuantity{
				Index:       i,
				Description: item.Description,
				Quantity:    item.Quantity,
			}, true
		}
	}
	return invalidDraftQuoteQuantity{}, false
}

func isVagueDraftQuoteQuantity(quantity string) bool {
	normalized := strings.ToLower(strings.TrimSpace(quantity))
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "?") {
		return true
	}
	replacer := strings.NewReplacer(".", "", "-", " ", "_", " ", "/", " ")
	normalized = strings.Join(strings.Fields(replacer.Replace(normalized)), " ")
	switch normalized {
	case "nader te bepalen", "nog te bepalen", "onbekend", "unknown", "tbd", "ntb", "nvt":
		return true
	default:
		return false
	}
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

func createDraftQuoteTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewDraftQuoteTool(func(ctx tool.Context, input DraftQuoteInput) (DraftQuoteOutput, error) {
		return handleDraftQuote(ctx, GetDependencies(ctx), input)
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
