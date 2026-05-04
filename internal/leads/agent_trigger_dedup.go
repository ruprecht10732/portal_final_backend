package leads

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/platform/logger"
)

const (
	estimatorTriggerFingerprintPrefix  = "leads:orchestrator:estimator:fingerprint"
	dispatcherTriggerFingerprintPrefix = "leads:orchestrator:dispatcher:fingerprint"
)

var compareAndStoreTriggerFingerprintScript = redis.NewScript(`
local current = redis.call("GET", KEYS[1])
if current == ARGV[1] then
	redis.call("PEXPIRE", KEYS[1], ARGV[2])
	return 0
end
redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
return 1
`)

type triggerFingerprintDeduper interface {
	ShouldEnqueue(serviceID uuid.UUID, fingerprint string) (bool, error)
}

type triggerFingerprintState struct {
	fingerprint string
	expiresAt   time.Time
}

type inMemoryTriggerFingerprintDeduper struct {
	states map[uuid.UUID]triggerFingerprintState
	ttl    time.Duration
	mu     sync.Mutex
}

type redisTriggerFingerprintDeduper struct {
	client *redis.Client
	prefix string
	ttl    time.Duration
	log    *logger.Logger
}

type estimatorTriggerFingerprintRepo interface {
	GetByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (repository.Lead, error)
	GetLeadServiceByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (repository.LeadService, error)
	ListNotesByService(ctx context.Context, leadID uuid.UUID, serviceID uuid.UUID, organizationID uuid.UUID) ([]repository.LeadNote, error)
	GetLatestAIAnalysis(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) (repository.AIAnalysis, error)
}

type dispatcherTriggerFingerprintRepo interface {
	GetByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (repository.Lead, error)
	GetLeadServiceByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (repository.LeadService, error)
	GetServiceStateAggregates(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) (repository.ServiceStateAggregates, error)
	GetInvitedPartnerIDs(ctx context.Context, serviceID uuid.UUID) ([]uuid.UUID, error)
	HasLinkedPartners(ctx context.Context, organizationID uuid.UUID, leadID uuid.UUID) (bool, error)
}

type estimatorTriggerSnapshot struct {
	Lead     gatekeeperLeadSnapshot     `json:"lead"`
	Service  estimatorServiceSnapshot   `json:"service"`
	Notes    []gatekeeperNoteSnapshot   `json:"notes,omitempty"`
	Analysis *estimatorAnalysisSnapshot `json:"analysis,omitempty"`
}

type estimatorServiceSnapshot struct {
	PipelineStage       string `json:"pipelineStage,omitempty"`
	ServiceType         string `json:"serviceType,omitempty"`
	ConsumerNote        string `json:"consumerNote,omitempty"`
	Source              string `json:"source,omitempty"`
	CustomerPreferences string `json:"customerPreferences,omitempty"`
}

type estimatorAnalysisSnapshot struct {
	UrgencyLevel            string             `json:"urgencyLevel,omitempty"`
	UrgencyReason           string             `json:"urgencyReason,omitempty"`
	LeadQuality             string             `json:"leadQuality,omitempty"`
	RecommendedAction       string             `json:"recommendedAction,omitempty"`
	MissingInformation      []string           `json:"missingInformation,omitempty"`
	ResolvedInformation     []string           `json:"resolvedInformation,omitempty"`
	ExtractedFacts          map[string]string  `json:"extractedFacts,omitempty"`
	PreferredContactChannel string             `json:"preferredContactChannel,omitempty"`
	Summary                 string             `json:"summary,omitempty"`
	CompositeConfidence     *float64           `json:"compositeConfidence,omitempty"`
	ConfidenceBreakdown     map[string]float64 `json:"confidenceBreakdown,omitempty"`
	RiskFlags               []string           `json:"riskFlags,omitempty"`
}

type dispatcherTriggerSnapshot struct {
	Lead                  dispatcherLeadSnapshot       `json:"lead"`
	Service               dispatcherServiceSnapshot    `json:"service"`
	Aggregates            dispatcherAggregatesSnapshot `json:"aggregates"`
	LinkedPartnersPresent bool                         `json:"linkedPartnersPresent"`
	ExcludedPartnerIDs    []string                     `json:"excludedPartnerIds,omitempty"`
}

type dispatcherLeadSnapshot struct {
	ZipCode string `json:"zipCode,omitempty"`
	City    string `json:"city,omitempty"`
}

type dispatcherServiceSnapshot struct {
	PipelineStage string `json:"pipelineStage,omitempty"`
	ServiceType   string `json:"serviceType,omitempty"`
}

