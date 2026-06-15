package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/db"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// deviceRef holds a WhatsApp device registration found in Postgres.
type deviceRef struct {
	scope          string // "organization" or "agent"
	organizationID uuid.UUID
	deviceID       string
}

type cleanupResult struct {
	processed   int
	kept        int
	cleared     int
	gowaDeleted int
	skipped     int
	failed      int
}

// cleanupEnv holds the dependencies and settings that are constant for every
// device reference processed in a single run.
type cleanupEnv struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	client  *whatsapp.Client
	log     *logger.Logger
	dryRun  bool
	delay   time.Duration
}

func main() {
	var dryRun bool
	var delayMs int
	flag.BoolVar(&dryRun, "dry-run", false, "log what would be cleaned without modifying the database or GoWA")
	flag.IntVar(&delayMs, "delay-ms", 500, "delay between GoWA status checks")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	log := logger.New(cfg.Env)
	log.Info("starting whatsapp device cleanup", "dryRun", dryRun)

	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		panic("failed to connect to database: " + err.Error())
	}
	defer pool.Close()

	waClient := whatsapp.NewClient(cfg, log)
	if waClient == nil {
		log.Warn("whatsapp service not configured, nothing to clean up")
		return
	}

	result := runCleanup(ctx, pool, waClient, log, dryRun, time.Duration(delayMs)*time.Millisecond)

	log.Info("whatsapp device cleanup completed",
		"processed", result.processed,
		"kept", result.kept,
		"cleared", result.cleared,
		"gowaDeleted", result.gowaDeleted,
		"skipped", result.skipped,
		"failed", result.failed,
	)
}

func runCleanup(ctx context.Context, pool *pgxpool.Pool, waClient *whatsapp.Client, log *logger.Logger, dryRun bool, delay time.Duration) cleanupResult {
	var result cleanupResult

	refs, err := listDeviceRefs(ctx, pool)
	if err != nil {
		log.Error("failed to list device refs", "error", err)
		return result
	}
	if len(refs) == 0 {
		log.Info("no device refs to evaluate")
		return result
	}

	log.Info("evaluating device refs", "count", len(refs))

	env := cleanupEnv{
		ctx:    ctx,
		pool:   pool,
		client: waClient,
		log:    log,
		dryRun: dryRun,
		delay:  delay,
	}

	for _, ref := range refs {
		result.processed++
		processRef(env, ref, &result)
	}

	return result
}

// processRef checks a single device reference against GoWA and clears it if it
// is stale. It mutates result to record the outcome.
func processRef(env cleanupEnv, ref deviceRef, result *cleanupResult) {
	defer func() { time.Sleep(env.delay) }()

	if ref.deviceID == "" {
		env.log.Debug("skipping empty device id", "scope", ref.scope, "organizationID", ref.organizationID)
		result.skipped++
		return
	}

	status, checkErr := env.client.GetDeviceStatus(env.ctx, ref.deviceID)
	if checkErr == nil {
		state := deviceState(status)
		env.log.Info("device is healthy, keeping", "deviceId", ref.deviceID, "scope", ref.scope, "state", state)
		result.kept++
		return
	}

	if !shouldClear(checkErr) {
		env.log.Warn("device status check failed with unexpected error, leaving alone",
			"deviceId", ref.deviceID, "scope", ref.scope, "error", checkErr)
		result.failed++
		return
	}

	reason := classifyError(checkErr)
	env.log.Info("device is stale, clearing", "deviceId", ref.deviceID, "scope", ref.scope, "reason", reason)

	if env.dryRun {
		result.cleared++
		return
	}

	// Best-effort delete from GoWA first; ignore errors because the device
	// may already be gone upstream.
	if delErr := env.client.DeleteDevice(env.ctx, ref.deviceID); delErr == nil {
		result.gowaDeleted++
	}

	if clearErr := clearDeviceRef(env.ctx, env.pool, ref); clearErr != nil {
		env.log.Error("failed to clear device ref from database",
			"deviceId", ref.deviceID, "scope", ref.scope, "error", clearErr)
		result.failed++
		return
	}

	result.cleared++
}

func deviceState(status *whatsapp.DeviceStatusResponse) string {
	if status.IsLoggedIn {
		return "logged_in"
	}
	if status.IsConnected {
		return "connected"
	}
	return "disconnected"
}

// shouldClear reports whether an upstream error means the local DB reference
// should be removed. We clear on "device not found" and "session deleted".
func shouldClear(err error) bool {
	if err == nil {
		return false
	}
	if apperr.Is(err, apperr.KindNotFound) {
		return true
	}
	if errors.Is(err, whatsapp.ErrSessionDeleted) {
		return true
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "session deleted") ||
		(strings.Contains(msg, "is not logged in") && strings.Contains(msg, "session")) ||
		strings.Contains(msg, "device not found") ||
		strings.Contains(msg, "device_not_found")
}

func classifyError(err error) string {
	if apperr.Is(err, apperr.KindNotFound) {
		return "device_not_found"
	}
	if errors.Is(err, whatsapp.ErrSessionDeleted) {
		return "session_deleted"
	}
	return "stale"
}

// listDeviceRefs returns all WhatsApp device references stored in Postgres.
// It scans both per-organization settings and the global agent config.
func listDeviceRefs(ctx context.Context, pool *pgxpool.Pool) ([]deviceRef, error) {
	rows, err := pool.Query(ctx, `
		SELECT 'organization' AS scope, organization_id AS id, whatsapp_device_id AS device_id
		FROM RAC_organization_settings
		WHERE whatsapp_device_id IS NOT NULL AND whatsapp_device_id <> ''
		UNION ALL
		SELECT 'agent' AS scope, id, device_id
		FROM RAC_whatsapp_agent_config
		WHERE device_id IS NOT NULL AND device_id <> ''
		ORDER BY scope, device_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []deviceRef
	for rows.Next() {
		var ref deviceRef
		var id uuid.UUID
		if err := rows.Scan(&ref.scope, &id, &ref.deviceID); err != nil {
			return nil, err
		}
		if ref.scope == "organization" {
			ref.organizationID = id
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

// clearDeviceRef removes the stale WhatsApp device reference from the database.
func clearDeviceRef(ctx context.Context, pool *pgxpool.Pool, ref deviceRef) error {
	switch ref.scope {
	case "organization":
		_, err := pool.Exec(ctx, `
			UPDATE RAC_organization_settings
			SET whatsapp_device_id = '',
			    whatsapp_account_jid = '',
			    updated_at = now()
			WHERE organization_id = $1
		`, ref.organizationID)
		return err
	case "agent":
		_, err := pool.Exec(ctx, `DELETE FROM RAC_whatsapp_agent_config WHERE device_id = $1`, ref.deviceID)
		return err
	default:
		return fmt.Errorf("unknown scope: %s", ref.scope)
	}
}
