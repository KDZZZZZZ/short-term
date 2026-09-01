// Command worker publishes committed Messaging Outbox events.
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
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/adapter/event"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/adapter/postgres"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/application"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "messaging worker: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, logging.Options{
		Service: cfg.Runtime.Service + "-worker", Environment: cfg.Runtime.Environment, Level: cfg.Runtime.LogLevel,
	})
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownTracing, err := observability.Setup(ctx, observability.Options{
		Service: cfg.Runtime.Service + "-worker", Environment: cfg.Runtime.Environment,
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
	pool, err := pg.NewPool(ctx, pg.PoolOptions{DSN: cfg.DatabaseURL, MaxConns: cfg.MaxDBConns})
	if err != nil {
		return err
	}
	defer pool.Close()
	outbox, err := application.NewOutboxService(
		postgres.NewOutboxRepository(pool), event.NewLogPublisher(logger), cfg.OutboxBatchSize, logger,
	)
	if err != nil {
		return err
	}
	return outbox.Run(ctx, cfg.OutboxInterval)
}
