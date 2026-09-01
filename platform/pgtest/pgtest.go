// Package pgtest gives integration tests a real, isolated PostgreSQL database.
//
// docs/software-design.md section 10 requires the transaction, concurrency and
// idempotency evidence to come from a real database rather than a mock, and
// tests must be able to run in parallel without seeing each other's rows. Each
// call to New creates a fresh database, applies the caller's migrations and
// drops the database when the test ends.
//
// This package imports testing on purpose and must only be imported from test
// files.
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

// DSNEnvVar names the environment variable holding an administrative
// PostgreSQL URL, for example
// postgres://postgres:postgres@127.0.0.1:5432/postgres. The account must be
// allowed to CREATE DATABASE.
//
// deploy/local/docker-compose.yml provides a suitable instance locally, and
// the backend CI workflow provides one for every pull request.
const DSNEnvVar = "SHORTTERM_TEST_POSTGRES_DSN"

// AdminDSN returns the administrative DSN, or skips the test when the
// environment does not provide a database.
func AdminDSN(t *testing.T) string {
	t.Helper()

	dsn := os.Getenv(DSNEnvVar)
	if dsn == "" {
		t.Skipf("%s is not set; run `docker compose -f deploy/local/docker-compose.yml up -d` to enable database tests", DSNEnvVar)
	}
	return dsn
}

// New creates an isolated database, applies the migrations found under dir in
// fsys, and returns a pool connected to it. The database is dropped when the
// test finishes.
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

// dropDatabase removes the test database, forcing open sessions to close so a
// leaked connection cannot leave databases behind.
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

// withDatabase rewrites the database name in a PostgreSQL URL.
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
