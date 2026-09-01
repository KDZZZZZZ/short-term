// Package pg holds the PostgreSQL plumbing shared by the services: pool
// construction, tracing, the transaction helper and migration execution.
//
// Each service owns its own database and database account
// (docs/software-design.md section 9.2), so nothing here knows about schemas
// or tables; it only knows how to open, trace and commit work.
package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolOptions configures a connection pool.
type PoolOptions struct {
	// DSN is the libpq connection string. It is a secret: never log it.
	DSN string
	// MaxConns bounds the pool. Zero uses the pgx default.
	MaxConns int32
	// ConnectTimeout bounds the initial connectivity check.
	ConnectTimeout time.Duration
}

// NewPool opens a pool and verifies connectivity before returning, so a
// misconfigured database fails at startup rather than on the first request.
func NewPool(ctx context.Context, opts PoolOptions) (*pgxpool.Pool, error) {
	if opts.DSN == "" {
		return nil, errors.New("pg: DSN is required")
	}
	if opts.ConnectTimeout <= 0 {
		opts.ConnectTimeout = 10 * time.Second
	}

	cfg, err := pgxpool.ParseConfig(opts.DSN)
	if err != nil {
		return nil, fmt.Errorf("pg: parse DSN: %w", err)
	}
	if opts.MaxConns > 0 {
		cfg.MaxConns = opts.MaxConns
	}
	cfg.ConnConfig.Tracer = NewQueryTracer()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pg: create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, opts.ConnectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pg: ping: %w", err)
	}
	return pool, nil
}

// Tx is the subset of pgx used by repositories. Accepting this interface lets
// a repository method run either on the pool or inside an open transaction.
type Tx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// InTx runs fn inside a transaction, committing on success and rolling back on
// any error or panic.
//
// The commands that change Product and Trade together depend on this: a
// partially applied action must never become visible
// (docs/state-machines.md, transaction boundaries).
func InTx(ctx context.Context, pool *pgxpool.Pool, opts pgx.TxOptions, fn func(pgx.Tx) error) (err error) {
	tx, err := pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("pg: begin: %w", err)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(recovered)
		}
		if err != nil {
			// Roll back on a context that is still live, otherwise a cancelled
			// request would leave the transaction open until the pool reaps it.
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("pg: commit: %w", err)
	}
	return nil
}

// IsUniqueViolation reports whether err is a PostgreSQL unique constraint
// violation, optionally restricted to a named constraint. Repositories use it
// to turn a race that the database already serialised into a domain error.
func IsUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	return constraint == "" || pgErr.ConstraintName == constraint
}

// IsNoRows reports whether err means the query matched nothing.
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
