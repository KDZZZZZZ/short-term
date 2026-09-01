package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/KDZZZZZZ/short-term/platform/auth"
	"github.com/KDZZZZZZ/short-term/platform/errs"
)

// TokenVerifier checks an access token and reports the acting user.
type TokenVerifier interface {
	Verify(token string) (auth.Claims, error)
}

// NewAuthentication verifies the bearer token and puts the acting user on the
// context.
//
// It authenticates only. Whether the acting user may read or change a given
// product, trade or conversation is decided by the service that owns it
// (docs/software-design.md section 7.2), because only that service knows the
// seller, buyer and participant identities.
func NewAuthentication(verifier TokenVerifier, responder ErrorWriter, isPublic func(*http.Request) bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublic != nil && isPublic(r) {
				next.ServeHTTP(w, r)
				return
			}

			token, err := bearerToken(r)
			if err != nil {
				responder.Error(w, r, errs.New(errs.CodeUnauthorized, "请先登录"))
				return
			}

			claims, err := verifier.Verify(token)
			if err != nil {
				// The reason is deliberately not reported: an expired token and
				// a forged one look the same to the caller.
				responder.Error(w, r, errs.Wrap(errs.CodeUnauthorized, "请先登录", err))
				return
			}

			next.ServeHTTP(w, r.WithContext(WithActorID(r.Context(), claims.Subject)))
		})
	}
}

// errNoBearer reports a missing or malformed Authorization header.
var errNoBearer = errors.New("missing bearer token")

// bearerToken extracts the token from the Authorization header. The scheme
// comparison is case-insensitive, as RFC 7235 requires.
func bearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", errNoBearer
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errNoBearer
	}
	return token, nil
}
