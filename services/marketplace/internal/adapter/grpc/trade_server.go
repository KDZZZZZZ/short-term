package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
)

// The trade state machine is milestone M3 of docs/backend-development-plan.md.
// Until it lands, these RPCs report Unimplemented rather than a domain error,
// so a caller can tell "this build cannot do that yet" from "your request was
// refused". The Gateway maps Unimplemented to 500 INTERNAL_ERROR, which is the
// only honest answer the approved contract can give for a missing capability.
func tradeNotImplemented() error {
	return status.Error(codes.Unimplemented, "交易功能尚未上线")
}

// CreateTrade is not implemented yet.
func (s *Server) CreateTrade(context.Context, *marketplacev1.CreateTradeRequest) (*marketplacev1.CreateTradeResponse, error) {
	return nil, tradeNotImplemented()
}

// ListTrades is not implemented yet.
func (s *Server) ListTrades(context.Context, *marketplacev1.ListTradesRequest) (*marketplacev1.ListTradesResponse, error) {
	return nil, tradeNotImplemented()
}

// GetTrade is not implemented yet.
func (s *Server) GetTrade(context.Context, *marketplacev1.GetTradeRequest) (*marketplacev1.GetTradeResponse, error) {
	return nil, tradeNotImplemented()
}

// AcceptTrade is not implemented yet.
func (s *Server) AcceptTrade(context.Context, *marketplacev1.AcceptTradeRequest) (*marketplacev1.AcceptTradeResponse, error) {
	return nil, tradeNotImplemented()
}

// RejectTrade is not implemented yet.
func (s *Server) RejectTrade(context.Context, *marketplacev1.RejectTradeRequest) (*marketplacev1.RejectTradeResponse, error) {
	return nil, tradeNotImplemented()
}

// CancelTrade is not implemented yet.
func (s *Server) CancelTrade(context.Context, *marketplacev1.CancelTradeRequest) (*marketplacev1.CancelTradeResponse, error) {
	return nil, tradeNotImplemented()
}

// ConfirmTrade is not implemented yet.
func (s *Server) ConfirmTrade(context.Context, *marketplacev1.ConfirmTradeRequest) (*marketplacev1.ConfirmTradeResponse, error) {
	return nil, tradeNotImplemented()
}

var _ marketplacev1.MarketplaceServiceServer = (*Server)(nil)
