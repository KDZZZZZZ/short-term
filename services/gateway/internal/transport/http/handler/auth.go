// Package handler 实现公开 REST 端点。处理器负责解析并校验请求、调用一个或多个
// 内部服务并映射结果；自身不持有业务规则。
package handler

import (
	"net/http"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/application/aggregation"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/mapper"
)

// Auth 提供 /auth/register、/auth/login 和 /auth/logout。
type Auth struct {
	accounts   accountv1.AccountServiceClient
	aggregator *aggregation.Aggregator
	responder  Responder
}

// NewAuth 构造认证处理器。
func NewAuth(accounts accountv1.AccountServiceClient, aggregator *aggregation.Aggregator, responder Responder) *Auth {
	return &Auth{accounts: accounts, aggregator: aggregator, responder: responder}
}

// Register 处理 POST /auth/register。
func (h *Auth) Register(w http.ResponseWriter, r *http.Request) {
	var body dto.RegisterRequest
	if !decodeJSON(w, r, h.responder, &body) {
		return
	}

	resp, err := h.accounts.Register(r.Context(), &accountv1.RegisterRequest{
		StudentNo: body.StudentNo,
		Password:  body.Password,
		Nickname:  body.Nickname,
		Wechat:    body.Wechat,
		Qq:        body.QQ,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	average, err := aggregatedAverageScore(r.Context(), h.aggregator, resp.GetAuth().GetUser().GetId())
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.Created(w, r, mapper.AuthData(resp.GetAuth(), average))
}

// Login 处理 POST /auth/login。
func (h *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var body dto.LoginRequest
	if !decodeJSON(w, r, h.responder, &body) {
		return
	}

	resp, err := h.accounts.Login(r.Context(), &accountv1.LoginRequest{
		StudentNo: body.StudentNo,
		Password:  body.Password,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	average, err := aggregatedAverageScore(r.Context(), h.aggregator, resp.GetAuth().GetUser().GetId())
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.OK(w, r, mapper.AuthData(resp.GetAuth(), average))
}

// Logout 处理 POST /auth/logout。
//
// 令牌不会在服务端撤销。docs/backend-development-plan.md 将此记录为默认方案，
// 因为“JWT logout 是否服务端撤销”尚未确认；已批准契约允许这样做：注销响应将会话
// 描述为已结束，具体可以是令牌失效，也可以是客户端丢弃令牌。日后若选择撤销，
// 只需在 Account Service 中增加 jti denylist 并在此处查询，无需修改其他代码。
//
// 该端点仍要求有效令牌，因此能够如实反映调用方是否有会话可结束。
func (h *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	h.responder.Empty(w, r)
}

// Responder 是处理器使用的响应写入器子集。
type Responder interface {
	OK(w http.ResponseWriter, r *http.Request, data any)
	Created(w http.ResponseWriter, r *http.Request, data any)
	Success(w http.ResponseWriter, r *http.Request, status int, data any)
	Empty(w http.ResponseWriter, r *http.Request)
	Error(w http.ResponseWriter, r *http.Request, err error)
	Fail(w http.ResponseWriter, r *http.Request, code errs.Code, message string)
}
