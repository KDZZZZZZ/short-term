// Package config loads Favorite Service settings from the environment.
package config

import (
	"time"

	platformconfig "github.com/KDZZZZZZ/short-term/platform/config"
)

// ServiceName identifies this deployable unit in logs and traces.
const ServiceName = "favorite"

// Config is a validated Favorite Service configuration.
type Config struct {
	Runtime platformconfig.Runtime

	GRPCAddr          string
	DatabaseURL       string
	AutoMigrate       bool
	MaxDBConns        int32
	HandlerTimeout    time.Duration
	MarketplaceTarget string
	DownstreamTimeout time.Duration
}

// Load reads configuration and reports all validation problems together.
func Load() (Config, error) {
	loader := platformconfig.NewLoader()
	cfg := Config{
		Runtime:           loader.LoadRuntime(ServiceName),
		GRPCAddr:          loader.String("FAVORITE_GRPC_ADDR", ":9004"),
		DatabaseURL:       loader.Required("FAVORITE_DATABASE_URL"),
		AutoMigrate:       loader.Bool("FAVORITE_AUTO_MIGRATE", true),
		MaxDBConns:        int32(loader.Int("FAVORITE_MAX_DB_CONNS", 10)),
		HandlerTimeout:    loader.Duration("FAVORITE_HANDLER_TIMEOUT", 10*time.Second),
		MarketplaceTarget: loader.String("MARKETPLACE_GRPC_TARGET", "marketplace:9002"),
		DownstreamTimeout: loader.Duration("FAVORITE_DOWNSTREAM_TIMEOUT", 5*time.Second),
	}
	return cfg, loader.Err()
}
