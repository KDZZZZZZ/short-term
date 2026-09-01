package domain

import (
	"errors"
	"time"
	"unicode/utf8"
)

// TradeStatus 是交易生命周期状态。
type TradeStatus string

// docs/state-machines.md 中定义的交易状态。COMPLETED 和 CANCELLED 是终态。
const (
	TradePending   TradeStatus = "PENDING"
	TradeAccepted  TradeStatus = "ACCEPTED"
	TradeCompleted TradeStatus = "COMPLETED"
	TradeCancelled TradeStatus = "CANCELLED"
)

// Valid 判断 s 是否为已知交易状态。
func (s TradeStatus) Valid() bool {
	switch s {
	case TradePending, TradeAccepted, TradeCompleted, TradeCancelled:
		return true
	default:
		return false
	}
}

// Terminal 判断是否无法再进行任何状态转换。
func (s TradeStatus) Terminal() bool { return s == TradeCompleted || s == TradeCancelled }

// MaxCancelReasonLength 是公开 ReasonRequest 的长度限制。
const MaxCancelReasonLength = 200

// 交易错误。它们区分“谁可以操作”和“当前状态允许什么”，
// 因为两者映射到不同的公开错误码。
var (
	ErrTradeIDRequired    = errors.New("trade id is required")
	ErrSelfTrade          = errors.New("a user cannot trade with themselves")
	ErrNotTradeParty      = errors.New("only the buyer or the seller may perform this action")
	ErrNotTradeBuyer      = errors.New("only the buyer may perform this action")
	ErrNotTradeSeller     = errors.New("only the seller may perform this action")
	ErrTradeNotPending    = errors.New("the trade is not pending")
	ErrTradeNotAccepted   = errors.New("the trade is not accepted")
	ErrCancelReasonLength = errors.New("reason must be 1-200 characters")
)

// Trade 是购买意向及其向线下交付推进的过程。
type Trade struct {
	ID string
	// ProductID 标识商品；交易永远不复制商品的可变字段。
	ProductID string
	BuyerID   string
	SellerID  string
	// ConversationID 可选地关联发起交易的聊天会话。
	ConversationID *string
	// PriceSnapshotMinor 是创建交易时的商品价格。
	// 创建后永不改变，因此之后修改价格不会改变已达成的交易。
	PriceSnapshotMinor int64
	Status             TradeStatus
	BuyerConfirmedAt   *time.Time
	SellerConfirmedAt  *time.Time
	CancelReason       *string
	CreatedAt          time.Time
	AcceptedAt         *time.Time
	CompletedAt        *time.Time
	CancelledAt        *time.Time
	UpdatedAt          time.Time
}

// NewTrade 针对商品创建待处理的购买意向。
//
// 商品必须可交易：docs/state-machines.md 禁止为 RESERVED、SOLD 或 OFF_SHELF 商品
// 创建交易。
func NewTrade(id string, product *Product, buyerID string, conversationID *string, now time.Time) (*Trade, error) {
	if id == "" {
		return nil, ErrTradeIDRequired
	}
	if buyerID == "" {
		return nil, ErrSellerRequired
	}
	if product.IsSeller(buyerID) {
		return nil, ErrSelfTrade
	}
	if !product.Tradable() {
		return nil, ErrNotOnSale
	}

	return &Trade{
		ID:                 id,
		ProductID:          product.ID,
		BuyerID:            buyerID,
		SellerID:           product.SellerID,
		ConversationID:     conversationID,
		PriceSnapshotMinor: product.PriceMinor,
		Status:             TradePending,
		CreatedAt:          now,
		UpdatedAt:          now,
	}, nil
}

// IsBuyer 判断 actorID 是否为买家。
func (t *Trade) IsBuyer(actorID string) bool { return actorID != "" && actorID == t.BuyerID }

// IsSeller 判断 actorID 是否为卖家。
func (t *Trade) IsSeller(actorID string) bool { return actorID != "" && actorID == t.SellerID }

// IsParty 判断 actorID 是否为交易双方之一。其他任何人都不能读取或修改交易。
func (t *Trade) IsParty(actorID string) bool { return t.IsBuyer(actorID) || t.IsSeller(actorID) }

