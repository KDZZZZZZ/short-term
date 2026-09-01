package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPMetricsUsesBoundedRoutePatternAndFinalStatus(t *testing.T) {
	metrics := NewHTTPMetrics()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /trades/{tradeId}/accept", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})
	resolve := func(r *http.Request) string {
		_, pattern := mux.Handler(r)
		return pattern
	}
	handler := metrics.Middleware(resolve)(mux)

	request := httptest.NewRequest(http.MethodPost, "/trades/t_secret/accept", nil)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	response := httptest.NewRecorder()
	metrics.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := response.Body.String()
	if !strings.Contains(body, `shortterm_gateway_http_requests_total{method="POST",route="POST /trades/{tradeId}/accept",status="409"} 1`) {
		t.Fatalf("metrics body does not contain the bounded counter:\n%s", body)
	}
	if strings.Contains(body, "t_secret") {
		t.Fatalf("metrics leaked a resource identifier:\n%s", body)
	}
	if got := response.Header().Get("Content-Type"); got != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestHTTPMetricsSortsOutputDeterministically(t *testing.T) {
	metrics := NewHTTPMetrics()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /z", func(http.ResponseWriter, *http.Request) {})
	mux.HandleFunc("GET /a", func(http.ResponseWriter, *http.Request) {})
	resolve := func(r *http.Request) string {
		_, pattern := mux.Handler(r)
		return pattern
	}
	handler := metrics.Middleware(resolve)(mux)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/z", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/a", nil))

	response := httptest.NewRecorder()
	metrics.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := response.Body.String()
	if strings.Index(body, `route="GET /a"`) > strings.Index(body, `route="GET /z"`) {
		t.Fatalf("metric samples are not sorted:\n%s", body)
	}
}
