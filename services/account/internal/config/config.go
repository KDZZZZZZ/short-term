// Package config loads the Account Service settings from the environment.
package config

import (
	"time"

	"github.com/KDZZZZZZ/short-term/platform/auth"
	platformconfig "github.com/KDZZZZZZ/short-term/platform/config"
)

// ServiceName is the deployable unit name used in logs, traces and metrics.
const ServiceName = "account"

// Config is the fully validated Account Service configuration.
type Config struct {
	Runtime platformconfig.Runtime

	// GRPCAddr is the private listen address. The Account Service is never
	// exposed to the public internet (docs/software-design.md section 9.2).
	GRPCAddr string
	// DatabaseURL points at the account database. It is a credential.
	DatabaseURL string
	// AutoMigrate applies pending migrations at startup.
	AutoMigrate bool
	// MaxDBConns bounds the connection pool.
	MaxDBConns int32
	// HandlerTimeout bounds a call whose caller sent no deadline.
	HandlerTimeout time.Duration

	// Token describes the access tokens this service signs.
	Token auth.Config
	// Argon2 holds the password hashing work factors.
	Argon2 auth.Argon2Params
}

// Load reads and validates the configuration, reporting every problem at once.
func Load() (Config, error) {
	loader := platformconfig.NewLoader()
	defaults := auth.DefaultArgon2Params()

	cfg := Config{
		Runtime:        loader.LoadRuntime(ServiceName),
		GRPCAddr:       loader.String("ACCOUNT_GRPC_ADDR", ":9001"),
		DatabaseURL:    loader.Required("ACCOUNT_DATABASE_URL"),
		AutoMigrate:    loader.Bool("ACCOUNT_AUTO_MIGRATE", true),
		MaxDBConns:     int32(loader.Int("ACCOUNT_MAX_DB_CONNS", 10)),
		HandlerTimeout: loader.Duration("ACCOUNT_HANDLER_TIMEOUT", 10*time.Second),
		Token: auth.Config{
			SigningKey: []byte(loader.Required("JWT_SIGNING_KEY")),
			Issuer:     loader.String("JWT_ISSUER", "shortterm-account"),
			Audience:   loader.String("JWT_AUDIENCE", "shortterm-api"),
			TTL:        loader.Duration("JWT_TTL", 24*time.Hour),
			Leeway:     loader.Duration("JWT_LEEWAY", 30*time.Second),
		},
		Argon2: auth.Argon2Params{
			Memory:      uint32(loader.Int("ARGON2_MEMORY_KIB", int(defaults.Memory))),
			Iterations:  uint32(loader.Int("ARGON2_ITERATIONS", int(defaults.Iterations))),
			Parallelism: uint8(loader.Int("ARGON2_PARALLELISM", int(defaults.Parallelism))),
			SaltLength:  defaults.SaltLength,
			KeyLength:   defaults.KeyLength,
		},
	}

	return cfg, loader.Err()
}