// Accept 为卖家执行 PENDING -> ACCEPTED。调用方负责在同一事务中预留商品，
// 并取消该商品的其他待处理交易。
func (t *Trade) Accept(actorID string, now time.Time) error {
	if !t.IsSeller(actorID) {
		return ErrNotTradeSeller
	}
	if t.Status != TradePending {
		return ErrTradeNotPending
	}
	t.Status = TradeAccepted
	t.AcceptedAt = &now
	t.UpdatedAt = now
	return nil
}

// Reject 为卖家执行 PENDING -> CANCELLED。商品不受影响。
func (t *Trade) Reject(actorID, reason string, now time.Time) error {
	if !t.IsSeller(actorID) {
		return ErrNotTradeSeller
	}
	if t.Status != TradePending {
		return ErrTradeNotPending
	}
	return t.cancel(reason, now)
}

// Cancel 将交易转换为 CANCELLED。
//
// 处于 PENDING 时只有买家可以取消；卖家的对应操作是 Reject。
// 处于 ACCEPTED 时双方都可以取消，调用方必须在同一事务中释放商品。
func (t *Trade) Cancel(actorID, reason string, now time.Time) error {
	switch t.Status {
	case TradePending:
		if !t.IsBuyer(actorID) {
			if t.IsSeller(actorID) {
				// 卖家退出待处理交易的方式是 Reject，这是一个具有独立审计含义的
				// 不同公开操作。
				return ErrNotTradeBuyer
			}
			return ErrNotTradeParty
		}
	case TradeAccepted:
		if !t.IsParty(actorID) {
			return ErrNotTradeParty
		}
	default:
		if !t.IsParty(actorID) {
			return ErrNotTradeParty
		}
		return ErrTradeNotPending
	}
	return t.cancel(reason, now)
}

// SystemCancel 在没有操作人的情况下取消待处理交易。
// 接受一笔交易取消商品的其他待处理交易时使用。
func (t *Trade) SystemCancel(reason string, now time.Time) error {
	if t.Status != TradePending {
		return ErrTradeNotPending
	}
	return t.cancel(reason, now)
}

// Confirm 记录一方对线下交付的确认，并返回交易是否已完成。
//
// 第一次确认只保存时间戳，交易仍保持 ACCEPTED。第二次确认会完成交易，
// 调用方必须在同一事务中将商品标记为 SOLD。重复确认具备幂等性：
// 既不会失败，也不会推进交易。
func (t *Trade) Confirm(actorID string, now time.Time) (completed bool, err error) {
	if !t.IsParty(actorID) {
		return false, ErrNotTradeParty
	}
	if t.Status != TradeAccepted {
		return false, ErrTradeNotAccepted
	}

	switch {
	case t.IsBuyer(actorID):
		if t.BuyerConfirmedAt == nil {
			t.BuyerConfirmedAt = &now
			t.UpdatedAt = now
		}
	default:
		if t.SellerConfirmedAt == nil {
			t.SellerConfirmedAt = &now
			t.UpdatedAt = now
		}
	}

	if t.BuyerConfirmedAt == nil || t.SellerConfirmedAt == nil {
		return false, nil
	}

	t.Status = TradeCompleted
	t.CompletedAt = &now
	t.UpdatedAt = now
	return true, nil
}

// BuyerConfirmed 判断买家是否已确认。
func (t *Trade) BuyerConfirmed() bool { return t.BuyerConfirmedAt != nil }

// SellerConfirmed 判断卖家是否已确认。
func (t *Trade) SellerConfirmed() bool { return t.SellerConfirmedAt != nil }

// cancel 在调用方检查操作权限后应用通用的取消效果。
func (t *Trade) cancel(reason string, now time.Time) error {
	if err := ValidateCancelReason(reason); err != nil {
		return err
	}
	stored := reason
	t.Status = TradeCancelled
	t.CancelReason = &stored
	t.CancelledAt = &now
	t.UpdatedAt = now
	return nil
}

// ValidateCancelReason 强制执行公开 ReasonRequest 限制。
func ValidateCancelReason(reason string) error {
	length := utf8.RuneCountInString(reason)
	if length < 1 || length > MaxCancelReasonLength {
		return ErrCancelReasonLength
	}
	return nil
}
