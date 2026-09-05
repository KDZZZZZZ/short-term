package postgres_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/platform/pgtest"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/postgres"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
	"github.com/KDZZZZZZ/short-term/services/marketplace/migrations"
)

// newTradeReviewRepository 通过真实数据库构造买家评价仓储，并返回连接池供测试播种。
func newTradeReviewRepository(t *testing.T) (*postgres.TradeReviewRepository, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.New(t, migrations.FS, migrations.Dir)
	return postgres.NewTradeReviewRepository(pool), pool
}

// seedTradeReview 直接写入一行已完成交易（买家评价的外键来源）。
func seedTradeReview(t *testing.T, pool *pgxpool.Pool, id, productID, buyerID, sellerID string) {
	t.Helper()

	seedProduct(t, pool, productID, sellerID)
	seedCompletedTrade(t, pool, id, productID, buyerID, sellerID)
}

// seedCompletedTrade 写入一行 COMPLETED 交易。评价仓储依赖的交易事实只有
// 状态和归属，两者都受 trades 表自身约束保护，因此用最小 SQL 播种即可。
func seedCompletedTrade(t *testing.T, pool *pgxpool.Pool, id, productID, buyerID, sellerID string) {
	t.Helper()

	const query = `INSERT INTO trades (id, product_id, buyer_id, seller_id, price_snapshot_minor, status,
		buyer_confirmed_at, seller_confirmed_at, cancel_reason, created_at, accepted_at, completed_at, cancelled_at, updated_at)
		VALUES ($1, $2, $3, $4, 12000, 'COMPLETED', $5, $5, NULL, $5, $5, $5, NULL, $5)`
	if _, err := pool.Exec(t.Context(), query,
		id, productID, buyerID, sellerID, created,
	); err != nil {
		t.Fatalf("seed completed trade %s: %v", id, err)
	}
}

func TestTradeReviewPersistsAndBatchesByProduct(t *testing.T) {
	t.Parallel()

	repo, pool := newTradeReviewRepository(t)
	seedTradeReview(t, pool, "t_1", "p_1", "u_buyer", "u_seller")
	seedTradeReview(t, pool, "t_2", "p_2", "u_buyer", "u_seller")

	first, err := domain.NewTradeReview("tr_1", &domain.Trade{ID: "t_1", ProductID: "p_1", BuyerID: "u_buyer"}, 5, "很好", created)
	if err != nil {
		t.Fatalf("NewTradeReview: %v", err)
	}
	if err := repo.Insert(t.Context(), first); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	second, err := domain.NewTradeReview("tr_2", &domain.Trade{ID: "t_2", ProductID: "p_2", BuyerID: "u_buyer"}, 4, "不错", created)
	if err != nil {
		t.Fatalf("NewTradeReview: %v", err)
	}
	if err := repo.Insert(t.Context(), second); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	reviews, err := repo.ByProductIDs(t.Context(), []string{"p_1", "p_2", "p_3"})
	if err != nil {
		t.Fatalf("ByProductIDs: %v", err)
	}
	if len(reviews) != 2 {
		t.Fatalf("batch returned %d reviews, want 2", len(reviews))
	}
	if reviews["p_1"].ID != "tr_1" || reviews["p_2"].ID != "tr_2" {
		t.Fatalf("batch is not indexed by product: %v", reviews)
	}
}

func TestTradeReviewRejectsASecondReviewForTheSameTrade(t *testing.T) {
	t.Parallel()

	repo, pool := newTradeReviewRepository(t)
	seedTradeReview(t, pool, "t_1", "p_1", "u_buyer", "u_seller")

	trade := &domain.Trade{ID: "t_1", ProductID: "p_1", BuyerID: "u_buyer"}
	first, err := domain.NewTradeReview("tr_1", trade, 5, "很好", created)
	if err != nil {
		t.Fatalf("NewTradeReview: %v", err)
	}
	if err := repo.Insert(t.Context(), first); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	second, err := domain.NewTradeReview("tr_2", trade, 3, "换一条", created)
	if err != nil {
		t.Fatalf("NewTradeReview: %v", err)
	}
	if err := repo.Insert(t.Context(), second); !errors.Is(err, application.ErrTradeReviewAlreadyExists) {
		t.Fatalf("duplicate review error = %v, want ErrTradeReviewAlreadyExists", err)
	}
}

func TestTradeReviewInsertMapsMissingTradeToNotFound(t *testing.T) {
	t.Parallel()

	repo, _ := newTradeReviewRepository(t)

	review, err := domain.NewTradeReview("tr_1", &domain.Trade{ID: "t_missing", ProductID: "p_1", BuyerID: "u_buyer"}, 5, "评论", created)
	if err != nil {
		t.Fatalf("NewTradeReview: %v", err)
	}
	if err := repo.Insert(t.Context(), review); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("missing trade insert error = %v, want ErrNotFound", err)
	}
}

