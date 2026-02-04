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

	runGeocodeBackfill(ctx, pool, mapsService, log)
}

func runGeocodeBackfill(ctx context.Context, pool *pgxpool.Pool, mapsService *maps.Service, log *logger.Logger) {
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
			if geocodeLead(ctx, pool, mapsService, lead, log) {
				progress = true
			}
		}

		if !progress {
			log.Info("no geocode progress in batch, stopping")
			return
		}
	}
}

func geocodeLead(ctx context.Context, pool *pgxpool.Pool, mapsService *maps.Service, lead leadAddress, log *logger.Logger) bool {
	if isInvalidAddress(lead) {
		log.Info("skipping invalid address", "leadId", lead.id)
		return false
	}

	address := fmt.Sprintf("%s %s, %s %s", lead.street, lead.houseNumber, lead.zipCode, lead.city)
	suggestions, err := mapsService.SearchAddress(ctx, address)
	if err != nil {
		log.Error("geocode failed", "leadId", lead.id, "error", err)
		time.Sleep(time.Second)
		return false
	}
	if len(suggestions) == 0 {
		log.Info("no geocode result", "leadId", lead.id, "address", address)
		time.Sleep(time.Second)
		return false
	}

	lat, err := parseCoordinate("latitude", suggestions[0].Lat, lead.id, log)
	if err != nil {
		time.Sleep(time.Second)
		return false
	}
	long, err := parseCoordinate("longitude", suggestions[0].Lon, lead.id, log)
	if err != nil {
		time.Sleep(time.Second)
		return false
	}

	if err := updateLeadCoordinates(ctx, pool, lead.id, lat, long); err != nil {
		log.Error("failed to update lead", "leadId", lead.id, "error", err)
		time.Sleep(time.Second)
		return false
	}

	log.Info("lead geocoded", "leadId", lead.id, "lat", lat, "lon", long)
	time.Sleep(time.Second)
	return true
}

func isInvalidAddress(lead leadAddress) bool {
	return lead.street == "Unknown" || lead.city == "Unknown" || lead.zipCode == "0000XX"
}

func parseCoordinate(kind string, value string, leadID uuid.UUID, log *logger.Logger) (float64, error) {
	coordinate, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Error("invalid "+kind, "leadId", leadID, "value", value)
		return 0, err
	}
	return coordinate, nil
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
