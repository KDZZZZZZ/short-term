// Command server runs the Account Service gRPC server.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	"github.com/KDZZZZZZ/short-term/platform/auth"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/platform/logging"
	"github.com/KDZZZZZZ/short-term/platform/observability"
	"github.com/KDZZZZZZ/short-term/platform/pg"
	grpcadapter "github.com/KDZZZZZZ/short-term/services/account/internal/adapter/grpc"
	"github.com/KDZZZZZZ/short-term/services/account/internal/adapter/postgres"
	"github.com/KDZZZZZZ/short-term/services/account/internal/adapter/system"
	"github.com/KDZZZZZZ/short-term/services/account/internal/application"
	"github.com/KDZZZZZZ/short-term/services/account/internal/config"
	"github.com/KDZZZZZZ/short-term/services/account/migrations"
)

func main() {
	if err := run(); err != nil {
		// The logger may not exist yet when configuration fails, so the last
		// resort is stderr.
		fmt.Fprintf(os.Stderr, "account: %v\n", err)
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

	hasher, err := auth.NewHasher(cfg.Argon2)
	if err != nil {
		return err
	}
	ids := system.NewIDs()
	issuer, err := auth.NewIssuer(cfg.Token, nil, func() string { return ids.New() })
	if err != nil {
		return err
	}

	app, err := application.NewService(
		postgres.NewAccountRepository(pool),
		hasher,
		issuer,
		ids,
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
	accountv1.RegisterAccountServiceServer(server, grpcadapter.NewServer(app))

	return grpcx.Serve(ctx, server, cfg.GRPCAddr, logger)
}
