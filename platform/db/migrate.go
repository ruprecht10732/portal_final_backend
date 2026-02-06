// Package db provides database connection infrastructure.
// This is part of the platform layer and contains no business logic.
package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"portal_final_backend/platform/config"

	"github.com/jackc/pgx/v5"
)

// RunMigrations applies all pending .sql migrations from the provided directory.
// It uses a _migrations tracking table, an advisory lock for concurrency safety,
// and SHA-256 checksums to detect drift in already-applied files.
func RunMigrations(ctx context.Context, cfg config.DatabaseConfig, migrationsDir string) error {
	if strings.TrimSpace(migrationsDir) == "" {
		return nil
	}

	cleaned := filepath.Clean(migrationsDir)
	if !filepath.IsAbs(cleaned) {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			return fmt.Errorf("resolve migrations dir: %w", err)
		}
		cleaned = abs
	}

	if stat, err := os.Stat(cleaned); err != nil || !stat.IsDir() {
		if err == nil {
			return fmt.Errorf("migrations dir is not a directory: %s", cleaned)
		}
		return fmt.Errorf("migrations dir not found: %s", cleaned)
	}

	// Collect .sql files sorted by name.
	entries, err := os.ReadDir(cleaned)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil
	}

	conn, err := pgx.Connect(ctx, cfg.GetDatabaseURL())
	if err != nil {
		return fmt.Errorf("connect for migrations: %w", err)
	}
	defer conn.Close(ctx)

	// Acquire an advisory lock so only one process runs migrations at a time.
	// The key 1234567890 is arbitrary but must be consistent across instances.
	const advisoryLockKey = 1234567890
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockKey); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, advisoryLockKey)
	}()

	// Ensure tracking table exists.
	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS _migrations (
			filename   TEXT PRIMARY KEY,
			checksum   TEXT NOT NULL DEFAULT '',
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create _migrations table: %w", err)
	}

	// Add checksum column if upgrading from earlier version of this table.
	_, _ = conn.Exec(ctx, `ALTER TABLE _migrations ADD COLUMN IF NOT EXISTS checksum TEXT NOT NULL DEFAULT ''`)

	// One-time bootstrap: if the DB was previously managed by another tool
	// (e.g. golang-migrate) and already has application tables, seed all
	// current files as applied to avoid re-running them.
	if err := bootstrapExistingDB(ctx, conn, cleaned, files); err != nil {
		return fmt.Errorf("bootstrap existing db: %w", err)
	}

	// Fetch already-applied filenames + checksums.
	rows, err := conn.Query(ctx, `SELECT filename, checksum FROM _migrations`)
	if err != nil {
		return fmt.Errorf("query _migrations: %w", err)
	}
	applied := make(map[string]string) // filename -> checksum
	for rows.Next() {
		var name, cs string
		if err := rows.Scan(&name, &cs); err != nil {
			rows.Close()
			return fmt.Errorf("scan _migrations row: %w", err)
		}
		applied[name] = cs
	}
	rows.Close()

	// Apply pending migrations in order; warn on checksum drift.
	for _, f := range files {
		content, err := os.ReadFile(filepath.Join(cleaned, f))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", f, err)
		}
		cs := sha256sum(content)

		if existingCS, ok := applied[f]; ok {
			if existingCS != "" && existingCS != cs {
				slog.Warn("migration checksum mismatch — file was modified after it was applied",
					"file", f, "expected", existingCS, "actual", cs)
			}
			continue
		}

		slog.Info("applying migration", "file", f)

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", f, err)
		}
		if _, err := tx.Exec(ctx, string(content)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("execute migration %s: %w", f, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO _migrations (filename, checksum) VALUES ($1, $2)`, f, cs); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", f, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", f, err)
		}
	}

	return nil
}

// bootstrapExistingDB seeds the _migrations table when switching from another
// migration tool (or manual DDL) to this runner. It only runs the INSERT loop
// when the table has fewer entries than there are files AND the DB already has
// known application tables.
func bootstrapExistingDB(ctx context.Context, conn *pgx.Conn, dir string, files []string) error {
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM _migrations`).Scan(&count); err != nil {
		return err
	}
	if count >= len(files) {
		return nil // fully seeded already
	}

	// Only auto-seed if the DB already has application tables.
	var hasExisting bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public'
			  AND table_name IN ('schema_migrations', 'rac_users', 'rac_leads', 'rac_roles')
		)
	`).Scan(&hasExisting); err != nil {
		return err
	}
	if !hasExisting {
		return nil // fresh database — run everything from scratch
	}

	slog.Info("bootstrapping _migrations table from existing database", "files", len(files), "already_tracked", count)

	for _, f := range files {
		content, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return fmt.Errorf("read %s for checksum: %w", f, err)
		}
		cs := sha256sum(content)
		if _, err := conn.Exec(ctx,
			`INSERT INTO _migrations (filename, checksum) VALUES ($1, $2) ON CONFLICT (filename) DO UPDATE SET checksum = EXCLUDED.checksum WHERE _migrations.checksum = ''`,
			f, cs,
		); err != nil {
			return fmt.Errorf("seed _migrations for %s: %w", f, err)
		}
	}

	return nil
}

func sha256sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
