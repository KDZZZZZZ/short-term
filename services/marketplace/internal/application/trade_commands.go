package application

import (
	"unicode/utf8"

	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// CreateTradeCommand 记录购买意向。
type CreateTradeCommand struct {
	ActorID   string
	ProductID string
	// ConversationIDPresent distinguishes an omitted property from an explicit
	// null. Existing intents only compare the binding when this is true.
	ConversationIDPresent bool
	ConversationID        *string
	IdempotencyKey        *string
}

// TradeActionCommand 是对现有交易执行的状态转换。
type TradeActionCommand struct {
	ActorID string
	TradeID string
	// Reason 由拒绝和取消操作要求，其他操作会忽略它。
	Reason         string
	IdempotencyKey *string
}

// ListTradesQuery 列出当前用户的交易。
type ListTradesQuery struct {
	ActorID string
	AsBuyer bool
	Status  *domain.TradeStatus
	Page    Page
}

// TradeResult 是交易命令返回给传输层的结果。
type TradeResult struct {
	Trade *domain.Trade
	// Product is the product representation from the same committed command
	// result. Commands use it instead of re-reading mutable current state.
	Product *domain.Product
	// Created distinguishes the first intent creation (HTTP 201) from a
	// create-or-get hit on the lifetime-unique intent (HTTP 200).
	Created bool
	// Replayed 表示这是之前相同命令的存储结果，而不是新的状态转换。
	Replayed bool
}

// idempotencyKey 为命令构造账本键，并根据公开契约限制校验客户端提供的值。
func idempotencyKey(actorID, operation string, key *string) (*IdempotencyKey, error) {
	if key == nil {
		return nil, nil
	}
	length := utf8.RuneCountInString(*key)
	if length < MinIdempotencyKeyLength || length > MaxIdempotencyKeyLength {
		return nil, errs.Newf(errs.CodeValidation, "Idempotency-Key 长度必须为 %d 至 %d 个字符",
			MinIdempotencyKeyLength, MaxIdempotencyKeyLength)
	}
	return &IdempotencyKey{ActorID: actorID, Operation: operation, Key: *key}, nil
}
