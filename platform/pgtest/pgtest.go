// Package pgtest 为集成测试提供真实且隔离的 PostgreSQL 数据库。
//
// docs/software-design.md 第 10 节要求事务、并发和幂等性证据来自真实数据库而非
// mock，并且测试必须能够并行运行而不会看到彼此的行。每次调用 New 都会创建一个
// 新数据库，应用调用方的迁移，并在测试结束时删除该数据库。
//
// 本包有意导入 testing，因此只能从测试文件导入。
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/platform/pg"
)

// DSNEnvVar 是保存管理员 PostgreSQL URL 的环境变量名，例如
// postgres://postgres:postgres@127.0.0.1:5432/postgres。该账户必须有权执行
// CREATE DATABASE。
//
// deploy/local/docker-compose.yml 在本地提供合适的实例，后端 CI 工作流则为每个
// pull request 提供一个实例。
const DSNEnvVar = "SHORTTERM_TEST_POSTGRES_DSN"

// AdminDSN 返回管理员 DSN；当环境没有提供数据库时跳过测试。
func AdminDSN(t *testing.T) string {
	t.Helper()

	dsn := os.Getenv(DSNEnvVar)
	if dsn == "" {
		t.Skipf("%s is not set; run `docker compose -f deploy/local/docker-compose.yml up -d` to enable database tests", DSNEnvVar)
	}
	return dsn
}

// New 创建隔离数据库，应用 fsys 的 dir 目录下的迁移，并返回连接到该数据库的连接池。
// 测试结束时删除该数据库。
func New(t *testing.T, fsys fs.FS, dir string) *pgxpool.Pool {
	t.Helper()

	adminDSN := AdminDSN(t)
	ctx := t.Context()

	name := "st_test_" + randomSuffix(t)
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("pgtest: connect as admin: %v", err)
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("pgtest: create database %s: %v", name, err)
	}
	if err := admin.Close(ctx); err != nil {
		t.Fatalf("pgtest: close admin connection: %v", err)
	}

	t.Cleanup(func() { dropDatabase(t, adminDSN, name) })

	dsn, err := withDatabase(adminDSN, name)
	if err != nil {
		t.Fatalf("pgtest: %v", err)
	}
	if _, err := pg.Migrate(dsn, fsys, dir); err != nil {
		t.Fatalf("pgtest: migrate %s: %v", name, err)
	}

	pool, err := pg.NewPool(ctx, pg.PoolOptions{DSN: dsn, MaxConns: 8})
	if err != nil {
		t.Fatalf("pgtest: open pool on %s: %v", name, err)
	}
	t.Cleanup(pool.Close)

	return pool
}

// dropDatabase 删除测试数据库，并强制关闭打开的会话，避免泄漏的连接遗留数据库。
func dropDatabase(t *testing.T, adminDSN, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Logf("pgtest: reconnect to drop %s: %v", name, err)
		return
	}
	defer func() { _ = admin.Close(ctx) }()

	if _, err := admin.Exec(ctx, `DROP DATABASE IF EXISTS `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`); err != nil {
		t.Logf("pgtest: drop %s: %v", name, err)
	}
}

// withDatabase 重写 PostgreSQL URL 中的数据库名称。
func withDatabase(dsn, name string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", DSNEnvVar, err)
	}
	if !strings.HasPrefix(parsed.Scheme, "postgres") {
		return "", fmt.Errorf("%s must be a postgres:// URL", DSNEnvVar)
	}
	parsed.Path = "/" + name
	return parsed.String(), nil
}

func randomSuffix(t *testing.T) string {
	t.Helper()

	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatalf("pgtest: read random suffix: %v", err)
	}
	return hex.EncodeToString(raw[:])
}
