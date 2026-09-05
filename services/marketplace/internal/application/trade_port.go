package application

import (
	"context"
	"errors"
	"time"

	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// 用作幂等键操作部分的交易命令名称。它们是稳定标识；
// 修改其中任何一个都会使已存储结果无法访问。
const (
	OpCreateTrade  = "trade.create"
	OpAcceptTrade  = "trade.accept"
	OpRejectTrade  = "trade.reject"
	OpCancelTrade  = "trade.cancel"
	OpConfirmTrade = "trade.confirm"
)

// SnapshotSchemaVersion 为存储的命令结果版本化。递增它会使旧快照不可读，
// 因此快照结构发生任何变化时都必须递增版本，并在记录存续期间保留旧版本读取器。
const SnapshotSchemaVersion int32 = 2

// 幂等键限制，与 openapi/components/parameters.yaml#/IdempotencyKey 一致。
const (
	MinIdempotencyKeyLength = 16
	MaxIdempotencyKeyLength = 128
)

// 交易存储结果。
var (
	// ErrIdempotencyRace 表示使用相同键的并发请求先提交。
	// 调用方重试并重放已存储结果。
	ErrIdempotencyRace = errors.New("a concurrent request with the same idempotency key committed first")
	// ErrTradeAlreadyAccepted 表示商品已经有一笔已接受交易，并被唯一索引拒绝。
	ErrTradeAlreadyAccepted = errors.New("the product already has an accepted trade")
	// ErrTradeIntentExists 表示同一商品和买家的终生唯一意向已经存在。
	ErrTradeIntentExists = errors.New("the buyer already has an intent on this product")
)

// IdempotencyKey 标识一次命令尝试。
//
// docs/state-machines.md 除客户端值外，还按用户和操作限定键的作用域，
// 因此一个客户端的键不能重放另一个客户端的命令，在不同端点复用的键也不会冲突。
type IdempotencyKey struct {
	ActorID   string
	Operation string
	Key       string
}

// CommandResult 是已提交命令的存储结果。
type CommandResult struct {
	// Code 说明命令执行了什么，供诊断和审计使用。
	Code string
	// Trade 是命令提交时交易的状态。
	Trade *domain.Trade
	// Product is the product projection committed with the command. Keeping it
	// in the result snapshot lets an idempotent replay rebuild the first HTTP
	// body even if the product changes later.
	Product *domain.Product
	// Created 只对 create-or-get 命令有意义，并决定公开响应是 201 还是 200。
	Created bool
}

// Event 是一条 Outbox 记录，与产生它的领域变更写入同一事务。
type Event struct {
	ID            string
	Type          string
	SchemaVersion int32
	AggregateType string
	AggregateID   string
	OccurredAt    time.Time
	TraceID       string
	Payload       []byte
}

// Marketplace Service 发布的事件类型。
const (
	EventProductStatusChanged = "marketplace.product.status_changed"
	EventTradeCreated         = "marketplace.trade.created"
	EventTradeAccepted        = "marketplace.trade.accepted"
	EventTradeCancelled       = "marketplace.trade.cancelled"
	EventTradeCompleted       = "marketplace.trade.completed"
)

// TradeTx 是交易命令执行所使用的事务接口。
//
// 加锁顺序始终是 Product 再 Trade。所有地方都采用相同顺序获取两把锁，
// 才能避免两个命令操作同一对资源时死锁（docs/state-machines.md，事务边界）。
type TradeTx interface {
	// LockProduct 读取商品，并持有行锁直到事务结束。
	LockProduct(ctx context.Context, productID string) (*domain.Product, error)
	// LockTradeWithProduct 按先锁交易所属商品、再锁交易的顺序加锁，并返回二者。
	//
	// 顺序统一定义在这里，而不是每个命令中，这样任何命令都不会写错：
	// 每条同时接触商品和交易的路径都以相同顺序获取两把锁。
	LockTradeWithProduct(ctx context.Context, tradeID string) (*domain.Product, *domain.Trade, error)
	// LockPendingTrades 读取并锁定商品的所有待处理交易，但排除 exceptID 指定的交易。
	LockPendingTrades(ctx context.Context, productID, exceptID string) ([]*domain.Trade, error)
	// TradeByBuyer 在 Product 锁持有期间读取该买家的进行中购买意向
	//（PENDING/ACCEPTED）；已取消的历史意向不算。
	TradeByBuyer(ctx context.Context, productID, buyerID string) (*domain.Trade, error)
	// InsertTrade 写入新交易。
	InsertTrade(ctx context.Context, trade *domain.Trade) error
	// UpdateTrade 回写交易。
	UpdateTrade(ctx context.Context, trade *domain.Trade) error
	// UpdateProduct 回写商品，并在行锁之上使用版本实现乐观并发控制。
	UpdateProduct(ctx context.Context, product *domain.Product, expectedVersion int64) error
	// AppendEvent 写入一条 Outbox 记录。
	AppendEvent(ctx context.Context, event Event) error
}

// TradeFilter 为列表查询筛选交易。
type TradeFilter struct {
	// ActorID 是要列出其交易的用户。
	ActorID string
	// AsBuyer 为 true 时列出当前用户作为买家的交易，否则列出其作为卖家的交易。
	AsBuyer bool
	// Status 限定状态。nil 表示所有状态。
	Status *domain.TradeStatus
}

// TradePage 是一页交易及总行数。
type TradePage struct {
	Items []*domain.Trade
	Page  int32
	Size  int32
	Total int64
}

// TradeRepository 存储交易、幂等账本和 Outbox。
type TradeRepository interface {
	// Execute 执行一条交易命令。
	//
	// 当 key 非 nil 且已存在已提交结果时，直接返回存储结果，不运行 fn，
	// 也不检查当前交易状态；这样丢失响应后的重试会返回第一次结果，而不是冲突。
	//
	// 否则 fn 在一个事务中运行。fn 返回的结果、它执行的领域写入以及追加的 Outbox
	// 记录要么一起提交，要么全部不提交。
	Execute(ctx context.Context, key *IdempotencyKey, fn func(ctx context.Context, tx TradeTx) (*CommandResult, error)) (result *CommandResult, replayed bool, err error)

	// ByID 在事务外读取一笔交易。
	ByID(ctx context.Context, tradeID string) (*domain.Trade, error)
	// List 按确定性顺序返回一页交易。
	List(ctx context.Context, filter TradeFilter, page Page) (TradePage, error)
	// CountCompletedBySeller 统计用户作为卖家已完成的交易数（终态 COMPLETED）。
	CountCompletedBySeller(ctx context.Context, sellerID string) (int64, error)
}

// Conversation 是 Messaging Service 拥有的、交易绑定校验所需的最小事实投影。
type Conversation struct {
	ID        string
	ProductID string
	BuyerID   string
	SellerID  string
}

// ConversationVerifier 读取调用方可见的会话。不存在或不可见必须返回
// RESOURCE_NOT_FOUND；Marketplace 再校验商品和双方是否完全一致。
type ConversationVerifier interface {
	Get(ctx context.Context, actorID, conversationID string) (Conversation, error)
}

// OutboxRepository 是发布器看到的 Outbox 接口。
type OutboxRepository interface {
	// Pending 返回最多 limit 条尚未发布的事件，按最早优先。
	Pending(ctx context.Context, limit int32) ([]Event, error)
	// MarkPublished 记录事件已交付。
	MarkPublished(ctx context.Context, eventID string, at time.Time) error
	// MarkFailed 记录失败的交付尝试，使事件可以重试且失败可见。
	MarkFailed(ctx context.Context, eventID string, cause string) error
}

// EventPublisher 将 Outbox 事件交付到事件总线。
type EventPublisher interface {
	Publish(ctx context.Context, event Event) error
}
