package application

import (
	"context"
	"errors"
	"log/slog"

	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/observability"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// autoCancelReason 记录在卖家接受交易时被取消的其他待处理交易上。
// 这是系统操作，不是交易一方声明的原因。
const autoCancelReason = "卖家已接受该商品的其他交易"

// TradeService 实现交易状态机。
//
// 每条同时修改商品和交易的命令都在一个数据库事务中运行，并按 Product 再 Trade 的
// 顺序加锁；Outbox 记录和幂等结果也写入同一事务。因此部分执行的动作永远不会可见
// （docs/state-machines.md，事务边界）。
type TradeService struct {
	trades        TradeRepository
	products      ProductRepository
	conversations ConversationVerifier
	ids           IDGenerator
	clock         Clock
	logger        *slog.Logger
}

// NewTradeService 组装交易用例。
func NewTradeService(trades TradeRepository, products ProductRepository, conversations ConversationVerifier, ids IDGenerator, clock Clock, logger *slog.Logger) (*TradeService, error) {
	if trades == nil || products == nil || conversations == nil || ids == nil || clock == nil {
		return nil, errors.New("application: every trade dependency is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &TradeService{
		trades: trades, products: products, conversations: conversations,
		ids: ids, clock: clock, logger: logger,
	}, nil
}

// Create 创建或读取买家针对该商品的终生唯一购买意向。首次创建保持商品 ON_SALE；
// 后续调用返回同一 Trade 的当前表示，不改变状态、价格或会话绑定。
func (s *TradeService) Create(ctx context.Context, cmd CreateTradeCommand) (TradeResult, error) {
	if cmd.ActorID == "" {
		return TradeResult{}, errs.New(errs.CodeUnauthorized, "请先登录")
	}

	key, err := idempotencyKey(cmd.ActorID, OpCreateTrade, cmd.IdempotencyKey)
	if err != nil {
		return TradeResult{}, err
	}

	conversationPresent := cmd.ConversationIDPresent || cmd.ConversationID != nil

	return s.run(ctx, key, func(ctx context.Context, tx TradeTx, events eventFactory) (*CommandResult, error) {
		// This lookup intentionally runs inside Execute's callback. Execute checks
		// the idempotency ledger before invoking the callback, so a same-key retry
		// replays the first success even if the conversation later disappears.
		var conversation *Conversation
		if conversationPresent && cmd.ConversationID != nil {
			verified, err := s.conversations.Get(ctx, cmd.ActorID, *cmd.ConversationID)
			if err != nil {
				return nil, conversationLookupError(err)
			}
			conversation = &verified
		}

		product, err := tx.LockProduct(ctx, cmd.ProductID)
		if err != nil {
			return nil, s.notFound(err, "商品不存在")
		}

		existing, err := tx.TradeByBuyer(ctx, product.ID, cmd.ActorID)
		switch {
		case err == nil:
			if conversationPresent && !sameOptionalString(existing.ConversationID, cmd.ConversationID) {
				return nil, errs.New(errs.CodeConversationMismatch, "会话与既有购买意向绑定不一致")
			}
			return &CommandResult{Code: "TRADE_EXISTS", Trade: existing, Product: product, Created: false}, nil
		case !errors.Is(err, ErrNotFound):
			return nil, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
		}

		if conversation != nil && (conversation.ProductID != product.ID ||
			conversation.BuyerID != cmd.ActorID || conversation.SellerID != product.SellerID) {
			return nil, errs.New(errs.CodeConversationMismatch, "会话与本次购买意向的商品或参与者不匹配")
		}

		trade, err := domain.NewTrade(s.ids.NewTradeID(), product, cmd.ActorID, cmd.ConversationID, events.now)
		if err != nil {
			return nil, createError(err)
		}
		if err := tx.InsertTrade(ctx, trade); err != nil {
			return nil, insertTradeError(err)
		}

		event, err := events.tradeEvent(EventTradeCreated, trade)
		if err != nil {
			return nil, err
		}
		if err := tx.AppendEvent(ctx, event); err != nil {
			return nil, err
		}

		return &CommandResult{Code: "TRADE_CREATED", Trade: trade, Product: product, Created: true}, nil
	})
}

// Accept 是卖家的 PENDING -> ACCEPTED 操作。
//
// 它在一个事务中接受目标交易、预留商品，并取消该商品的其他待处理交易，
// 因此不会有买家在已预留商品上留下待处理交易。
func (s *TradeService) Accept(ctx context.Context, cmd TradeActionCommand) (TradeResult, error) {
	key, err := s.actionKey(cmd, OpAcceptTrade)
	if err != nil {
		return TradeResult{}, err
	}

	return s.run(ctx, key, func(ctx context.Context, tx TradeTx, events eventFactory) (*CommandResult, error) {
		product, trade, err := s.lockPair(ctx, tx, cmd.TradeID)
		if err != nil {
			return nil, err
		}
		if err := hideFromNonParty(trade, cmd.ActorID); err != nil {
			return nil, err
		}
		// The trade's own state is checked first: re-accepting a trade that is
		// already accepted is a trade conflict, and saying so is more useful
		// than reporting that the product it reserved is unavailable.
		if err := trade.Accept(cmd.ActorID, events.now); err != nil {
			return nil, transitionError(err)
		}
		if !product.Tradable() {
			return nil, errs.New(errs.CodeProductNotAvailable, "商品当前不可交易")
		}

		expectedVersion := product.Version
		if err := product.Reserve(events.now); err != nil {
			return nil, transitionError(err)
		}

		if err := tx.UpdateTrade(ctx, trade); err != nil {
			return nil, acceptWriteError(err)
		}
		if err := tx.UpdateProduct(ctx, product, expectedVersion); err != nil {
			return nil, s.writeError(err)
		}

		if err := s.cancelCompetingTrades(ctx, tx, events, trade); err != nil {
			return nil, err
		}

		if err := s.appendAll(ctx, tx, func() ([]Event, error) {
			accepted, err := events.tradeEvent(EventTradeAccepted, trade)
			if err != nil {
				return nil, err
			}
			reserved, err := events.productStatusEvent(product, trade.ID)
			if err != nil {
				return nil, err
			}
			return []Event{accepted, reserved}, nil
		}); err != nil {
			return nil, err
		}

		return &CommandResult{Code: "TRADE_ACCEPTED", Trade: trade, Product: product}, nil
	})
}

// Reject 是卖家的 PENDING -> CANCELLED 操作。商品不受影响。
func (s *TradeService) Reject(ctx context.Context, cmd TradeActionCommand) (TradeResult, error) {
	key, err := s.actionKey(cmd, OpRejectTrade)
	if err != nil {
		return TradeResult{}, err
	}
	if err := domain.ValidateCancelReason(cmd.Reason); err != nil {
		return TradeResult{}, errs.Wrap(errs.CodeValidation, "拒绝原因长度必须为 1 至 200 个字符", err)
	}

	return s.run(ctx, key, func(ctx context.Context, tx TradeTx, events eventFactory) (*CommandResult, error) {
		product, trade, err := s.lockPair(ctx, tx, cmd.TradeID)
		if err != nil {
			return nil, err
		}
		if err := hideFromNonParty(trade, cmd.ActorID); err != nil {
			return nil, err
		}

		if err := trade.Reject(cmd.ActorID, cmd.Reason, events.now); err != nil {
			return nil, transitionError(err)
		}
		if err := tx.UpdateTrade(ctx, trade); err != nil {
			return nil, s.writeError(err)
		}

		event, err := events.tradeEvent(EventTradeCancelled, trade)
		if err != nil {
			return nil, err
		}
		if err := tx.AppendEvent(ctx, event); err != nil {
			return nil, err
		}

		return &CommandResult{Code: "TRADE_REJECTED", Trade: trade, Product: product}, nil
	})
}

// Cancel 取消交易。取消已接受交易时，在同一事务中将商品释放回 ON_SALE。
func (s *TradeService) Cancel(ctx context.Context, cmd TradeActionCommand) (TradeResult, error) {
	key, err := s.actionKey(cmd, OpCancelTrade)
	if err != nil {
		return TradeResult{}, err
	}
	if err := domain.ValidateCancelReason(cmd.Reason); err != nil {
		return TradeResult{}, errs.Wrap(errs.CodeValidation, "取消原因长度必须为 1 至 200 个字符", err)
	}

	return s.run(ctx, key, func(ctx context.Context, tx TradeTx, events eventFactory) (*CommandResult, error) {
		product, trade, err := s.lockPair(ctx, tx, cmd.TradeID)
		if err != nil {
			return nil, err
		}
		if err := hideFromNonParty(trade, cmd.ActorID); err != nil {
			return nil, err
		}

		releaseProduct := trade.Status == domain.TradeAccepted
		if err := trade.Cancel(cmd.ActorID, cmd.Reason, events.now); err != nil {
			return nil, transitionError(err)
		}
		if err := tx.UpdateTrade(ctx, trade); err != nil {
			return nil, s.writeError(err)
		}

		newEvents := func() ([]Event, error) {
			cancelled, err := events.tradeEvent(EventTradeCancelled, trade)
			if err != nil {
				return nil, err
			}
			return []Event{cancelled}, nil
		}

		if releaseProduct {
			expectedVersion := product.Version
			if err := product.Release(events.now); err != nil {
				return nil, transitionError(err)
			}
			if err := tx.UpdateProduct(ctx, product, expectedVersion); err != nil {
				return nil, s.writeError(err)
			}
			newEvents = func() ([]Event, error) {
				cancelled, err := events.tradeEvent(EventTradeCancelled, trade)
				if err != nil {
					return nil, err
				}
				released, err := events.productStatusEvent(product, trade.ID)
				if err != nil {
					return nil, err
				}
				return []Event{cancelled, released}, nil
			}
		}

		if err := s.appendAll(ctx, tx, newEvents); err != nil {
			return nil, err
		}
		return &CommandResult{Code: "TRADE_CANCELLED", Trade: trade, Product: product}, nil
	})
}

// Confirm 记录一方的确认。第二次确认会完成交易，并在同一事务中将商品标记为 SOLD。
func (s *TradeService) Confirm(ctx context.Context, cmd TradeActionCommand) (TradeResult, error) {
	key, err := s.actionKey(cmd, OpConfirmTrade)
	if err != nil {
		return TradeResult{}, err
	}

	return s.run(ctx, key, func(ctx context.Context, tx TradeTx, events eventFactory) (*CommandResult, error) {
		product, trade, err := s.lockPair(ctx, tx, cmd.TradeID)
		if err != nil {
			return nil, err
		}
		if err := hideFromNonParty(trade, cmd.ActorID); err != nil {
			return nil, err
		}
		// Confirm is semantically idempotent even without an Idempotency-Key.
		// A completed trade already records both parties' confirmation, so a
		// repeated call returns its current representation without another write.
		if trade.Status == domain.TradeCompleted {
			return &CommandResult{Code: "TRADE_CONFIRMED", Trade: trade, Product: product}, nil
		}

		expectedVersion := product.Version
		completed, err := trade.Confirm(cmd.ActorID, events.now)
		if err != nil {
			return nil, transitionError(err)
		}
		if err := tx.UpdateTrade(ctx, trade); err != nil {
			return nil, s.writeError(err)
		}

		if !completed {
			return &CommandResult{Code: "TRADE_CONFIRMED", Trade: trade, Product: product}, nil
		}

		if err := product.MarkSold(events.now); err != nil {
			return nil, transitionError(err)
		}
		if err := tx.UpdateProduct(ctx, product, expectedVersion); err != nil {
			return nil, s.writeError(err)
		}

		if err := s.appendAll(ctx, tx, func() ([]Event, error) {
			done, err := events.tradeEvent(EventTradeCompleted, trade)
			if err != nil {
				return nil, err
			}
			sold, err := events.productStatusEvent(product, trade.ID)
			if err != nil {
				return nil, err
			}
			return []Event{done, sold}, nil
		}); err != nil {
			return nil, err
		}

		return &CommandResult{Code: "TRADE_COMPLETED", Trade: trade, Product: product}, nil
	})
}

// Get 返回一笔交易。只有买家和卖家可以读取它。
func (s *TradeService) Get(ctx context.Context, actorID, tradeID string) (*domain.Trade, error) {
	if actorID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "请先登录")
	}

	trade, err := s.trades.ByID(ctx, tradeID)
	if err != nil {
		return nil, s.notFound(err, "交易不存在")
	}
	if !trade.IsParty(actorID) {
		// 对非交易方只告知交易不存在，而不是告知交易存在但无权访问：
		// 他们连该标识本身都不应获知。
		return nil, errs.New(errs.CodeResourceNotFound, "交易不存在")
	}
	return trade, nil
}

