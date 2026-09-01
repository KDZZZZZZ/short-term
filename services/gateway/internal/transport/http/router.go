package http

import (
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/KDZZZZZZ/short-term/platform/auth"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/handler"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/middleware"
)

// RouterOptions configures the public HTTP surface.
type RouterOptions struct {
	// BasePath is the API root. openapi/openapi.yaml declares /api/v1.
	BasePath string
	// Verifier checks bearer tokens.
	Verifier middleware.TokenVerifier
	// MaxBodyBytes bounds a JSON request body.
	MaxBodyBytes int64
	// Logger receives access logs and failures.
	Logger *slog.Logger
	// Ready reports whether the process should receive traffic.
	Ready func() error

	Auth  *handler.Auth
	Users *handler.Users
}

// publicPaths are the only endpoints reachable without a token. Everything
// else in openapi/openapi.yaml inherits the global bearerAuth requirement.
var publicPaths = map[string]struct{}{
	"/auth/register": {},
	"/auth/login":    {},
}

// NewRouter builds the public handler, including health endpoints that sit
// outside the versioned API.
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

	// An unmatched path inside the API must still answer with the contract's
	// error envelope rather than net/http's plain-text 404.
	api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		responder.Fail(w, r, errs.CodeResourceNotFound, "资源不存在")
	})

	apiHandler := middleware.Chain(api,
		middleware.NewRequestID(),
		middleware.NewRecovery(opts.Logger, responder),
		middleware.NewAccessLog(opts.Logger),
		middleware.NewBodyLimit(opts.MaxBodyBytes),
		middleware.NewAuthentication(opts.Verifier, responder, isPublic),
	)

	root := http.NewServeMux()
	root.Handle(base+"/", http.StripPrefix(base, apiHandler))
	root.HandleFunc("GET /healthz", liveness)
	root.HandleFunc("GET /readyz", readiness(opts.Ready))

	return otelhttp.NewHandler(root, "gateway")
}

// isPublic reports whether a request may proceed without authentication.
func isPublic(r *http.Request) bool {
	_, ok := publicPaths[r.URL.Path]
	return ok
}

// liveness reports that the process is running. It never touches a dependency:
// a failing database must not cause the container to be restarted in a loop.
func liveness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// readiness reports whether the process should receive traffic. It must fail
// while a required dependency is unavailable rather than accept requests it
// cannot serve (docs/software-design.md section 8.4).
func readiness(ready func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if ready != nil {
			if err := ready(); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"status":"unavailable"}`))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}
}

// ensure the verifier interface stays satisfied by the platform type.
var _ middleware.TokenVerifier = (*auth.Verifier)(nil)
