package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/platform/pgtest"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/postgres"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
	"github.com/KDZZZZZZ/short-term/services/marketplace/migrations"
)

// newReviewRepository 通过真实数据库构造评论仓储，并返回连接池供测试播种。
func newReviewRepository(t *testing.T) (*postgres.ReviewRepository, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.New(t, migrations.FS, migrations.Dir)
	return postgres.NewReviewRepository(pool), pool
}

// seedProduct 通过真实商品仓储写入一个在售商品。
func seedProduct(t *testing.T, pool *pgxpool.Pool, id, sellerID string) {
	t.Helper()

	product, err := domain.NewProduct(id, sellerID, "机械键盘", 12000, domain.CategoryDigital, "九成新", created)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	if err := postgres.NewProductRepository(pool).Create(t.Context(), product); err != nil {
		t.Fatalf("Create product: %v", err)
	}
}

// seedTrade 直接写入一行交易。评论仓储依赖的交易事实只有状态和归属，
// 两者都受 trades 表自身约束保护，因此用最小 SQL 播种即可。
func seedTrade(t *testing.T, pool *pgxpool.Pool, id, productID, buyerID, sellerID, status string) {
	t.Helper()

	var accepted, buyerConfirmed, sellerConfirmed, completed, cancelled *time.Time
	switch status {
	case "COMPLETED":
		accepted, buyerConfirmed, sellerConfirmed, completed = &created, &created, &created, &created
	case "ACCEPTED":
		accepted = &created
	case "CANCELLED":
		cancelled = &created
	}

	const query = `INSERT INTO trades (id, product_id, buyer_id, seller_id, price_snapshot_minor, status,
		buyer_confirmed_at, seller_confirmed_at, cancel_reason, created_at, accepted_at, completed_at, cancelled_at, updated_at)
		VALUES ($1, $2, $3, $4, 12000, $5, $6, $7, NULL, $8, $9, $10, $11, $8)`
	if _, err := pool.Exec(t.Context(), query,
		id, productID, buyerID, sellerID, status, buyerConfirmed, sellerConfirmed,
		created, accepted, completed, cancelled,
	); err != nil {
		t.Fatalf("seed trade %s: %v", id, err)
	}
}

// mustCreateReview 在事务中执行一次评论插入，并要求成功。
func mustCreateReview(t *testing.T, repo *postgres.ReviewRepository, review *domain.Review, completedTrade bool) {
	t.Helper()

	err := createReview(t, repo, review, completedTrade)
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
}

func createReview(t *testing.T, repo *postgres.ReviewRepository, review *domain.Review, completedTrade bool) error {
	t.Helper()

	return repo.Execute(t.Context(), func(ctx context.Context, tx application.ReviewTx) error {
		if completedTrade {
			trade, err := tx.CompletedTradeForBuyer(ctx, review.ProductID, review.BuyerID)
			if err != nil {
				return err
			}
			review.TradeID = trade.ID
		}
		return tx.InsertReview(ctx, review)
	})
}

