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
	"strconv"
	"strings"
	"time"

	"portal_final_backend/platform/config"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
)

const advisoryLockKey = int64(1234567890)

// RunMigrations applies all pending migrations using goose.
func RunMigrations(ctx context.Context, cfg config.DatabaseConfig, migrationsDir string) error {
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
	defer db.Close()

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
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, "SELECT pg_advisory_lock($1)", advisoryLockKey); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	return nil
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
	var gooseTableExists bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)
	`, goose.TableName()).Scan(&gooseTableExists); err != nil {
		return fmt.Errorf("check goose table: %w", err)
	}

	// Only seed versions when bootstrapping an existing database that didn't have
	// goose history yet. If the goose table already exists, new migration files
	// must be applied normally via goose.Up.
	createdGooseTable := false

	var hasExisting bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public'
			  AND table_name IN ('rac_users', 'rac_leads', 'rac_roles')
		)
	`).Scan(&hasExisting); err != nil {
		return fmt.Errorf("check existing tables: %w", err)
	}

	if !hasExisting {
		return nil
	}

	if !gooseTableExists {
		if _, err := goose.EnsureDBVersionContext(ctx, db); err != nil {
			return fmt.Errorf("ensure goose table: %w", err)
		}
		createdGooseTable = true
	}

	if !createdGooseTable {
		return nil
	}

	latest, err := latestMigrationVersion(dir)
	if err != nil {
		return err
	}
	if latest == 0 {
		return nil
	}

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

	missing := make([]int64, 0, len(migrations))
	for _, m := range migrations {
		if _, ok := existingSet[m.Version]; !ok {
			missing = append(missing, m.Version)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin seed tx: %w", err)
	}
	for _, v := range missing {
		if err := store.Insert(ctx, tx, database.InsertRequest{Version: v}); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("seed goose version %d: %w", v, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed tx: %w", err)
	}

	return nil
}

func latestMigrationVersion(dir string) (int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read migrations dir: %w", err)
	}

	prefixRe := regexp.MustCompile(`^(\d+)`)
	var maxVersion int64

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		match := prefixRe.FindStringSubmatch(name)
		if len(match) != 2 {
			continue
		}
		v, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			continue
		}
		if v > maxVersion {
			maxVersion = v
		}
	}

	return maxVersion, nil
}
