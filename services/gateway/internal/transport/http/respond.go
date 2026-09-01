// Package http implements the public REST surface defined by
// openapi/openapi.yaml. Every response body, status code and error code in
// this package must match that contract; the contract is the source of truth
// and this code follows it, never the other way round.
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

// httpStatus maps a contract error code to its public HTTP status, following
// docs/software-design.md section 7.3 and the responses each path declares in
// openapi/paths.
//
// The contract has no 503 or 504, so a downstream that is unavailable or out
// of time is reported as 500 INTERNAL_ERROR. Whether to extend the contract is
// an open question in docs/software-design.md section 11.3.
var httpStatus = map[errs.Code]int{
	errs.CodeValidation:           http.StatusBadRequest,
	errs.CodeContactRequired:      http.StatusBadRequest,
	errs.CodeSelfActionNotAllowed: http.StatusBadRequest,
	errs.CodeUnauthorized:         http.StatusUnauthorized,
	errs.CodeForbidden:            http.StatusForbidden,
	errs.CodeResourceNotFound:     http.StatusNotFound,
	errs.CodeStudentNoExists:      http.StatusConflict,
	errs.CodeImageLimitExceeded:   http.StatusConflict,
	errs.CodeProductNotAvailable:  http.StatusConflict,
	errs.CodeProductStateConflict: http.StatusConflict,
	errs.CodeTradeStateConflict:   http.StatusConflict,
	errs.CodePayloadTooLarge:      http.StatusRequestEntityTooLarge,
	errs.CodeInternal:             http.StatusInternalServerError,
}

// StatusFor reports the public HTTP status for a contract error code.
func StatusFor(code errs.Code) int {
	if status, ok := httpStatus[code]; ok {
		return status
	}
	return http.StatusInternalServerError
}

// Responder writes contract-shaped responses and logs failures.
type Responder struct {
	logger *slog.Logger
}

// NewResponder builds a Responder.
func NewResponder(logger *slog.Logger) *Responder {
	if logger == nil {
		logger = slog.Default()
	}
	return &Responder{logger: logger}
}

// OK writes a 200 success envelope.
func (r *Responder) OK(w http.ResponseWriter, req *http.Request, data any) {
	r.Success(w, req, http.StatusOK, data)
}

// Created writes a 201 success envelope.
func (r *Responder) Created(w http.ResponseWriter, req *http.Request, data any) {
	r.Success(w, req, http.StatusCreated, data)
}

// Empty writes a success envelope whose data is the empty object, which is
// what the contract's EmptySuccess schema requires.
func (r *Responder) Empty(w http.ResponseWriter, req *http.Request) {
	r.Success(w, req, http.StatusOK, struct{}{})
}

// Success writes a success envelope with an explicit status.
func (r *Responder) Success(w http.ResponseWriter, req *http.Request, status int, data any) {
	r.write(w, req, status, dto.SuccessEnvelope{Code: "OK", Message: "success", Data: data})
}

// Error maps err to its contract error code and writes the matching response.
// The cause is logged, never sent: it may contain database or driver text.
func (r *Responder) Error(w http.ResponseWriter, req *http.Request, err error) {
	code := errs.CodeOf(err)
	status := StatusFor(code)
	message := errs.MessageOf(err)
	if code == errs.CodeInternal {
		// A downstream failure message is written for operators, not clients.
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

// Fail writes a specific contract error code with a client-safe message.
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
