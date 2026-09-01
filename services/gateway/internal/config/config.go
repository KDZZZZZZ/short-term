// Package config loads the Gateway settings from the environment.
package config

import (
	"time"

	"github.com/KDZZZZZZ/short-term/platform/auth"
	platformconfig "github.com/KDZZZZZZ/short-term/platform/config"
	grpcclient "github.com/KDZZZZZZ/short-term/services/gateway/internal/client/grpc"
)

// ServiceName is the deployable unit name used in logs, traces and metrics.
const ServiceName = "gateway"

// Config is the fully validated Gateway configuration.
type Config struct {
	Runtime platformconfig.Runtime

	// HTTPAddr is the public listen address. The Gateway is the only unit that
	// binds a public port (docs/software-design.md section 9.2).
	HTTPAddr string
	// BasePath is the API root declared by openapi/openapi.yaml.
	BasePath string
	// MaxBodyBytes bounds a JSON request body. Multipart uploads are bounded
	// separately by the product handler.
	MaxBodyBytes int64
	// ReadHeaderTimeout bounds the request header read, which is the defence
	// against a slow-header denial of service.
	ReadHeaderTimeout time.Duration
	// WriteTimeout bounds writing one response.
	WriteTimeout time.Duration
	// DownstreamTimeout is the default deadline for internal calls.
	DownstreamTimeout time.Duration
	// UploadTimeout is the deadline for calls that carry image bytes, which
	// need a larger budget than a query.
	UploadTimeout time.Duration
	// MediaDir, when set, serves stored product images from this directory.
	// It is for single-host deployments where the Gateway and the Marketplace
	// object store share a volume; object storage or a CDN leaves it empty.
	MediaDir string
	// MediaPath is the URL prefix media files are served under.
	MediaPath string
	// FavoritesEnabled wires the real Favorite Service into product detail
	// responses. It stays false until the Favorite Service is deployed
	// (milestone M4 of docs/backend-development-plan.md); until then
	// is_favorited is reported as false rather than failing the request.
	FavoritesEnabled bool

	// Targets are the internal service addresses.
	Targets grpcclient.Targets
	// Token describes the access tokens the Gateway accepts. The signing key
	// must match the Account Service configuration.
	Token auth.Config
}

// Load reads and validates the configuration, reporting every problem at once.
func Load() (Config, error) {
	loader := platformconfig.NewLoader()

	cfg := Config{
		Runtime:           loader.LoadRuntime(ServiceName),
		HTTPAddr:          loader.String("GATEWAY_HTTP_ADDR", ":8080"),
		BasePath:          loader.String("GATEWAY_BASE_PATH", "/api/v1"),
		MaxBodyBytes:      int64(loader.Int("GATEWAY_MAX_BODY_BYTES", 1<<20)),
		ReadHeaderTimeout: loader.Duration("GATEWAY_READ_HEADER_TIMEOUT", 10*time.Second),
		WriteTimeout:      loader.Duration("GATEWAY_WRITE_TIMEOUT", 30*time.Second),
		DownstreamTimeout: loader.Duration("GATEWAY_DOWNSTREAM_TIMEOUT", 5*time.Second),
		UploadTimeout:     loader.Duration("GATEWAY_UPLOAD_TIMEOUT", 30*time.Second),
		MediaDir:          loader.String("GATEWAY_MEDIA_DIR", ""),
		MediaPath:         loader.String("GATEWAY_MEDIA_PATH", "/media"),
		FavoritesEnabled:  loader.Bool("GATEWAY_FAVORITES_ENABLED", false),
		Targets: grpcclient.Targets{
			Account:     loader.String("ACCOUNT_GRPC_TARGET", "account:9001"),
			Marketplace: loader.String("MARKETPLACE_GRPC_TARGET", "marketplace:9002"),
			Messaging:   loader.String("MESSAGING_GRPC_TARGET", "messaging:9003"),
			Favorite:    loader.String("FAVORITE_GRPC_TARGET", "favorite:9004"),
		},
		Token: auth.Config{
			SigningKey: []byte(loader.Required("JWT_SIGNING_KEY")),
			Issuer:     loader.String("JWT_ISSUER", "shortterm-account"),
			Audience:   loader.String("JWT_AUDIENCE", "shortterm-api"),
			// The Gateway only verifies tokens, so the lifetime is the
			// issuer's concern; a value is still required by the shared type.
			TTL:    loader.Duration("JWT_TTL", 24*time.Hour),
			Leeway: loader.Duration("JWT_LEEWAY", 30*time.Second),
		},
	}

	return cfg, loader.Err()
}
