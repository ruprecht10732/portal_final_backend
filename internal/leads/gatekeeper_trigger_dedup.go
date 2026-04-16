package leads

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/platform/logger"
)

const gatekeeperTriggerFingerprintTTL = 24 * time.Hour

// gatekeeperAbortCooldownTTL is the period after an aborted run during which
// no new gatekeeper run will be enqueued for the same service, regardless of
// data-fingerprint changes.
const gatekeeperAbortCooldownTTL = 10 * time.Minute

const gatekeeperTriggerFingerprintPrefix = "leads:orchestrator:gatekeeper:fingerprint"

var compareAndStoreGatekeeperFingerprintScript = redis.NewScript(`
local current = redis.call("GET", KEYS[1])
if current == ARGV[1] then
	redis.call("PEXPIRE", KEYS[1], ARGV[2])
	return 0
end
redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
return 1
`)

type gatekeeperTriggerDeduper interface {
	ShouldEnqueue(serviceID uuid.UUID, fingerprint string) (bool, error)
	// RecordAbort records that a gatekeeper run for the given service was
	// aborted (e.g. tool-call budget exceeded). Consecutive aborts for the
	// same service trigger an abort cooldown that blocks re-enqueue for a
	// short period, preventing rapid re-trigger loops.
	RecordAbort(serviceID uuid.UUID)
}

type gatekeeperTriggerFingerprintRepo interface {
	GetByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (repository.Lead, error)
	GetLeadServiceByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (repository.LeadService, error)
	ListNotesByService(ctx context.Context, leadID uuid.UUID, serviceID uuid.UUID, organizationID uuid.UUID) ([]repository.LeadNote, error)
	ListAttachmentsByService(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) ([]repository.Attachment, error)
	GetLatestPhotoAnalysis(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) (repository.PhotoAnalysis, error)
	GetLatestAppointmentVisitReportByService(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) (*repository.AppointmentVisitReport, error)
}

type gatekeeperTriggerFingerprintState struct {
	fingerprint string
	expiresAt   time.Time
}

type gatekeeperAbortCooldownState struct {
	expiresAt time.Time
}

type inMemoryGatekeeperTriggerDeduper struct {
	mu             sync.Mutex
	ttl            time.Duration
	states         map[uuid.UUID]gatekeeperTriggerFingerprintState
	abortCooldowns map[uuid.UUID]gatekeeperAbortCooldownState
}

type redisGatekeeperTriggerDeduper struct {
	client *redis.Client
	ttl    time.Duration
	log    *logger.Logger
}

type gatekeeperTriggerSnapshot struct {
	Lead        gatekeeperLeadSnapshot         `json:"lead"`
	Service     gatekeeperServiceSnapshot      `json:"service"`
	Notes       []gatekeeperNoteSnapshot       `json:"notes,omitempty"`
	Attachments []gatekeeperAttachmentSummary  `json:"attachments,omitempty"`
	Photo       *gatekeeperPhotoSnapshot       `json:"photo,omitempty"`
	VisitReport *gatekeeperVisitReportSnapshot `json:"visitReport,omitempty"`
}

type gatekeeperLeadSnapshot struct {
	FirstName     string `json:"firstName,omitempty"`
	LastName      string `json:"lastName,omitempty"`
	Phone         string `json:"phone,omitempty"`
	Email         string `json:"email,omitempty"`
	Role          string `json:"role,omitempty"`
	Street        string `json:"street,omitempty"`
	HouseNumber   string `json:"houseNumber,omitempty"`
	ZipCode       string `json:"zipCode,omitempty"`
	City          string `json:"city,omitempty"`
	WhatsAppOptIn bool   `json:"whatsAppOptIn"`
	AssignedAgent string `json:"assignedAgent,omitempty"`
}

type gatekeeperServiceSnapshot struct {
	ServiceType         string `json:"serviceType,omitempty"`
	ConsumerNote        string `json:"consumerNote,omitempty"`
	Source              string `json:"source,omitempty"`
	CustomerPreferences string `json:"customerPreferences,omitempty"`
}

