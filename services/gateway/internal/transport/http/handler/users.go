package handler

import (
	"net/http"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/mapper"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/middleware"
)

// Users serves /users/me and /users/me/password.
type Users struct {
	accounts  accountv1.AccountServiceClient
	responder Responder
}

// NewUsers builds the current-user handler.
func NewUsers(accounts accountv1.AccountServiceClient, responder Responder) *Users {
	return &Users{accounts: accounts, responder: responder}
}

// Me handles GET /users/me.
func (h *Users) Me(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.ActorID(r.Context())

	resp, err := h.accounts.GetProfile(grpcx.WithActor(r.Context(), actorID), &accountv1.GetProfileRequest{UserId: actorID})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.OK(w, r, mapper.UserMe(resp.GetUser()))
}

// UpdateMe handles PATCH /users/me.
func (h *Users) UpdateMe(w http.ResponseWriter, r *http.Request) {
	var body dto.UpdateProfileRequest
	if !decodeJSON(w, r, h.responder, &body) {
		return
	}

	req := &accountv1.UpdateProfileRequest{UserId: middleware.ActorID(r.Context())}

	if body.Nickname.Present {
		if body.Nickname.IsNull() {
			h.responder.Fail(w, r, errs.CodeValidation, "昵称不能为 null")
			return
		}
		nickname, err := body.Nickname.String()
		if err != nil {
			h.responder.Fail(w, r, errs.CodeValidation, "昵称必须是字符串")
			return
		}
		req.Nickname = &nickname
	}

	wechat, ok := nullablePatch(w, r, h.responder, body.Wechat, "微信号")
	if !ok {
		return
	}
	req.Wechat = wechat

	qq, ok := nullablePatch(w, r, h.responder, body.QQ, "QQ 号")
	if !ok {
		return
	}
	req.Qq = qq

	if req.Nickname == nil && req.Wechat == nil && req.Qq == nil {
		// UpdateProfileRequest declares minProperties: 1.
		h.responder.Fail(w, r, errs.CodeValidation, "请至少修改一项资料")
		return
	}

	resp, err := h.accounts.UpdateProfile(grpcx.WithActor(r.Context(), req.GetUserId()), req)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.OK(w, r, mapper.UserMe(resp.GetUser()))
}

// ChangePassword handles PUT /users/me/password.
func (h *Users) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var body dto.ChangePasswordRequest
	if !decodeJSON(w, r, h.responder, &body) {
		return
	}

	actorID := middleware.ActorID(r.Context())
	_, err := h.accounts.ChangePassword(grpcx.WithActor(r.Context(), actorID), &accountv1.ChangePasswordRequest{
		UserId:      actorID,
		OldPassword: body.OldPassword,
		NewPassword: body.NewPassword,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.Empty(w, r)
}

// nullablePatch converts a three-state HTTP field into the wire patch message:
// absent stays nil, null becomes an explicit clear, and a string becomes a set.
func nullablePatch(w http.ResponseWriter, r *http.Request, responder Responder, field dto.RawField, label string) (*accountv1.NullableStringPatch, bool) {
	if !field.Present {
		return nil, true
	}
	if field.IsNull() {
		return &accountv1.NullableStringPatch{Value: &accountv1.NullableStringPatch_NullValue{}}, true
	}

	value, err := field.String()
	if err != nil {
		responder.Fail(w, r, errs.CodeValidation, label+"必须是字符串或 null")
		return nil, false
	}
	return &accountv1.NullableStringPatch{
		Value: &accountv1.NullableStringPatch_StringValue{StringValue: value},
	}, true
}
