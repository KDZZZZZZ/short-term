package http

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/KDZZZZZZ/short-term/platform/auth"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/handler"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/middleware"
)

// RouterOptions 配置公开 HTTP 接口。
type RouterOptions struct {
	// BasePath 是 API 根路径。openapi/openapi.yaml 声明为 /api/v1。
	BasePath string
	// Verifier 校验 bearer 令牌。
	Verifier middleware.TokenVerifier
	// MaxBodyBytes 限制 JSON 请求正文大小。
	MaxBodyBytes int64
	// Logger 接收访问日志和失败信息。
	Logger *slog.Logger
	// Ready 返回进程是否应接收流量。
	Ready func(context.Context) error
	// MediaDir 设置后，通过 MediaPath 从磁盘提供已存储的商品图片。
	// 它用于文件系统对象存储与 Gateway 共享卷的本地和单机开发；
	// 使用对象存储或 CDN 的部署将其留空，改为使用存储自身的公开 URL 提供图片。
	MediaDir string
	// MediaPath 是提供媒体文件时使用的 URL 前缀。
	MediaPath string
	// RateLimiter protects the abuse-sensitive routes that declare 429 in the
	// public contract. Nil disables limiting, primarily for focused unit tests.
	RateLimiter *middleware.RateLimiter
	// Metrics receives bounded route/status counters. Nil disables collection in
	// focused handler tests; production exposes it on a private management port.
	Metrics *middleware.HTTPMetrics

	Auth      *handler.Auth
	Users     *handler.Users
	Products  *handler.Products
	Trades    *handler.Trades
	Comments  *handler.Comments
	Favorites *handler.Favorites
	Messaging *handler.Messaging
}

// publicPaths 是无需令牌即可访问的唯一端点。openapi/openapi.yaml 中的其他端点
// 都继承全局 bearerAuth 要求。
var publicPaths = map[string]struct{}{
	"/auth/register": {},
	"/auth/login":    {},
}