type dispatcherAggregatesSnapshot struct {
	AcceptedOffers int  `json:"acceptedOffers,omitempty"`
	PendingOffers  int  `json:"pendingOffers,omitempty"`
	AcceptedQuotes int  `json:"acceptedQuotes,omitempty"`
	SentQuotes     int  `json:"sentQuotes,omitempty"`
	DraftQuotes    int  `json:"draftQuotes,omitempty"`
	HasVisitReport bool `json:"hasVisitReport"`
}

type estimatorEnqueueRequest struct {
	ctx       context.Context
	repo      estimatorTriggerFingerprintRepo
	deduper   triggerFingerprintDeduper
	queue     scheduler.AgentTaskScheduler
	log       *logger.Logger
	leadID    uuid.UUID
	serviceID uuid.UUID
	tenantID  uuid.UUID
	force     bool
	source    string
}

type dispatcherEnqueueRequest struct {
	ctx       context.Context
	repo      dispatcherTriggerFingerprintRepo
	deduper   triggerFingerprintDeduper
	queue     scheduler.AgentTaskScheduler
	log       *logger.Logger
	leadID    uuid.UUID
	serviceID uuid.UUID
	tenantID  uuid.UUID
	source    string
}

func newTriggerFingerprintDeduper(redisClient *redis.Client, ttl time.Duration, prefix string, log *logger.Logger) triggerFingerprintDeduper {
	if ttl <= 0 {
		ttl = gatekeeperTriggerFingerprintTTL
	}
	if redisClient == nil {
		return &inMemoryTriggerFingerprintDeduper{
			states: make(map[uuid.UUID]triggerFingerprintState),
			ttl:    ttl,
		}
	}
	return &redisTriggerFingerprintDeduper{client: redisClient, prefix: prefix, ttl: ttl, log: log}
}

func (d *inMemoryTriggerFingerprintDeduper) ShouldEnqueue(serviceID uuid.UUID, fingerprint string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	if state, ok := d.states[serviceID]; ok {
		if now.Before(state.expiresAt) && state.fingerprint == fingerprint {
			state.expiresAt = now.Add(d.ttl)
			d.states[serviceID] = state
			return false, nil
		}
	}

	d.states[serviceID] = triggerFingerprintState{fingerprint: fingerprint, expiresAt: now.Add(d.ttl)}
	return true, nil
}

func (d *redisTriggerFingerprintDeduper) ShouldEnqueue(serviceID uuid.UUID, fingerprint string) (bool, error) {
	result, err := compareAndStoreTriggerFingerprintScript.Run(
		context.Background(),
		d.client,
		[]string{triggerFingerprintKey(d.prefix, serviceID)},
		fingerprint,
		d.ttl.Milliseconds(),
	).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func triggerFingerprintKey(prefix string, serviceID uuid.UUID) string {
	return fmt.Sprintf("%s:%s", prefix, serviceID)
}

func buildEstimatorTriggerFingerprint(ctx context.Context, repo estimatorTriggerFingerprintRepo, leadID, serviceID, tenantID uuid.UUID) (string, error) {
	lead, err := repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return "", err
	}
	service, err := repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return "", err
	}
	notes, err := repo.ListNotesByService(ctx, leadID, serviceID, tenantID)
	if err != nil {
		return "", err
	}

	var analysis *repository.AIAnalysis
	if current, analysisErr := repo.GetLatestAIAnalysis(ctx, serviceID, tenantID); analysisErr == nil {
		analysis = &current
	} else if analysisErr != repository.ErrNotFound {
		return "", analysisErr
	}

	snapshot := estimatorTriggerSnapshot{
		Lead: gatekeeperLeadSnapshot{
			FirstName:     normalizeTriggerText(lead.ConsumerFirstName),
			LastName:      normalizeTriggerText(lead.ConsumerLastName),
			Phone:         normalizeTriggerText(lead.ConsumerPhone),
			Email:         normalizeOptionalTriggerText(lead.ConsumerEmail),
			Role:          normalizeTriggerText(lead.ConsumerRole),
			Street:        normalizeTriggerText(lead.AddressStreet),
			HouseNumber:   normalizeTriggerText(lead.AddressHouseNumber),
			ZipCode:       normalizeTriggerText(lead.AddressZipCode),
			City:          normalizeTriggerText(lead.AddressCity),
			WhatsAppOptIn: lead.WhatsAppOptedIn,
			AssignedAgent: normalizeUUIDPtr(lead.AssignedAgentID),
		},
		Service: estimatorServiceSnapshot{
			PipelineStage:       normalizeTriggerText(service.PipelineStage),
			ServiceType:         normalizeTriggerText(service.ServiceType),
			ConsumerNote:        normalizeOptionalTriggerText(service.ConsumerNote),
			Source:              normalizeOptionalTriggerText(service.Source),
			CustomerPreferences: normalizePreferencesJSON(service.CustomerPreferences),
		},
		Notes:    summarizeGatekeeperNotes(notes),
		Analysis: summarizeEstimatorAnalysis(analysis),
	}

	return marshalTriggerFingerprint(snapshot)
}

