// Package pg 保存各服务共享的 PostgreSQL 基础设施：连接池构造、追踪、事务辅助函数
// 和迁移执行。
//
// 每个服务拥有自己的数据库和数据库账户（docs/software-design.md 第 9.2 节），
// 因此这里不涉及任何 schema 或表，只负责打开连接、追踪并提交工作。
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

// PoolOptions 配置连接池。
type PoolOptions struct {
	// DSN 是 libpq 连接字符串，属于密钥，绝不能记录到日志中。
	DSN string
	// MaxConns 限制连接池大小。零值使用 pgx 默认值。
	MaxConns int32
	// ConnectTimeout 限制初始连通性检查。
	ConnectTimeout time.Duration
}

// NewPool 打开连接池并在返回前验证连通性，使错误配置的数据库在启动时失败，
// 而不是等到第一个请求到达时才失败。
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

// Tx 是仓储使用的 pgx 子集。接受此接口后，仓储方法既可以在连接池上运行，
// 也可以在已打开的事务中运行。
type Tx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// InTx 在事务中运行 fn，成功时提交，出现任何错误或 panic 时回滚。
//
// 同时修改 Product 和 Trade 的命令依赖这一点：部分执行的动作绝不能对外可见
// （docs/state-machines.md，事务边界）。
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
			// 在仍然有效的上下文中回滚，否则已取消的请求会让事务保持打开，
			// 直到连接池回收它。
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

// IsUniqueViolation 判断 err 是否为 PostgreSQL 唯一约束冲突，也可以限定为指定约束。
// 仓储用它将数据库已经串行化的竞争转换为领域错误。
func IsUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	return constraint == "" || pgErr.ConstraintName == constraint
}

// IsNoRows 判断 err 是否表示查询没有匹配任何记录。
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
