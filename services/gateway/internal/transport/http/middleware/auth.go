package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/KDZZZZZZ/short-term/platform/auth"
	"github.com/KDZZZZZZ/short-term/platform/errs"
)

// TokenVerifier 校验访问令牌并返回当前用户。
type TokenVerifier interface {
	Verify(token string) (auth.Claims, error)
}

// NewAuthentication 校验 bearer 令牌，并将当前用户放入上下文。
//
// 它只负责身份认证。当前用户能否读取或修改某个商品、交易或会话，由资源所属服务
// 决定（docs/software-design.md 第 7.2 节），因为只有该服务知道卖家、买家和参与者身份。
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
				// 有意不报告具体原因：过期令牌和伪造令牌对调用方应表现相同。
				responder.Error(w, r, errs.Wrap(errs.CodeUnauthorized, "请先登录", err))
				return
			}

			next.ServeHTTP(w, r.WithContext(WithActorID(r.Context(), claims.Subject)))
		})
	}
}

// errNoBearer 表示 Authorization 请求头缺失或格式错误。
var errNoBearer = errors.New("missing bearer token")

// bearerToken 从 Authorization 请求头提取令牌。按照 RFC 7235 的要求，
// scheme 比较不区分大小写。
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