func buildDispatcherTriggerFingerprint(ctx context.Context, repo dispatcherTriggerFingerprintRepo, leadID, serviceID, tenantID uuid.UUID) (string, error) {
	lead, err := repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return "", err
	}
	service, err := repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return "", err
	}
	aggs, err := repo.GetServiceStateAggregates(ctx, serviceID, tenantID)
	if err != nil {
		return "", err
	}
	excludedPartnerIDs, err := repo.GetInvitedPartnerIDs(ctx, serviceID)
	if err != nil {
		return "", err
	}
	linkedPartners, err := repo.HasLinkedPartners(ctx, tenantID, leadID)
	if err != nil {
		return "", err
	}

	snapshot := dispatcherTriggerSnapshot{
		Lead: dispatcherLeadSnapshot{
			ZipCode: normalizeTriggerText(lead.AddressZipCode),
			City:    normalizeTriggerText(lead.AddressCity),
		},
		Service: dispatcherServiceSnapshot{
			PipelineStage: normalizeTriggerText(service.PipelineStage),
			ServiceType:   normalizeTriggerText(service.ServiceType),
		},
		Aggregates: dispatcherAggregatesSnapshot{
			AcceptedOffers: aggs.AcceptedOffers,
			PendingOffers:  aggs.PendingOffers,
			AcceptedQuotes: aggs.AcceptedQuotes,
			SentQuotes:     aggs.SentQuotes,
			DraftQuotes:    aggs.DraftQuotes,
			HasVisitReport: aggs.HasVisitReport,
		},
		LinkedPartnersPresent: linkedPartners,
		ExcludedPartnerIDs:    normalizeUUIDs(excludedPartnerIDs),
	}

	return marshalTriggerFingerprint(snapshot)
}

func maybeEnqueueEstimatorRun(request estimatorEnqueueRequest) bool {
	if request.queue == nil {
		return false
	}

	fingerprint := request.buildFingerprint()
	if request.shouldSkipDuplicateFingerprint(fingerprint) {
		return true
	}

	request.enqueue(fingerprint)
	return true
}

func maybeEnqueueDispatcherRun(request dispatcherEnqueueRequest) bool {
	if request.queue == nil {
		return false
	}

	fingerprint := request.buildFingerprint()
	if request.shouldSkipDuplicateFingerprint(fingerprint) {
		return true
	}

	request.enqueue(fingerprint)
	return true
}

func (r estimatorEnqueueRequest) buildFingerprint() string {
	if r.force || r.repo == nil {
		return ""
	}
	currentFingerprint, err := buildEstimatorTriggerFingerprint(r.ctx, r.repo, r.leadID, r.serviceID, r.tenantID)
	if err != nil {
		if r.log != nil {
			r.log.Warn("estimator: failed to build trigger fingerprint; enqueueing without semantic dedupe", "error", err, "leadId", r.leadID, "serviceId", r.serviceID, "source", r.source)
		}
		return ""
	}
	return currentFingerprint
}

func (r estimatorEnqueueRequest) shouldSkipDuplicateFingerprint(fingerprint string) bool {
	if r.force || fingerprint == "" || r.deduper == nil {
		return false
	}
	shouldEnqueue, dedupeErr := r.deduper.ShouldEnqueue(r.serviceID, fingerprint)
	if dedupeErr != nil {
		if r.log != nil {
			r.log.Warn("estimator: semantic trigger dedupe failed; enqueueing anyway", "error", dedupeErr, "leadId", r.leadID, "serviceId", r.serviceID, "source", r.source)
		}
		return false
	}
	if shouldEnqueue {
		return false
	}
	if r.log != nil {
		r.log.Info("estimator: unchanged input fingerprint; skipping enqueue", "leadId", r.leadID, "serviceId", r.serviceID, "source", r.source, "fingerprint", fingerprint[:12])
	}
	return true
}

