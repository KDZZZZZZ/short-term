// Command server runs the API Gateway, the only unit exposed to the public
// internet.
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
	grpcclient "github.com/KDZZZZZZ/short-term/services/gateway/internal/client/grpc"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/config"
	gatewayhttp "github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/handler"
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

	responder := gatewayhttp.NewResponder(logger)
	router := gatewayhttp.NewRouter(gatewayhttp.RouterOptions{
		BasePath:     cfg.BasePath,
		Verifier:     verifier,
		MaxBodyBytes: cfg.MaxBodyBytes,
		Logger:       logger,
		Ready:        func() error { return nil },
		Auth:         handler.NewAuth(clients.Account, responder),
		Users:        handler.NewUsers(clients.Account, responder),
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
	}

	served := make(chan error, 1)
	go func() { served <- server.ListenAndServe() }()
	logger.Info("gateway listening", slog.String("addr", cfg.HTTPAddr), slog.String("base_path", cfg.BasePath))

	select {
	case err := <-served:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		logger.Info("gateway shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Runtime.ShutdownTimeout)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
