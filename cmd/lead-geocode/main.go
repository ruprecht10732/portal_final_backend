package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"portal_final_backend/internal/maps"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/db"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type leadAddress struct {
	id          uuid.UUID
	street      string
	houseNumber string
	zipCode     string
	city        string
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	log := logger.New(cfg.Env)
	log.Info("starting lead geocode backfill")

	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		panic("failed to connect to database: " + err.Error())
	}
	defer pool.Close()

	mapsService := maps.NewService(log)

	const batchSize = 25
	for {
		leads, err := listLeadsMissingCoordinates(ctx, pool, batchSize)
		if err != nil {
			log.Error("failed to list leads", "error", err)
			return
		}
		if len(leads) == 0 {
			log.Info("no leads left to geocode")
			return
		}

		progress := false

		for _, lead := range leads {
			if lead.street == "Unknown" || lead.city == "Unknown" || lead.zipCode == "0000XX" {
				log.Info("skipping invalid address", "leadId", lead.id)
				continue
			}

			address := fmt.Sprintf("%s %s, %s %s", lead.street, lead.houseNumber, lead.zipCode, lead.city)
			suggestions, err := mapsService.SearchAddress(ctx, address)
			if err != nil {
				log.Error("geocode failed", "leadId", lead.id, "error", err)
				time.Sleep(time.Second)
				continue
			}
			if len(suggestions) == 0 {
				log.Info("no geocode result", "leadId", lead.id, "address", address)
				time.Sleep(time.Second)
				continue
			}

			lat, err := strconv.ParseFloat(suggestions[0].Lat, 64)
			if err != nil {
				log.Error("invalid latitude", "leadId", lead.id, "value", suggestions[0].Lat)
				time.Sleep(time.Second)
				continue
			}
			lon, err := strconv.ParseFloat(suggestions[0].Lon, 64)
			if err != nil {
				log.Error("invalid longitude", "leadId", lead.id, "value", suggestions[0].Lon)
				time.Sleep(time.Second)
				continue
			}

			if err := updateLeadCoordinates(ctx, pool, lead.id, lat, lon); err != nil {
				log.Error("failed to update lead", "leadId", lead.id, "error", err)
				time.Sleep(time.Second)
				continue
			}

			log.Info("lead geocoded", "leadId", lead.id, "lat", lat, "lon", lon)
			progress = true
			time.Sleep(time.Second)
		}

		if !progress {
			log.Info("no geocode progress in batch, stopping")
			return
		}
	}
}

func listLeadsMissingCoordinates(ctx context.Context, pool *pgxpool.Pool, limit int) ([]leadAddress, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, address_street, address_house_number, address_zip_code, address_city
		FROM leads
		WHERE deleted_at IS NULL
		  AND (latitude IS NULL OR longitude IS NULL)
		ORDER BY created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	leads := make([]leadAddress, 0)
	for rows.Next() {
		var lead leadAddress
		if err := rows.Scan(&lead.id, &lead.street, &lead.houseNumber, &lead.zipCode, &lead.city); err != nil {
			return nil, err
		}
		leads = append(leads, lead)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return leads, nil
}

func updateLeadCoordinates(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, lat float64, lon float64) error {
	_, err := pool.Exec(ctx, `
		UPDATE leads
		SET latitude = $2, longitude = $3, updated_at = now()
		WHERE id = $1
	`, id, lat, lon)
	return err
}
