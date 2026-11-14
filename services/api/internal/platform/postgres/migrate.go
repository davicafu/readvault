package postgres

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the pgx5:// driver
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"readvault/api/migrations"
)

// DevOwnerID is the fixed development owner seeded by the first migration.
// It stands in for the authenticated user until authentication exists.
const DevOwnerID = "00000000-0000-0000-0000-000000000001"

// MigrateUp applies every pending migration. It is a no-op when the schema is
// current. golang-migrate holds an advisory lock, so concurrent runs are safe.
func MigrateUp(databaseURL string) error {
	return runMigration(databaseURL, func(m *migrate.Migrate) error { return m.Up() })
}

// MigrateDown rolls back the most recent migration.
func MigrateDown(databaseURL string) error {
	return runMigration(databaseURL, func(m *migrate.Migrate) error { return m.Steps(-1) })
}

func runMigration(databaseURL string, step func(*migrate.Migrate) error) (err error) {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("loading migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, pgx5URL(databaseURL))
	if err != nil {
		return fmt.Errorf("connecting for migrations: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		err = errors.Join(err, srcErr, dbErr)
	}()

	if err := step(m); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running migrations: %w", err)
	}
	return nil
}

// pgx5URL maps a postgres:// URL to the scheme the migrate pgx/v5 driver expects.
func pgx5URL(databaseURL string) string {
	for _, prefix := range []string{"postgres://", "postgresql://"} {
		if rest, ok := strings.CutPrefix(databaseURL, prefix); ok {
			return "pgx5://" + rest
		}
	}
	return databaseURL
}
