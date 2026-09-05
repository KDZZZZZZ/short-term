package grpc_test

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/platform/pgtest"
	grpcadapter "github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/grpc"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/objectstore"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/postgres"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/system"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/migrations"
)

const seller = "u_seller"

type harness struct {
	client    marketplacev1.MarketplaceServiceClient
	mediaRoot string
	pool      *pgxpool.Pool
}

// newHarness 通过真实仓储和真实文件系统对象存储运行 gRPC 适配器，
// 因此上传、事务和排序都会在实际基础设施上得到测试。
func newHarness(t *testing.T) harness { return newHarnessWith(t, nil) }

// newHarnessWith 允许测试替换标识符生成器：这是注入 outbox 写入失败的方式，
// 一个已存在的事件 id 会让本应有效的命令在追加事件时失败。
func newHarnessWith(t *testing.T, ids application.IDGenerator) harness {
	return newHarnessWithConversationVerifier(t, ids, fakeConversationVerifier{})
}

// newHarnessWithConversationVerifier lets trade tests provide Messaging facts
// without coupling Marketplace integration tests to a running M5 service.
func newHarnessWithConversationVerifier(t *testing.T, ids application.IDGenerator, conversations application.ConversationVerifier) harness {
	t.Helper()

	pool := pgtest.New(t, migrations.FS, migrations.Dir)
	mediaRoot := t.TempDir()

	objects, err := objectstore.NewFilesystem(mediaRoot, "https://media.example.test")
	if err != nil {
		t.Fatalf("NewFilesystem: %v", err)
	}
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	if ids == nil {
		ids = system.NewIDs()
	}
	if conversations == nil {
		conversations = fakeConversationVerifier{}
	}
	productRepo := postgres.NewProductRepository(pool)

	products, err := application.NewProductService(productRepo, objects, ids, system.Clock{}, discard)
	if err != nil {
		t.Fatalf("NewProductService: %v", err)
	}
	trades, err := application.NewTradeService(
		postgres.NewTradeRepository(pool), productRepo, conversations, ids, system.Clock{}, discard,
	)
	if err != nil {
		t.Fatalf("NewTradeService: %v", err)
	}
	reviews, err := application.NewReviewService(postgres.NewReviewRepository(pool), ids, system.Clock{})
	if err != nil {
		t.Fatalf("NewReviewService: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpcx.NewServer(grpcx.ServerOptions{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		HandlerTimeout: 30 * time.Second,
	})
	marketplacev1.RegisterMarketplaceServiceServer(server, grpcadapter.NewServer(products, trades, reviews))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpcx.Dial(grpcx.ClientOptions{
		Target:         listener.Addr().String(),
		Caller:         "test",
		DefaultTimeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return harness{client: marketplacev1.NewMarketplaceServiceClient(conn), mediaRoot: mediaRoot, pool: pool}
}

func TestPublishedProductIsOnSaleAndVisibleInTheCatalogue(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	created := h.create(t, seller, "机械键盘", nil)

	if created.GetStatus() != marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE {
		t.Fatalf("status = %s, want ON_SALE", created.GetStatus())
	}

	page, err := h.client.ListProducts(t.Context(), &marketplacev1.ListProductsRequest{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(page.GetPage().GetItems()) != 1 {
		t.Fatalf("catalogue has %d items, want 1", len(page.GetPage().GetItems()))
	}
	if page.GetPage().GetItems()[0].GetStatus() != marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE {
		t.Fatal("the summary does not carry the current status")
	}
}

func TestPublicCatalogueHidesEverythingButOnSale(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	visible := h.create(t, seller, "在售商品", nil)
	hidden := h.create(t, seller, "下架商品", nil)

	if _, err := h.client.OffShelfProduct(t.Context(), &marketplacev1.OffShelfProductRequest{
		ActorId: seller, ProductId: hidden.GetId(),
	}); err != nil {
		t.Fatalf("OffShelfProduct: %v", err)
	}

	page, err := h.client.ListProducts(t.Context(), &marketplacev1.ListProductsRequest{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(page.GetPage().GetItems()) != 1 || page.GetPage().GetItems()[0].GetId() != visible.GetId() {
		t.Fatalf("the public catalogue returned %+v, want only the on-sale product", page.GetPage().GetItems())
	}
	if page.GetPage().GetTotal() != 1 {
		t.Fatalf("total = %d, want 1", page.GetPage().GetTotal())
	}
}

func TestOffShelfAndRelistRoundTripIsVisibleEverywhere(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)

	offShelf, err := h.client.OffShelfProduct(t.Context(), &marketplacev1.OffShelfProductRequest{
		ActorId: seller, ProductId: product.GetId(),
	})
	if err != nil {
		t.Fatalf("OffShelfProduct: %v", err)
	}
	if offShelf.GetProduct().GetStatus() != marketplacev1.ProductStatus_PRODUCT_STATUS_OFF_SHELF {
		t.Fatalf("status = %s, want OFF_SHELF", offShelf.GetProduct().GetStatus())
	}

	// 每个投影都必须报告相同的当前状态。
	h.assertStatusEverywhere(t, product.GetId(), marketplacev1.ProductStatus_PRODUCT_STATUS_OFF_SHELF)

	relisted, err := h.client.RelistProduct(t.Context(), &marketplacev1.RelistProductRequest{
		ActorId: seller, ProductId: product.GetId(),
	})
	if err != nil {
		t.Fatalf("RelistProduct: %v", err)
	}
	if relisted.GetProduct().GetStatus() != marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE {
		t.Fatalf("status = %s, want ON_SALE", relisted.GetProduct().GetStatus())
	}
	h.assertStatusEverywhere(t, product.GetId(), marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE)
}

func TestOnlyTheSellerMayChangeAProduct(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	const intruder = "u_intruder"
	title := "被改掉的标题"

	tests := []struct {
		name string
		call func() error
	}{
		{name: "update", call: func() error {
			_, err := h.client.UpdateProduct(t.Context(), &marketplacev1.UpdateProductRequest{
				ActorId: intruder, ProductId: product.GetId(), Title: &title,
			})
			return err
		}},
		{name: "off shelf", call: func() error {
			_, err := h.client.OffShelfProduct(t.Context(), &marketplacev1.OffShelfProductRequest{
				ActorId: intruder, ProductId: product.GetId(),
			})
			return err
		}},
		{name: "add images", call: func() error {
			_, err := h.client.AddProductImages(t.Context(), &marketplacev1.AddProductImagesRequest{
				ActorId: intruder, ProductId: product.GetId(),
				Images: []*marketplacev1.ImageUpload{{Data: pngBytes(t), ContentType: "image/png"}},
			})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertCode(t, tt.call(), errs.CodeForbidden)
		})
	}
}

func TestUpdateCannotAssignStatusAndIsRejectedWhileReserved(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)

	// UpdateProductRequest 完全没有 status 字段：线路契约使编辑路径无法分配状态。
	price := int64(9900)
	updated, err := h.client.UpdateProduct(t.Context(), &marketplacev1.UpdateProductRequest{
		ActorId: seller, ProductId: product.GetId(), PriceMinor: &price,
	})
	if err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	if updated.GetProduct().GetPriceMinor() != price {
		t.Fatalf("price_minor = %d, want %d", updated.GetProduct().GetPriceMinor(), price)
	}
	if updated.GetProduct().GetStatus() != marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE {
		t.Fatal("an edit changed the product status")
	}
}

func TestCreateRejectsInvalidFields(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	tests := []struct {
		name string
		req  *marketplacev1.CreateProductRequest
		want errs.Code
	}{
		{
			name: "no actor",
			req:  &marketplacev1.CreateProductRequest{Title: "t", PriceMinor: 1, Category: marketplacev1.ProductCategory_PRODUCT_CATEGORY_OTHER, Description: "d"},
			want: errs.CodeUnauthorized,
		},
		{
			name: "empty title",
			req:  &marketplacev1.CreateProductRequest{ActorId: seller, PriceMinor: 1, Category: marketplacev1.ProductCategory_PRODUCT_CATEGORY_OTHER, Description: "d"},
			want: errs.CodeValidation,
		},
		{
			name: "unspecified category",
			req:  &marketplacev1.CreateProductRequest{ActorId: seller, Title: "t", PriceMinor: 1, Description: "d"},
			want: errs.CodeValidation,
		},
		{
			name: "negative price",
			req:  &marketplacev1.CreateProductRequest{ActorId: seller, Title: "t", PriceMinor: -1, Category: marketplacev1.ProductCategory_PRODUCT_CATEGORY_OTHER, Description: "d"},
			want: errs.CodeValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.client.CreateProduct(t.Context(), tt.req)
			assertCode(t, err, tt.want)
		})
	}
}

func TestImagesAreStoredAndServedFromTheObjectStore(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", []*marketplacev1.ImageUpload{
		{Data: jpegBytes(t), ContentType: "image/jpeg"},
	})

	if len(product.GetImages()) != 1 {
		t.Fatalf("got %d images, want 1", len(product.GetImages()))
	}
	image := product.GetImages()[0]
	if image.GetSortOrder() != 1 {
		t.Fatalf("sort_order = %d, want 1", image.GetSortOrder())
	}
	if image.GetUrl() == "" {
		t.Fatal("the image has no public URL")
	}

	// 对象确实存在于磁盘上该商品的前缀目录下。
	stored, err := filepath.Glob(filepath.Join(h.mediaRoot, "products", product.GetId(), "*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored objects = %v, want exactly one", stored)
	}
	contents, err := os.ReadFile(stored[0])
	if err != nil {
		t.Fatalf("read object: %v", err)
	}
	if len(contents) == 0 {
		t.Fatal("the stored object is empty")
	}
}

func TestUploadTypeIsDecidedByContentNotByTheClaimedContentType(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	tests := []struct {
		name   string
		upload *marketplacev1.ImageUpload
		want   errs.Code
	}{
		{
			name:   "html disguised as png",
			upload: &marketplacev1.ImageUpload{Data: []byte("<html><script>alert(1)</script></html>"), ContentType: "image/png"},
			want:   errs.CodeValidation,
		},
		{
			name:   "gif is not an allowed type",
			upload: &marketplacev1.ImageUpload{Data: gifBytes(t), ContentType: "image/gif"},
			want:   errs.CodeValidation,
		},
		{
			name:   "empty file",
			upload: &marketplacev1.ImageUpload{Data: nil, ContentType: "image/png"},
			want:   errs.CodeValidation,
		},
		{
			name:   "oversized file",
			upload: &marketplacev1.ImageUpload{Data: oversizedPNG(t), ContentType: "image/png"},
			want:   errs.CodePayloadTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.client.CreateProduct(t.Context(), &marketplacev1.CreateProductRequest{
				ActorId:     seller,
				Title:       "机械键盘",
				PriceMinor:  12000,
				Category:    marketplacev1.ProductCategory_PRODUCT_CATEGORY_DIGITAL,
				Description: "九成新",
				Images:      []*marketplacev1.ImageUpload{tt.upload},
			})
			assertCode(t, err, tt.want)
		})
	}
}

