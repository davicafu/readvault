//go:build integration

package postgres

import "github.com/golang-migrate/migrate/v4"

// MigrateTo migrates to one version, so a test can seed data the way an
// older schema held it before running the next migrations.
func MigrateTo(databaseURL string, version uint) error {
	return runMigration(databaseURL, func(m *migrate.Migrate) error { return m.Migrate(version) })
}
