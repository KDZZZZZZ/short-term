// Package config 从环境变量加载 Account Service 设置。
package config

import (
	"time"

	"github.com/KDZZZZZZ/short-term/platform/auth"
	platformconfig "github.com/KDZZZZZZ/short-term/platform/config"
)

// ServiceName 是日志、追踪和指标中使用的可部署单元名称。
const ServiceName = "account"

// Config 是完成校验的 Account Service 配置。
type Config struct {
	Runtime platformconfig.Runtime

	// GRPCAddr 是私有监听地址。Account Service 永不暴露到公网
	// （docs/software-design.md 第 9.2 节）。
	GRPCAddr string
	// DatabaseURL 指向账户数据库，属于凭据。
	DatabaseURL string
	// AutoMigrate 在启动时应用待执行的迁移。
	AutoMigrate bool
	// MaxDBConns 限制连接池大小。
	MaxDBConns int32
	// HandlerTimeout 限制调用方未发送截止时间的调用。
	HandlerTimeout time.Duration

	// Token 描述该服务签发的访问令牌。
	Token auth.Config
	// Argon2 保存密码哈希工作因子。
	Argon2 auth.Argon2Params
}

// Load 读取并校验配置，一次性报告所有问题。
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
