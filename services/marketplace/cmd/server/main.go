// Command server 运行 Marketplace Service gRPC 服务端。
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
	grpcadapter "github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/grpc"
	messageadapter "github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/messaging"
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

	ids := system.NewIDs()
	clock := system.Clock{}
	productRepo := postgres.NewProductRepository(pool)
	messagingConn, err := grpcx.Dial(grpcx.ClientOptions{
		Target: cfg.MessagingTarget, Caller: config.ServiceName, DefaultTimeout: cfg.DownstreamTimeout,
	})
	if err != nil {
		return err
	}
	defer func() { _ = messagingConn.Close() }()

	products, err := application.NewProductService(productRepo, objects, ids, clock, logger)
	if err != nil {
		return err
	}
	trades, err := application.NewTradeService(
		postgres.NewTradeRepository(pool),
		productRepo,
		messageadapter.NewVerifier(messagingv1.NewMessagingServiceClient(messagingConn)),
		ids,
		clock,
		logger,
	)
	if err != nil {
		return err
	}
	comments, err := application.NewCommentService(postgres.NewCommentRepository(pool), ids, clock)
	if err != nil {
		return err
	}
	tradeReviews, err := application.NewTradeReviewService(
		postgres.NewTradeRepository(pool),
		postgres.NewTradeReviewRepository(pool),
		ids,
		clock,
	)
	if err != nil {
		return err
	}

	server := grpcx.NewServer(grpcx.ServerOptions{
		Logger:         logger,
		HandlerTimeout: cfg.HandlerTimeout,
	})
	grpcx.RegisterHealthServer(server, pool.Ping)
	marketplacev1.RegisterMarketplaceServiceServer(server, grpcadapter.NewServer(products, trades, comments, tradeReviews))

	logger.Info("media store ready", slog.String("root", objects.Root()), slog.String("public_url", cfg.MediaPublicURL))
	return grpcx.Serve(ctx, server, cfg.GRPCAddr, logger)
}
