// Package db provides database connection infrastructure.
// This is part of the platform layer and contains no business logic.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"portal_final_backend/platform/config"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
)

const advisoryLockKey = int64(1234567890)
const advisoryLockWait = 30 * time.Second
const advisoryLockPollInterval = 250 * time.Millisecond

// RunMigrations applies all pending migrations using goose.
func RunMigrations(ctx context.Context, cfg config.DatabaseConfig, migrationsDir string) (retErr error) {
	if strings.TrimSpace(migrationsDir) == "" {
		return nil
	}

	dir, err := resolveMigrationsDir(migrationsDir)
	if err != nil {
		return err
	}

	db, err := sql.Open("pgx", cfg.GetDatabaseURL())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil && retErr == nil {
			retErr = fmt.Errorf("close database: %w", closeErr)
		}
	}()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	if err := acquireAdvisoryLock(ctx, db); err != nil {
		return err
	}
	defer releaseAdvisoryLock(ctx, db)

	if err := bootstrapGoose(ctx, db, dir); err != nil {
		return err
	}

	if err := goose.UpContext(ctx, db, dir); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}

	return nil
}

func acquireAdvisoryLock(ctx context.Context, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, advisoryLockWait)
	defer cancel()

	ticker := time.NewTicker(advisoryLockPollInterval)
	defer ticker.Stop()
	loggedWaiting := false

	for {
		locked, err := tryAdvisoryLock(ctx, db)
		if err != nil {
			return fmt.Errorf("acquire advisory lock: %w", err)
		}
		if locked {
			if loggedWaiting {
				fmt.Printf("migrations: acquired advisory lock after waiting\n")
			}
			return nil
		}
		if !loggedWaiting {
			fmt.Printf("migrations: advisory lock busy, waiting up to %s\n", advisoryLockWait)
			loggedWaiting = true
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("acquire advisory lock timed out after %s: %w", advisoryLockWait, ctx.Err())
		case <-ticker.C:
		}
	}
}

func tryAdvisoryLock(ctx context.Context, db *sql.DB) (bool, error) {
	var locked bool
	err := db.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", advisoryLockKey).Scan(&locked)
	if err != nil {
		return false, err
	}
	return locked, nil
}

func releaseAdvisoryLock(ctx context.Context, db *sql.DB) {
	_, _ = db.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", advisoryLockKey)
}

func resolveMigrationsDir(dir string) (string, error) {
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

func bootstrapGoose(ctx context.Context, db *sql.DB, dir string) error {
	gooseTableExists, err := gooseTableAlreadyExists(ctx, db)
	if err != nil {
		return err
	}
	if gooseTableExists {
		return nil
	}

	hasExisting, err := hasExistingSchema(ctx, db)
	if err != nil {
		return err
	}
	if !hasExisting {
		return nil
	}

	if _, err := goose.EnsureDBVersionContext(ctx, db); err != nil {
		return fmt.Errorf("ensure goose table: %w", err)
	}

	return seedGooseHistoryForExistingDB(ctx, db, dir)
}

func gooseTableAlreadyExists(ctx context.Context, db *sql.DB) (bool, error) {
	var exists bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)
	`, goose.TableName()).Scan(&exists); err != nil {
		return false, fmt.Errorf("check goose table: %w", err)
	}
	return exists, nil
}

func hasExistingSchema(ctx context.Context, db *sql.DB) (bool, error) {
	var hasExisting bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public'
			  AND table_name IN ('rac_users', 'rac_leads', 'rac_roles')
		)
	`).Scan(&hasExisting); err != nil {
		return false, fmt.Errorf("check existing tables: %w", err)
	}
	return hasExisting, nil
}

func seedGooseHistoryForExistingDB(ctx context.Context, db *sql.DB, dir string) error {
	store, err := database.NewStore(database.DialectPostgres, goose.TableName())
	if err != nil {
		return fmt.Errorf("init goose store: %w", err)
	}

	migrations, err := goose.CollectMigrations(dir, 0, goose.MaxVersion)
	if err != nil {
		if err == goose.ErrNoMigrationFiles {
			return nil
		}
		return fmt.Errorf("collect migrations: %w", err)
	}
	if len(migrations) == 0 {
		return nil
	}

	existing, err := store.ListMigrations(ctx, db)
	if err != nil {
		return fmt.Errorf("list goose migrations: %w", err)
	}

	existingSet := make(map[int64]struct{}, len(existing))
	for _, m := range existing {
		if m.IsApplied {
			existingSet[m.Version] = struct{}{}
		}
	}

	// IMPORTANT:
	// This database already has schema, but has no goose history. We must NOT
	// blindly mark all migrations as applied (that would skip new migrations).
	// Instead, seed only those migrations that appear already applied by checking
	// for the presence of tables/columns referenced in the migration's UP SQL.
	// Remaining migrations are left unapplied and will be executed by goose.Up.
	return seedAppliedMigrations(ctx, db, store, migrations, existingSet)
}

