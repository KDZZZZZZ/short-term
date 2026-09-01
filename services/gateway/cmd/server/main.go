// Command server 运行 API Gateway，它是唯一暴露到公网的单元。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/KDZZZZZZ/short-term/platform/auth"
	"github.com/KDZZZZZZ/short-term/platform/logging"
	"github.com/KDZZZZZZ/short-term/platform/observability"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/application/aggregation"
	grpcclient "github.com/KDZZZZZZ/short-term/services/gateway/internal/client/grpc"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/config"
	gatewayhttp "github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/handler"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/middleware"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "gateway: %v\n", err)
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

	verifier, err := auth.NewVerifier(cfg.Token, nil)
	if err != nil {
		return err
	}

	clients, err := grpcclient.Dial(cfg.Targets, cfg.Runtime.Service, cfg.DownstreamTimeout)
	if err != nil {
		return err
	}
	defer func() {
		if err := clients.Close(); err != nil {
			logger.Warn("closing downstream connections failed", slog.String("error", err.Error()))
		}
	}()

	aggregator := aggregation.New(
		clients.Account,
		clients.Marketplace,
		aggregation.NewGRPCFavorites(clients.Favorite),
	)

	responder := gatewayhttp.NewResponder(logger)
	rateLimiter, err := middleware.NewRateLimiter(middleware.RateLimitConfig{
		Window:            cfg.RateLimitWindow,
		Register:          cfg.RateLimitRegister,
		Login:             cfg.RateLimitLogin,
		Message:           cfg.RateLimitMessage,
		Upload:            cfg.RateLimitUpload,
		Trade:             cfg.RateLimitTrade,
		MaxKeys:           cfg.RateLimitMaxKeys,
		TrustProxyHeaders: cfg.TrustProxyHeaders,
	}, responder)
	if err != nil {
		return err
	}
	httpMetrics := middleware.NewHTTPMetrics()
	router := gatewayhttp.NewRouter(gatewayhttp.RouterOptions{
		BasePath:     cfg.BasePath,
		Verifier:     verifier,
		MaxBodyBytes: cfg.MaxBodyBytes,
		Logger:       logger,
		Ready:        clients.Ready,
		MediaDir:     cfg.MediaDir,
		MediaPath:    cfg.MediaPath,
		RateLimiter:  rateLimiter,
		Metrics:      httpMetrics,
		Auth:         handler.NewAuth(clients.Account, responder),
		Users:        handler.NewUsers(clients.Account, responder),
		Products:     handler.NewProducts(clients.Marketplace, clients.Account, aggregator, responder),
		Trades:       handler.NewTrades(clients.Marketplace, aggregator, responder),
		Favorites:    handler.NewFavorites(clients.Favorite, aggregator, responder),
		Messaging:    handler.NewMessaging(clients.Messaging, aggregator, responder),
	})

	publicServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
	}
	managementMux := http.NewServeMux()
	managementMux.Handle("GET /metrics", httpMetrics)
	managementServer := &http.Server{
		Addr:              cfg.ManagementAddr,
		Handler:           managementMux,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
	}

	type serverResult struct {
		name string
		err  error
	}
	served := make(chan serverResult, 2)
	go func() { served <- serverResult{name: "public", err: publicServer.ListenAndServe()} }()
	go func() { served <- serverResult{name: "management", err: managementServer.ListenAndServe()} }()
	logger.Info("gateway listening", slog.String("addr", cfg.HTTPAddr), slog.String("base_path", cfg.BasePath))
	logger.Info("gateway management listening", slog.String("addr", cfg.ManagementAddr))

	shutdown := func() error {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Runtime.ShutdownTimeout)
		defer cancel()
		return errors.Join(publicServer.Shutdown(shutdownCtx), managementServer.Shutdown(shutdownCtx))
	}

	select {
	case result := <-served:
		if errors.Is(result.err, http.ErrServerClosed) {
			return nil
		}
		_ = shutdown()
		return fmt.Errorf("%s HTTP server: %w", result.name, result.err)
	case <-ctx.Done():
		logger.Info("gateway shutting down")
		return shutdown()
	}
}