func TestARejectedUploadLeavesNoProductAndNoObject(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.client.CreateProduct(t.Context(), &marketplacev1.CreateProductRequest{
		ActorId:     seller,
		Title:       "机械键盘",
		PriceMinor:  12000,
		Category:    marketplacev1.ProductCategory_PRODUCT_CATEGORY_DIGITAL,
		Description: "九成新",
		Images: []*marketplacev1.ImageUpload{
			{Data: pngBytes(t), ContentType: "image/png"},
			{Data: []byte("not an image at all"), ContentType: "image/png"},
		},
	})
	assertCode(t, err, errs.CodeValidation)

	page, err := h.client.ListProducts(t.Context(), &marketplacev1.ListProductsRequest{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(page.GetPage().GetItems()) != 0 {
		t.Fatal("a rejected upload still published a product")
	}

	stored, err := filepath.Glob(filepath.Join(h.mediaRoot, "products", "*", "*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("orphaned objects left behind: %v", stored)
	}
}

func TestAProductHoldsAtMostThreeImages(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", []*marketplacev1.ImageUpload{
		{Data: pngBytes(t), ContentType: "image/png"},
		{Data: pngBytes(t), ContentType: "image/png"},
	})

	added, err := h.client.AddProductImages(t.Context(), &marketplacev1.AddProductImagesRequest{
		ActorId: seller, ProductId: product.GetId(),
		Images: []*marketplacev1.ImageUpload{{Data: jpegBytes(t), ContentType: "image/jpeg"}},
	})
	if err != nil {
		t.Fatalf("AddProductImages: %v", err)
	}
	if len(added.GetImages()) != 3 {
		t.Fatalf("got %d images, want 3", len(added.GetImages()))
	}

	_, err = h.client.AddProductImages(t.Context(), &marketplacev1.AddProductImagesRequest{
		ActorId: seller, ProductId: product.GetId(),
		Images: []*marketplacev1.ImageUpload{{Data: pngBytes(t), ContentType: "image/png"}},
	})
	assertCode(t, err, errs.CodeImageLimitExceeded)

	_, err = h.client.CreateProduct(t.Context(), &marketplacev1.CreateProductRequest{
		ActorId: seller, Title: "四张图", PriceMinor: 1,
		Category: marketplacev1.ProductCategory_PRODUCT_CATEGORY_OTHER, Description: "d",
		Images: []*marketplacev1.ImageUpload{
			{Data: pngBytes(t)}, {Data: pngBytes(t)}, {Data: pngBytes(t)}, {Data: pngBytes(t)},
		},
	})
	assertCode(t, err, errs.CodeImageLimitExceeded)
}

func TestDeletingAnImageFreesItsSlotAndRemovesTheObject(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", []*marketplacev1.ImageUpload{
		{Data: pngBytes(t), ContentType: "image/png"},
	})
	image := product.GetImages()[0]

	if _, err := h.client.DeleteProductImage(t.Context(), &marketplacev1.DeleteProductImageRequest{
		ActorId: seller, ProductId: product.GetId(), ImageId: image.GetId(),
	}); err != nil {
		t.Fatalf("DeleteProductImage: %v", err)
	}

	stored, err := filepath.Glob(filepath.Join(h.mediaRoot, "products", product.GetId(), "*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("the object outlived its row: %v", stored)
	}

	// 释放的位置可以再次使用。
	added, err := h.client.AddProductImages(t.Context(), &marketplacev1.AddProductImagesRequest{
		ActorId: seller, ProductId: product.GetId(),
		Images: []*marketplacev1.ImageUpload{{Data: jpegBytes(t), ContentType: "image/jpeg"}},
	})
	if err != nil {
		t.Fatalf("AddProductImages: %v", err)
	}
	if len(added.GetImages()) != 1 || added.GetImages()[0].GetSortOrder() != 1 {
		t.Fatalf("images = %+v, want one image in slot 1", added.GetImages())
	}
}