func seedAppliedMigrations(
	ctx context.Context,
	db *sql.DB,
	store database.Store,
	migrations goose.Migrations,
	existingSet map[int64]struct{},
) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin seed tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, m := range migrations {
		if _, ok := existingSet[m.Version]; ok {
			continue
		}

		shouldSeed, _, err := shouldSeedMigration(ctx, db, m)
		if err != nil {
			return fmt.Errorf("evaluate migration %d for seeding: %w", m.Version, err)
		}
		if !shouldSeed {
			continue
		}

		if err := store.Insert(ctx, tx, database.InsertRequest{Version: m.Version}); err != nil {
			return fmt.Errorf("seed goose version %d: %w", m.Version, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed tx: %w", err)
	}
	return nil
}

type migrationRequirement struct {
	Table  string
	Column string // optional
}

func shouldSeedMigration(ctx context.Context, db *sql.DB, m *goose.Migration) (bool, string, error) {
	upSQL, reason, err := loadUpSQLFromMigration(m)
	if err != nil {
		return false, reason, err
	}
	if reason != "" {
		return true, reason, nil
	}
	return requirementsSatisfied(ctx, db, upSQL)
}

func loadUpSQLFromMigration(m *goose.Migration) (string, string, error) {
	// For non-SQL migrations (or missing source), default to seeding. This avoids
	// rerunning potentially non-idempotent data migrations on existing databases.
	if m == nil || m.Type != goose.TypeSQL || strings.TrimSpace(m.Source) == "" {
		return "", "non-sql-or-no-source", nil
	}

	contents, err := os.ReadFile(m.Source)
	if err != nil {
		return "", "read-failed", fmt.Errorf("read migration source: %w", err)
	}

	upSQL := string(contents)
	if idx := strings.Index(upSQL, "-- +goose Down"); idx >= 0 {
		upSQL = upSQL[:idx]
	}

	return upSQL, "", nil
}

func requirementsSatisfied(ctx context.Context, db *sql.DB, upSQL string) (bool, string, error) {
	reqs := parseMigrationRequirements(upSQL)
	if len(reqs) == 0 {
		// No detectable schema change; assume already applied on existing DB.
		return true, "no-detectable-requirements", nil
	}

	for _, r := range reqs {
		ok, reason, err := requirementSatisfied(ctx, db, r)
		if err != nil {
			return false, reason, err
		}
		if !ok {
			return false, reason, nil
		}
	}

	return true, "requirements-present", nil
}

func requirementSatisfied(ctx context.Context, db *sql.DB, r migrationRequirement) (bool, string, error) {
	if strings.TrimSpace(r.Table) == "" {
		return true, "empty-table", nil
	}
	exists, err := tableExists(ctx, db, r.Table)
	if err != nil {
		return false, "table-exists-check-failed", err
	}
	if !exists {
		return false, "table-missing", nil
	}
	if strings.TrimSpace(r.Column) == "" {
		return true, "table-present", nil
	}
	colExists, err := columnExists(ctx, db, r.Table, r.Column)
	if err != nil {
		return false, "column-exists-check-failed", err
	}
	if !colExists {
		return false, "column-missing", nil
	}
	return true, "column-present", nil
}

func parseMigrationRequirements(sqlText string) []migrationRequirement {
	text := strings.TrimSpace(sqlText)
	if text == "" {
		return nil
	}

	createTableRe := regexp.MustCompile(`(?im)\bcreate\s+table\s+(?:if\s+not\s+exists\s+)?([\w\."]+)`)
	alterAddColumnRe := regexp.MustCompile(`(?im)\balter\s+table\s+([\w\."]+)\s+add\s+column\s+(?:if\s+not\s+exists\s+)?([\w\."]+)`)

	seen := make(map[string]struct{})
	var reqs []migrationRequirement

	for _, match := range createTableRe.FindAllStringSubmatch(text, -1) {
		table := submatch(match, 1)
		appendUniqueRequirement(&reqs, seen, migrationRequirement{Table: normalizeIdent(table)})
	}

	for _, match := range alterAddColumnRe.FindAllStringSubmatch(text, -1) {
		table := submatch(match, 1)
		column := submatch(match, 2)
		appendUniqueRequirement(&reqs, seen, migrationRequirement{Table: normalizeIdent(table), Column: normalizeIdent(column)})
	}

	return reqs
}

func submatch(match []string, idx int) string {
	if idx < 0 || idx >= len(match) {
		return ""
	}
	return match[idx]
}

func appendUniqueRequirement(reqs *[]migrationRequirement, seen map[string]struct{}, r migrationRequirement) {
	if r.Table == "" {
		return
	}
	key := r.Table + "|" + r.Column
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*reqs = append(*reqs, r)
}

func normalizeIdent(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, `"`)
	if s == "" {
		return ""
	}
	if strings.Contains(s, ".") {
		parts := strings.Split(s, ".")
		s = parts[len(parts)-1]
		s = strings.Trim(s, `"`)
	}
	return strings.ToLower(strings.TrimSpace(s))
}

func tableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)
	`, normalizeIdent(table)).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check table exists (%s): %w", table, err)
	}
	return exists, nil
}

func columnExists(ctx context.Context, db *sql.DB, table string, column string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
		)
	`, normalizeIdent(table), normalizeIdent(column)).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check column exists (%s.%s): %w", table, column, err)
	}
	return exists, nil
}
