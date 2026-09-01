package application

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// EventSchemaVersion 为下方载荷版本化。消费者在解析载荷前读取它，
// 载荷结构发生变化时递增该版本。
const EventSchemaVersion int32 = 1

// Outbox 记录携带的聚合类型名称。
const (
	AggregateProduct = "product"
	AggregateTrade   = "trade"
)

// tradeEventPayload 是已发布交易事实的结构。
//
// 它是专用的事件 DTO：既不复用数据库行，也不复用公开 HTTP 正文，
// 因此任一方发生变化都不会静默改变消费者接收到的内容
// （docs/software-design.md 第 4.1 节）。
type tradeEventPayload struct {
	TradeID            string `json:"trade_id"`
	ProductID          string `json:"product_id"`
	BuyerID            string `json:"buyer_id"`
	SellerID           string `json:"seller_id"`
	Status             string `json:"status"`
	PriceSnapshotMinor int64  `json:"price_snapshot_minor"`
}

// productStatusEventPayload 是已发布商品状态变更的结构。
type productStatusEventPayload struct {
	ProductID string `json:"product_id"`
	SellerID  string `json:"seller_id"`
	Status    string `json:"status"`
	// TradeID 指出导致此次变更的交易（如果存在）。
	TradeID string `json:"trade_id,omitempty"`
}

// eventFactory 构造已附加标识、时间戳和追踪上下文的 Outbox 记录。
type eventFactory struct {
	newID   func() string
	now     time.Time
	traceID string
}

func (f eventFactory) tradeEvent(eventType string, trade *domain.Trade) (Event, error) {
	payload, err := json.Marshal(tradeEventPayload{
		TradeID:            trade.ID,
		ProductID:          trade.ProductID,
		BuyerID:            trade.BuyerID,
		SellerID:           trade.SellerID,
		Status:             string(trade.Status),
		PriceSnapshotMinor: trade.PriceSnapshotMinor,
	})
	if err != nil {
		return Event{}, fmt.Errorf("application: encode trade event: %w", err)
	}
	return f.event(eventType, AggregateTrade, trade.ID, payload), nil
}

func (f eventFactory) productStatusEvent(product *domain.Product, tradeID string) (Event, error) {
	payload, err := json.Marshal(productStatusEventPayload{
		ProductID: product.ID,
		SellerID:  product.SellerID,
		Status:    string(product.Status),
		TradeID:   tradeID,
	})
	if err != nil {
		return Event{}, fmt.Errorf("application: encode product status event: %w", err)
	}
	return f.event(EventProductStatusChanged, AggregateProduct, product.ID, payload), nil
}

func (f eventFactory) event(eventType, aggregateType, aggregateID string, payload []byte) Event {
	return Event{
		ID:            f.newID(),
		Type:          eventType,
		SchemaVersion: EventSchemaVersion,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		OccurredAt:    f.now,
		TraceID:       f.traceID,
		Payload:       payload,
	}
}
