// Package http 实现 openapi/openapi.yaml 定义的公开 REST 接口。
// 本包中的每个响应正文、状态码和错误码都必须符合该契约；契约是事实真源，
// 代码遵循契约，而不是反过来。
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/logging"
	"github.com/KDZZZZZZ/short-term/platform/observability"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
)

// httpStatus 按照 docs/software-design.md 第 7.3 节及 openapi/paths 中各路径声明的
// 响应，将契约错误码映射为公开 HTTP 状态。
//
// 契约没有 503 或 504，因此不可用或超时的下游会报告为 500 INTERNAL_ERROR。
// 是否扩展契约是 docs/software-design.md 第 11.3 节中的未决问题。
var httpStatus = map[errs.Code]int{
	errs.CodeValidation:           http.StatusBadRequest,
	errs.CodeContactRequired:      http.StatusBadRequest,
	errs.CodeImageLimitExceeded:   http.StatusBadRequest,
	errs.CodeUnauthorized:         http.StatusUnauthorized,
	errs.CodeForbidden:            http.StatusForbidden,
	errs.CodeResourceNotFound:     http.StatusNotFound,
	errs.CodeStudentNoExists:      http.StatusConflict,
	errs.CodeProductNotAvailable:  http.StatusConflict,
	errs.CodeProductStateConflict: http.StatusConflict,
	errs.CodeTradeStateConflict:   http.StatusConflict,
	errs.CodeConversationMismatch: http.StatusConflict,
	errs.CodeSelfActionNotAllowed: http.StatusConflict,
	errs.CodeTradeReviewExists:    http.StatusConflict,
	errs.CodePayloadTooLarge:      http.StatusRequestEntityTooLarge,
	errs.CodeRateLimited:          http.StatusTooManyRequests,
	errs.CodeInternal:             http.StatusInternalServerError,
}

// StatusFor 返回契约错误码对应的公开 HTTP 状态。
func StatusFor(code errs.Code) int {
	if status, ok := httpStatus[code]; ok {
		return status
	}
	return http.StatusInternalServerError
}

// Responder 写入符合契约结构的响应并记录失败。
type Responder struct {
	logger *slog.Logger
}

// NewResponder 构造 Responder。
func NewResponder(logger *slog.Logger) *Responder {
	if logger == nil {
		logger = slog.Default()
	}
	return &Responder{logger: logger}
}

// OK 写入 200 成功信封。
func (r *Responder) OK(w http.ResponseWriter, req *http.Request, data any) {
	r.Success(w, req, http.StatusOK, data)
}

// Created 写入 201 成功信封。
func (r *Responder) Created(w http.ResponseWriter, req *http.Request, data any) {
	r.Success(w, req, http.StatusCreated, data)
}

// Empty 写入 data 为空对象的成功信封，这是契约 EmptySuccess schema 所要求的形式。
func (r *Responder) Empty(w http.ResponseWriter, req *http.Request) {
	r.Success(w, req, http.StatusOK, struct{}{})
}

// Success 以指定状态写入成功信封。
func (r *Responder) Success(w http.ResponseWriter, req *http.Request, status int, data any) {
	r.write(w, req, status, dto.SuccessEnvelope{Code: "OK", Message: "success", Data: data})
}

// Error 将 err 映射为契约错误码，并写入匹配的响应。
// 原因只记录到日志而不发送给客户端，因为其中可能包含数据库或驱动文本。
func (r *Responder) Error(w http.ResponseWriter, req *http.Request, err error) {
	code := errs.CodeOf(err)
	status := StatusFor(code)
	message := errs.MessageOf(err)
	if code == errs.CodeInternal {
		// 下游失败消息供运维人员使用，不返回给客户端。
		message = "服务暂时不可用"
	}
	if message == "" {
		message = string(code)
	}

	entry := observability.LoggerWith(req.Context(), r.logger).With(
		slog.String("method", req.Method),
		slog.String("path", req.URL.Path),
		slog.String(logging.FieldErrorCode, string(code)),
		slog.Int("status", status),
	)
	if status >= http.StatusInternalServerError {
		entry.Error("request failed", slog.String("error", err.Error()))
	} else {
		entry.Info("request rejected")
	}

	r.write(w, req, status, dto.ErrorEnvelope{Code: string(code), Message: message})
}

// Fail 使用客户端安全消息写入指定的契约错误码。
func (r *Responder) Fail(w http.ResponseWriter, req *http.Request, code errs.Code, message string) {
	r.Error(w, req, errs.New(code, message))
}

func (r *Responder) write(w http.ResponseWriter, req *http.Request, status int, body any) {
	payload, err := json.Marshal(body)
	if err != nil {
		observability.LoggerWith(req.Context(), r.logger).Error("encode response failed",
			slog.String("error", err.Error()))
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"INTERNAL_ERROR","message":"服务暂时不可用"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(payload); err != nil {
		observability.LoggerWith(req.Context(), r.logger).Warn("write response failed",
			slog.String("error", err.Error()))
	}
}
