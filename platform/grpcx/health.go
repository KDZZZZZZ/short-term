package grpcx

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// HealthCheck reports whether a service can currently accept requests.
// Implementations must be cheap and bounded by the caller's context.
type HealthCheck func(context.Context) error

// RegisterHealthServer exposes the standard gRPC health protocol. The empty
// service name represents the whole process, as recommended by the gRPC health
// specification. A failed dependency check is reported as NOT_SERVING rather
// than as an RPC error so readiness probes can distinguish a reachable but
// unavailable process from a broken transport.
func RegisterHealthServer(server *grpc.Server, check HealthCheck) {
	healthv1.RegisterHealthServer(server, &healthServer{check: check})
}

type healthServer struct {
	healthv1.UnimplementedHealthServer
	check HealthCheck
}

func (s *healthServer) Check(ctx context.Context, req *healthv1.HealthCheckRequest) (*healthv1.HealthCheckResponse, error) {
	if req.GetService() != "" {
		return nil, status.Error(codes.NotFound, "health service is unknown")
	}
	return &healthv1.HealthCheckResponse{Status: s.servingStatus(ctx)}, nil
}

func (s *healthServer) List(ctx context.Context, _ *healthv1.HealthListRequest) (*healthv1.HealthListResponse, error) {
	return &healthv1.HealthListResponse{Statuses: map[string]*healthv1.HealthCheckResponse{
		"": {Status: s.servingStatus(ctx)},
	}}, nil
}

func (s *healthServer) servingStatus(ctx context.Context) healthv1.HealthCheckResponse_ServingStatus {
	if s.check != nil {
		if err := s.check(ctx); err != nil {
			return healthv1.HealthCheckResponse_NOT_SERVING
		}
	}
	return healthv1.HealthCheckResponse_SERVING
}
