package pg

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrate 将 fsys 中所有待执行的迁移应用到 dsn 指定的数据库，并返回应用后的版本。
//
// 迁移是嵌入所属服务的普通 SQL 文件，按照 docs/backend-conventions.md 命名为
// NNNNNN_description.up.sql，并配有对应的 .down.sql。golang-migrate 在执行期间
// 获取咨询锁，因此可以安全地同时从多个副本运行。
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

// migratePgxDSN 将 libpq DSN 改写为 golang-migrate 的 pgx 驱动注册使用的 URL scheme。
func migratePgxDSN(dsn string) string {
	if len(dsn) > 11 && dsn[:11] == "postgres://" {
		return "pgx5://" + dsn[11:]
	}
	if len(dsn) > 13 && dsn[:13] == "postgresql://" {
		return "pgx5://" + dsn[13:]
	}
	return dsn
}

// 确保链接 pgx/v5 迁移驱动；它会在 init 中自行注册。
var _ = migratepgx.Postgres{}
