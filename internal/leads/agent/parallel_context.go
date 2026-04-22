package agent

import (
	"context"
	"log"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"portal_final_backend/internal/leads/repository"
)

// leadContext holds the result of parallel lead data fetching.
type leadContext struct {
	lead    repository.Lead
	service repository.LeadService
	notes   []repository.LeadNote
	photo   *repository.PhotoAnalysis
}

// fetchLeadContextParallel fetches lead, service, notes, and photo analysis
// concurrently using errgroup, reducing latency for the quoting agent's
// initial data-gathering phase.
func fetchLeadContextParallel(ctx context.Context, repo repository.LeadsRepository, leadID, serviceID, tenantID uuid.UUID) leadContext {
	var result leadContext
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		lead, err := repo.GetByID(gctx, leadID, tenantID)
		if err != nil {
			return err
		}
		result.lead = lead
		return nil
	})

	g.Go(func() error {
		service, err := repo.GetLeadServiceByID(gctx, serviceID, tenantID)
		if err != nil {
			return err
		}
		result.service = service
		return nil
	})

	g.Go(func() error {
		notes, err := repo.ListNotesByService(gctx, leadID, serviceID, tenantID)
		if err != nil {
			log.Printf("quoting-agent: notes fetch failed: %v", err)
			// Non-fatal: notes are optional enrichment
		}
		result.notes = notes
		return nil
	})

	g.Go(func() error {
		if analysis, err := repo.GetLatestPhotoAnalysis(gctx, serviceID, tenantID); err == nil {
			result.photo = &analysis
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		log.Printf("quoting-agent: parallel context fetch failed: %v", err)
	}
	return result
}
