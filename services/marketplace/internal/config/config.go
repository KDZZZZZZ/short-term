// Package config loads the Marketplace Service settings from the environment.
package config

import (
	"time"

	platformconfig "github.com/KDZZZZZZ/short-term/platform/config"
)

// ServiceName is the deployable unit name used in logs, traces and metrics.
const ServiceName = "marketplace"

// Config is the fully validated Marketplace Service configuration.
type Config struct {
	Runtime platformconfig.Runtime

	// GRPCAddr is the private listen address.
	GRPCAddr string
	// DatabaseURL points at the marketplace database. It is a credential.
	DatabaseURL string
	// AutoMigrate applies pending migrations at startup.
	AutoMigrate bool
	// MaxDBConns bounds the connection pool.
	MaxDBConns int32
	// HandlerTimeout bounds a call whose caller sent no deadline.
	HandlerTimeout time.Duration

	// MediaRoot is the directory the filesystem object store writes to.
	MediaRoot string
	// MediaPublicURL is the prefix clients use to fetch stored images.
	MediaPublicURL string
}

// Load reads and validates the configuration, reporting every problem at once.
func Load() (Config, error) {
	loader := platformconfig.NewLoader()

	cfg := Config{
		Runtime:     loader.LoadRuntime(ServiceName),
		GRPCAddr:    loader.String("MARKETPLACE_GRPC_ADDR", ":9002"),
		DatabaseURL: loader.Required("MARKETPLACE_DATABASE_URL"),
		AutoMigrate: loader.Bool("MARKETPLACE_AUTO_MIGRATE", true),
		MaxDBConns:  int32(loader.Int("MARKETPLACE_MAX_DB_CONNS", 10)),
		// Image uploads travel inside the RPC, so a product with three 5 MiB
		// images needs a longer budget than a plain query.
		HandlerTimeout: loader.Duration("MARKETPLACE_HANDLER_TIMEOUT", 30*time.Second),
		MediaRoot:      loader.String("MEDIA_ROOT", "/var/lib/shortterm/media"),
		MediaPublicURL: loader.String("MEDIA_PUBLIC_URL", "/media"),
	}

	return cfg, loader.Err()
}
