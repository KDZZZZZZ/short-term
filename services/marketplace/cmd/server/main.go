// Command server runs the Marketplace Service gRPC server.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/platform/logging"
	"github.com/KDZZZZZZ/short-term/platform/observability"
	"github.com/KDZZZZZZ/short-term/platform/pg"
	grpcadapter "github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/grpc"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/objectstore"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/postgres"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/system"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/config"
	"github.com/KDZZZZZZ/short-term/services/marketplace/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "marketplace: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(os.Stdout, logging.Options{
		Service:     cfg.Runtime.Service,
		Environment: cfg.Runtime.Environment,
		Level:       cfg.Runtime.LogLevel,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := observability.Setup(ctx, observability.Options{
		Service:      cfg.Runtime.Service,
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

	objects, err := objectstore.NewFilesystem(cfg.MediaRoot, cfg.MediaPublicURL)
	if err != nil {
		return err
	}

	products, err := application.NewProductService(
		postgres.NewProductRepository(pool),
		objects,
		system.NewIDs(),
		system.Clock{},
		logger,
	)
	if err != nil {
		return err
	}

	server := grpcx.NewServer(grpcx.ServerOptions{
		Logger:         logger,
		HandlerTimeout: cfg.HandlerTimeout,
	})
	marketplacev1.RegisterMarketplaceServiceServer(server, grpcadapter.NewServer(products))

	logger.Info("media store ready", slog.String("root", objects.Root()), slog.String("public_url", cfg.MediaPublicURL))
	return grpcx.Serve(ctx, server, cfg.GRPCAddr, logger)
}