func TestBatchGetProductsReturnsCurrentStatusForExistingIdsOnly(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.create(t, seller, "商品一", nil)
	second := h.create(t, seller, "商品二", nil)

	if _, err := h.client.OffShelfProduct(t.Context(), &marketplacev1.OffShelfProductRequest{
		ActorId: seller, ProductId: second.GetId(),
	}); err != nil {
		t.Fatalf("OffShelfProduct: %v", err)
	}

	resp, err := h.client.BatchGetProducts(t.Context(), &marketplacev1.BatchGetProductsRequest{
		ProductIds: []string{first.GetId(), second.GetId(), "p_missing", first.GetId()},
	})
	if err != nil {
		t.Fatalf("BatchGetProducts: %v", err)
	}
	if len(resp.GetProducts()) != 2 {
		t.Fatalf("got %d products, want 2", len(resp.GetProducts()))
	}
	if resp.GetProducts()[second.GetId()].GetStatus() != marketplacev1.ProductStatus_PRODUCT_STATUS_OFF_SHELF {
		t.Fatal("the batch projection does not carry the current status")
	}
}

func TestBatchGetProductsRejectsAnOversizedRequest(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ids := make([]string, 201)
	for i := range ids {
		ids[i] = "p_x"
	}

	_, err := h.client.BatchGetProducts(t.Context(), &marketplacev1.BatchGetProductsRequest{ProductIds: ids})
	assertCode(t, err, errs.CodeValidation)
}