func TestReviewPersistsAndListsNewestFirst(t *testing.T) {
	t.Parallel()

	repo, pool := newReviewRepository(t)
	seedProduct(t, pool, "p_1", "u_seller")
	seedTrade(t, pool, "t_1", "p_1", "u_buyer_1", "u_seller", "COMPLETED")
	seedTrade(t, pool, "t_2", "p_1", "u_buyer_2", "u_seller", "COMPLETED")

	first, err := domain.NewReview("rv_1", "p_1", "u_buyer_1", "很好的卖家", created)
	if err != nil {
		t.Fatalf("NewReview: %v", err)
	}
	mustCreateReview(t, repo, first, true)

	later := created.Add(time.Hour)
	second, err := domain.NewReview("rv_2", "p_1", "u_buyer_2", "物有所值", later)
	if err != nil {
		t.Fatalf("NewReview: %v", err)
	}
	mustCreateReview(t, repo, second, true)

	page, err := repo.ListProductReviews(t.Context(), "p_1", application.Page{Number: 1, Size: 20})
	if err != nil {
		t.Fatalf("ListProductReviews: %v", err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("page = %+v, want 2 rows", page)
	}
	if page.Items[0].ID != "rv_2" || page.Items[1].ID != "rv_1" {
		t.Fatalf("reviews are not ordered newest first: %s, %s", page.Items[0].ID, page.Items[1].ID)
	}
	if page.Items[1].TradeID != "t_1" {
		t.Fatalf("the stored review lost its trade provenance: %+v", page.Items[1])
	}
}

func TestReviewRejectsASecondReviewByTheSameBuyer(t *testing.T) {
	t.Parallel()

	repo, pool := newReviewRepository(t)
	seedProduct(t, pool, "p_1", "u_seller")
	seedTrade(t, pool, "t_1", "p_1", "u_buyer", "u_seller", "COMPLETED")

	first, err := domain.NewReview("rv_1", "p_1", "u_buyer", "很好", created)
	if err != nil {
		t.Fatalf("NewReview: %v", err)
	}
	mustCreateReview(t, repo, first, true)

	second, err := domain.NewReview("rv_2", "p_1", "u_buyer", "换一条", created)
	if err != nil {
		t.Fatalf("NewReview: %v", err)
	}
	if err := createReview(t, repo, second, true); !errors.Is(err, application.ErrReviewAlreadyExists) {
		t.Fatalf("duplicate review error = %v, want ErrReviewAlreadyExists", err)
	}
}

func TestReviewArbitratesConcurrentDuplicates(t *testing.T) {
	t.Parallel()

	repo, pool := newReviewRepository(t)
	seedProduct(t, pool, "p_1", "u_seller")
	seedTrade(t, pool, "t_1", "p_1", "u_buyer", "u_seller", "COMPLETED")

	const attempts = 8
	reviews := make([]*domain.Review, attempts)
	for i := range attempts {
		review, err := domain.NewReview(applicationReviewID(i), "p_1", "u_buyer", "并发评论", created)
		if err != nil {
			t.Fatalf("NewReview: %v", err)
		}
		reviews[i] = review
	}

	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for _, review := range reviews {
		wg.Add(1)
		go func(review *domain.Review) {
			defer wg.Done()
			results <- createReview(t, repo, review, true)
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
		case errors.Is(err, application.ErrReviewAlreadyExists):
			duplicated++
		default:
			t.Fatalf("unexpected insert error: %v", err)
		}
	}
	if succeeded != 1 || duplicated != attempts-1 {
		t.Fatalf("succeeded = %d, duplicated = %d, want exactly one success", succeeded, duplicated)
	}
}

func TestReviewCreateRequiresACompletedTrade(t *testing.T) {
	t.Parallel()

	repo, pool := newReviewRepository(t)
	seedProduct(t, pool, "p_1", "u_seller")
	seedTrade(t, pool, "t_1", "p_1", "u_buyer", "u_seller", "PENDING")

	var lastErr error
	err := repo.Execute(t.Context(), func(ctx context.Context, tx application.ReviewTx) error {
		_, lastErr = tx.CompletedTradeForBuyer(ctx, "p_1", "u_buyer")
		return lastErr
	})
	if !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("pending trade error = %v, want ErrNotFound", err)
	}

	seedTrade(t, pool, "t_2", "p_1", "u_other", "u_seller", "CANCELLED")
	err = repo.Execute(t.Context(), func(ctx context.Context, tx application.ReviewTx) error {
		_, err := tx.CompletedTradeForBuyer(ctx, "p_1", "u_other")
		return err
	})
	if !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("cancelled trade error = %v, want ErrNotFound", err)
	}
}

func TestReviewInsertMapsMissingProductToNotFound(t *testing.T) {
	t.Parallel()

	repo, _ := newReviewRepository(t)

	review, err := domain.NewReview("rv_1", "p_missing", "u_buyer", "评论", created)
	if err != nil {
		t.Fatalf("NewReview: %v", err)
	}
	err = repo.Execute(t.Context(), func(ctx context.Context, tx application.ReviewTx) error {
		review.TradeID = "t_missing"
		return tx.InsertReview(ctx, review)
	})
	if !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("missing product insert error = %v, want ErrNotFound", err)
	}
}

func TestProductExistsReportsExistence(t *testing.T) {
	t.Parallel()

	repo, pool := newReviewRepository(t)
	seedProduct(t, pool, "p_1", "u_seller")

	if exists, err := repo.ProductExists(t.Context(), "p_1"); err != nil || !exists {
		t.Fatalf("ProductExists(p_1) = %v, %v, want true", exists, err)
	}
	if exists, err := repo.ProductExists(t.Context(), "p_missing"); err != nil || exists {
		t.Fatalf("ProductExists(p_missing) = %v, %v, want false", exists, err)
	}
}

// applicationReviewID 生成测试用的唯一评论标识。
func applicationReviewID(i int) string {
	return fmt.Sprintf("rv_concurrent_%d", i)
}
