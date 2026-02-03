package main

import (
	"context"
	"errors"
	"time"

	"portal_final_backend/internal/adapters"
	"portal_final_backend/internal/energylabel"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/db"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type leadEnergyAddress struct {
	id       uuid.UUID
	tenantID uuid.UUID
	street   string
	house    string
	zip      string
	city     string
}

type energyLabelUpdater interface {
	UpdateEnergyLabel(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params repository.UpdateEnergyLabelParams) error
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	log := logger.New(cfg.Env)
	log.Info("starting energy label backfill")

	if !cfg.IsEnergyLabelEnabled() {
		log.Warn("energy label module disabled, skipping backfill")
		return
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		panic("failed to connect to database: " + err.Error())
	}
	defer pool.Close()

	energyModule := energylabel.NewModule(cfg, log)
	if !energyModule.IsEnabled() {
		log.Warn("energy label service not available, skipping backfill")
		return
	}

	enricher := adapters.NewEnergyLabelAdapter(energyModule.Service())
	if enricher == nil {
		log.Warn("energy label enricher unavailable, skipping backfill")
		return
	}

	repo := repository.New(pool)

	const batchSize = 25
	const delayBetweenCalls = 500 * time.Millisecond

	var processed int
	var succeeded int

	for {
		leads, err := listLeadsMissingEnergyLabel(ctx, pool, batchSize)
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

			if !isAddressValid(lead) {
				log.Info("skipping lead with invalid address", "leadId", lead.id, "tenantId", lead.tenantID)
				continue
			}

			if err := backfillLeadEnergyLabel(ctx, repo, enricher, lead, log); err != nil {
				log.Error("failed to backfill energy label", "leadId", lead.id, "tenantId", lead.tenantID, "error", err)
				// Back off slightly on failure to avoid hammering the API
				time.Sleep(time.Second)
				continue
			}

			succeeded++
			time.Sleep(delayBetweenCalls)
		}
	}

	log.Info("energy label backfill completed", "processed", processed, "updated", succeeded)
}

func listLeadsMissingEnergyLabel(ctx context.Context, pool *pgxpool.Pool, limit int) ([]leadEnergyAddress, error) {
	rows, err := pool.Query(ctx, `
        SELECT id, organization_id, address_street, address_house_number, address_zip_code, address_city
        FROM leads
        WHERE deleted_at IS NULL
          AND energy_label_fetched_at IS NULL
          AND address_zip_code <> ''
          AND address_house_number <> ''
        ORDER BY created_at ASC
        LIMIT $1
    `, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	leads := make([]leadEnergyAddress, 0)
	for rows.Next() {
		var lead leadEnergyAddress
		if err := rows.Scan(&lead.id, &lead.tenantID, &lead.street, &lead.house, &lead.zip, &lead.city); err != nil {
			return nil, err
		}
		leads = append(leads, lead)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return leads, nil
}

func isAddressValid(lead leadEnergyAddress) bool {
	if lead.zip == "" || lead.house == "" {
		return false
	}
	if lead.zip == "0000XX" || lead.street == "Unknown" || lead.city == "Unknown" {
		return false
	}
	return true
}

func backfillLeadEnergyLabel(parentCtx context.Context, repo energyLabelUpdater, enricher ports.EnergyLabelEnricher, lead leadEnergyAddress, log *logger.Logger) error {
	if enricher == nil {
		return errors.New("energy label enricher not configured")
	}

	// Use a timeout per lead to avoid hanging on slow API calls
	ctx, cancel := context.WithTimeout(parentCtx, 15*time.Second)
	defer cancel()

	data, err := enricher.EnrichLead(ctx, ports.EnrichLeadParams{
		Postcode:   lead.zip,
		Huisnummer: lead.house,
	})
	if err != nil {
		return err
	}

	fetchedAt := time.Now().UTC()

	var classPtr *string
	var indexPtr *float64
	var bouwjaarPtr *int
	var gebouwtypePtr *string
	var validUntilPtr *time.Time
	var registeredPtr *time.Time
	var primairPtr *float64
	var bagPtr *string

	if data != nil {
		if data.Energieklasse != "" {
			val := data.Energieklasse
			classPtr = &val
		}
		if data.EnergieIndex != nil {
			val := *data.EnergieIndex
			indexPtr = &val
		}
		if data.Bouwjaar != 0 {
			val := data.Bouwjaar
			bouwjaarPtr = &val
		}
		if data.Gebouwtype != "" {
			val := data.Gebouwtype
			gebouwtypePtr = &val
		}
		if data.GeldigTot != nil {
			val := *data.GeldigTot
			validUntilPtr = &val
		}
		if data.Registratiedatum != nil {
			val := *data.Registratiedatum
			registeredPtr = &val
		}
		if data.PrimaireFossieleEnergie != nil {
			val := *data.PrimaireFossieleEnergie
			primairPtr = &val
		}
		if data.BAGVerblijfsobjectID != "" {
			val := data.BAGVerblijfsobjectID
			bagPtr = &val
		}
	}

	params := repository.UpdateEnergyLabelParams{
		Class:          classPtr,
		Index:          indexPtr,
		Bouwjaar:       bouwjaarPtr,
		Gebouwtype:     gebouwtypePtr,
		ValidUntil:     validUntilPtr,
		RegisteredAt:   registeredPtr,
		PrimairFossiel: primairPtr,
		BAGObjectID:    bagPtr,
		FetchedAt:      fetchedAt,
	}

	if err := repo.UpdateEnergyLabel(ctx, lead.id, lead.tenantID, params); err != nil {
		return err
	}

	if data == nil {
		log.Info("no energy label found", "leadId", lead.id, "tenantId", lead.tenantID)
	} else {
		log.Info("energy label updated", "leadId", lead.id, "tenantId", lead.tenantID, "class", data.Energieklasse)
	}

	return nil
}
