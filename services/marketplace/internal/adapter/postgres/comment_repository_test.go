package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/platform/pgtest"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/postgres"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
	"github.com/KDZZZZZZ/short-term/services/marketplace/migrations"
)

// newCommentRepository 通过真实数据库构造评论仓储，并返回连接池供测试播种。
func newCommentRepository(t *testing.T) (*postgres.CommentRepository, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.New(t, migrations.FS, migrations.Dir)
	return postgres.NewCommentRepository(pool), pool
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

// mustInsertComment 向商品写入一条评论并要求成功。
func mustInsertComment(t *testing.T, repo *postgres.CommentRepository, id, productID, userID, content string, at time.Time) *domain.ProductComment {
	t.Helper()

	comment, err := domain.NewProductComment(id, productID, userID, content, at)
	if err != nil {
		t.Fatalf("NewProductComment: %v", err)
	}
	if err := repo.Insert(t.Context(), comment); err != nil {
		t.Fatalf("Insert comment %s: %v", id, err)
	}
	return comment
}

func TestCommentsPersistAndListNewestFirst(t *testing.T) {
	t.Parallel()

	repo, pool := newCommentRepository(t)
	seedProduct(t, pool, "p_1", "u_seller")

	mustInsertComment(t, repo, "cm_1", "p_1", "u_user", "第一条评论", created)
	mustInsertComment(t, repo, "cm_2", "p_1", "u_user", "同一用户的第二条评论", created.Add(time.Hour))
	mustInsertComment(t, repo, "cm_3", "p_1", "u_other", "另一位用户的评论", created.Add(30*time.Minute))

	page, err := repo.ListProductComments(t.Context(), "p_1", application.Page{Number: 1, Size: 20})
	if err != nil {
		t.Fatalf("ListProductComments: %v", err)
	}
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("page = %+v, want 3 rows", page)
	}
	if page.Items[0].ID != "cm_2" || page.Items[1].ID != "cm_3" || page.Items[2].ID != "cm_1" {
		t.Fatalf("comments are not ordered newest first: %s, %s, %s",
			page.Items[0].ID, page.Items[1].ID, page.Items[2].ID)
	}
}

func TestCommentListIsEmptyForACommentlessProduct(t *testing.T) {
	t.Parallel()

	repo, pool := newCommentRepository(t)
	seedProduct(t, pool, "p_1", "u_seller")

	page, err := repo.ListProductComments(t.Context(), "p_1", application.Page{Number: 1, Size: 20})
	if err != nil {
		t.Fatalf("ListProductComments: %v", err)
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("page = %+v, want an empty page", page)
	}
}

func TestCommentInsertOnMissingProductIsNotFound(t *testing.T) {
	t.Parallel()

	repo, _ := newCommentRepository(t)

	comment, err := domain.NewProductComment("cm_1", "p_missing", "u_user", "评论", created)
	if err != nil {
		t.Fatalf("NewProductComment: %v", err)
	}
	if err := repo.Insert(t.Context(), comment); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("missing product insert error = %v, want ErrNotFound", err)
	}
}

func TestProductExistsReportsExistence(t *testing.T) {
	t.Parallel()

	repo, pool := newCommentRepository(t)
	seedProduct(t, pool, "p_1", "u_seller")

	if exists, err := repo.ProductExists(t.Context(), "p_1"); err != nil || !exists {
		t.Fatalf("ProductExists(p_1) = %v, %v, want true", exists, err)
	}
	if exists, err := repo.ProductExists(t.Context(), "p_missing"); err != nil || exists {
		t.Fatalf("ProductExists(p_missing) = %v, %v, want false", exists, err)
	}
}