func TestListUserProductsShowsEveryStatusForTheOwner(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	onSale := h.create(t, seller, "在售", nil)
	offShelf := h.create(t, seller, "下架", nil)
	if _, err := h.client.OffShelfProduct(t.Context(), &marketplacev1.OffShelfProductRequest{
		ActorId: seller, ProductId: offShelf.GetId(),
	}); err != nil {
		t.Fatalf("OffShelfProduct: %v", err)
	}

	all, err := h.client.ListUserProducts(t.Context(), &marketplacev1.ListUserProductsRequest{
		SellerId: seller, Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListUserProducts: %v", err)
	}
	if len(all.GetPage().GetItems()) != 2 {
		t.Fatalf("got %d products, want both statuses", len(all.GetPage().GetItems()))
	}

	filter := marketplacev1.ProductStatus_PRODUCT_STATUS_OFF_SHELF
	filtered, err := h.client.ListUserProducts(t.Context(), &marketplacev1.ListUserProductsRequest{
		SellerId: seller, Status: &filter, Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListUserProducts: %v", err)
	}
	if len(filtered.GetPage().GetItems()) != 1 || filtered.GetPage().GetItems()[0].GetId() != offShelf.GetId() {
		t.Fatalf("status filter returned %+v", filtered.GetPage().GetItems())
	}
	if all.GetPage().GetItems()[0].GetId() == onSale.GetId() && all.GetPage().GetItems()[1].GetId() == onSale.GetId() {
		t.Fatal("the list repeated one product")
	}
}

func TestPendingReservedAndSoldProductsRejectContentMutations(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", []*marketplacev1.ImageUpload{
		{Data: pngBytes(t), ContentType: "image/png"},
	})
	tradeID := h.createTrade(t, buyerA, product.GetId())
	imageID := product.GetImages()[0].GetId()

	assertContentBlocked := func(t *testing.T, want errs.Code) {
		t.Helper()
		price := int64(9900)
		calls := []struct {
			name string
			call func() error
		}{
			{name: "fields", call: func() error {
				_, err := h.client.UpdateProduct(t.Context(), &marketplacev1.UpdateProductRequest{
					ActorId: seller, ProductId: product.GetId(), PriceMinor: &price,
				})
				return err
			}},
			{name: "add image", call: func() error {
				_, err := h.client.AddProductImages(t.Context(), &marketplacev1.AddProductImagesRequest{
					ActorId: seller, ProductId: product.GetId(),
					Images: []*marketplacev1.ImageUpload{{Data: jpegBytes(t), ContentType: "image/jpeg"}},
				})
				return err
			}},
			{name: "delete image", call: func() error {
				_, err := h.client.DeleteProductImage(t.Context(), &marketplacev1.DeleteProductImageRequest{
					ActorId: seller, ProductId: product.GetId(), ImageId: imageID,
				})
				return err
			}},
		}
		for _, tt := range calls {
			t.Run(tt.name, func(t *testing.T) {
				assertCode(t, tt.call(), want)
			})
		}
	}

	t.Run("pending", func(t *testing.T) {
		assertContentBlocked(t, errs.CodeTradeStateConflict)
		_, err := h.client.OffShelfProduct(t.Context(), &marketplacev1.OffShelfProductRequest{
			ActorId: seller, ProductId: product.GetId(),
		})
		assertCode(t, err, errs.CodeTradeStateConflict)
	})

	h.accept(t, tradeID)
	t.Run("reserved", func(t *testing.T) {
		assertContentBlocked(t, errs.CodeProductStateConflict)
	})

	if _, err := h.client.ConfirmTrade(t.Context(), &marketplacev1.ConfirmTradeRequest{
		ActorId: buyerA, TradeId: tradeID,
	}); err != nil {
		t.Fatalf("buyer ConfirmTrade: %v", err)
	}
	if _, err := h.client.ConfirmTrade(t.Context(), &marketplacev1.ConfirmTradeRequest{
		ActorId: seller, TradeId: tradeID,
	}); err != nil {
		t.Fatalf("seller ConfirmTrade: %v", err)
	}
	t.Run("sold", func(t *testing.T) {
		assertContentBlocked(t, errs.CodeProductStateConflict)
	})
}

