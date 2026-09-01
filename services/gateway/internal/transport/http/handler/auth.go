// Package handler implements the public REST endpoints. Handlers parse and
// validate the request, call one or more internal services, and map the
// result; they hold no business rules of their own.
package handler

import (
	"net/http"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/mapper"
)

// Auth serves /auth/register, /auth/login and /auth/logout.
type Auth struct {
	accounts  accountv1.AccountServiceClient
	responder Responder
}

// NewAuth builds the authentication handler.
func NewAuth(accounts accountv1.AccountServiceClient, responder Responder) *Auth {
	return &Auth{accounts: accounts, responder: responder}
}

// Register handles POST /auth/register.
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

	h.responder.Created(w, r, mapper.AuthData(resp.GetAuth()))
}

// Login handles POST /auth/login.
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

	h.responder.OK(w, r, mapper.AuthData(resp.GetAuth()))
}

// Logout handles POST /auth/logout.
//
// The token is not revoked server-side. docs/backend-development-plan.md
// records this as the default while "JWT logout 是否服务端撤销" is unconfirmed,
// and the approved contract allows it: the logout response describes the
// session as ended either by invalidating the token or by the client
// discarding it. Choosing revocation later means adding a jti denylist in the
// Account Service and consulting it here; no other code changes.
//
// The endpoint still requires a valid token, so it reports honestly whether
// the caller had a session to end.
func (h *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	h.responder.Empty(w, r)
}

// Responder is the subset of the response writer the handlers use.
type Responder interface {
	OK(w http.ResponseWriter, r *http.Request, data any)
	Created(w http.ResponseWriter, r *http.Request, data any)
	Success(w http.ResponseWriter, r *http.Request, status int, data any)
	Empty(w http.ResponseWriter, r *http.Request)
	Error(w http.ResponseWriter, r *http.Request, err error)
	Fail(w http.ResponseWriter, r *http.Request, code errs.Code, message string)
}
