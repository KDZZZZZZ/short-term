package middleware

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// HTTPMetrics 保存按固定路由、方法和状态码聚合的 Gateway 请求计数。
// 路由标签来自 ServeMux 的模式而不是原始 URL，因此商品、交易和会话 ID
// 不会造成无界标签基数，也不会进入指标输出。
type HTTPMetrics struct {
	mu       sync.RWMutex
	requests map[httpMetricKey]uint64
}

type httpMetricKey struct {
	method string
	route  string
	status int
}

// NewHTTPMetrics 构造进程内指标注册表。计数随 Gateway 进程重启而重置。
func NewHTTPMetrics() *HTTPMetrics {
	return &HTTPMetrics{requests: make(map[httpMetricKey]uint64)}
}

// Middleware 记录每个请求的最终状态。resolveRoute 必须返回有限集合中的路由模式；
// NewRouter 使用 ServeMux.Handler 实现该约束，并让身份认证或限流提前结束的请求
// 仍能归属到正确路由。
func (m *HTTPMetrics) Middleware(resolveRoute func(*http.Request) string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := "unmatched"
			if resolveRoute != nil {
				if resolved := resolveRoute(r); resolved != "" && resolved != "/" {
					route = resolved
				}
			}

			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)

			m.mu.Lock()
			m.requests[httpMetricKey{method: r.Method, route: route, status: recorder.status}]++
			m.mu.Unlock()
		})
	}
}

// ServeHTTP 输出 Prometheus 文本格式。该处理器只挂载到单独的管理监听器，
// 生产部署把对应主机端口绑定到 127.0.0.1。
func (m *HTTPMetrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	m.mu.RLock()
	keys := make([]httpMetricKey, 0, len(m.requests))
	values := make(map[httpMetricKey]uint64, len(m.requests))
	for key, value := range m.requests {
		keys = append(keys, key)
		values[key] = value
	}
	m.mu.RUnlock()

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].route != keys[j].route {
			return keys[i].route < keys[j].route
		}
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		return keys[i].status < keys[j].status
	})

	_, _ = fmt.Fprintln(w, "# HELP shortterm_gateway_http_requests_total Gateway HTTP requests grouped by bounded route, method, and status.")
	_, _ = fmt.Fprintln(w, "# TYPE shortterm_gateway_http_requests_total counter")
	for _, key := range keys {
		_, _ = fmt.Fprintf(w,
			"shortterm_gateway_http_requests_total{method=%s,route=%s,status=%s} %d\n",
			prometheusQuote(key.method), prometheusQuote(key.route), prometheusQuote(strconv.Itoa(key.status)), values[key],
		)
	}
}

func prometheusQuote(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}
