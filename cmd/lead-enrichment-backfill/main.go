package main

import (
	"context"
	"errors"
	"time"

	"portal_final_backend/internal/adapters"
	"portal_final_backend/internal/leadenrichment"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/scoring"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/db"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type leadAddress struct {
	id        uuid.UUID
	tenantID  uuid.UUID
	zip       string
	house     string
	createdAt time.Time
}

type leadEnrichmentUpdater interface {
	UpdateLeadEnrichment(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params repository.UpdateLeadEnrichmentParams) error
	UpdateLeadScore(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params repository.UpdateLeadScoreParams) error
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	log := logger.New(cfg.Env)
	log.Info("starting lead enrichment backfill")

	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		panic("failed to connect to database: " + err.Error())
	}
	defer pool.Close()

	enrichmentModule := leadenrichment.NewModule(log)
	enricher := adapters.NewLeadEnrichmentAdapter(enrichmentModule.Service())
	if enricher == nil {
		log.Warn("lead enrichment adapter unavailable, skipping backfill")
		return
	}

	repo := repository.New(pool)
	scorer := scoring.New(repo, log)

	const batchSize = 50
	const delayBetweenCalls = 300 * time.Millisecond

	var processed int
	var succeeded int

	cursorTime := time.Time{}
	cursorID := uuid.Nil

	for {
		leads, err := listLeads(ctx, pool, batchSize, cursorTime, cursorID)
		if err != nil {
			log.Error("failed to list leads", "error", err)
			break
		}
		if len(leads) == 0 {
			log.Info("no leads left to backfill", "processed", processed, "updated", succeeded)
			break
		}

		for _, lead := range leads {
			processed++
			cursorTime = lead.createdAt
			cursorID = lead.id

			if !isAddressValid(lead) {
				log.Info("skipping lead with invalid address", "leadId", lead.id, "tenantId", lead.tenantID)
				continue
			}

			if err := backfillLead(ctx, repo, scorer, enricher, lead, log); err != nil {
				log.Error("failed to backfill lead enrichment", "leadId", lead.id, "tenantId", lead.tenantID, "error", err)
				time.Sleep(time.Second)
				continue
			}

			succeeded++
			time.Sleep(delayBetweenCalls)
		}
	}

	log.Info("lead enrichment backfill completed", "processed", processed, "updated", succeeded)
}

func listLeads(ctx context.Context, pool *pgxpool.Pool, limit int, cursorTime time.Time, cursorID uuid.UUID) ([]leadAddress, error) {
	rows, err := pool.Query(ctx, `
    SELECT id, organization_id, address_zip_code, address_house_number, created_at
    FROM leads
    WHERE deleted_at IS NULL
      AND address_zip_code <> ''
      AND address_house_number <> ''
      AND (created_at > $1 OR (created_at = $1 AND id > $2))
    ORDER BY created_at ASC, id ASC
    LIMIT $3
  `, cursorTime, cursorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	leads := make([]leadAddress, 0)
	for rows.Next() {
		var lead leadAddress
		if err := rows.Scan(&lead.id, &lead.tenantID, &lead.zip, &lead.house, &lead.createdAt); err != nil {
			return nil, err
		}
		leads = append(leads, lead)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return leads, nil
}

func isAddressValid(lead leadAddress) bool {
	if lead.zip == "" || lead.house == "" {
		return false
	}
	if lead.zip == "0000XX" {
		return false
	}
	return true
}

func backfillLead(parentCtx context.Context, repo leadEnrichmentUpdater, scorer *scoring.Service, enricher ports.LeadEnricher, lead leadAddress, log *logger.Logger) error {
	if scorer == nil {
		return errors.New("lead scorer not configured")
	}

	ctx, cancel := context.WithTimeout(parentCtx, 20*time.Second)
	defer cancel()

	data, err := enricher.EnrichLead(ctx, lead.zip)
	if err != nil {
		return err
	}

	scoreResult, err := scorer.Recalculate(ctx, lead.id, nil, lead.tenantID, true)
	if err != nil {
		return err
	}

	if data != nil {
		fetchedAt := time.Now().UTC()
		updateParams := repository.UpdateLeadEnrichmentParams{
			Source:                    toPtr(data.Source),
			Postcode6:                 toPtr(data.Postcode6),
			Postcode4:                 toPtr(data.Postcode4),
			Buurtcode:                 toPtr(data.Buurtcode),
			DataYear:                  data.DataYear,
			GemAardgasverbruik:        data.GemAardgasverbruik,
			GemElektriciteitsverbruik: data.GemElektriciteitsverbruik,
			HuishoudenGrootte:         data.HuishoudenGrootte,
			KoopwoningenPct:           data.KoopwoningenPct,
			BouwjaarVanaf2000Pct:      data.BouwjaarVanaf2000Pct,
			WOZWaarde:                 data.WOZWaarde,
			MediaanVermogenX1000:      data.MediaanVermogenX1000,
			GemInkomen:                data.GemInkomenHuishouden,
			PctHoogInkomen:            data.PctHoogInkomen,
			PctLaagInkomen:            data.PctLaagInkomen,
			HuishoudensMetKinderenPct: data.HuishoudensMetKinderenPct,
			Stedelijkheid:             data.Stedelijkheid,
			Confidence:                data.Confidence,
			FetchedAt:                 fetchedAt,
			Score:                     &scoreResult.Score,
			ScorePreAI:                &scoreResult.ScorePreAI,
			ScoreFactors:              scoreResult.FactorsJSON,
			ScoreVersion:              toPtr(scoreResult.Version),
			ScoreUpdatedAt:            &scoreResult.UpdatedAt,
		}

		if err := repo.UpdateLeadEnrichment(ctx, lead.id, lead.tenantID, updateParams); err != nil {
			return err
		}

		log.Info("lead enrichment updated", "leadId", lead.id, "tenantId", lead.tenantID, "score", scoreResult.Score)
		return nil
	}

	if err := repo.UpdateLeadScore(ctx, lead.id, lead.tenantID, repository.UpdateLeadScoreParams{
		Score:          &scoreResult.Score,
		ScorePreAI:     &scoreResult.ScorePreAI,
		ScoreFactors:   scoreResult.FactorsJSON,
		ScoreVersion:   toPtr(scoreResult.Version),
		ScoreUpdatedAt: scoreResult.UpdatedAt,
	}); err != nil {
		return err
	}

	log.Info("lead score updated without enrichment", "leadId", lead.id, "tenantId", lead.tenantID, "score", scoreResult.Score)
	return nil
}

func toPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
