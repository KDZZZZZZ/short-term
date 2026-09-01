package grpcx

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func TestHealthServerReflectsDependencyState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		check HealthCheck
		want  healthv1.HealthCheckResponse_ServingStatus
	}{
		{name: "serving", check: func(context.Context) error { return nil }, want: healthv1.HealthCheckResponse_SERVING},
		{name: "dependency down", check: func(context.Context) error { return errors.New("database down") }, want: healthv1.HealthCheckResponse_NOT_SERVING},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := &healthServer{check: tt.check}
			response, err := server.Check(t.Context(), &healthv1.HealthCheckRequest{})
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if response.GetStatus() != tt.want {
				t.Fatalf("status = %s, want %s", response.GetStatus(), tt.want)
			}
		})
	}
}

func TestHealthServerRejectsUnknownNamedService(t *testing.T) {
	t.Parallel()

	server := &healthServer{}
	_, err := server.Check(t.Context(), &healthv1.HealthCheckRequest{Service: "unknown"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %s, want NotFound", status.Code(err))
	}
}
