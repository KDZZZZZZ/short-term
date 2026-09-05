package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/platform/pg"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// 交易规则依赖的唯一索引。它们的名称属于行为契约：代码会将每种冲突转换为特定的领域结果。
const (
	acceptedTradeIndex = "trades_one_accepted_per_product_idx"
	activeIntentIndex  = "trades_one_active_intent_per_buyer_idx"
	idempotencyPK      = "idempotency_records_pkey"
)

// tradeColumns 是所有交易读取共享的字段投影。
const tradeColumns = `id, product_id, buyer_id, seller_id, conversation_id, price_snapshot_minor,
	status, buyer_confirmed_at, seller_confirmed_at, cancel_reason,
	created_at, accepted_at, completed_at, cancelled_at, updated_at`

// idempotencyLockNamespace 将此建议锁空间与同一数据库中其他建议锁的使用隔离开。
const idempotencyLockNamespace int32 = 0x5354

// TradeRepository 存储交易、幂等账本和 Outbox。
type TradeRepository struct {
	pool *pgxpool.Pool
}

// NewTradeRepository 基于已打开的连接池构造仓储。
func NewTradeRepository(pool *pgxpool.Pool) *TradeRepository {
	return &TradeRepository{pool: pool}
}

var _ application.TradeRepository = (*TradeRepository)(nil)

// Execute 执行一条交易命令。
//
// 使用幂等键时，事务首先针对该键获取事务级建议锁，因此两个使用相同幂等键的并发请求
// 会被串行化，而不会重复执行。后来的请求会找到已提交的记录并重放结果。账本上的主键
// 是最后一道保障：即使两个事务因某种原因都走到写入处，也只有一个能够提交，另一个会
// 完整回滚且不留下任何副作用。
//
// 存储结果与领域变更写入同一事务，因此已提交的变更不可能缺少结果，回滚的尝试也不可能
// 留下结果。
func (r *TradeRepository) Execute(
	ctx context.Context,
	key *application.IdempotencyKey,
	fn func(ctx context.Context, tx application.TradeTx) (*application.CommandResult, error),
) (*application.CommandResult, bool, error) {
	var (
		result   *application.CommandResult
		replayed bool
	)

	err := pg.InTx(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if key != nil {
			if err := lockIdempotencyKey(ctx, tx, *key); err != nil {
				return err
			}

			stored, found, err := readIdempotencyRecord(ctx, tx, *key)
			if err != nil {
				return err
			}
			if found {
				// 重放发生在任何状态检查之前，因此响应丢失后的重试会返回第一次的结果，
				// 即使交易此后已经推进（docs/state-machines.md，幂等命令）。
				result, replayed = stored, true
				return nil
			}
		}

		commandResult, err := fn(ctx, &tradeTx{tx: tx})
		if err != nil {
			return err
		}

		if key != nil {
			if err := writeIdempotencyRecord(ctx, tx, *key, commandResult); err != nil {
				return err
			}
		}
		result = commandResult
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return result, replayed, nil
}

// ByID 在事务外读取一笔交易。
func (r *TradeRepository) ByID(ctx context.Context, tradeID string) (*domain.Trade, error) {
	const query = `SELECT ` + tradeColumns + ` FROM trades WHERE id = $1`

	trade, err := scanTrade(r.pool.QueryRow(ctx, query, tradeID))
	if err != nil {
		return nil, fmt.Errorf("postgres: select trade: %w", err)
	}
	return trade, nil
}

// CountCompletedBySeller 统计用户作为卖家已完成的交易数。
func (r *TradeRepository) CountCompletedBySeller(ctx context.Context, sellerID string) (int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM trades WHERE seller_id = $1 AND status = 'COMPLETED'`, sellerID,
	).Scan(&total); err != nil {
		return 0, fmt.Errorf("postgres: count completed trades: %w", err)
	}
	return total, nil
}

// List 返回操作人交易的一页，按最新优先，并以标识作为确定性的平局裁决。
func (r *TradeRepository) List(ctx context.Context, filter application.TradeFilter, page application.Page) (application.TradePage, error) {
	party := "seller_id"
	if filter.AsBuyer {
		party = "buyer_id"
	}

	args := []any{filter.ActorID}
	where := " WHERE " + party + " = $1"
	if filter.Status != nil {
		args = append(args, string(*filter.Status))
		where += fmt.Sprintf(" AND status = $%d", len(args))
	}

	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM trades`+where, args...).Scan(&total); err != nil {
		return application.TradePage{}, fmt.Errorf("postgres: count trades: %w", err)
	}

	listQuery := fmt.Sprintf(
		`SELECT %s FROM trades%s ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d`,
		tradeColumns, where, len(args)+1, len(args)+2,
	)
	args = append(args, page.Size, page.Offset())

	rows, err := r.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return application.TradePage{}, fmt.Errorf("postgres: select trades: %w", err)
	}
	defer rows.Close()

	var trades []*domain.Trade
	for rows.Next() {
		trade, err := scanTrade(rows)
		if err != nil {
			return application.TradePage{}, fmt.Errorf("postgres: scan trade: %w", err)
		}
		trades = append(trades, trade)
	}
	if err := rows.Err(); err != nil {
		return application.TradePage{}, fmt.Errorf("postgres: read trades: %w", err)
	}

	return application.TradePage{Items: trades, Page: page.Number, Size: page.Size, Total: total}, nil
}

