// Package config 从环境变量加载 Gateway 设置。
package config

import (
	"time"

	"github.com/KDZZZZZZ/short-term/platform/auth"
	platformconfig "github.com/KDZZZZZZ/short-term/platform/config"
	grpcclient "github.com/KDZZZZZZ/short-term/services/gateway/internal/client/grpc"
)

// ServiceName 是日志、追踪和指标中使用的可部署单元名称。
const ServiceName = "gateway"

// Config 是完成校验的 Gateway 配置。
type Config struct {
	Runtime platformconfig.Runtime

	// HTTPAddr 是公网监听地址。Gateway 是唯一绑定公网端口的单元
	// （docs/software-design.md 第 9.2 节）。
	HTTPAddr string
	// ManagementAddr 仅提供内部指标；生产环境通过主机回环端口访问，不能暴露公网。
	ManagementAddr string
	// BasePath 是 openapi/openapi.yaml 声明的 API 根路径。
	BasePath string
	// MaxBodyBytes 限制 JSON 请求正文大小。Multipart 上传由商品处理器单独限制。
	MaxBodyBytes int64
	// ReadHeaderTimeout 限制请求头读取时间，用于防御慢请求头拒绝服务攻击。
	ReadHeaderTimeout time.Duration
	// WriteTimeout 限制写入一条响应的时间。
	WriteTimeout time.Duration
	// DownstreamTimeout 是内部调用的默认截止时间。
	DownstreamTimeout time.Duration
	// UploadTimeout 是携带图片字节的调用截止时间；这类调用需要比查询更大的时间预算。
	UploadTimeout time.Duration
	// MediaDir 设置后，从该目录提供已存储的商品图片。
	// 它用于 Gateway 与 Marketplace 对象存储共享卷的单机部署；
	// 使用对象存储或 CDN 时留空。
	MediaDir string
	// MediaPath 是提供媒体文件时使用的 URL 前缀。
	MediaPath string
	// Targets 是内部服务地址。
	Targets grpcclient.Targets
	// Token 描述 Gateway 接受的访问令牌。签名密钥必须与 Account Service 的配置一致。
	Token auth.Config

	// RateLimitWindow and route budgets are conservative single-instance
	// defaults. They are configurable because final values require production
	// traffic and abuse measurements rather than guesses in source code.
	RateLimitWindow   time.Duration
	RateLimitRegister int
	RateLimitLogin    int
	RateLimitMessage  int
	RateLimitUpload   int
	RateLimitTrade    int
	RateLimitMaxKeys  int
	TrustProxyHeaders bool
}

// Load 读取并校验配置，一次性报告所有问题。
func Load() (Config, error) {
	loader := platformconfig.NewLoader()

	cfg := Config{
		Runtime:           loader.LoadRuntime(ServiceName),
		HTTPAddr:          loader.String("GATEWAY_HTTP_ADDR", ":8080"),
		ManagementAddr:    loader.String("GATEWAY_MANAGEMENT_ADDR", ":9090"),
		BasePath:          loader.String("GATEWAY_BASE_PATH", "/api/v1"),
		MaxBodyBytes:      int64(loader.Int("GATEWAY_MAX_BODY_BYTES", 1<<20)),
		ReadHeaderTimeout: loader.Duration("GATEWAY_READ_HEADER_TIMEOUT", 10*time.Second),
		WriteTimeout:      loader.Duration("GATEWAY_WRITE_TIMEOUT", 30*time.Second),
		DownstreamTimeout: loader.Duration("GATEWAY_DOWNSTREAM_TIMEOUT", 5*time.Second),
		UploadTimeout:     loader.Duration("GATEWAY_UPLOAD_TIMEOUT", 30*time.Second),
		MediaDir:          loader.String("GATEWAY_MEDIA_DIR", ""),
		MediaPath:         loader.String("GATEWAY_MEDIA_PATH", "/media"),
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
			// Gateway 只验证令牌，因此有效期由签发者负责；
			// 共享类型仍然要求提供该值。
			TTL:    loader.Duration("JWT_TTL", 24*time.Hour),
			Leeway: loader.Duration("JWT_LEEWAY", 30*time.Second),
		},
		RateLimitWindow:   loader.Duration("GATEWAY_RATE_LIMIT_WINDOW", time.Minute),
		RateLimitRegister: loader.Int("GATEWAY_RATE_LIMIT_REGISTER", 5),
		RateLimitLogin:    loader.Int("GATEWAY_RATE_LIMIT_LOGIN", 10),
		RateLimitMessage:  loader.Int("GATEWAY_RATE_LIMIT_MESSAGE", 60),
		RateLimitUpload:   loader.Int("GATEWAY_RATE_LIMIT_UPLOAD", 20),
		RateLimitTrade:    loader.Int("GATEWAY_RATE_LIMIT_TRADE", 30),
		RateLimitMaxKeys:  loader.Int("GATEWAY_RATE_LIMIT_MAX_KEYS", 10000),
		TrustProxyHeaders: loader.Bool("GATEWAY_TRUST_PROXY_HEADERS", false),
	}

	return cfg, loader.Err()
}
