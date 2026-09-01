package postgres_test

import (
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

var created = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

func newRepository(t *testing.T) (*postgres.ProductRepository, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.New(t, migrations.FS, migrations.Dir)
	return postgres.NewProductRepository(pool), pool
}

func newProduct(t *testing.T, id, sellerID, title string, at time.Time) *domain.Product {
	t.Helper()

	product, err := domain.NewProduct(id, sellerID, title, 12000, domain.CategoryDigital, "九成新", at)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	return product
}

func TestCreateAndReadProductWithImages(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)
	product := newProduct(t, "p_1", "u_seller", "机械键盘", created)
	product.Images = []domain.Image{
		{ID: "img_2", ProductID: "p_1", ObjectKey: "products/p_1/img_2.png", SortOrder: 2, CreatedAt: created},
		{ID: "img_1", ProductID: "p_1", ObjectKey: "products/p_1/img_1.jpg", SortOrder: 1, CreatedAt: created},
	}

	if err := repo.Create(t.Context(), product); err != nil {
		t.Fatalf("Create: %v", err)
	}

	loaded, err := repo.ByID(t.Context(), "p_1")
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if loaded.Status != domain.StatusOnSale {
		t.Fatalf("Status = %s, want ON_SALE", loaded.Status)
	}
	if len(loaded.Images) != 2 {
		t.Fatalf("got %d images, want 2", len(loaded.Images))
	}
	// Images come back ordered by slot, so the cover is deterministic.
	if loaded.Images[0].SortOrder != 1 || loaded.Images[1].SortOrder != 2 {
		t.Fatalf("images are not ordered by slot: %+v", loaded.Images)
	}
	if got := loaded.CoverObjectKey(); got != "products/p_1/img_1.jpg" {
		t.Fatalf("CoverObjectKey = %q", got)
	}
}