// --- 事务接口 ----------------------------------------------------------------

// tradeTx 基于一个已打开的事务实现 application.TradeTx。
type tradeTx struct {
	tx pgx.Tx
}

// LockProduct 读取商品，并持有其行锁直到事务提交。
func (t *tradeTx) LockProduct(ctx context.Context, productID string) (*domain.Product, error) {
	const query = `SELECT ` + productColumns + ` FROM products WHERE id = $1 FOR UPDATE`

	product, err := scanProduct(t.tx.QueryRow(ctx, query, productID))
	if err != nil {
		return nil, fmt.Errorf("postgres: lock product: %w", err)
	}
	// Product mutations take the same row lock before changing image rows, so
	// once this lock is held the image projection is the committed version that
	// belongs in an idempotent command-result snapshot.
	images, err := lockedImages(ctx, t.tx, productID)
	if err != nil {
		return nil, err
	}
	product.Images = images
	return product, nil
}

// LockTradeWithProduct 先获取商品锁，再获取交易锁。
//
// 首先无锁读取商品标识是安全的，因为交易关联的商品不会改变；随后按既定顺序锁定
// 命令实际要写入的行。
func (t *tradeTx) LockTradeWithProduct(ctx context.Context, tradeID string) (*domain.Product, *domain.Trade, error) {
	var productID string
	err := t.tx.QueryRow(ctx, `SELECT product_id FROM trades WHERE id = $1`, tradeID).Scan(&productID)
	if pg.IsNoRows(err) {
		return nil, nil, application.ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("postgres: read trade product: %w", err)
	}

	product, err := t.LockProduct(ctx, productID)
	if err != nil {
		return nil, nil, err
	}

	const query = `SELECT ` + tradeColumns + ` FROM trades WHERE id = $1 FOR UPDATE`
	trade, err := scanTrade(t.tx.QueryRow(ctx, query, tradeID))
	if err != nil {
		return nil, nil, fmt.Errorf("postgres: lock trade: %w", err)
	}
	return product, trade, nil
}

// LockPendingTrades 锁定商品的其他待处理交易，使接受交易的事务能够取消它们。
func (t *tradeTx) LockPendingTrades(ctx context.Context, productID, exceptID string) ([]*domain.Trade, error) {
	const query = `
		SELECT ` + tradeColumns + `
		  FROM trades
		 WHERE product_id = $1 AND id <> $2 AND status = 'PENDING'
		 ORDER BY id
		   FOR UPDATE`

	rows, err := t.tx.Query(ctx, query, productID, exceptID)
	if err != nil {
		return nil, fmt.Errorf("postgres: lock pending trades: %w", err)
	}
	defer rows.Close()

	var trades []*domain.Trade
	for rows.Next() {
		trade, err := scanTrade(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan pending trade: %w", err)
		}
		trades = append(trades, trade)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: read pending trades: %w", err)
	}
	return trades, nil
}

// TradeByBuyer 读取同一商品和买家进行中（PENDING/ACCEPTED）的购买意向；
// 已取消的历史意向不算。调用方已经持有 Product 锁，因此不存在进行中记录时，
// 另一个创建请求不能在当前事务提交前插入同商品意向（进行中唯一索引兜底）。
func (t *tradeTx) TradeByBuyer(ctx context.Context, productID, buyerID string) (*domain.Trade, error) {
	const query = `SELECT ` + tradeColumns + `
		FROM trades
		WHERE product_id = $1 AND buyer_id = $2 AND status IN ('PENDING', 'ACCEPTED')
		FOR UPDATE`

	trade, err := scanTrade(t.tx.QueryRow(ctx, query, productID, buyerID))
	if err != nil {
		return nil, fmt.Errorf("postgres: read buyer intent: %w", err)
	}
	return trade, nil
}

