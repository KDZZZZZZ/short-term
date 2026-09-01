package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KDZZZZZZ/short-term/platform/errs"
)

type rateLimitErrorWriter struct{}

func (rateLimitErrorWriter) Error(w http.ResponseWriter, _ *http.Request, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": string(errs.CodeOf(err))})
}

func TestRateLimiterSeparatesRulesAndActors(t *testing.T) {
	t.Parallel()

	limiter := newTestRateLimiter(t, RateLimitConfig{
		Window: time.Minute, Register: 1, Login: 1, Message: 1, Upload: 1, Trade: 1, MaxKeys: 100,
	})
	handler := limiter.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	assertLimitedOnSecondRequest(t, handler, http.MethodPost, "/auth/register", "", "192.0.2.1:1234")
	assertLimitedOnSecondRequest(t, handler, http.MethodPost, "/auth/login", "", "192.0.2.1:1234")
	assertLimitedOnSecondRequest(t, handler, http.MethodPost, "/conversations/c_1/messages", "u_1", "192.0.2.1:1234")
	assertLimitedOnSecondRequest(t, handler, http.MethodPost, "/products", "u_2", "192.0.2.1:1234")
	assertLimitedOnSecondRequest(t, handler, http.MethodPost, "/trades/t_1/accept", "u_3", "192.0.2.1:1234")
}

func TestRateLimiterResetsAfterWindow(t *testing.T) {
	t.Parallel()

	limiter := newTestRateLimiter(t, RateLimitConfig{Window: time.Minute, Register: 1, MaxKeys: 10})
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }
	handler := limiter.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	if status := serveRateRequest(handler, http.MethodPost, "/auth/register", "", "192.0.2.2:1", nil).Code; status != http.StatusNoContent {
		t.Fatalf("first status = %d", status)
	}
	if response := serveRateRequest(handler, http.MethodPost, "/auth/register", "", "192.0.2.2:1", nil); response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
		t.Fatalf("limited response = %d headers=%v", response.Code, response.Header())
	}

	now = now.Add(time.Minute)
	if status := serveRateRequest(handler, http.MethodPost, "/auth/register", "", "192.0.2.2:1", nil).Code; status != http.StatusNoContent {
		t.Fatalf("status after reset = %d", status)
	}
}

func TestRateLimiterUsesForwardedIPOnlyWhenTrusted(t *testing.T) {
	t.Parallel()

	limiter := newTestRateLimiter(t, RateLimitConfig{Window: time.Minute, Login: 1, MaxKeys: 10, TrustProxyHeaders: true})
	handler := limiter.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	headers := map[string]string{"X-Forwarded-For": "198.51.100.7, 10.0.0.1"}
	first := serveRateRequest(handler, http.MethodPost, "/auth/login", "", "10.0.0.1:20", headers)
	second := serveRateRequest(handler, http.MethodPost, "/auth/login", "", "10.0.0.2:20", headers)
	if first.Code != http.StatusNoContent || second.Code != http.StatusTooManyRequests {
		t.Fatalf("statuses = %d, %d", first.Code, second.Code)
	}
}

func TestRateLimiterIgnoresRoutesWithoutContract429(t *testing.T) {
	t.Parallel()

	limiter := newTestRateLimiter(t, RateLimitConfig{Window: time.Minute, Register: 1, MaxKeys: 10})
	handler := limiter.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for range 3 {
		if status := serveRateRequest(handler, http.MethodGet, "/products", "u_1", "192.0.2.1:1", nil).Code; status != http.StatusNoContent {
			t.Fatalf("status = %d", status)
		}
	}
}

func newTestRateLimiter(t *testing.T, config RateLimitConfig) *RateLimiter {
	t.Helper()
	limiter, err := NewRateLimiter(config, rateLimitErrorWriter{})
	if err != nil {
		t.Fatalf("NewRateLimiter: %v", err)
	}
	return limiter
}

func assertLimitedOnSecondRequest(t *testing.T, handler http.Handler, method, path, actor, remote string) {
	t.Helper()
	first := serveRateRequest(handler, method, path, actor, remote, nil)
	second := serveRateRequest(handler, method, path, actor, remote, nil)
	if first.Code != http.StatusNoContent || second.Code != http.StatusTooManyRequests {
		t.Fatalf("%s statuses = %d, %d", path, first.Code, second.Code)
	}
	if second.Header().Get("Retry-After") == "" || second.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%s headers = %v", path, second.Header())
	}
	if body := second.Body.String(); body == "" {
		t.Fatal("rate limit response body is empty")
	}
}

func serveRateRequest(handler http.Handler, method, path, actor, remote string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	request.RemoteAddr = remote
	if actor != "" {
		request = request.WithContext(WithActorID(context.Background(), actor))
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
