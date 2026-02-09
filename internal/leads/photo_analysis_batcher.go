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

	triggerImmediate := false

	b.mu.Lock()
	batch, exists := b.batches[serviceID]
	if !exists {
		batch = &photoAnalysisBatch{}
		b.batches[serviceID] = batch
		triggerImmediate = true
	}
	batch.leadID = leadID
	batch.tenantID = tenantID

	if batch.timer != nil {
		batch.timer.Stop()
	}

	batch.timer = time.AfterFunc(b.window, func() {
		b.triggerBatch(serviceID)
	})
	b.mu.Unlock()

	if triggerImmediate {
		b.handler.RunAutoAnalysis(leadID, serviceID, tenantID)
	}
}

func (b *photoAnalysisBatcher) triggerBatch(serviceID uuid.UUID) {
	b.mu.Lock()
	batch, ok := b.batches[serviceID]
	if !ok {
		b.mu.Unlock()
		return
	}
	leadID := batch.leadID
	tenantID := batch.tenantID
	delete(b.batches, serviceID)
	b.mu.Unlock()

	b.handler.RunAutoAnalysis(leadID, serviceID, tenantID)
}

func isImageContentType(contentType string) bool {
	return strings.HasPrefix(contentType, "image/")
}