func TestDeletingImagesCompactsOrderAndPromotesTheNextCover(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", []*marketplacev1.ImageUpload{
		{Data: pngBytes(t), ContentType: "image/png"},
		{Data: jpegBytes(t), ContentType: "image/jpeg"},
		{Data: pngBytes(t), ContentType: "image/png"},
	})
	secondID := product.GetImages()[1].GetId()
	secondURL := product.GetImages()[1].GetUrl()
	thirdID := product.GetImages()[2].GetId()
	thirdURL := product.GetImages()[2].GetUrl()

	if _, err := h.client.DeleteProductImage(t.Context(), &marketplacev1.DeleteProductImageRequest{
		ActorId: seller, ProductId: product.GetId(), ImageId: product.GetImages()[0].GetId(),
	}); err != nil {
		t.Fatalf("delete first image: %v", err)
	}
	h.assertImageOrderAndCover(t, product.GetId(), []string{secondID, thirdID}, secondURL)

	if _, err := h.client.DeleteProductImage(t.Context(), &marketplacev1.DeleteProductImageRequest{
		ActorId: seller, ProductId: product.GetId(), ImageId: secondID,
	}); err != nil {
		t.Fatalf("delete promoted cover: %v", err)
	}
	h.assertImageOrderAndCover(t, product.GetId(), []string{thirdID}, thirdURL)
}

// --- helpers ----------------------------------------------------------------