// List 返回当前用户作为买家或卖家的交易。
func (s *TradeService) List(ctx context.Context, query ListTradesQuery) (TradePage, error) {
	if query.ActorID == "" {
		return TradePage{}, errs.New(errs.CodeUnauthorized, "请先登录")
	}
	if query.Status != nil && !query.Status.Valid() {
		return TradePage{}, errs.New(errs.CodeValidation, "交易状态不合法")
	}

	page, err := s.trades.List(ctx, TradeFilter{
		ActorID: query.ActorID,
		AsBuyer: query.AsBuyer,
		Status:  query.Status,
	}, query.Page.normalize())
	if err != nil {
		return TradePage{}, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return page, nil
}

// Products 暴露商品仓储，使调用方可以用商品当前状态补全交易响应中的商品投影。
func (s *TradeService) Products() ProductRepository { return s.products }

// --- 共享机制 ----------------------------------------------------------------

// run 执行一条命令；使用相同幂等键的并发请求赢得竞争时重试一次。
//
// 重试会将这场竞争转换为文档规定的结果：失败方事务回滚且没有副作用，
// 第二次尝试找到胜者存储的结果并将其重放。
func (s *TradeService) run(ctx context.Context, key *IdempotencyKey, command func(context.Context, TradeTx, eventFactory) (*CommandResult, error)) (TradeResult, error) {
	for attempt := range 2 {
		result, replayed, err := s.trades.Execute(ctx, key, func(ctx context.Context, tx TradeTx) (*CommandResult, error) {
			return command(ctx, tx, eventFactory{
				newID:   s.ids.NewEventID,
				now:     s.clock.Now(),
				traceID: observability.TraceID(ctx),
			})
		})
		switch {
		case err == nil:
			return TradeResult{
				Trade: result.Trade, Product: result.Product,
				Created: result.Created, Replayed: replayed,
			}, nil
		case errors.Is(err, ErrIdempotencyRace) && attempt == 0:
			continue
		default:
			return TradeResult{}, s.commandError(err)
		}
	}
	return TradeResult{}, errs.New(errs.CodeInternal, "服务暂时不可用")
}

// actionKey 校验操作人并为操作构造幂等键。
func (s *TradeService) actionKey(cmd TradeActionCommand, operation string) (*IdempotencyKey, error) {
	if cmd.ActorID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "请先登录")
	}
	return idempotencyKey(cmd.ActorID, operation, cmd.IdempotencyKey)
}

