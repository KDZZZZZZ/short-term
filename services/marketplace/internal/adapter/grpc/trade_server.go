package grpc

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// CreateTrade 记录买家的购买意向。
func (s *Server) CreateTrade(ctx context.Context, req *marketplacev1.CreateTradeRequest) (*marketplacev1.CreateTradeResponse, error) {
	result, err := s.trades.Create(ctx, application.CreateTradeCommand{
		ActorID:               req.GetActorId(),
		ProductID:             req.GetProductId(),
		ConversationIDPresent: req.GetConversationIdPresent(),
		ConversationID:        req.ConversationId,
		IdempotencyKey:        req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}

	trade, err := s.commandTrade(result)
	if err != nil {
		return nil, err
	}
	return &marketplacev1.CreateTradeResponse{
		Trade: trade, Replayed: result.Replayed, Created: result.Created,
	}, nil
}

// AcceptTrade 是卖家的 PENDING -> ACCEPTED 操作。
func (s *Server) AcceptTrade(ctx context.Context, req *marketplacev1.AcceptTradeRequest) (*marketplacev1.AcceptTradeResponse, error) {
	result, err := s.trades.Accept(ctx, application.TradeActionCommand{
		ActorID:        req.GetActorId(),
		TradeID:        req.GetTradeId(),
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}

	trade, err := s.commandTrade(result)
	if err != nil {
		return nil, err
	}
	return &marketplacev1.AcceptTradeResponse{Trade: trade, Replayed: result.Replayed}, nil
}

// RejectTrade 是卖家的 PENDING -> CANCELLED 操作。
func (s *Server) RejectTrade(ctx context.Context, req *marketplacev1.RejectTradeRequest) (*marketplacev1.RejectTradeResponse, error) {
	result, err := s.trades.Reject(ctx, application.TradeActionCommand{
		ActorID:        req.GetActorId(),
		TradeID:        req.GetTradeId(),
		Reason:         req.GetReason(),
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}

	trade, err := s.commandTrade(result)
	if err != nil {
		return nil, err
	}
	return &marketplacev1.RejectTradeResponse{Trade: trade, Replayed: result.Replayed}, nil
}

// CancelTrade 取消交易；交易已接受时同时释放商品。
func (s *Server) CancelTrade(ctx context.Context, req *marketplacev1.CancelTradeRequest) (*marketplacev1.CancelTradeResponse, error) {
	result, err := s.trades.Cancel(ctx, application.TradeActionCommand{
		ActorID:        req.GetActorId(),
		TradeID:        req.GetTradeId(),
		Reason:         req.GetReason(),
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}

	trade, err := s.commandTrade(result)
	if err != nil {
		return nil, err
	}
	return &marketplacev1.CancelTradeResponse{Trade: trade, Replayed: result.Replayed}, nil
}

// ConfirmTrade 记录一方对线下交付的确认。
func (s *Server) ConfirmTrade(ctx context.Context, req *marketplacev1.ConfirmTradeRequest) (*marketplacev1.ConfirmTradeResponse, error) {
	result, err := s.trades.Confirm(ctx, application.TradeActionCommand{
		ActorID:        req.GetActorId(),
		TradeID:        req.GetTradeId(),
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}

	trade, err := s.commandTrade(result)
	if err != nil {
		return nil, err
	}
	return &marketplacev1.ConfirmTradeResponse{Trade: trade, Replayed: result.Replayed}, nil
}

// GetTrade 向交易双方之一返回一笔交易。
func (s *Server) GetTrade(ctx context.Context, req *marketplacev1.GetTradeRequest) (*marketplacev1.GetTradeResponse, error) {
	trade, err := s.trades.Get(ctx, req.GetActorId(), req.GetTradeId())
	if err != nil {
		return nil, err
	}

	mapped, err := s.trade(ctx, trade)
	if err != nil {
		return nil, err
	}
	return &marketplacev1.GetTradeResponse{Trade: mapped}, nil
}

// ListTrades 返回当前用户作为买家或卖家的交易。
func (s *Server) ListTrades(ctx context.Context, req *marketplacev1.ListTradesRequest) (*marketplacev1.ListTradesResponse, error) {
	query := application.ListTradesQuery{
		ActorID: req.GetActorId(),
		AsBuyer: req.GetAsBuyer(),
		Page:    application.Page{Number: req.GetPage(), Size: req.GetPageSize()},
	}
	if req.Status != nil {
		value := tradeStatus(req.GetStatus())
		query.Status = &value
	}

	page, err := s.trades.List(ctx, query)
	if err != nil {
		return nil, err
	}

	items, err := s.tradeList(ctx, page.Items)
	if err != nil {
		return nil, err
	}
	return &marketplacev1.ListTradesResponse{Page: &marketplacev1.TradePage{
		Items:    items,
		Page:     page.Page,
		PageSize: page.Size,
		Total:    page.Total,
	}}, nil
}

// --- 映射 -------------------------------------------------------------------

// commandTrade renders the product projection captured in the same command
// result. Re-reading here would make an idempotent retry return a different
// body if the product changed after the first response.
func (s *Server) commandTrade(result application.TradeResult) (*marketplacev1.Trade, error) {
	if result.Trade == nil || result.Product == nil {
		return nil, errs.New(errs.CodeInternal, "交易命令结果不完整")
	}
	return s.tradeProto(result.Trade, result.Product), nil
}

// trade 渲染一笔交易，并用商品当前状态补全商品投影。
func (s *Server) trade(ctx context.Context, trade *domain.Trade) (*marketplacev1.Trade, error) {
	mapped, err := s.tradeList(ctx, []*domain.Trade{trade})
	if err != nil {
		return nil, err
	}
	return mapped[0], nil
}

// tradeList 渲染多笔交易，并一次批量读取所有被引用商品，
// 从而避免一页交易按行查询。
//
// 商品投影在响应时读取，而不是随交易创建快照，这使 TradeProduct.status 始终是当前状态
// （docs/software-design.md 第 4.2 节）。
func (s *Server) tradeList(ctx context.Context, trades []*domain.Trade) ([]*marketplacev1.Trade, error) {
	ids := make([]string, 0, len(trades))
	for _, trade := range trades {
		ids = append(ids, trade.ProductID)
	}

	products, err := s.products.BatchGet(ctx, ids)
	if err != nil {
		return nil, err
	}

	mapped := make([]*marketplacev1.Trade, 0, len(trades))
	for _, trade := range trades {
		product, ok := products[trade.ProductID]
		if !ok {
			// 交易始终引用存在的商品行；如果商品缺失，说明二者已经不同步，
			// 不能用虚构的投影掩盖这一问题。
			return nil, errs.Newf(errs.CodeInternal, "交易 %s 关联的商品缺失", trade.ID)
		}
		mapped = append(mapped, s.tradeProto(trade, product))
	}
	return mapped, nil
}

func (s *Server) tradeProto(trade *domain.Trade, product *domain.Product) *marketplacev1.Trade {
	tradeProduct := &marketplacev1.TradeProduct{
		Id:     product.ID,
		Title:  product.Title,
		Status: statusProto(product.Status),
	}
	if key := product.CoverObjectKey(); key != "" {
		url := s.products.ImageURL(key)
		tradeProduct.CoverUrl = &url
	}

	return &marketplacev1.Trade{
		Id:                 trade.ID,
		Product:            tradeProduct,
		BuyerId:            trade.BuyerID,
		SellerId:           trade.SellerID,
		ConversationId:     trade.ConversationID,
		PriceSnapshotMinor: trade.PriceSnapshotMinor,
		Status:             tradeStatusProto(trade.Status),
		BuyerConfirmed:     trade.BuyerConfirmed(),
		SellerConfirmed:    trade.SellerConfirmed(),
		CancelReason:       trade.CancelReason,
		CreatedAt:          timestamppb.New(trade.CreatedAt),
		AcceptedAt:         optionalTimestamp(trade.AcceptedAt),
		CompletedAt:        optionalTimestamp(trade.CompletedAt),
		CancelledAt:        optionalTimestamp(trade.CancelledAt),
		UpdatedAt:          timestamppb.New(trade.UpdatedAt),
	}
}

func optionalTimestamp(value *time.Time) *timestamppb.Timestamp {
	if value == nil {
		return nil
	}
	return timestamppb.New(*value)
}

func tradeStatus(value marketplacev1.TradeStatus) domain.TradeStatus {
	switch value {
	case marketplacev1.TradeStatus_TRADE_STATUS_PENDING:
		return domain.TradePending
	case marketplacev1.TradeStatus_TRADE_STATUS_ACCEPTED:
		return domain.TradeAccepted
	case marketplacev1.TradeStatus_TRADE_STATUS_COMPLETED:
		return domain.TradeCompleted
	case marketplacev1.TradeStatus_TRADE_STATUS_CANCELLED:
		return domain.TradeCancelled
	default:
		return ""
	}
}

func tradeStatusProto(value domain.TradeStatus) marketplacev1.TradeStatus {
	switch value {
	case domain.TradePending:
		return marketplacev1.TradeStatus_TRADE_STATUS_PENDING
	case domain.TradeAccepted:
		return marketplacev1.TradeStatus_TRADE_STATUS_ACCEPTED
	case domain.TradeCompleted:
		return marketplacev1.TradeStatus_TRADE_STATUS_COMPLETED
	case domain.TradeCancelled:
		return marketplacev1.TradeStatus_TRADE_STATUS_CANCELLED
	default:
		return marketplacev1.TradeStatus_TRADE_STATUS_UNSPECIFIED
	}
}

var _ marketplacev1.MarketplaceServiceServer = (*Server)(nil)
