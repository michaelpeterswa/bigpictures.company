package db

import (
	"errors"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
	"github.com/michaelpeterswa/bigpictures.company/migrations"
)

// Migrator drives golang-migrate against an embedded SQL source. Build one
// with [NewMigrator]; call Up/Down/Version/Force as needed.
type Migrator struct{ m *migrate.Migrate }

// NewMigrator constructs a Migrator using migrations embedded in the binary.
// The database URL must use a pgx5-compatible scheme; the function rewrites
// "postgres://" / "postgresql://" to "pgx5://" so callers can pass the same
// URL they use for runtime pgx access.
func NewMigrator(databaseURL string) (*Migrator, error) {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, problems.New(
			"db/migration-failed",
			"Could not open embedded migration source",
			err.Error(),
		)
	}
	rewrittenURL := rewriteSchemeForPGX5(databaseURL)
	m, err := migrate.NewWithSourceInstance("iofs", src, rewrittenURL)
	if err != nil {
		return nil, problems.New(
			"db/migration-failed",
			"Could not initialize migrator",
			err.Error(),
		)
	}
	return &Migrator{m: m}, nil
}

// Up applies all pending migrations. Returns nil if already at the latest
// version (golang-migrate's "no change" is treated as success).
func (m *Migrator) Up() error { return wrapMigrateErr(m.m.Up()) }

// Down rolls back all migrations. Useful for tests; destructive in production.
func (m *Migrator) Down() error { return wrapMigrateErr(m.m.Down()) }

// Steps applies n migrations forward (n>0) or backward (n<0).
func (m *Migrator) Steps(n int) error { return wrapMigrateErr(m.m.Steps(n)) }

// Force sets the current version to v without running any migrations and
// clears the dirty flag. Use to recover from a partially-applied migration.
func (m *Migrator) Force(v int) error { return wrapMigrateErr(m.m.Force(v)) }

// Version returns the current migration version and dirty flag, or
// (0, false, nil) when no migrations have ever been applied.
func (m *Migrator) Version() (uint, bool, error) {
	v, dirty, err := m.m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, problems.New(
			"db/migration-failed",
			"Could not read migration version",
			err.Error(),
		)
	}
	return v, dirty, nil
}

// Close releases the underlying source and database resources.
func (m *Migrator) Close() error {
	srcErr, dbErr := m.m.Close()
	if srcErr != nil {
		return problems.New("db/migration-failed", "Could not close migration source", srcErr.Error())
	}
	if dbErr != nil {
		return problems.New("db/migration-failed", "Could not close migration database", dbErr.Error())
	}
	return nil
}

func wrapMigrateErr(err error) error {
	if err == nil || errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return problems.New(
		"db/migration-failed",
		"Migration failed",
		err.Error(),
	)
}

// rewriteSchemeForPGX5 maps postgres:// or postgresql:// URLs to pgx5:// so
// the golang-migrate pgx5 driver recognizes them. Anything else is returned
// unchanged.
func rewriteSchemeForPGX5(url string) string {
	for _, scheme := range []string{"postgres://", "postgresql://"} {
		if rest, ok := strings.CutPrefix(url, scheme); ok {
			return "pgx5://" + rest
		}
	}
	return url
}

