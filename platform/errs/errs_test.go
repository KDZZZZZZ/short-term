package errs

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCodeSurvivesGRPCRoundTrip(t *testing.T) {
	t.Parallel()

	for code := range grpcCodes {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()

			// 服务端的 status.FromError 就是 gRPC 运行时写入 trailer 前执行的操作；
			// 将其解析回来就是客户端看到的结果。
			sent := New(code, "boom")
			wire := status.FromProto(status.Convert(sent).Proto()).Err()

			if got := CodeOf(wire); got != code {
				t.Fatalf("CodeOf after round trip = %q, want %q", got, code)
			}
			if got := MessageOf(wire); got != "boom" {
				t.Fatalf("MessageOf after round trip = %q, want boom", got)
			}
		})
	}
}

func TestCodeOfClassifiesForeignErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want Code
	}{
		{name: "plain error", err: errors.New("pq: connection refused"), want: CodeInternal},
		{name: "wrapped domain error", err: fmt.Errorf("load: %w", New(CodeForbidden, "no")), want: CodeForbidden},
		{name: "bare deadline status", err: status.Error(codes.DeadlineExceeded, "timeout"), want: CodeInternal},
		{name: "bare not found status", err: status.Error(codes.NotFound, "missing"), want: CodeResourceNotFound},
		{name: "nil", err: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := CodeOf(tt.err); got != tt.want {
				t.Fatalf("CodeOf = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWrapKeepsCauseOutOfTheClientMessage(t *testing.T) {
	t.Parallel()

	cause := errors.New("duplicate key value violates unique constraint")
	err := Wrap(CodeStudentNoExists, "该学号已注册", cause)

	if got := MessageOf(err); got != "该学号已注册" {
		t.Fatalf("MessageOf = %q, want the client-safe message", got)
	}
	if !errors.Is(err, cause) {
		t.Fatal("the cause should stay reachable for logs")
	}
	if got := status.Convert(err).Message(); got != "该学号已注册" {
		t.Fatalf("gRPC message = %q, want the client-safe message", got)
	}
}

func TestGRPCStatusUsesTheDesignedCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code Code
		want codes.Code
	}{
		{code: CodeValidation, want: codes.InvalidArgument},
		{code: CodeUnauthorized, want: codes.Unauthenticated},
		{code: CodeForbidden, want: codes.PermissionDenied},
		{code: CodeResourceNotFound, want: codes.NotFound},
		{code: CodeProductNotAvailable, want: codes.FailedPrecondition},
		{code: CodeTradeStateConflict, want: codes.Aborted},
		{code: CodeInternal, want: codes.Internal},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			t.Parallel()

			if got := status.Convert(New(tt.code, "x")).Code(); got != tt.want {
				t.Fatalf("gRPC code = %v, want %v", got, tt.want)
			}
		})
	}
}
