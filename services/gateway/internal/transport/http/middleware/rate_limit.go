package middleware

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KDZZZZZZ/short-term/platform/errs"
)

// RateLimitConfig defines fixed-window limits for the abuse-sensitive routes
// declared with 429 responses in OpenAPI. Limits are per authenticated actor,
// except register and login which are keyed by client IP.
type RateLimitConfig struct {
	Window            time.Duration
	Register          int
	Login             int
	Message           int
	Upload            int
	Trade             int
	MaxKeys           int
	TrustProxyHeaders bool
}

type rateEntry struct {
	started time.Time
	count   int
}

// RateLimiter is an in-process limiter intended for the initial single-Gateway
// deployment. A shared limiter is required before Gateway gains replicas.
type RateLimiter struct {
	config    RateLimitConfig
	responder ErrorWriter
	now       func() time.Time

	mu        sync.Mutex
	entries   map[string]rateEntry
	lastPrune time.Time
}

// NewRateLimiter validates and constructs the limiter. A zero route limit
// disables only that route class; negative limits are configuration errors.
func NewRateLimiter(config RateLimitConfig, responder ErrorWriter) (*RateLimiter, error) {
	if responder == nil {
		return nil, errors.New("gateway: rate limiter responder is required")
	}
	if config.Window <= 0 {
		return nil, errors.New("gateway: rate limit window must be positive")
	}
	if config.MaxKeys <= 0 {
		return nil, errors.New("gateway: rate limit max keys must be positive")
	}
	if config.Register < 0 || config.Login < 0 || config.Message < 0 || config.Upload < 0 || config.Trade < 0 {
		return nil, errors.New("gateway: rate limits cannot be negative")
	}
	return &RateLimiter{
		config:    config,
		responder: responder,
		now:       time.Now,
		entries:   make(map[string]rateEntry),
	}, nil
}

// Middleware rejects requests after the configured route-specific budget is
// exhausted. Authentication must run before this middleware so protected
// routes can use the stable actor ID instead of a spoofable request field.
func (l *RateLimiter) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rule, limit := l.rule(r)
			if limit == 0 {
				next.ServeHTTP(w, r)
				return
			}

			identity := ActorID(r.Context())
			if identity == "" {
				identity = l.clientIP(r)
			}
			allowed, retryAfter := l.allow(rule+"\x00"+identity, limit, l.now())
			if allowed {
				next.ServeHTTP(w, r)
				return
			}

			seconds := int64(retryAfter/time.Second) + 1
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
			w.Header().Set("Cache-Control", "no-store")
			l.responder.Error(w, r, errs.New(errs.CodeRateLimited, "请求过于频繁，请稍后重试"))
		})
	}
}

func (l *RateLimiter) rule(r *http.Request) (string, int) {
	if r.Method != http.MethodPost {
		return "", 0
	}
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	switch {
	case path == "auth/register":
		return "register", l.config.Register
	case path == "auth/login":
		return "login", l.config.Login
	case path == "products":
		return "upload", l.config.Upload
	case len(parts) == 3 && parts[0] == "products" && parts[1] != "" && parts[2] == "images":
		return "upload", l.config.Upload
	case len(parts) == 3 && parts[0] == "conversations" && parts[1] != "" && parts[2] == "messages":
		return "message", l.config.Message
	case len(parts) == 3 && parts[0] == "products" && parts[1] != "" && parts[2] == "trades":
		return "trade", l.config.Trade
	case len(parts) == 3 && parts[0] == "trades" && parts[1] != "" && isTradeAction(parts[2]):
		return "trade", l.config.Trade
	default:
		return "", 0
	}
}

func isTradeAction(action string) bool {
	switch action {
	case "accept", "reject", "cancel", "confirm":
		return true
	default:
		return false
	}
}

func (l *RateLimiter) allow(key string, limit int, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lastPrune.IsZero() || now.Sub(l.lastPrune) >= l.config.Window {
		for entryKey, entry := range l.entries {
			if now.Sub(entry.started) >= l.config.Window {
				delete(l.entries, entryKey)
			}
		}
		l.lastPrune = now
	}

	entry, exists := l.entries[key]
	if exists && now.Sub(entry.started) >= l.config.Window {
		entry = rateEntry{}
		exists = false
	}
	if !exists {
		if len(l.entries) >= l.config.MaxKeys {
			return false, l.config.Window
		}
		entry = rateEntry{started: now}
	}
	if entry.count >= limit {
		remaining := l.config.Window - now.Sub(entry.started)
		if remaining < 0 {
			remaining = 0
		}
		return false, remaining
	}

	entry.count++
	l.entries[key] = entry
	return true, 0
}

func (l *RateLimiter) clientIP(r *http.Request) string {
	if l.config.TrustProxyHeaders {
		if forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ","); len(forwarded) > 0 {
			if ip := net.ParseIP(strings.TrimSpace(forwarded[0])); ip != nil {
				return ip.String()
			}
		}
		if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != nil {
			return ip.String()
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(r.RemoteAddr); ip != nil {
		return ip.String()
	}
	return "unknown"
}