func TestCreateRollsBackWhenAnImageIsInvalid(t *testing.T) {
	t.Parallel()

	repo, pool := newRepository(t)
	product := newProduct(t, "p_1", "u_seller", "机械键盘", created)
	product.Images = []domain.Image{
		{ID: "img_1", ProductID: "p_1", ObjectKey: "ok", SortOrder: 1, CreatedAt: created},
		// Slot 4 violates the CHECK constraint.
		{ID: "img_4", ProductID: "p_1", ObjectKey: "bad", SortOrder: 4, CreatedAt: created},
	}

	if err := repo.Create(t.Context(), product); err == nil {
		t.Fatal("Create accepted an out-of-range image slot")
	}

	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM products`).Scan(&count); err != nil {
		t.Fatalf("count products: %v", err)
	}
	if count != 0 {
		t.Fatalf("product rows = %d, want 0: the failed image did not roll back the product", count)
	}
}

func TestByIDReportsAMissingProduct(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)

	if _, err := repo.ByID(t.Context(), "p_missing"); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("ByID = %v, want ErrNotFound", err)
	}
}

func TestListOrdersNewestFirstWithADeterministicTieBreaker(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)

	// Every product shares one timestamp, so only the identifier can order
	// them. Without the tie-breaker, paging could repeat or skip rows.
	for i := range 10 {
		product := newProduct(t, fmt.Sprintf("p_%02d", i), "u_seller", fmt.Sprintf("商品 %d", i), created)
		if err := repo.Create(t.Context(), product); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	onSale := domain.StatusOnSale
	var seen []string
	for pageNumber := int32(1); pageNumber <= 4; pageNumber++ {
		page, err := repo.List(t.Context(), application.ProductFilter{Status: &onSale}, application.Page{Number: pageNumber, Size: 3})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if page.Total != 10 {
			t.Fatalf("Total = %d, want 10", page.Total)
		}
		for _, product := range page.Items {
			seen = append(seen, product.ID)
		}
	}

	if len(seen) != 10 {
		t.Fatalf("paging returned %d products, want 10: %v", len(seen), seen)
	}
	unique := map[string]bool{}
	for _, id := range seen {
		if unique[id] {
			t.Fatalf("product %s appeared on two pages: %v", id, seen)
		}
		unique[id] = true
	}
	for i := 1; i < len(seen); i++ {
		if seen[i-1] <= seen[i] {
			t.Fatalf("results are not ordered by id descending: %v", seen)
		}
	}
}

func TestListFiltersByStatusCategoryAndSeller(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)

	onSale := newProduct(t, "p_1", "u_seller", "在售", created)
	offShelf := newProduct(t, "p_2", "u_seller", "下架", created)
	if err := offShelf.OffShelf("u_seller", created); err != nil {
		t.Fatalf("OffShelf: %v", err)
	}
	other := newProduct(t, "p_3", "u_other", "别人的", created)
	other.Category = domain.CategoryTextbook

	for _, product := range []*domain.Product{onSale, offShelf, other} {
		if err := repo.Create(t.Context(), product); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	statusOnSale := domain.StatusOnSale
	textbook := domain.CategoryTextbook

	tests := []struct {
		name   string
		filter application.ProductFilter
		want   []string
	}{
		{name: "on sale only", filter: application.ProductFilter{Status: &statusOnSale}, want: []string{"p_3", "p_1"}},
		{name: "by seller", filter: application.ProductFilter{SellerID: "u_seller"}, want: []string{"p_2", "p_1"}},
		{name: "by category", filter: application.ProductFilter{Category: &textbook}, want: []string{"p_3"}},
		{name: "seller and status", filter: application.ProductFilter{SellerID: "u_seller", Status: &statusOnSale}, want: []string{"p_1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			page, err := repo.List(t.Context(), tt.filter, application.Page{Number: 1, Size: 20})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			var ids []string
			for _, product := range page.Items {
				ids = append(ids, product.ID)
			}
			if fmt.Sprint(ids) != fmt.Sprint(tt.want) {
				t.Fatalf("ids = %v, want %v", ids, tt.want)
			}
		})
	}
}

func TestListKeywordMatchesLiterallyAndIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)

	for id, title := range map[string]string{
		"p_1": "Mechanical Keyboard",
		"p_2": "机械键盘 100% 新",
		"p_3": "无关商品",
	} {
		if err := repo.Create(t.Context(), newProduct(t, id, "u_seller", title, created)); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	tests := []struct {
		name    string
		keyword string
		want    []string
	}{
		{name: "case insensitive", keyword: "keyboard", want: []string{"p_1"}},
		{name: "chinese substring", keyword: "键盘", want: []string{"p_2"}},
		// A wildcard in user input must match the literal characters, not
		// every row.
		{name: "percent is literal", keyword: "100%", want: []string{"p_2"}},
		{name: "underscore is literal", keyword: "_", want: nil},
		{name: "no match", keyword: "自行车", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			keyword := tt.keyword
			page, err := repo.List(t.Context(), application.ProductFilter{Keyword: &keyword}, application.Page{Number: 1, Size: 20})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			var ids []string
			for _, product := range page.Items {
				ids = append(ids, product.ID)
			}
			if fmt.Sprint(ids) != fmt.Sprint(tt.want) {
				t.Fatalf("ids = %v, want %v", ids, tt.want)
			}
		})
	}
}

func TestUpdateDetectsAConcurrentChange(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)
	if err := repo.Create(t.Context(), newProduct(t, "p_1", "u_seller", "机械键盘", created)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	first, err := repo.ByID(t.Context(), "p_1")
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	second, err := repo.ByID(t.Context(), "p_1")
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}

	title := "先改的标题"
	if err := first.Edit("u_seller", &title, nil, nil, nil, created.Add(time.Minute)); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if err := repo.Update(t.Context(), first, 1); err != nil {
		t.Fatalf("first Update: %v", err)
	}

	lateTitle := "后改的标题"
	if err := second.Edit("u_seller", &lateTitle, nil, nil, nil, created.Add(2*time.Minute)); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if err := repo.Update(t.Context(), second, 1); !errors.Is(err, application.ErrVersionConflict) {
		t.Fatalf("second Update = %v, want ErrVersionConflict", err)
	}

	reloaded, err := repo.ByID(t.Context(), "p_1")
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if reloaded.Title != title {
		t.Fatalf("Title = %q, want the first writer's value", reloaded.Title)
	}
}

func TestUpdateReportsAMissingProduct(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)

	err := repo.Update(t.Context(), newProduct(t, "p_missing", "u_seller", "t", created), 1)
	if !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("Update = %v, want ErrNotFound", err)
	}
}

func TestConcurrentImageWritesCannotExceedThreeSlots(t *testing.T) {
	t.Parallel()

	repo, pool := newRepository(t)
	if err := repo.Create(t.Context(), newProduct(t, "p_1", "u_seller", "机械键盘", created)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Every writer races for slot 1. Only the unique index can decide.
	const attempts = 8
	results := make([]error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = repo.AddImage(t.Context(), &domain.Image{
				ID:        fmt.Sprintf("img_%d", i),
				ProductID: "p_1",
				ObjectKey: fmt.Sprintf("products/p_1/img_%d.jpg", i),
				SortOrder: 1,
				CreatedAt: created,
			})
		}()
	}
	wg.Wait()

	var succeeded int
	for _, err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, application.ErrImageSlotTaken):
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d writers took slot 1, want exactly 1", succeeded)
	}

	var stored int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM product_images WHERE product_id = $1`, "p_1").Scan(&stored); err != nil {
		t.Fatalf("count images: %v", err)
	}
	if stored != 1 {
		t.Fatalf("stored images = %d, want 1", stored)
	}
}

