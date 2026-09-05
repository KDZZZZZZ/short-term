package handler

import (
	"context"
	"net/http"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/application/aggregation"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/mapper"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/middleware"
)

// Users 提供 /users/me 和 /users/me/password。
type Users struct {
	accounts   accountv1.AccountServiceClient
	aggregator *aggregation.Aggregator
	responder  Responder
}

// NewUsers 构造当前用户处理器。
func NewUsers(accounts accountv1.AccountServiceClient, aggregator *aggregation.Aggregator, responder Responder) *Users {
	return &Users{accounts: accounts, aggregator: aggregator, responder: responder}
}

// Me 处理 GET /users/me。
func (h *Users) Me(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.ActorID(r.Context())

	resp, err := h.accounts.GetProfile(grpcx.WithActor(r.Context(), actorID), &accountv1.GetProfileRequest{UserId: actorID})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	average, err := aggregatedAverageScore(r.Context(), h.aggregator, actorID)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.OK(w, r, mapper.UserMe(resp.GetUser(), average))
}

// UpdateMe 处理 PATCH /users/me。
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

	wechat, ok := contactPatch(w, r, h.responder, body.Wechat, "微信号")
	if !ok {
		return
	}
	req.Wechat = wechat

	qq, ok := contactPatch(w, r, h.responder, body.QQ, "QQ 号")
	if !ok {
		return
	}
	req.Qq = qq

	if req.Nickname == nil && req.Wechat == nil && req.Qq == nil {
		// UpdateProfileRequest 声明 minProperties: 1。
		h.responder.Fail(w, r, errs.CodeValidation, "请至少修改一项资料")
		return
	}

	resp, err := h.accounts.UpdateProfile(grpcx.WithActor(r.Context(), req.GetUserId()), req)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	average, err := aggregatedAverageScore(r.Context(), h.aggregator, req.GetUserId())
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.OK(w, r, mapper.UserMe(resp.GetUser(), average))
}

// Profile 处理 GET /users/{userId}。
//
// 公开资料只包含昵称与卖家平均分；GetUser 的响应类型不包含学号，
// 这里也不会读取其联系方式字段。
func (h *Users) Profile(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userId")

	resp, err := h.accounts.GetUser(grpcx.WithActor(r.Context(), middleware.ActorID(r.Context())), &accountv1.GetUserRequest{UserId: userID})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	average, err := aggregatedAverageScore(r.Context(), h.aggregator, userID)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	stats, err := h.aggregator.SellerStats(r.Context(), userID)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.OK(w, r, mapper.UserProfile(resp.GetUser().GetId(), resp.GetUser().GetNickname(), average, stats))
}

// ChangePassword 处理 PUT /users/me/password。
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

// aggregatedAverageScore 读取用户作为卖家收到的评分平均值；
// 没有评分时返回 nil，映射为契约中的 null。
func aggregatedAverageScore(ctx context.Context, aggregator *aggregation.Aggregator, userID string) (*string, error) {
	if aggregator == nil {
		return nil, nil
	}
	scores, err := aggregator.AverageScores(ctx, []string{userID})
	if err != nil {
		return nil, err
	}
	if score, ok := scores[userID]; ok {
		return &score, nil
	}
	return nil, nil
}

// contactPatch 将更新输入转换为线路上的 patch 消息。省略表示保持不变，
// null 被公开契约禁止，字符串表示新增或修改。
func contactPatch(w http.ResponseWriter, r *http.Request, responder Responder, field dto.RawField, label string) (*accountv1.NullableStringPatch, bool) {
	if !field.Present {
		return nil, true
	}
	if field.IsNull() {
		responder.Fail(w, r, errs.CodeValidation, label+"不能为 null")
		return nil, false
	}

	value, err := field.String()
	if err != nil {
		responder.Fail(w, r, errs.CodeValidation, label+"必须是非空字符串")
		return nil, false
	}
	return &accountv1.NullableStringPatch{
		Value: &accountv1.NullableStringPatch_StringValue{StringValue: value},
	}, true
}