// lockPair 先锁定商品再锁定交易，并将缺失交易报告为契约错误。
func (s *TradeService) lockPair(ctx context.Context, tx TradeTx, tradeID string) (*domain.Product, *domain.Trade, error) {
	product, trade, err := tx.LockTradeWithProduct(ctx, tradeID)
	if err != nil {
		return nil, nil, s.notFound(err, "交易不存在")
	}
	return product, trade, nil
}

// cancelCompetingTrades 在接受交易的事务中，取消已接受交易所属商品的其他待处理交易。
func (s *TradeService) cancelCompetingTrades(ctx context.Context, tx TradeTx, events eventFactory, accepted *domain.Trade) error {
	competing, err := tx.LockPendingTrades(ctx, accepted.ProductID, accepted.ID)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}

	for _, trade := range competing {
		if err := trade.SystemCancel(autoCancelReason, events.now); err != nil {
			return transitionError(err)
		}
		if err := tx.UpdateTrade(ctx, trade); err != nil {
			return s.writeError(err)
		}
		event, err := events.tradeEvent(EventTradeCancelled, trade)
		if err != nil {
			return err
		}
		if err := tx.AppendEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// appendAll 写入一组 Outbox 记录；其中任何一条无法写入都会使整条命令失败。
func (s *TradeService) appendAll(ctx context.Context, tx TradeTx, build func() ([]Event, error)) error {
	events, err := build()
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := tx.AppendEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// notFound 将仓储未找到结果映射为 RESOURCE_NOT_FOUND。
func (s *TradeService) notFound(err error, message string) error {
	if errors.Is(err, ErrNotFound) {
		return errs.Wrap(errs.CodeResourceNotFound, message, err)
	}
	return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
}

// writeError 映射状态转换期间的写入失败。
func (s *TradeService) writeError(err error) error {
	switch {
	case errors.Is(err, ErrVersionConflict):
		return errs.Wrap(errs.CodeTradeStateConflict, "商品或交易状态已变化，请重试", err)
	case errors.Is(err, ErrNotFound):
		return errs.Wrap(errs.CodeResourceNotFound, "资源不存在", err)
	default:
		return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
}

// commandError 映射从事务中逸出的失败。
func (s *TradeService) commandError(err error) error {
	if errs.CodeOf(err) != errs.CodeInternal {
		return err
	}
	if errors.Is(err, ErrIdempotencyRace) {
		return errs.Wrap(errs.CodeTradeStateConflict, "请求正在处理中，请稍后重试", err)
	}
	return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
}

// createError 将被拒绝的交易创建映射为对应的契约错误码。
func createError(err error) error {
	switch {
	case errors.Is(err, domain.ErrSelfTrade):
		return errs.Wrap(errs.CodeSelfActionNotAllowed, "不能购买自己发布的商品", err)
	case errors.Is(err, domain.ErrNotOnSale):
		return errs.Wrap(errs.CodeProductNotAvailable, "商品当前不可交易", err)
	default:
		return errs.Wrap(errs.CodeValidation, err.Error(), err)
	}
}

// insertTradeError 映射数据库写入失败。创建路径持有 Product 锁并在插入前读取终生唯一
// 意向，因此正常并发不会走到唯一冲突；命中该分支表示存储不变量被绕过。
func insertTradeError(err error) error {
	if errors.Is(err, ErrTradeIntentExists) {
		return errs.Wrap(errs.CodeInternal, "购买意向唯一性发生异常", err)
	}
	if errors.Is(err, ErrNotFound) {
		return errs.Wrap(errs.CodeResourceNotFound, "商品不存在", err)
	}
	return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
}

func hideFromNonParty(trade *domain.Trade, actorID string) error {
	if trade.IsParty(actorID) {
		return nil
	}
	return errs.New(errs.CodeResourceNotFound, "交易不存在")
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func conversationLookupError(err error) error {
	if errs.CodeOf(err) == errs.CodeResourceNotFound {
		return errs.New(errs.CodeResourceNotFound, "会话不存在")
	}
	return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
}

// acceptWriteError 映射“每个商品只能有一笔已接受交易”的唯一索引冲突：
// 另一项卖家操作赢得了竞争。
func acceptWriteError(err error) error {
	if errors.Is(err, ErrTradeAlreadyAccepted) {
		return errs.Wrap(errs.CodeTradeStateConflict, "该商品已有其他已接受的交易", err)
	}
	if errors.Is(err, ErrVersionConflict) {
		return errs.Wrap(errs.CodeTradeStateConflict, "商品或交易状态已变化，请重试", err)
	}
	if errors.Is(err, ErrNotFound) {
		return errs.Wrap(errs.CodeResourceNotFound, "交易不存在", err)
	}
	return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
}
