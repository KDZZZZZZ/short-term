// Package errs defines the stable internal error model shared by every
// service. A domain code maps to a gRPC status on the wire, and the Gateway
// maps the same code to the public HTTP status and ErrorCode defined by
// openapi/components/schemas.yaml#/ErrorCode.
//
// The error code travels in a google.rpc.ErrorInfo detail rather than in the
// status message, so the Gateway never has to parse human-readable text.
package errs

import (
	"errors"
	"fmt"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Domain is the ErrorInfo domain identifying this system as the error source.
const Domain = "shortterm"

// Code is a public ErrorCode from the OpenAPI contract. The internal model
// deliberately reuses the public enum so that no service can invent an error
// the contract cannot express.
type Code string

// The full set of ErrorCode values in the approved OpenAPI contract.
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
	CodeSelfActionNotAllowed Code = "SELF_ACTION_NOT_ALLOWED"
	CodeInternal             Code = "INTERNAL_ERROR"
)

// grpcCodes follows docs/software-design.md section 7.3. Aborted is reserved
// for conflicts a client may retry after re-reading state; FailedPrecondition
// marks conflicts that repeat until the resource itself changes.
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
	CodeSelfActionNotAllowed: codes.FailedPrecondition,
	CodeInternal:             codes.Internal,
}

// Error is a domain error carrying a contract error code. The wrapped cause is
// kept for logs only; it is never sent to a client.
type Error struct {
	Code  Code
	Msg   string
	cause error
}

// New builds a domain error without a cause.
func New(code Code, msg string) *Error { return &Error{Code: code, Msg: msg} }

// Newf builds a domain error with a formatted message.
func Newf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Wrap attaches a cause to a domain error so logs keep the original failure.
func Wrap(code Code, msg string, cause error) *Error {
	return &Error{Code: code, Msg: msg, cause: cause}
}

func (e *Error) Error() string {
	if e.cause != nil {
		return string(e.Code) + ": " + e.Msg + ": " + e.cause.Error()
	}
	return string(e.Code) + ": " + e.Msg
}

// Unwrap exposes the cause to errors.Is and errors.As.
func (e *Error) Unwrap() error { return e.cause }

// GRPCStatus lets the gRPC runtime serialise the code and message directly.
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

// CodeOf reports the contract error code carried by err. An error that did not
// originate from this package is reported as INTERNAL_ERROR so an unexpected
// failure is never silently rendered as a client mistake.
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

// MessageOf returns the client-safe message for err, falling back to the gRPC
// status message when the error crossed a service boundary.
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

// fallbackCode maps a bare gRPC status from a downstream that did not attach
// ErrorInfo, for example a deadline enforced by the transport itself.
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
