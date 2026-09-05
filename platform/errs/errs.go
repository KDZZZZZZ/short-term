// Package errs 定义所有服务共享的稳定内部错误模型。领域错误码映射为线路上的
// gRPC 状态，网关再将同一错误码映射为 openapi/components/schemas.yaml#/ErrorCode
// 定义的公开 HTTP 状态和 ErrorCode。
//
// 错误码放在 google.rpc.ErrorInfo 详情中，而不是状态消息中，因此网关无需解析
// 人类可读文本。
package errs

import (
	"errors"
	"fmt"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Domain 是 ErrorInfo 的域，用于标识本系统为错误来源。
const Domain = "shortterm"

// Code 是 OpenAPI 契约中的公开 ErrorCode。内部模型有意复用公开枚举，
// 使任何服务都无法发明契约无法表达的错误。
type Code string

// 已批准 OpenAPI 契约中的全部 ErrorCode 值。
const (
	CodeValidation           Code = "VALIDATION_ERROR"
	CodeContactRequired      Code = "CONTACT_REQUIRED"
	CodeImageLimitExceeded   Code = "IMAGE_LIMIT_EXCEEDED"
	CodePayloadTooLarge      Code = "PAYLOAD_TOO_LARGE"
	CodeUnauthorized         Code = "UNAUTHORIZED"
	CodeForbidden            Code = "FORBIDDEN"
	CodeResourceNotFound     Code = "RESOURCE_NOT_FOUND"
	CodeStudentNoExists      Code = "STUDENT_NO_EXISTS"
	CodeProductNotAvailable  Code = "PRODUCT_NOT_AVAILABLE"
	CodeTradeStateConflict   Code = "TRADE_STATE_CONFLICT"
	CodeProductStateConflict Code = "PRODUCT_STATE_CONFLICT"
	CodeConversationMismatch Code = "CONVERSATION_MISMATCH"
	CodeSelfActionNotAllowed Code = "SELF_ACTION_NOT_ALLOWED"
	CodeTradeReviewExists    Code = "TRADE_REVIEW_ALREADY_EXISTS"
	CodeRateLimited          Code = "RATE_LIMITED"
	CodeInternal             Code = "INTERNAL_ERROR"
)

// grpcCodes 遵循 docs/software-design.md 第 7.3 节。Aborted 用于客户端重新读取
// 状态后可以重试的冲突；FailedPrecondition 用于只有资源本身变化后才会消失的冲突。
var grpcCodes = map[Code]codes.Code{
	CodeValidation:           codes.InvalidArgument,
	CodeContactRequired:      codes.FailedPrecondition,
	CodeImageLimitExceeded:   codes.FailedPrecondition,
	CodePayloadTooLarge:      codes.InvalidArgument,
	CodeUnauthorized:         codes.Unauthenticated,
	CodeForbidden:            codes.PermissionDenied,
	CodeResourceNotFound:     codes.NotFound,
	CodeStudentNoExists:      codes.AlreadyExists,
	CodeProductNotAvailable:  codes.FailedPrecondition,
	CodeTradeStateConflict:   codes.Aborted,
	CodeProductStateConflict: codes.FailedPrecondition,
	CodeConversationMismatch: codes.FailedPrecondition,
	CodeSelfActionNotAllowed: codes.FailedPrecondition,
	CodeTradeReviewExists:    codes.AlreadyExists,
	CodeRateLimited:          codes.ResourceExhausted,
	CodeInternal:             codes.Internal,
}

// Error 是携带契约错误码的领域错误。被包装的原因只保留用于日志，绝不发送给客户端。
type Error struct {
	Code  Code
	Msg   string
	cause error
}

// New 构造不带原因的领域错误。
func New(code Code, msg string) *Error { return &Error{Code: code, Msg: msg} }

// Newf 使用格式化消息构造领域错误。
func Newf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Wrap 将原因附加到领域错误，使日志保留原始失败信息。
func Wrap(code Code, msg string, cause error) *Error {
	return &Error{Code: code, Msg: msg, cause: cause}
}

func (e *Error) Error() string {
	if e.cause != nil {
		return string(e.Code) + ": " + e.Msg + ": " + e.cause.Error()
	}
	return string(e.Code) + ": " + e.Msg
}

// Unwrap 向 errors.Is 和 errors.As 暴露原因。
func (e *Error) Unwrap() error { return e.cause }

// GRPCStatus 让 gRPC 运行时直接序列化错误码和消息。
func (e *Error) GRPCStatus() *status.Status {
	code, ok := grpcCodes[e.Code]
	if !ok {
		code = codes.Internal
	}
	st := status.New(code, e.Msg)
	detailed, err := st.WithDetails(&errdetails.ErrorInfo{
		Reason: string(e.Code),
		Domain: Domain,
	})
	if err != nil {
		return st
	}
	return detailed
}

// CodeOf 返回 err 携带的契约错误码。不是由本包产生的错误会报告为 INTERNAL_ERROR，
// 以免意外失败被默默呈现为客户端错误。
func CodeOf(err error) Code {
	if err == nil {
		return ""
	}
	var domainErr *Error
	if errors.As(err, &domainErr) {
		return domainErr.Code
	}
	st, ok := status.FromError(err)
	if !ok {
		return CodeInternal
	}
	for _, detail := range st.Details() {
		info, isInfo := detail.(*errdetails.ErrorInfo)
		if !isInfo || info.GetDomain() != Domain {
			continue
		}
		if _, known := grpcCodes[Code(info.GetReason())]; known {
			return Code(info.GetReason())
		}
	}
	return fallbackCode(st.Code())
}

// MessageOf 返回 err 的客户端安全消息；错误跨越服务边界时回退到 gRPC 状态消息。
func MessageOf(err error) string {
	if err == nil {
		return ""
	}
	var domainErr *Error
	if errors.As(err, &domainErr) {
		return domainErr.Msg
	}
	if st, ok := status.FromError(err); ok {
		return st.Message()
	}
	return err.Error()
}

// fallbackCode 将未附加 ErrorInfo 的下游原始 gRPC 状态映射为契约错误码，
// 例如传输层自身强制执行的截止时间。
func fallbackCode(code codes.Code) Code {
	switch code {
	case codes.InvalidArgument, codes.OutOfRange:
		return CodeValidation
	case codes.Unauthenticated:
		return CodeUnauthorized
	case codes.PermissionDenied:
		return CodeForbidden
	case codes.NotFound:
		return CodeResourceNotFound
	case codes.AlreadyExists:
		return CodeStudentNoExists
	case codes.Aborted:
		return CodeTradeStateConflict
	default:
		return CodeInternal
	}
}