// NewRouter 构造公开处理器，其中包括位于版本化 API 之外的健康检查端点。
func NewRouter(opts RouterOptions) http.Handler {
	responder := NewResponder(opts.Logger)
	base := strings.TrimSuffix(opts.BasePath, "/")

	api := http.NewServeMux()
	api.HandleFunc("POST /auth/register", opts.Auth.Register)
	api.HandleFunc("POST /auth/login", opts.Auth.Login)
	api.HandleFunc("POST /auth/logout", opts.Auth.Logout)

	api.HandleFunc("GET /users/me", opts.Users.Me)
	api.HandleFunc("PATCH /users/me", opts.Users.UpdateMe)
	api.HandleFunc("PUT /users/me/password", opts.Users.ChangePassword)
	api.HandleFunc("GET /users/me/products", opts.Products.ListMine)

	api.HandleFunc("GET /products", opts.Products.List)
	api.HandleFunc("POST /products", opts.Products.Create)
	api.HandleFunc("GET /products/{productId}", opts.Products.Get)
	api.HandleFunc("PATCH /products/{productId}", opts.Products.Update)
	api.HandleFunc("POST /products/{productId}/images", opts.Products.AddImages)
	api.HandleFunc("DELETE /products/{productId}/images/{imageId}", opts.Products.DeleteImage)
	api.HandleFunc("POST /products/{productId}/off-shelf", opts.Products.OffShelf)
	api.HandleFunc("POST /products/{productId}/relist", opts.Products.Relist)
	api.HandleFunc("GET /products/{productId}/comments", opts.Comments.List)
	api.HandleFunc("POST /products/{productId}/comments", opts.Comments.Create)

	api.HandleFunc("POST /products/{productId}/trades", opts.Trades.Create)
	api.HandleFunc("GET /trades", opts.Trades.List)
	api.HandleFunc("GET /trades/{tradeId}", opts.Trades.Get)
	api.HandleFunc("POST /trades/{tradeId}/accept", opts.Trades.Accept)
	api.HandleFunc("POST /trades/{tradeId}/reject", opts.Trades.Reject)
	api.HandleFunc("POST /trades/{tradeId}/cancel", opts.Trades.Cancel)
	api.HandleFunc("POST /trades/{tradeId}/confirm", opts.Trades.Confirm)

	api.HandleFunc("GET /favorites", opts.Favorites.List)
	api.HandleFunc("PUT /favorites/{productId}", opts.Favorites.Add)
	api.HandleFunc("DELETE /favorites/{productId}", opts.Favorites.Remove)

	api.HandleFunc("POST /products/{productId}/conversations", opts.Messaging.GetOrCreate)
	api.HandleFunc("GET /conversations", opts.Messaging.List)
	api.HandleFunc("GET /conversations/unread-count", opts.Messaging.UnreadCount)
	api.HandleFunc("GET /conversations/{conversationId}/messages", opts.Messaging.ListMessages)
	api.HandleFunc("POST /conversations/{conversationId}/messages", opts.Messaging.SendMessage)
	api.HandleFunc("POST /conversations/{conversationId}/read", opts.Messaging.MarkRead)

	// API 内未匹配的路径仍必须返回契约错误信封，而不是 net/http 的纯文本 404。
	api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		responder.Fail(w, r, errs.CodeResourceNotFound, "资源不存在")
	})

	middlewares := []middleware.Middleware{
		middleware.NewRequestID(),
		middleware.NewRecovery(opts.Logger, responder),
		middleware.NewAccessLog(opts.Logger),
	}
	if opts.Metrics != nil {
		middlewares = append(middlewares, opts.Metrics.Middleware(func(r *http.Request) string {
			_, pattern := api.Handler(r)
			return pattern
		}))
	}
	middlewares = append(middlewares,
		middleware.NewBodyLimit(opts.MaxBodyBytes),
		middleware.NewAuthentication(opts.Verifier, responder, isPublic),
	)
	if opts.RateLimiter != nil {
		middlewares = append(middlewares, opts.RateLimiter.Middleware())
	}
	apiHandler := middleware.Chain(api, middlewares...)

	root := http.NewServeMux()
	root.Handle(base+"/", http.StripPrefix(base, apiHandler))
	root.HandleFunc("GET /healthz", liveness)
	root.HandleFunc("GET /readyz", readiness(opts.Ready))
	if opts.MediaDir != "" && opts.MediaPath != "" {
		mediaPrefix := strings.TrimSuffix(opts.MediaPath, "/") + "/"
		root.Handle("GET "+mediaPrefix, http.StripPrefix(mediaPrefix, mediaServer(opts.MediaDir)))
	}

	return otelhttp.NewHandler(root, "gateway")
}

// isPublic 判断请求是否可以不经身份认证继续处理。
func isPublic(r *http.Request) bool {
	_, ok := publicPaths[r.URL.Path]
	return ok
}

// liveness 报告进程正在运行。它从不访问依赖项：数据库故障不能导致容器反复重启。
func liveness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// readiness 报告进程是否应接收流量。必需依赖不可用时必须失败，
// 而不是接受无法处理的请求（docs/software-design.md 第 8.4 节）。
func readiness(ready func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if ready != nil {
			if err := ready(request.Context()); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"status":"unavailable"}`))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}
}

// mediaServer 从磁盘提供已存储的图片。
//
// 处理器设置 nosniff 请求头和明确的 disposition，因为文件由用户上传：
// 即使 Marketplace Service 已验证每个存储对象确实是 JPEG、PNG 或 WebP，
// 这里也不应依赖浏览器猜测类型。
func mediaServer(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		files.ServeHTTP(w, r)
	})
}

// 确保 platform 类型持续满足验证器接口。
var _ middleware.TokenVerifier = (*auth.Verifier)(nil)
