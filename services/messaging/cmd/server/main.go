// Command server runs the Messaging Service gRPC server.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	messagingv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/messaging/v1"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/platform/logging"
	"github.com/KDZZZZZZ/short-term/platform/observability"
	"github.com/KDZZZZZZ/short-term/platform/pg"
	grpcadapter "github.com/KDZZZZZZ/short-term/services/messaging/internal/adapter/grpc"
	marketadapter "github.com/KDZZZZZZ/short-term/services/messaging/internal/adapter/marketplace"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/adapter/postgres"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/adapter/system"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/application"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/config"
	"github.com/KDZZZZZZ/short-term/services/messaging/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "messaging: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, logging.Options{
		Service: cfg.Runtime.Service, Environment: cfg.Runtime.Environment, Level: cfg.Runtime.LogLevel,
	})
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := observability.Setup(ctx, observability.Options{
		Service: cfg.Runtime.Service, Environment: cfg.Runtime.Environment, OTLPEndpoint: cfg.Runtime.OTLPEndpoint,
	})
	if err != nil {
		return err
	}
	defer func() {
		if err := shutdownTracing(context.WithoutCancel(ctx)); err != nil {
			logger.Warn("tracer shutdown failed", slog.String("error", err.Error()))
		}
	}()

	if cfg.AutoMigrate {
		version, err := pg.Migrate(cfg.DatabaseURL, migrations.FS, migrations.Dir)
		if err != nil {
			return err
		}
		logger.Info("database migrated", slog.Uint64("schema_version", uint64(version)))
	}
	pool, err := pg.NewPool(ctx, pg.PoolOptions{DSN: cfg.DatabaseURL, MaxConns: cfg.MaxDBConns})
	if err != nil {
		return err
	}
	defer pool.Close()

	marketplaceConn, err := grpcx.Dial(grpcx.ClientOptions{
		Target: cfg.MarketplaceTarget, Caller: config.ServiceName, DefaultTimeout: cfg.DownstreamTimeout,
	})
	if err != nil {
		return err
	}
	defer func() { _ = marketplaceConn.Close() }()

	service, err := application.NewService(
		postgres.NewRepository(pool),
		marketadapter.NewProductReader(marketplacev1.NewMarketplaceServiceClient(marketplaceConn)),
		system.NewIDs(), system.Clock{}, logger,
	)
	if err != nil {
		return err
	}
	server := grpcx.NewServer(grpcx.ServerOptions{Logger: logger, HandlerTimeout: cfg.HandlerTimeout})
	grpcx.RegisterHealthServer(server, pool.Ping)
	messagingv1.RegisterMessagingServiceServer(server, grpcadapter.NewServer(service))
	return grpcx.Serve(ctx, server, cfg.GRPCAddr, logger)
}
