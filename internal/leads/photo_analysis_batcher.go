package leads

import (
	"strings"
	"sync"
	"time"

	"portal_final_backend/internal/leads/handler"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

type photoAnalysisBatch struct {
	leadID   uuid.UUID
	tenantID uuid.UUID
	timer    *time.Timer
	running  bool
	lastSeen time.Time
}

type photoAnalysisBatcher struct {
	mu      sync.Mutex
	window  time.Duration
	batches map[uuid.UUID]*photoAnalysisBatch
	handler *handler.PhotoAnalysisHandler
	log     *logger.Logger
}

func newPhotoAnalysisBatcher(handler *handler.PhotoAnalysisHandler, window time.Duration, log *logger.Logger) *photoAnalysisBatcher {
	return &photoAnalysisBatcher{
		window:  window,
		batches: make(map[uuid.UUID]*photoAnalysisBatch),
		handler: handler,
		log:     log,
	}
}

func (b *photoAnalysisBatcher) OnImageUploaded(leadID, serviceID, tenantID uuid.UUID) {
	if b.handler == nil {
		return
	}

	b.mu.Lock()
	batch, exists := b.batches[serviceID]
	if !exists {
		batch = &photoAnalysisBatch{}
		b.batches[serviceID] = batch
	}
	batch.leadID = leadID
	batch.tenantID = tenantID
	batch.lastSeen = time.Now().UTC()

	if batch.timer != nil {
		batch.timer.Stop()
	}

	batch.timer = time.AfterFunc(b.window, func() {
		b.triggerBatch(serviceID)
	})

	if b.log != nil {
		b.log.Debug("photo batcher: queued image upload",
			"serviceId", serviceID,
			"leadId", leadID,
			"tenantId", tenantID,
			"running", batch.running,
			"window", b.window.String(),
		)
	}
	b.mu.Unlock()
}

func (b *photoAnalysisBatcher) triggerBatch(serviceID uuid.UUID) {
	b.mu.Lock()
	batch, ok := b.batches[serviceID]
	if !ok {
		b.mu.Unlock()
		return
	}

	if batch.running {
		b.mu.Unlock()
		return
	}

	now := time.Now().UTC()
	quietFor := now.Sub(batch.lastSeen)
	if quietFor < b.window {
		waitFor := b.window - quietFor
		if batch.timer != nil {
			batch.timer.Stop()
		}
		batch.timer = time.AfterFunc(waitFor, func() {
			b.triggerBatch(serviceID)
		})
		b.mu.Unlock()
		return
	}

	batch.running = true
	runStartedAt := now
	leadID := batch.leadID
	tenantID := batch.tenantID
	batch.timer = nil
	b.mu.Unlock()

	b.handler.RunAutoAnalysis(leadID, serviceID, tenantID)

	b.mu.Lock()
	defer b.mu.Unlock()
	current, stillPresent := b.batches[serviceID]
	if !stillPresent {
		return
	}

	current.running = false
	hasNewUploads := current.lastSeen.After(runStartedAt)
	if !hasNewUploads {
		delete(b.batches, serviceID)
		if b.log != nil {
			b.log.Debug("photo batcher: analysis cycle complete", "serviceId", serviceID, "leadId", leadID, "tenantId", tenantID)
		}
		return
	}

	now = time.Now().UTC()
	quietFor = now.Sub(current.lastSeen)
	waitFor := b.window - quietFor
	if waitFor < 0 {
		waitFor = 0
	}
	if current.timer != nil {
		current.timer.Stop()
	}
	current.timer = time.AfterFunc(waitFor, func() {
		b.triggerBatch(serviceID)
	})

	if b.log != nil {
		b.log.Debug("photo batcher: scheduled follow-up analysis cycle",
			"serviceId", serviceID,
			"leadId", current.leadID,
			"tenantId", current.tenantID,
			"waitFor", waitFor.String(),
		)
	}
}

func isImageContentType(contentType string) bool {
	return strings.HasPrefix(contentType, "image/")
}
