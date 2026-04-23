package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/scoring"
	"portal_final_backend/platform/ai/embeddings"
	"portal_final_backend/platform/qdrant"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/tool"
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
	agentCycleThreshold              = 3

	// toolIOTimeout is the independent timeout for network I/O operations
	// inside tool handlers (embeddings, vector search, catalog reads). It
	// prevents an expired parent deadline from cascading to tool execution.
	toolIOTimeout = 30 * time.Second

	maxCalculatorExprLength = 1000
	maxCalculatorDepth      = 100
)

const highConfidenceScoreThreshold = 0.45
const catalogEarlyReturnScoreThreshold = 0.55
const (
	defaultHouthandelCollection   = "houthandel_products"
	defaultBouwmaatCollection     = "bouwmaat_products"
	gatekeeperLoopDetectedTrigger = "ai_loop_detected"
	gatekeeperLoopReasonCode      = "nurturing_loop_threshold"
	gatekeeperLoopDetectedSummary = "Systeem: AI zat in een lus. Menselijke controle vereist."
	agentCycleDetectedTrigger     = "agent_cycle_detected"
	agentCycleReasonCode          = "cross_agent_cycle_threshold"
	agentCycleDetectedSummary     = "Systeem: Service pingpongt tussen Gatekeeper en Estimator. Menselijke controle vereist."
)

type ToolDependencies struct {
	Repo                        repository.LeadsRepository
	Scorer                      *scoring.Service
	EventBus                    events.Bus
	EmbeddingClient             *embeddings.Client
	QdrantClient                *qdrant.Client
	BouwmaatQdrantClient        *qdrant.Client
	CatalogQdrantClient         *qdrant.Client
	CatalogReader               ports.CatalogReader // optional: hydrate search results from DB
	QuoteDrafter                ports.QuoteDrafter  // optional: draft quotes from agent
	PricingIntelligence         ports.PricingIntelligenceReader
	OfferCreator                ports.PartnerOfferCreator
	CouncilService              MultiAgentCouncil
	OrgSettingsReader           ports.OrganizationAISettingsReader
	mu                          sync.RWMutex
	tenantID                    *uuid.UUID
	leadID                      *uuid.UUID
	serviceID                   *uuid.UUID
	actorType                   string
	actorName                   string
	actorRoles                  []string
	orgSettings                 *ports.OrganizationAISettings
	existingQuoteID             *uuid.UUID     // If set, DraftQuote updates this quote instead of creating new
	lastAnalysisMetadata        map[string]any // Populated by SaveAnalysis for use in stage_change events
	lastEstimationMetadata      map[string]any // Populated by SaveEstimation for use in stage_change events
	lastEstimateSnapshot        *EstimateComputationSnapshot
	lastCouncilMetadata         map[string]any          // Populated by council evaluation for stage/quote governance events
	saveAnalysisCalled          bool                    // Track if SaveAnalysis was called
	saveEstimationCalled        bool                    // Track if SaveEstimation was called
	stageUpdateCalled           bool                    // Track if UpdatePipelineStage was called
	lastStageUpdated            string                  // Track last pipeline stage written
	draftQuoteCalled            bool                    // Track if DraftQuote was called
	offerCreated                bool                    // Track if CreatePartnerOffer was called
	lastDraftResult             *ports.DraftQuoteResult // Captured by handleDraftQuote for generate endpoint
	lastDraftInput              *DraftQuoteInput        // Snapshot of the latest drafted line items for downstream review
	lastQuoteReviewResult       *ports.QuoteAIReviewResult
	lastQuoteCritiqueInput      *SubmitQuoteCritiqueInput
	quoteCriticAttempt          int
	quoteCritiqueSubmittedForAt int            // tracks the attempt number for which SubmitQuoteCritique was already called
	scopeArtifact               *ScopeArtifact // Produced by Scope Analyzer and consumed by Quote Builder
	clarificationAsked          bool           // Track if AskCustomerClarification was called in investigative mode
	runID                       string         // Correlates all tool calls within one agent run
	forceDraftQuote             bool           // Allows manual runs to bypass draft governance (intake + council)
	searchCache                 map[string]SearchProductMaterialsOutput
	emittedAlertKeys            map[string]struct{} // Dedupe identical alerts within a single agent run
	sessionDoneFunc             context.CancelFunc  // Optional: called after successful UpdatePipelineStage to end session early
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

func (d *ToolDependencies) SetActorRoles(roles []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.actorRoles = append([]string(nil), roles...)
}

// SetSessionDoneFunc sets an optional callback invoked after a successful
// UpdatePipelineStage commit to signal that the agent session should end.
// This is a defense-in-depth mechanism: the prompt instructs the LLM to stop
// after the final tool, but if it doesn't, cancelling the context forces
// termination without burning additional tool-call budget.
func (d *ToolDependencies) SetSessionDoneFunc(fn context.CancelFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sessionDoneFunc = fn
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
	return deepCopyMap(d.lastCouncilMetadata)
}

func (d *ToolDependencies) GetActor() (string, string) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.actorType == "" {
		return "AI", "Agent"
	}
	return d.actorType, d.actorName
}

