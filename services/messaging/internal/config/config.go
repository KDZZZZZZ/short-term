// Package config loads Messaging Service settings from environment variables.
package config

import (
	"time"

	platformconfig "github.com/KDZZZZZZ/short-term/platform/config"
)

const ServiceName = "messaging"

type Config struct {
	Runtime platformconfig.Runtime

	GRPCAddr          string
	DatabaseURL       string
	AutoMigrate       bool
	MaxDBConns        int32
	HandlerTimeout    time.Duration
	MarketplaceTarget string
	DownstreamTimeout time.Duration
	OutboxInterval    time.Duration
	OutboxBatchSize   int32
}

func Load() (Config, error) {
	loader := platformconfig.NewLoader()
	cfg := Config{
		Runtime:           loader.LoadRuntime(ServiceName),
		GRPCAddr:          loader.String("MESSAGING_GRPC_ADDR", ":9003"),
		DatabaseURL:       loader.Required("MESSAGING_DATABASE_URL"),
		AutoMigrate:       loader.Bool("MESSAGING_AUTO_MIGRATE", true),
		MaxDBConns:        int32(loader.Int("MESSAGING_MAX_DB_CONNS", 10)),
		HandlerTimeout:    loader.Duration("MESSAGING_HANDLER_TIMEOUT", 10*time.Second),
		MarketplaceTarget: loader.String("MARKETPLACE_GRPC_TARGET", "marketplace:9002"),
		DownstreamTimeout: loader.Duration("MESSAGING_DOWNSTREAM_TIMEOUT", 5*time.Second),
		OutboxInterval:    loader.Duration("OUTBOX_INTERVAL", time.Second),
		OutboxBatchSize:   int32(loader.Int("OUTBOX_BATCH_SIZE", 100)),
	}
	return cfg, loader.Err()
}