func TestAverageScoresByUserIDs(t *testing.T) {
	t.Parallel()

	repo, pool := newTradeReviewRepository(t)
	seedTradeReview(t, pool, "t_1", "p_1", "u_buyer_1", "u_seller")
	seedTradeReview(t, pool, "t_2", "p_2", "u_buyer_2", "u_seller")
	seedTradeReview(t, pool, "t_3", "p_3", "u_buyer_3", "u_other")

	for _, tt := range []struct {
		id    string
		trade string
		score int32
	}{
		{id: "tr_1", trade: "t_1", score: 5},
		{id: "tr_2", trade: "t_2", score: 4},
	} {
		review, err := domain.NewTradeReview(tt.id, &domain.Trade{ID: tt.trade, ProductID: "p_" + tt.trade[2:], BuyerID: "u_buyer"}, tt.score, "评论", created)
		if err != nil {
			t.Fatalf("NewTradeReview: %v", err)
		}
		if err := repo.Insert(t.Context(), review); err != nil {
			t.Fatalf("Insert %s: %v", tt.id, err)
		}
	}

	scores, err := repo.AverageScoresByUserIDs(t.Context(), []string{"u_seller", "u_other"})
	if err != nil {
		t.Fatalf("AverageScoresByUserIDs: %v", err)
	}
	if got := scores["u_seller"]; got != "4.50" {
		t.Fatalf("average = %q, want 4.50", got)
	}
	if _, exists := scores["u_other"]; exists {
		t.Fatalf("a seller without scores must be absent: %v", scores)
	}

	empty, err := repo.AverageScoresByUserIDs(t.Context(), nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty input = %v, %v, want an empty map", empty, err)
	}
}

func TestTradeReviewArbitratesConcurrentDuplicates(t *testing.T) {
	t.Parallel()

	repo, pool := newTradeReviewRepository(t)
	seedTradeReview(t, pool, "t_1", "p_1", "u_buyer", "u_seller")

	trade := &domain.Trade{ID: "t_1", ProductID: "p_1", BuyerID: "u_buyer"}
	const attempts = 8
	reviews := make([]*domain.TradeReview, attempts)
	for i := range attempts {
		review, err := domain.NewTradeReview(fmt.Sprintf("tr_concurrent_%d", i), trade, 5, "并发评价", created)
		if err != nil {
			t.Fatalf("NewTradeReview: %v", err)
		}
		reviews[i] = review
	}

	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for _, review := range reviews {
		wg.Add(1)
		go func(review *domain.TradeReview) {
			defer wg.Done()
			results <- repo.Insert(t.Context(), review)
		}(review)
	}
	wg.Wait()
	close(results)

	succeeded := 0
	duplicated := 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, application.ErrTradeReviewAlreadyExists):
			duplicated++
		default:
			t.Fatalf("unexpected insert error: %v", err)
		}
	}
	if succeeded != 1 || duplicated != attempts-1 {
		t.Fatalf("succeeded = %d, duplicated = %d, want exactly one success", succeeded, duplicated)
	}
}

// TestTradeReviewBatchSkipsLegacyRowsWithoutScore 回归：000006 迁移之前的
// 历史评价行 score 为 NULL，批量读取必须跳过它们而不是把扫描错误冒泡成 500。
func TestTradeReviewBatchSkipsLegacyRowsWithoutScore(t *testing.T) {
	t.Parallel()

	repo, pool := newTradeReviewRepository(t)
	seedTradeReview(t, pool, "t_legacy", "p_legacy", "u_buyer", "u_seller")
	const legacy = `INSERT INTO trade_reviews (id, trade_id, product_id, buyer_id, content, score, created_at)
		VALUES ('tr_legacy', 't_legacy', 'p_legacy', 'u_buyer', '旧版纯文字评价', NULL, $1)`
	if _, err := pool.Exec(t.Context(), legacy, created); err != nil {
		t.Fatalf("seed legacy review: %v", err)
	}

	seedTradeReview(t, pool, "t_scored", "p_scored", "u_buyer", "u_seller")
	scored, err := domain.NewTradeReview("tr_scored", &domain.Trade{ID: "t_scored", ProductID: "p_scored", BuyerID: "u_buyer"}, 4, "新评分", created)
	if err != nil {
		t.Fatalf("NewTradeReview: %v", err)
	}
	if err := repo.Insert(t.Context(), scored); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	reviews, err := repo.ByProductIDs(t.Context(), []string{"p_legacy", "p_scored", "p_missing"})
	if err != nil {
		t.Fatalf("ByProductIDs with legacy row: %v", err)
	}
	if _, ok := reviews["p_legacy"]; ok {
		t.Fatalf("legacy row without score must be skipped, got %v", reviews["p_legacy"])
	}
	if reviews["p_scored"] == nil || reviews["p_scored"].Score != 4 {
		t.Fatalf("scored review missing or wrong score: %v", reviews["p_scored"])
	}
}