// InsertTrade 写入一笔新交易。
func (t *tradeTx) InsertTrade(ctx context.Context, trade *domain.Trade) error {
	const query = `
		INSERT INTO trades (` + tradeColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`

	_, err := t.tx.Exec(ctx, query,
		trade.ID, trade.ProductID, trade.BuyerID, trade.SellerID, trade.ConversationID,
		trade.PriceSnapshotMinor, string(trade.Status), trade.BuyerConfirmedAt, trade.SellerConfirmedAt,
		trade.CancelReason, trade.CreatedAt, trade.AcceptedAt, trade.CompletedAt, trade.CancelledAt, trade.UpdatedAt,
	)
	switch {
	case pg.IsUniqueViolation(err, activeIntentIndex):
		return application.ErrTradeIntentExists
	case pg.IsUniqueViolation(err, acceptedTradeIndex):
		return application.ErrTradeAlreadyAccepted
	case isForeignKeyViolation(err):
		return application.ErrNotFound
	case err != nil:
		return fmt.Errorf("postgres: insert trade: %w", err)
	}
	return nil
}

// UpdateTrade 回写一笔交易。
func (t *tradeTx) UpdateTrade(ctx context.Context, trade *domain.Trade) error {
	const query = `
		UPDATE trades
		   SET conversation_id = $2, status = $3, buyer_confirmed_at = $4, seller_confirmed_at = $5,
		       cancel_reason = $6, accepted_at = $7, completed_at = $8, cancelled_at = $9, updated_at = $10
		 WHERE id = $1`

	tag, err := t.tx.Exec(ctx, query,
		trade.ID, trade.ConversationID, string(trade.Status), trade.BuyerConfirmedAt, trade.SellerConfirmedAt,
		trade.CancelReason, trade.AcceptedAt, trade.CompletedAt, trade.CancelledAt, trade.UpdatedAt,
	)
	switch {
	case pg.IsUniqueViolation(err, acceptedTradeIndex):
		return application.ErrTradeAlreadyAccepted
	case pg.IsUniqueViolation(err, activeIntentIndex):
		return application.ErrTradeIntentExists
	case err != nil:
		return fmt.Errorf("postgres: update trade: %w", err)
	case tag.RowsAffected() == 0:
		return application.ErrNotFound
	}
	return nil
}

// UpdateProduct 回写商品，并检查读取时的版本。
func (t *tradeTx) UpdateProduct(ctx context.Context, product *domain.Product, expectedVersion int64) error {
	const query = `
		UPDATE products
		   SET status = $3, version = $4, updated_at = $5
		 WHERE id = $1 AND version = $2`

	tag, err := t.tx.Exec(ctx, query, product.ID, expectedVersion, string(product.Status), product.Version, product.UpdatedAt)
	if err != nil {
		return fmt.Errorf("postgres: update product status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrVersionConflict
	}
	return nil
}

// AppendEvent 在命令事务中写入一条 Outbox 记录。
func (t *tradeTx) AppendEvent(ctx context.Context, event application.Event) error {
	const query = `
		INSERT INTO outbox_events (event_id, event_type, schema_version, aggregate_type, aggregate_id, occurred_at, trace_id, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := t.tx.Exec(ctx, query,
		event.ID, event.Type, event.SchemaVersion, event.AggregateType,
		event.AggregateID, event.OccurredAt, event.TraceID, event.Payload,
	)
	if err != nil {
		return fmt.Errorf("postgres: append outbox event: %w", err)
	}
	return nil
}

// --- 幂等性 ----------------------------------------------------------------

// storedResult 是持久化快照的结构。它是规范化的命令结果，而不是 HTTP 正文：
// Gateway 根据它重建第一次响应。
type storedResult struct {
	Code    string          `json:"code"`
	Trade   *storedTrade    `json:"trade"`
	Product *storedProduct  `json:"product"`
	Created bool            `json:"created,omitempty"`
	Extra   json.RawMessage `json:"extra,omitempty"`
}

type storedProduct struct {
	ID          string        `json:"id"`
	SellerID    string        `json:"seller_id"`
	Title       string        `json:"title"`
	PriceMinor  int64         `json:"price_minor"`
	Category    string        `json:"category"`
	Description string        `json:"description"`
	Status      string        `json:"status"`
	Version     int64         `json:"version"`
	Images      []storedImage `json:"images"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

type storedImage struct {
	ID        string    `json:"id"`
	ProductID string    `json:"product_id"`
	ObjectKey string    `json:"object_key"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

// storedTrade 镜像传输层重建响应所需的交易字段。其版本由
// idempotency_records.schema_version 标识。
type storedTrade struct {
	ID                 string     `json:"id"`
	ProductID          string     `json:"product_id"`
	BuyerID            string     `json:"buyer_id"`
	SellerID           string     `json:"seller_id"`
	ConversationID     *string    `json:"conversation_id"`
	PriceSnapshotMinor int64      `json:"price_snapshot_minor"`
	Status             string     `json:"status"`
	BuyerConfirmedAt   *time.Time `json:"buyer_confirmed_at"`
	SellerConfirmedAt  *time.Time `json:"seller_confirmed_at"`
	CancelReason       *string    `json:"cancel_reason"`
	CreatedAt          time.Time  `json:"created_at"`
	AcceptedAt         *time.Time `json:"accepted_at"`
	CompletedAt        *time.Time `json:"completed_at"`
	CancelledAt        *time.Time `json:"cancelled_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// lockIdempotencyKey 串行化共享同一幂等键的并发请求。该锁属于事务作用域，
// 会在提交或回滚时释放，不会泄漏。
func lockIdempotencyKey(ctx context.Context, tx pgx.Tx, key application.IdempotencyKey) error {
	const query = `SELECT pg_advisory_xact_lock($1, $2)`

	if _, err := tx.Exec(ctx, query, idempotencyLockNamespace, advisoryLockID(key)); err != nil {
		return fmt.Errorf("postgres: lock idempotency key: %w", err)
	}
	return nil
}

// advisoryLockID derives the lock identifier from the key.
//
// The hash is computed here rather than by PostgreSQL's hashtext because the
// client-supplied key is arbitrary text: passing it to the database as a lock
// argument would mean any byte a client can send has to be a legal SQL text
// value, and a NUL byte is not. Length prefixes keep the three parts from
// running together, so ("ab","c") and ("a","bc") hash differently.
//
// A hash collision only makes two unrelated commands take turns; correctness
// still rests on the ledger primary key, which cannot collide.
func advisoryLockID(key application.IdempotencyKey) int32 {
	digest := sha256.New()
	for _, part := range []string{key.ActorID, key.Operation, key.Key} {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(part)))
		digest.Write(length[:])
		digest.Write([]byte(part))
	}
	sum := digest.Sum(nil)
	return int32(binary.BigEndian.Uint32(sum[:4]))
}

// readIdempotencyRecord 返回某个幂等键已提交的存储结果（如果存在）。
func readIdempotencyRecord(ctx context.Context, tx pgx.Tx, key application.IdempotencyKey) (*application.CommandResult, bool, error) {
	const query = `
		SELECT result_code, schema_version, result
		  FROM idempotency_records
		 WHERE actor_id = $1 AND operation = $2 AND idempotency_key = $3`

	var (
		code    string
		version int32
		payload []byte
	)
	err := tx.QueryRow(ctx, query, key.ActorID, key.Operation, key.Key).Scan(&code, &version, &payload)
	if pg.IsNoRows(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("postgres: read idempotency record: %w", err)
	}
	if version != application.SnapshotSchemaVersion {
		// 当前构建无法读取的快照不能靠猜测处理。明确失败比重放误解的结果更安全。
		return nil, false, fmt.Errorf("postgres: idempotency snapshot version %d is not readable by this build", version)
	}

	var stored storedResult
	if err := json.Unmarshal(payload, &stored); err != nil {
		return nil, false, fmt.Errorf("postgres: decode idempotency snapshot: %w", err)
	}
	return &application.CommandResult{
		Code: code, Trade: stored.Trade.toDomain(), Product: stored.Product.toDomain(), Created: stored.Created,
	}, true, nil
}

// writeIdempotencyRecord 在命令自身的事务中存储已提交的结果。
func writeIdempotencyRecord(ctx context.Context, tx pgx.Tx, key application.IdempotencyKey, result *application.CommandResult) error {
	payload, err := json.Marshal(storedResult{
		Code: result.Code, Trade: fromDomainTrade(result.Trade),
		Product: fromDomainProduct(result.Product), Created: result.Created,
	})
	if err != nil {
		return fmt.Errorf("postgres: encode idempotency snapshot: %w", err)
	}

	const query = `
		INSERT INTO idempotency_records (actor_id, operation, idempotency_key, result_code, schema_version, result)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err = tx.Exec(ctx, query, key.ActorID, key.Operation, key.Key, result.Code, application.SnapshotSchemaVersion, payload)
	if pg.IsUniqueViolation(err, idempotencyPK) {
		// 另一个使用相同幂等键的事务先提交了。当前事务会完整回滚，因此不会留下领域变更。
		return application.ErrIdempotencyRace
	}
	if err != nil {
		return fmt.Errorf("postgres: write idempotency record: %w", err)
	}
	return nil
}

func fromDomainTrade(trade *domain.Trade) *storedTrade {
	if trade == nil {
		return nil
	}
	return &storedTrade{
		ID:                 trade.ID,
		ProductID:          trade.ProductID,
		BuyerID:            trade.BuyerID,
		SellerID:           trade.SellerID,
		ConversationID:     trade.ConversationID,
		PriceSnapshotMinor: trade.PriceSnapshotMinor,
		Status:             string(trade.Status),
		BuyerConfirmedAt:   trade.BuyerConfirmedAt,
		SellerConfirmedAt:  trade.SellerConfirmedAt,
		CancelReason:       trade.CancelReason,
		CreatedAt:          trade.CreatedAt,
		AcceptedAt:         trade.AcceptedAt,
		CompletedAt:        trade.CompletedAt,
		CancelledAt:        trade.CancelledAt,
		UpdatedAt:          trade.UpdatedAt,
	}
}

func (s *storedTrade) toDomain() *domain.Trade {
	if s == nil {
		return nil
	}
	return &domain.Trade{
		ID:                 s.ID,
		ProductID:          s.ProductID,
		BuyerID:            s.BuyerID,
		SellerID:           s.SellerID,
		ConversationID:     s.ConversationID,
		PriceSnapshotMinor: s.PriceSnapshotMinor,
		Status:             domain.TradeStatus(s.Status),
		BuyerConfirmedAt:   s.BuyerConfirmedAt,
		SellerConfirmedAt:  s.SellerConfirmedAt,
		CancelReason:       s.CancelReason,
		CreatedAt:          s.CreatedAt,
		AcceptedAt:         s.AcceptedAt,
		CompletedAt:        s.CompletedAt,
		CancelledAt:        s.CancelledAt,
		UpdatedAt:          s.UpdatedAt,
	}
}

func fromDomainProduct(product *domain.Product) *storedProduct {
	if product == nil {
		return nil
	}
	images := make([]storedImage, len(product.Images))
	for i, image := range product.Images {
		images[i] = storedImage{
			ID: image.ID, ProductID: image.ProductID, ObjectKey: image.ObjectKey,
			SortOrder: image.SortOrder, CreatedAt: image.CreatedAt,
		}
	}
	return &storedProduct{
		ID: product.ID, SellerID: product.SellerID, Title: product.Title,
		PriceMinor: product.PriceMinor, Category: string(product.Category), Description: product.Description,
		Status: string(product.Status), Version: product.Version, Images: images,
		CreatedAt: product.CreatedAt, UpdatedAt: product.UpdatedAt,
	}
}

func (s *storedProduct) toDomain() *domain.Product {
	if s == nil {
		return nil
	}
	images := make([]domain.Image, len(s.Images))
	for i, image := range s.Images {
		images[i] = domain.Image{
			ID: image.ID, ProductID: image.ProductID, ObjectKey: image.ObjectKey,
			SortOrder: image.SortOrder, CreatedAt: image.CreatedAt,
		}
	}
	return &domain.Product{
		ID: s.ID, SellerID: s.SellerID, Title: s.Title, PriceMinor: s.PriceMinor,
		Category: domain.Category(s.Category), Description: s.Description, Status: domain.Status(s.Status),
		Version: s.Version, Images: images, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

func scanTrade(row pgx.Row) (*domain.Trade, error) {
	var (
		trade  domain.Trade
		status string
	)
	err := row.Scan(
		&trade.ID, &trade.ProductID, &trade.BuyerID, &trade.SellerID, &trade.ConversationID,
		&trade.PriceSnapshotMinor, &status, &trade.BuyerConfirmedAt, &trade.SellerConfirmedAt,
		&trade.CancelReason, &trade.CreatedAt, &trade.AcceptedAt, &trade.CompletedAt,
		&trade.CancelledAt, &trade.UpdatedAt,
	)
	if pg.IsNoRows(err) {
		return nil, application.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	trade.Status = domain.TradeStatus(status)
	return &trade, nil
}