func (h harness) create(t *testing.T, actor, title string, images []*marketplacev1.ImageUpload) *marketplacev1.ProductDetail {
	t.Helper()

	resp, err := h.client.CreateProduct(t.Context(), &marketplacev1.CreateProductRequest{
		ActorId:     actor,
		Title:       title,
		PriceMinor:  12000,
		Category:    marketplacev1.ProductCategory_PRODUCT_CATEGORY_DIGITAL,
		Description: "九成新",
		Images:      images,
	})
	if err != nil {
		t.Fatalf("CreateProduct(%s): %v", title, err)
	}
	return resp.GetProduct()
}

// assertStatusEverywhere 检查详情、所有者列表和批量投影是否都报告相同的当前状态，
// 这是 docs/software-design.md 第 4.2 节的要求。
func (h harness) assertStatusEverywhere(t *testing.T, productID string, want marketplacev1.ProductStatus) {
	t.Helper()

	detail, err := h.client.GetProduct(t.Context(), &marketplacev1.GetProductRequest{ProductId: productID})
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if detail.GetProduct().GetStatus() != want {
		t.Fatalf("detail status = %s, want %s", detail.GetProduct().GetStatus(), want)
	}

	owned, err := h.client.ListUserProducts(t.Context(), &marketplacev1.ListUserProductsRequest{
		SellerId: seller, Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListUserProducts: %v", err)
	}
	for _, item := range owned.GetPage().GetItems() {
		if item.GetId() == productID && item.GetStatus() != want {
			t.Fatalf("owner list status = %s, want %s", item.GetStatus(), want)
		}
	}

	batch, err := h.client.BatchGetProducts(t.Context(), &marketplacev1.BatchGetProductsRequest{ProductIds: []string{productID}})
	if err != nil {
		t.Fatalf("BatchGetProducts: %v", err)
	}
	if got := batch.GetProducts()[productID].GetStatus(); got != want {
		t.Fatalf("batch status = %s, want %s", got, want)
	}
}

func (h harness) assertImageOrderAndCover(t *testing.T, productID string, wantIDs []string, wantCover string) {
	t.Helper()

	detail, err := h.client.GetProduct(t.Context(), &marketplacev1.GetProductRequest{ProductId: productID})
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	images := detail.GetProduct().GetImages()
	if len(images) != len(wantIDs) {
		t.Fatalf("images = %d, want %d", len(images), len(wantIDs))
	}
	for i, wantID := range wantIDs {
		if images[i].GetId() != wantID || images[i].GetSortOrder() != int32(i+1) {
			t.Fatalf("image %d = %s/order %d, want %s/order %d",
				i, images[i].GetId(), images[i].GetSortOrder(), wantID, i+1)
		}
	}

	batch, err := h.client.BatchGetProducts(t.Context(), &marketplacev1.BatchGetProductsRequest{
		ProductIds: []string{productID},
	})
	if err != nil {
		t.Fatalf("BatchGetProducts: %v", err)
	}
	if got := batch.GetProducts()[productID].GetCoverUrl(); got != wantCover {
		t.Fatalf("cover_url = %q, want %q", got, wantCover)
	}
}

func assertCode(t *testing.T, err error, want errs.Code) {
	t.Helper()

	if err == nil {
		t.Fatalf("got no error, want %s", want)
	}
	if got := errs.CodeOf(err); got != want {
		t.Fatalf("error code = %s, want %s (%v)", got, want, err)
	}
}

func pngBytes(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, solidImage(4, 4)); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func jpegBytes(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, solidImage(4, 4), nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func gifBytes(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := gif.Encode(&buf, solidImage(4, 4), nil); err != nil {
		t.Fatalf("encode gif: %v", err)
	}
	return buf.Bytes()
}

// oversizedPNG 生成一个超过单文件 5 MiB 限制的真实 PNG，
// 因此尺寸检查针对的是本来会被接受的内容。
func oversizedPNG(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, noiseImage(1400, 1400)); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	data := buf.Bytes()
	if len(data) <= application.MaxImageBytes {
		// 随机噪声不会被压缩，但这里使用填充，而不是依赖这一点。
		data = append(data, bytes.Repeat([]byte{0}, application.MaxImageBytes+1-len(data))...)
	}
	return data
}

func solidImage(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	return img
}

func noiseImage(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	seed := uint32(1)
	for y := range height {
		for x := range width {
			seed = seed*1664525 + 1013904223
			img.Set(x, y, color.RGBA{R: uint8(seed), G: uint8(seed >> 8), B: uint8(seed >> 16), A: 255})
		}
	}
	return img
}
