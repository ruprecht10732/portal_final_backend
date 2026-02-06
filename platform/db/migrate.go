// Package db provides database connection infrastructure.
// This is part of the platform layer and contains no business logic.
package db

import (
	"context"
	"errors"
	"strings"

	"portal_final_backend/platform/config"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// RunMigrations applies all pending migrations from the provided directory.
func RunMigrations(_ context.Context, cfg config.DatabaseConfig, migrationsDir string) error {
	if strings.TrimSpace(migrationsDir) == "" {
		return nil
	}

	m, err := migrate.New("file://"+migrationsDir, cfg.GetDatabaseURL())
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}