func (d *ToolDependencies) GetActorRoles() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]string(nil), d.actorRoles...)
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

// deepCopyMap recursively copies a map[string]any, creating new slices and maps
// so that mutations in the returned value do not affect the original.
func deepCopyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	cp := make(map[string]any, len(src))
	for k, v := range src {
		cp[k] = deepCopyValue(v)
	}
	return cp
}

func deepCopyValue(v any) any {
	switch val := v.(type) {
	case []any:
		cp := make([]any, len(val))
		for i, item := range val {
			cp[i] = deepCopyValue(item)
		}
		return cp
	case []string:
		cp := make([]string, len(val))
		copy(cp, val)
		return cp
	case map[string]any:
		return deepCopyMap(val)
	case map[string]string:
		cp := make(map[string]string, len(val))
		for mk, mv := range val {
			cp[mk] = mv
		}
		return cp
	default:
		// For primitives and other immutable types, return as-is.
		return v
	}
}

// GetLastAnalysisMetadata retrieves a deep copy of the analysis metadata saved by SaveAnalysis.
func (d *ToolDependencies) GetLastAnalysisMetadata() map[string]any {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.lastAnalysisMetadata == nil {
		return nil
	}
	return deepCopyMap(d.lastAnalysisMetadata)
}

func (d *ToolDependencies) GetLastEstimationMetadata() map[string]any {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.lastEstimationMetadata == nil {
		return nil
	}
	return deepCopyMap(d.lastEstimationMetadata)
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
	d.quoteCritiqueSubmittedForAt = 0
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

// normalizedSearchCacheKey creates a word-order-independent cache key to catch
// slight query variations like "kunststof deur wit" vs "witte kunststof deur".
func normalizedSearchCacheKey(query string, limit int, useCatalog bool, scoreThreshold float64) string {
	words := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	sort.Strings(words)
	return fmt.Sprintf("~%s|%d|%t|%.4f", strings.Join(words, " "), limit, useCatalog, scoreThreshold)
}

func (d *ToolDependencies) getSearchCacheNormalized(key string) (SearchProductMaterialsOutput, bool) {
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
	d.quoteCritiqueSubmittedForAt = 0 // reset on new attempt
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

func applyRBACToolsets(toolsets []tool.Toolset) []tool.Toolset {
	var filtered []tool.Toolset
	for _, ts := range toolsets {
		filtered = append(filtered, tool.FilterToolset(ts, rbacPredicate))
	}
	return filtered
}

func rbacPredicate(ctx agent.ReadonlyContext, t tool.Tool) bool {
	// 1. Safe read-only tools are always allowed
	switch t.Name() {
	case "GetLeadDetails", "SearchProductMaterials", "PreloadMemory", "AskCustomerClarification", "SearchCouncilDirectives":
		return true
	}

	// 2. Fetch the dynamically injected dependencies for the current request
	deps, err := GetDependencies(ctx)
	if err != nil {
		return true // Fail-open for non-request contexts or system runs
	}

	roles := deps.GetActorRoles()
	if len(roles) == 0 {
		return true // System actors or background workers
	}

	// 3. Admins have full access
	for _, r := range roles {
		if r == "admin" || r == "super_admin" {
			return true
		}
	}

	// 4. Restrict state-mutating tools for standard support users
	restricted := map[string]bool{
		"UpdatePipelineStage": true,
		"DraftQuote":          true,
		"CreatePartnerOffer":  true,
	}

	return !restricted[t.Name()]
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
