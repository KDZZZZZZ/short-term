package pg

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrate applies every pending migration in fsys to the database named by
// dsn and reports the version in effect afterwards.
//
// Migrations are plain SQL files embedded in the owning service, named
// NNNNNN_description.up.sql with a matching .down.sql, per
// docs/backend-conventions.md. golang-migrate takes an advisory lock for the
// duration, so running this from several replicas at once is safe.
func Migrate(dsn string, fsys fs.FS, dir string) (version uint, err error) {
	source, err := iofs.New(fsys, dir)
	if err != nil {
		return 0, fmt.Errorf("pg: open migration source: %w", err)
	}

	migrator, err := migrate.NewWithSourceInstance("iofs", source, migratePgxDSN(dsn))
	if err != nil {
		return 0, fmt.Errorf("pg: open migrator: %w", err)
	}
	defer func() {
		sourceErr, dbErr := migrator.Close()
		err = errors.Join(err, sourceErr, dbErr)
	}()

	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return 0, fmt.Errorf("pg: apply migrations: %w", err)
	}

	version, dirty, err := migrator.Version()
	if err != nil {
		return 0, fmt.Errorf("pg: read schema version: %w", err)
	}
	if dirty {
		return version, fmt.Errorf("pg: schema version %d is dirty; repair it before starting", version)
	}
	return version, nil
}

// migratePgxDSN rewrites a libpq DSN into the URL scheme golang-migrate's pgx
// driver registers itself under.
func migratePgxDSN(dsn string) string {
	if len(dsn) > 11 && dsn[:11] == "postgres://" {
		return "pgx5://" + dsn[11:]
	}
	if len(dsn) > 13 && dsn[:13] == "postgresql://" {
		return "pgx5://" + dsn[13:]
	}
	return dsn
}

// ensure the pgx/v5 migrate driver is linked in; it registers itself in init.
var _ = migratepgx.Postgres{}
