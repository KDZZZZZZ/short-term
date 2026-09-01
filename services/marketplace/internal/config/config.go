// Package config 从环境变量加载 Marketplace Service 设置。
package config

import (
	"time"

	platformconfig "github.com/KDZZZZZZ/short-term/platform/config"
)

// ServiceName 是日志、追踪和指标中使用的可部署单元名称。
const ServiceName = "marketplace"

// Config 是完成校验的 Marketplace Service 配置。
type Config struct {
	Runtime platformconfig.Runtime

	// GRPCAddr 是私有监听地址。
	GRPCAddr string
	// DatabaseURL 指向 Marketplace 数据库，属于凭据。
	DatabaseURL string
	// AutoMigrate 在启动时应用待执行的迁移。
	AutoMigrate bool
	// MaxDBConns 限制连接池大小。
	MaxDBConns int32
	// HandlerTimeout 限制调用方未发送截止时间的调用。
	HandlerTimeout time.Duration
	// MessagingTarget 是会话绑定校验所使用的 Messaging Service 地址。
	MessagingTarget string
	// DownstreamTimeout 限制 Marketplace 到事实源的内部调用。
	DownstreamTimeout time.Duration

	// MediaRoot 是文件系统对象存储写入的目录。
	MediaRoot string
	// MediaPublicURL 是客户端获取已存储图片时使用的前缀。
	MediaPublicURL string

	// OutboxInterval 是 worker 清空 Outbox 的间隔。
	OutboxInterval time.Duration
	// OutboxBatchSize 限制每次清空处理的事件数。
	OutboxBatchSize int32
}

// Load 读取并校验配置，一次性报告所有问题。
func Load() (Config, error) {
	loader := platformconfig.NewLoader()

	cfg := Config{
		Runtime:     loader.LoadRuntime(ServiceName),
		GRPCAddr:    loader.String("MARKETPLACE_GRPC_ADDR", ":9002"),
		DatabaseURL: loader.Required("MARKETPLACE_DATABASE_URL"),
		AutoMigrate: loader.Bool("MARKETPLACE_AUTO_MIGRATE", true),
		MaxDBConns:  int32(loader.Int("MARKETPLACE_MAX_DB_CONNS", 10)),
		// 图片上传在 RPC 中传输，因此包含三张 5 MiB 图片的商品需要比普通查询更长的时间预算。
		HandlerTimeout:    loader.Duration("MARKETPLACE_HANDLER_TIMEOUT", 30*time.Second),
		MessagingTarget:   loader.String("MESSAGING_GRPC_TARGET", "messaging:9003"),
		DownstreamTimeout: loader.Duration("MARKETPLACE_DOWNSTREAM_TIMEOUT", 5*time.Second),
		MediaRoot:         loader.String("MEDIA_ROOT", "/var/lib/shortterm/media"),
		MediaPublicURL:    loader.String("MEDIA_PUBLIC_URL", "/media"),

		OutboxInterval:  loader.Duration("OUTBOX_INTERVAL", time.Second),
		OutboxBatchSize: int32(loader.Int("OUTBOX_BATCH_SIZE", 100)),
	}

	return cfg, loader.Err()
}