func TestAddImageForAMissingProduct(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)

	err := repo.AddImage(t.Context(), &domain.Image{
		ID: "img_1", ProductID: "p_missing", ObjectKey: "k", SortOrder: 1, CreatedAt: created,
	})
	if !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("AddImage = %v, want ErrNotFound", err)
	}
}

func TestDeleteImageReturnsTheObjectKeyOnce(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)
	product := newProduct(t, "p_1", "u_seller", "机械键盘", created)
	product.Images = []domain.Image{{ID: "img_1", ProductID: "p_1", ObjectKey: "products/p_1/img_1.jpg", SortOrder: 1, CreatedAt: created}}
	if err := repo.Create(t.Context(), product); err != nil {
		t.Fatalf("Create: %v", err)
	}

	key, err := repo.DeleteImage(t.Context(), "p_1", "img_1")
	if err != nil {
		t.Fatalf("DeleteImage: %v", err)
	}
	if key != "products/p_1/img_1.jpg" {
		t.Fatalf("object key = %q", key)
	}

	if _, err := repo.DeleteImage(t.Context(), "p_1", "img_1"); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("a second DeleteImage = %v, want ErrNotFound", err)
	}
}

func TestDeletingAProductRemovesItsImages(t *testing.T) {
	t.Parallel()

	repo, pool := newRepository(t)
	product := newProduct(t, "p_1", "u_seller", "机械键盘", created)
	product.Images = []domain.Image{{ID: "img_1", ProductID: "p_1", ObjectKey: "k", SortOrder: 1, CreatedAt: created}}
	if err := repo.Create(t.Context(), product); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := pool.Exec(t.Context(), `DELETE FROM products WHERE id = $1`, "p_1"); err != nil {
		t.Fatalf("delete product: %v", err)
	}

	var orphans int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM product_images`).Scan(&orphans); err != nil {
		t.Fatalf("count images: %v", err)
	}
	if orphans != 0 {
		t.Fatalf("orphaned image rows = %d, want 0", orphans)
	}
}
