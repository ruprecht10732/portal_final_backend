package adapters

import (
	"context"
	"encoding/json"
	"time"

	"portal_final_backend/internal/quotes/repository"

	"github.com/google/uuid"
)

// QuoteActivityWriterAdapter implements notification.QuoteActivityWriter
// by delegating to the quotes repository.
type QuoteActivityWriterAdapter struct {
	repo *repository.Repository
}

// NewQuoteActivityWriter creates a new adapter that persists quote activity to the DB.
func NewQuoteActivityWriter(repo *repository.Repository) *QuoteActivityWriterAdapter {
	return &QuoteActivityWriterAdapter{repo: repo}
}

// CreateActivity persists a single quote activity record.
func (a *QuoteActivityWriterAdapter) CreateActivity(ctx context.Context, quoteID, orgID uuid.UUID, eventType, message string, metadata map[string]interface{}) error {
	var metaJSON []byte
	if metadata != nil {
		var err error
		metaJSON, err = json.Marshal(metadata)
		if err != nil {
			metaJSON = nil
		}
	}

	activity := &repository.QuoteActivity{
		ID:             uuid.New(),
		QuoteID:        quoteID,
		OrganizationID: orgID,
		EventType:      eventType,
		Message:        message,
		Metadata:       metaJSON,
		CreatedAt:      time.Now(),
	}
	return a.repo.CreateActivity(ctx, activity)
}