type gatekeeperNoteSnapshot struct {
	Type string `json:"type,omitempty"`
	Body string `json:"body,omitempty"`
}

type gatekeeperAttachmentSummary struct {
	FileKey     string `json:"fileKey,omitempty"`
	FileName    string `json:"fileName,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
}

type gatekeeperPhotoSnapshot struct {
	Summary                string   `json:"summary,omitempty"`
	ScopeAssessment        string   `json:"scopeAssessment,omitempty"`
	ConfidenceLevel        string   `json:"confidenceLevel,omitempty"`
	PhotoCount             int      `json:"photoCount,omitempty"`
	Observations           []string `json:"observations,omitempty"`
	NeedsOnsiteMeasurement []string `json:"needsOnsiteMeasurement,omitempty"`
	ExtractedText          []string `json:"extractedText,omitempty"`
}

type gatekeeperVisitReportSnapshot struct {
	Measurements     string `json:"measurements,omitempty"`
	AccessDifficulty string `json:"accessDifficulty,omitempty"`
	Notes            string `json:"notes,omitempty"`
}

type gatekeeperEnqueueRequest struct {
	ctx       context.Context
	repo      gatekeeperTriggerFingerprintRepo
	deduper   gatekeeperTriggerDeduper
	queue     scheduler.GatekeeperScheduler
	log       *logger.Logger
	leadID    uuid.UUID
	serviceID uuid.UUID
	tenantID  uuid.UUID
	source    string
}

func newGatekeeperTriggerDeduper(redisClient *redis.Client, ttl time.Duration, log *logger.Logger) gatekeeperTriggerDeduper {
	if ttl <= 0 {
		ttl = gatekeeperTriggerFingerprintTTL
	}
	if redisClient == nil {
		return newInMemoryGatekeeperTriggerDeduper(ttl)
	}
	return &redisGatekeeperTriggerDeduper{client: redisClient, ttl: ttl, log: log}
}

func newInMemoryGatekeeperTriggerDeduper(ttl time.Duration) gatekeeperTriggerDeduper {
	if ttl <= 0 {
		ttl = gatekeeperTriggerFingerprintTTL
	}
	return &inMemoryGatekeeperTriggerDeduper{
		ttl:            ttl,
		states:         make(map[uuid.UUID]gatekeeperTriggerFingerprintState),
		abortCooldowns: make(map[uuid.UUID]gatekeeperAbortCooldownState),
	}
}

func (d *inMemoryGatekeeperTriggerDeduper) ShouldEnqueue(serviceID uuid.UUID, fingerprint string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	// Check abort cooldown: if the service recently had an aborted run,
	// block re-enqueue regardless of fingerprint changes.
	if cooldown, ok := d.abortCooldowns[serviceID]; ok {
		if now.Before(cooldown.expiresAt) {
			return false, nil
		}
		delete(d.abortCooldowns, serviceID)
	}

	if state, ok := d.states[serviceID]; ok {
		if now.Before(state.expiresAt) && state.fingerprint == fingerprint {
			state.expiresAt = now.Add(d.ttl)
			d.states[serviceID] = state
			return false, nil
		}
	}

	d.states[serviceID] = gatekeeperTriggerFingerprintState{
		fingerprint: fingerprint,
		expiresAt:   now.Add(d.ttl),
	}
	return true, nil
}

func (d *inMemoryGatekeeperTriggerDeduper) RecordAbort(serviceID uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.abortCooldowns[serviceID] = gatekeeperAbortCooldownState{
		expiresAt: time.Now().Add(gatekeeperAbortCooldownTTL),
	}
}

func (d *redisGatekeeperTriggerDeduper) ShouldEnqueue(serviceID uuid.UUID, fingerprint string) (bool, error) {
	// Check abort cooldown first.
	cooldownKey := gatekeeperAbortCooldownKey(serviceID)
	exists, err := d.client.Exists(context.Background(), cooldownKey).Result()
	if err != nil {
		if d.log != nil {
			d.log.Warn("gatekeeper: abort cooldown check failed; proceeding", "error", err, "serviceId", serviceID)
		}
	} else if exists > 0 {
		return false, nil
	}

	result, err := compareAndStoreGatekeeperFingerprintScript.Run(
		context.Background(),
		d.client,
		[]string{gatekeeperTriggerFingerprintKey(serviceID)},
		fingerprint,
		d.ttl.Milliseconds(),
	).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (d *redisGatekeeperTriggerDeduper) RecordAbort(serviceID uuid.UUID) {
	key := gatekeeperAbortCooldownKey(serviceID)
	if err := d.client.Set(context.Background(), key, "1", gatekeeperAbortCooldownTTL).Err(); err != nil {
		if d.log != nil {
			d.log.Warn("gatekeeper: failed to record abort cooldown", "error", err, "serviceId", serviceID)
		}
	}
}

func gatekeeperAbortCooldownKey(serviceID uuid.UUID) string {
	return fmt.Sprintf("%s:abort-cooldown:%s", gatekeeperTriggerFingerprintPrefix, serviceID)
}

func gatekeeperTriggerFingerprintKey(serviceID uuid.UUID) string {
	return fmt.Sprintf("%s:%s", gatekeeperTriggerFingerprintPrefix, serviceID)
}

func buildGatekeeperTriggerFingerprint(ctx context.Context, repo gatekeeperTriggerFingerprintRepo, leadID, serviceID, tenantID uuid.UUID) (string, error) {
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
	attachments, err := repo.ListAttachmentsByService(ctx, serviceID, tenantID)
	if err != nil {
		return "", err
	}

	var photo *repository.PhotoAnalysis
	if current, photoErr := repo.GetLatestPhotoAnalysis(ctx, serviceID, tenantID); photoErr == nil {
		photo = &current
	} else if !errors.Is(photoErr, repository.ErrPhotoAnalysisNotFound) {
		return "", photoErr
	}

	visitReport, err := repo.GetLatestAppointmentVisitReportByService(ctx, serviceID, tenantID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return "", err
	}

	snapshot := gatekeeperTriggerSnapshot{
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
		Service: gatekeeperServiceSnapshot{
			ServiceType:         normalizeTriggerText(service.ServiceType),
			ConsumerNote:        normalizeOptionalTriggerText(service.ConsumerNote),
			Source:              normalizeOptionalTriggerText(service.Source),
			CustomerPreferences: normalizePreferencesJSON(service.CustomerPreferences),
		},
		Notes:       summarizeGatekeeperNotes(notes),
		Attachments: summarizeGatekeeperAttachments(attachments),
		Photo:       summarizeGatekeeperPhoto(photo),
		VisitReport: summarizeGatekeeperVisitReport(visitReport),
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func maybeEnqueueGatekeeperRun(request gatekeeperEnqueueRequest) bool {
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

func (r gatekeeperEnqueueRequest) buildFingerprint() string {
	if r.repo == nil {
		return ""
	}
	currentFingerprint, err := buildGatekeeperTriggerFingerprint(r.ctx, r.repo, r.leadID, r.serviceID, r.tenantID)
	if err != nil {
		if r.log != nil {
			r.log.Warn("gatekeeper: failed to build trigger fingerprint; enqueueing without semantic dedupe", "error", err, "leadId", r.leadID, "serviceId", r.serviceID, "source", r.source)
		}
		return ""
	}
	return currentFingerprint
}

func (r gatekeeperEnqueueRequest) shouldSkipDuplicateFingerprint(fingerprint string) bool {
	if fingerprint == "" || r.deduper == nil {
		return false
	}
	shouldEnqueue, dedupeErr := r.deduper.ShouldEnqueue(r.serviceID, fingerprint)
	if dedupeErr != nil {
		if r.log != nil {
			r.log.Warn("gatekeeper: semantic trigger dedupe failed; enqueueing anyway", "error", dedupeErr, "leadId", r.leadID, "serviceId", r.serviceID, "source", r.source)
		}
		return false
	}
	if shouldEnqueue {
		return false
	}
	if r.log != nil {
		r.log.Info("gatekeeper: unchanged input fingerprint; skipping enqueue", "leadId", r.leadID, "serviceId", r.serviceID, "source", r.source, "fingerprint", fingerprint[:12])
	}
	return true
}

func (r gatekeeperEnqueueRequest) enqueue(fingerprint string) {
	if err := r.queue.EnqueueGatekeeperRun(r.ctx, scheduler.GatekeeperRunPayload{
		TenantID:      r.tenantID.String(),
		LeadID:        r.leadID.String(),
		LeadServiceID: r.serviceID.String(),
		Fingerprint:   fingerprint,
	}); err != nil && r.log != nil {
		r.log.Error("gatekeeper queue enqueue failed", "error", err, "leadId", r.leadID, "serviceId", r.serviceID, "source", r.source)
	}
}

func summarizeGatekeeperNotes(notes []repository.LeadNote) []gatekeeperNoteSnapshot {
	if len(notes) == 0 {
		return nil
	}
	items := make([]gatekeeperNoteSnapshot, 0, len(notes))
	for _, note := range notes {
		items = append(items, gatekeeperNoteSnapshot{
			Type: normalizeTriggerText(note.Type),
			Body: normalizeTriggerText(note.Body),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Type != items[j].Type {
			return items[i].Type < items[j].Type
		}
		return items[i].Body < items[j].Body
	})
	return items
}

func summarizeGatekeeperAttachments(attachments []repository.Attachment) []gatekeeperAttachmentSummary {
	if len(attachments) == 0 {
		return nil
	}
	items := make([]gatekeeperAttachmentSummary, 0, len(attachments))
	for _, attachment := range attachments {
		items = append(items, gatekeeperAttachmentSummary{
			FileKey:     normalizeTriggerText(attachment.FileKey),
			FileName:    normalizeTriggerText(attachment.FileName),
			ContentType: normalizeOptionalTriggerText(attachment.ContentType),
			SizeBytes:   optionalInt64Value(attachment.SizeBytes),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].FileKey != items[j].FileKey {
			return items[i].FileKey < items[j].FileKey
		}
		if items[i].FileName != items[j].FileName {
			return items[i].FileName < items[j].FileName
		}
		if items[i].ContentType != items[j].ContentType {
			return items[i].ContentType < items[j].ContentType
		}
		return items[i].SizeBytes < items[j].SizeBytes
	})
	return items
}

func summarizeGatekeeperPhoto(photo *repository.PhotoAnalysis) *gatekeeperPhotoSnapshot {
	if photo == nil {
		return nil
	}
	observations := sortedNormalizedCopy(photo.Observations)
	needsOnsite := sortedNormalizedCopy(photo.NeedsOnsiteMeasurement)
	extractedText := sortedNormalizedCopy(photo.ExtractedText)
	return &gatekeeperPhotoSnapshot{
		Summary:                normalizeTriggerText(photo.Summary),
		ScopeAssessment:        normalizeTriggerText(photo.ScopeAssessment),
		ConfidenceLevel:        normalizeTriggerText(photo.ConfidenceLevel),
		PhotoCount:             photo.PhotoCount,
		Observations:           observations,
		NeedsOnsiteMeasurement: needsOnsite,
		ExtractedText:          extractedText,
	}
}

func summarizeGatekeeperVisitReport(report *repository.AppointmentVisitReport) *gatekeeperVisitReportSnapshot {
	if report == nil {
		return nil
	}
	return &gatekeeperVisitReportSnapshot{
		Measurements:     normalizeOptionalTriggerText(report.Measurements),
		AccessDifficulty: normalizeOptionalTriggerText(report.AccessDifficulty),
		Notes:            normalizeOptionalTriggerText(report.Notes),
	}
}

func normalizePreferencesJSON(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return string(trimmed)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return string(trimmed)
	}
	return string(canonical)
}

func normalizeTriggerText(value string) string {
	return strings.TrimSpace(value)
}

func normalizeOptionalTriggerText(value *string) string {
	if value == nil {
		return ""
	}
	return normalizeTriggerText(*value)
}

func normalizeUUIDPtr(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func optionalInt64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func sortedNormalizedCopy(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := normalizeTriggerText(value)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	sort.Strings(result)
	return result
}
