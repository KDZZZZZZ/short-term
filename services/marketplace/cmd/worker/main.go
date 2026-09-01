// Command worker 发布 Marketplace Outbox 中的事件。
//
// 它与服务端使用同一个镜像，但通过不同的启动命令运行；
// 这正是 docs/software-design.md 第 9.2 节描述的部署形态。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/KDZZZZZZ/short-term/platform/logging"
	"github.com/KDZZZZZZ/short-term/platform/observability"
	"github.com/KDZZZZZZ/short-term/platform/pg"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/event"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/postgres"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "marketplace worker: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(os.Stdout, logging.Options{
		Service:     cfg.Runtime.Service + "-worker",
		Environment: cfg.Runtime.Environment,
		Level:       cfg.Runtime.LogLevel,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := observability.Setup(ctx, observability.Options{
		Service:      cfg.Runtime.Service + "-worker",
		Environment:  cfg.Runtime.Environment,
		OTLPEndpoint: cfg.Runtime.OTLPEndpoint,
	})
	if err != nil {
		return err
	}
	defer func() {
		if err := shutdownTracing(context.WithoutCancel(ctx)); err != nil {
			logger.Warn("tracer shutdown failed", slog.String("error", err.Error()))
		}
	}()

	// Worker 永不执行迁移：schema 变更属于发布步骤，让 worker 与服务端竞争执行迁移
	// 只会引入一个多余的进程。
	pool, err := pg.NewPool(ctx, pg.PoolOptions{DSN: cfg.DatabaseURL, MaxConns: cfg.MaxDBConns})
	if err != nil {
		return err
	}
	defer pool.Close()

	outbox, err := application.NewOutboxService(
		postgres.NewOutboxRepository(pool),
		event.NewLogPublisher(logger),
		cfg.OutboxBatchSize,
		logger,
	)
	if err != nil {
		return err
	}

	logger.Info("outbox worker started",
		slog.Duration("interval", cfg.OutboxInterval),
		slog.Int("batch_size", int(cfg.OutboxBatchSize)),
	)
	return outbox.Run(ctx, cfg.OutboxInterval)
}