func (r estimatorEnqueueRequest) enqueue(fingerprint string) {
	if err := r.queue.EnqueueAgentTask(r.ctx, scheduler.AgentTaskPayload{
		Workspace:     "calculator",
		Mode:          "estimator",
		TenantID:      r.tenantID.String(),
		LeadID:        r.leadID.String(),
		LeadServiceID: r.serviceID.String(),
		Force:         r.force,
		Fingerprint:   fingerprint,
	}); err != nil && r.log != nil {
		r.log.Error("estimator queue enqueue failed", "error", err, "leadId", r.leadID, "serviceId", r.serviceID, "source", r.source)
	}
}

func (r dispatcherEnqueueRequest) buildFingerprint() string {
	if r.repo == nil {
		return ""
	}
	currentFingerprint, err := buildDispatcherTriggerFingerprint(r.ctx, r.repo, r.leadID, r.serviceID, r.tenantID)
	if err != nil {
		if r.log != nil {
			r.log.Warn("dispatcher: failed to build trigger fingerprint; enqueueing without semantic dedupe", "error", err, "leadId", r.leadID, "serviceId", r.serviceID, "source", r.source)
		}
		return ""
	}
	return currentFingerprint
}

func (r dispatcherEnqueueRequest) shouldSkipDuplicateFingerprint(fingerprint string) bool {
	if fingerprint == "" || r.deduper == nil {
		return false
	}
	shouldEnqueue, dedupeErr := r.deduper.ShouldEnqueue(r.serviceID, fingerprint)
	if dedupeErr != nil {
		if r.log != nil {
			r.log.Warn("dispatcher: semantic trigger dedupe failed; enqueueing anyway", "error", dedupeErr, "leadId", r.leadID, "serviceId", r.serviceID, "source", r.source)
		}
		return false
	}
	if shouldEnqueue {
		return false
	}
	if r.log != nil {
		r.log.Info("dispatcher: unchanged input fingerprint; skipping enqueue", "leadId", r.leadID, "serviceId", r.serviceID, "source", r.source, "fingerprint", fingerprint[:12])
	}
	return true
}

func (r dispatcherEnqueueRequest) enqueue(fingerprint string) {
	if err := r.queue.EnqueueAgentTask(r.ctx, scheduler.AgentTaskPayload{
		Workspace:     "matchmaker",
		TenantID:      r.tenantID.String(),
		LeadID:        r.leadID.String(),
		LeadServiceID: r.serviceID.String(),
		Fingerprint:   fingerprint,
	}); err != nil && r.log != nil {
		r.log.Error("dispatcher queue enqueue failed", "error", err, "leadId", r.leadID, "serviceId", r.serviceID, "source", r.source)
	}
}

func summarizeEstimatorAnalysis(analysis *repository.AIAnalysis) *estimatorAnalysisSnapshot {
	if analysis == nil {
		return nil
	}
	return &estimatorAnalysisSnapshot{
		UrgencyLevel:            normalizeTriggerText(analysis.UrgencyLevel),
		UrgencyReason:           normalizeOptionalTriggerText(analysis.UrgencyReason),
		LeadQuality:             normalizeTriggerText(analysis.LeadQuality),
		RecommendedAction:       normalizeTriggerText(analysis.RecommendedAction),
		MissingInformation:      sortedNormalizedCopy(analysis.MissingInformation),
		ResolvedInformation:     sortedNormalizedCopy(analysis.ResolvedInformation),
		ExtractedFacts:          normalizeStringMap(analysis.ExtractedFacts),
		PreferredContactChannel: normalizeTriggerText(analysis.PreferredContactChannel),
		Summary:                 normalizeTriggerText(analysis.Summary),
		CompositeConfidence:     analysis.CompositeConfidence,
		ConfidenceBreakdown:     normalizeFloat64Map(analysis.ConfidenceBreakdown),
		RiskFlags:               sortedNormalizedCopy(analysis.RiskFlags),
	}
}

func marshalTriggerFingerprint(snapshot any) (string, error) {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func normalizeUUIDs(values []uuid.UUID) []string {
	if len(values) == 0 {
		return nil
	}
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, value.String())
	}
	sort.Strings(items)
	return items
}

func normalizeStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		trimmedKey := normalizeTriggerText(key)
		trimmedValue := normalizeTriggerText(value)
		if trimmedKey == "" && trimmedValue == "" {
			continue
		}
		result[trimmedKey] = trimmedValue
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeFloat64Map(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]float64, len(values))
	for key, value := range values {
		trimmedKey := normalizeTriggerText(key)
		if trimmedKey == "" {
			continue
		}
		result[trimmedKey] = value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
