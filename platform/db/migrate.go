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
	dir, err := resolveMigrationsDir(migrationsDir)
	if err != nil {
		return err
	}
	if dir == "" {
		return nil // empty input — nothing to do
	}

	files, err := collectSQLFiles(dir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}

	conn, err := pgx.Connect(ctx, cfg.GetDatabaseURL())
	if err != nil {
		return fmt.Errorf("connect for migrations: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if err := acquireAdvisoryLock(ctx, conn); err != nil {
		return err
	}
	defer releaseAdvisoryLock(ctx, conn)

	if err := ensureTrackingTable(ctx, conn); err != nil {
		return err
	}

	if err := bootstrapExistingDB(ctx, conn, dir, files); err != nil {
		return fmt.Errorf("bootstrap existing db: %w", err)
	}

	applied, err := loadAppliedMigrations(ctx, conn)
	if err != nil {
		return err
	}

	return applyPendingMigrations(ctx, conn, dir, files, applied)
}

// resolveMigrationsDir validates and resolves the migrations directory path.
func resolveMigrationsDir(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", nil
	}

	cleaned := filepath.Clean(dir)
	if !filepath.IsAbs(cleaned) {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			return "", fmt.Errorf("resolve migrations dir: %w", err)
		}
		cleaned = abs
	}

	stat, err := os.Stat(cleaned)
	if err != nil {
		return "", fmt.Errorf("migrations dir not found: %s", cleaned)
	}
	if !stat.IsDir() {
		return "", fmt.Errorf("migrations dir is not a directory: %s", cleaned)
	}

	return cleaned, nil
}

// collectSQLFiles reads a directory and returns sorted .sql filenames.
func collectSQLFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

const advisoryLockKey = 1234567890

// acquireAdvisoryLock obtains a PostgreSQL advisory lock for migration safety.
func acquireAdvisoryLock(ctx context.Context, conn *pgx.Conn) error {
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockKey); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	return nil
}

// releaseAdvisoryLock releases the PostgreSQL advisory lock.
func releaseAdvisoryLock(ctx context.Context, conn *pgx.Conn) {
	_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, advisoryLockKey)
}

// ensureTrackingTable creates or upgrades the _migrations tracking table.
func ensureTrackingTable(ctx context.Context, conn *pgx.Conn) error {
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

	return nil
}

// loadAppliedMigrations returns a map of filename → checksum for all applied migrations.
func loadAppliedMigrations(ctx context.Context, conn *pgx.Conn) (map[string]string, error) {
	rows, err := conn.Query(ctx, `SELECT filename, checksum FROM _migrations`)
	if err != nil {
		return nil, fmt.Errorf("query _migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]string)
	for rows.Next() {
		var name, cs string
		if err := rows.Scan(&name, &cs); err != nil {
			return nil, fmt.Errorf("scan _migrations row: %w", err)
		}
		applied[name] = cs
	}
	return applied, nil
}

// applyPendingMigrations applies all migrations not yet tracked in _migrations.
func applyPendingMigrations(ctx context.Context, conn *pgx.Conn, dir string, files []string, applied map[string]string) error {
	for _, f := range files {
		content, err := os.ReadFile(filepath.Join(dir, f))
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

		if err := applyMigration(ctx, conn, f, string(content), cs); err != nil {
			return err
		}
	}
	return nil
}

// applyMigration runs a single migration file inside a transaction and records it.
func applyMigration(ctx context.Context, conn *pgx.Conn, filename, sql, checksum string) error {
	slog.Info("applying migration", "file", filename)

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx for %s: %w", filename, err)
	}

	if _, err := tx.Exec(ctx, sql); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("execute migration %s: %w", filename, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO _migrations (filename, checksum) VALUES ($1, $2)`, filename, checksum); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("record migration %s: %w", filename, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", filename, err)
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
