package application

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/observability"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// ProductService implements the product use cases.
type ProductService struct {
	products ProductRepository
	objects  ObjectStore
	ids      IDGenerator
	clock    Clock
	logger   *slog.Logger
}

// NewProductService wires the product use cases.
func NewProductService(products ProductRepository, objects ObjectStore, ids IDGenerator, clock Clock, logger *slog.Logger) (*ProductService, error) {
	if products == nil || objects == nil || ids == nil || clock == nil {
		return nil, errors.New("application: every product dependency is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ProductService{products: products, objects: objects, ids: ids, clock: clock, logger: logger}, nil
}

// Create publishes a listing, storing any images that came with it.
func (s *ProductService) Create(ctx context.Context, cmd CreateProductCommand) (*domain.Product, error) {
	if cmd.ActorID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "请先登录")
	}
	if len(cmd.Images) > domain.MaxImages {
		return nil, errs.Newf(errs.CodeImageLimitExceeded, "最多上传 %d 张图片", domain.MaxImages)
	}

	now := s.clock.Now()
	product, err := domain.NewProduct(s.ids.NewProductID(), cmd.ActorID, cmd.Title, cmd.PriceMinor, cmd.Category, cmd.Description, now)
	if err != nil {
		return nil, validationError(err)
	}

	// Images are validated and stored before the row is written, so a rejected
	// upload leaves no product behind. Objects written for a product whose
	// insert then fails are unreferenced and are removed here.
	images, err := s.storeImages(ctx, product.ID, 1, cmd.Images, now)
	if err != nil {
		return nil, err
	}
	product.Images = images

	if err := s.products.Create(ctx, product); err != nil {
		s.discardObjects(ctx, images)
		return nil, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return product, nil
}

// Get returns one product with its images.
func (s *ProductService) Get(ctx context.Context, productID string) (*domain.Product, error) {
	if productID == "" {
		return nil, errs.New(errs.CodeValidation, "商品标识不能为空")
	}

	product, err := s.products.ByID(ctx, productID)
	if err != nil {
		return nil, s.readError(err, "商品不存在")
	}
	return product, nil
}

// BatchGet returns the products that exist among ids.
//
// It is the batch call that keeps favorite, conversation and trade lists from
// issuing one RPC per row (docs/software-design.md section 3.3), and it is
// what makes those projections carry the current product status rather than a
// stale copy.
func (s *ProductService) BatchGet(ctx context.Context, ids []string) (map[string]*domain.Product, error) {
	unique := dedupe(ids)
	if len(unique) == 0 {
		return map[string]*domain.Product{}, nil
	}

	products, err := s.products.ByIDs(ctx, unique)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}

	byID := make(map[string]*domain.Product, len(products))
	for _, product := range products {
		byID[product.ID] = product
	}
	return byID, nil
}

// List browses the public catalogue.
//
// The public list returns ON_SALE products only. Whether it should also show
// RESERVED products is an open product question
// (docs/software-design.md section 11.3); until the contract changes, this
// filter is fixed here rather than taken from the caller.
func (s *ProductService) List(ctx context.Context, query ListProductsQuery) (ProductPage, error) {
	onSale := domain.StatusOnSale
	filter := ProductFilter{Status: &onSale, Category: query.Category}

	if query.Keyword != nil {
		keyword := strings.TrimSpace(*query.Keyword)
		if keyword == "" {
			return ProductPage{}, errs.New(errs.CodeValidation, "搜索关键词不能为空")
		}
		filter.Keyword = &keyword
	}
	if filter.Category != nil && !filter.Category.Valid() {
		return ProductPage{}, errs.New(errs.CodeValidation, "商品分类不合法")
	}

	page, err := s.products.List(ctx, filter, query.Page.normalize())
	if err != nil {
		return ProductPage{}, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return page, nil
}

// ListBySeller lists one seller's own products in any status.
func (s *ProductService) ListBySeller(ctx context.Context, query ListUserProductsQuery) (ProductPage, error) {
	if query.SellerID == "" {
		return ProductPage{}, errs.New(errs.CodeUnauthorized, "请先登录")
	}
	if query.Status != nil && !query.Status.Valid() {
		return ProductPage{}, errs.New(errs.CodeValidation, "商品状态不合法")
	}

	page, err := s.products.List(ctx, ProductFilter{SellerID: query.SellerID, Status: query.Status}, query.Page.normalize())
	if err != nil {
		return ProductPage{}, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return page, nil
}

// Update edits a listing's descriptive fields.
func (s *ProductService) Update(ctx context.Context, cmd UpdateProductCommand) (*domain.Product, error) {
	product, err := s.Get(ctx, cmd.ProductID)
	if err != nil {
		return nil, err
	}
	expected := product.Version

	if err := product.Edit(cmd.ActorID, cmd.Title, cmd.PriceMinor, cmd.Category, cmd.Description, s.clock.Now()); err != nil {
		return nil, transitionError(err)
	}
	if err := s.products.Update(ctx, product, expected); err != nil {
		return nil, s.writeError(err)
	}
	return product, nil
}

// OffShelf performs the seller's ON_SALE -> OFF_SHELF action.
func (s *ProductService) OffShelf(ctx context.Context, cmd ProductActionCommand) (*domain.Product, error) {
	return s.transition(ctx, cmd, func(product *domain.Product) error {
		return product.OffShelf(cmd.ActorID, s.clock.Now())
	})
}

// Relist performs the seller's OFF_SHELF -> ON_SALE action.
func (s *ProductService) Relist(ctx context.Context, cmd ProductActionCommand) (*domain.Product, error) {
	return s.transition(ctx, cmd, func(product *domain.Product) error {
		return product.Relist(cmd.ActorID, s.clock.Now())
	})
}

// AddImages appends images to an existing listing.
func (s *ProductService) AddImages(ctx context.Context, cmd AddImagesCommand) ([]domain.Image, error) {
	if len(cmd.Images) == 0 {
		return nil, errs.New(errs.CodeValidation, "请至少上传一张图片")
	}

	product, err := s.Get(ctx, cmd.ProductID)
	if err != nil {
		return nil, err
	}
	if !product.IsSeller(cmd.ActorID) {
		return nil, errs.New(errs.CodeForbidden, "无权修改该商品")
	}
	if len(product.Images)+len(cmd.Images) > domain.MaxImages {
		return nil, errs.Newf(errs.CodeImageLimitExceeded, "商品最多保留 %d 张图片", domain.MaxImages)
	}

	slot, err := product.NextImageSlot()
	if err != nil {
		return nil, errs.Wrap(errs.CodeImageLimitExceeded, "商品图片数量已达上限", err)
	}

	stored, err := s.storeImages(ctx, product.ID, slot, cmd.Images, s.clock.Now())
	if err != nil {
		return nil, err
	}

	for i := range stored {
		if err := s.products.AddImage(ctx, &stored[i]); err != nil {
			// Roll back the objects written by this call. Rows already
			// inserted stay: they are valid images of the same product, and
			// the response returns the product's full current image set.
			s.discardObjects(ctx, stored[i:])
			if errors.Is(err, ErrImageSlotTaken) {
				return nil, errs.Wrap(errs.CodeImageLimitExceeded, "商品图片数量已达上限", err)
			}
			if errors.Is(err, ErrNotFound) {
				return nil, errs.Wrap(errs.CodeResourceNotFound, "商品不存在", err)
			}
			return nil, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
		}
	}

	reloaded, err := s.Get(ctx, cmd.ProductID)
	if err != nil {
		return nil, err
	}
	return reloaded.Images, nil
}

// DeleteImage removes one image from a listing.
func (s *ProductService) DeleteImage(ctx context.Context, cmd DeleteImageCommand) error {
	product, err := s.Get(ctx, cmd.ProductID)
	if err != nil {
		return err
	}
	if !product.IsSeller(cmd.ActorID) {
		return errs.New(errs.CodeForbidden, "无权修改该商品")
	}

	key, err := s.products.DeleteImage(ctx, cmd.ProductID, cmd.ImageID)
	if err != nil {
		return s.readError(err, "图片不存在")
	}

	// The row is gone, so the image is no longer part of the product. A failed
	// object delete leaves an orphan the cleanup path can retry; it must not
	// turn a completed deletion into an error for the seller.
	if err := s.objects.Delete(ctx, key); err != nil {
		observability.LoggerWith(ctx, s.logger).Warn("orphaned image object",
			slog.String("object_key", key),
			slog.String("error", err.Error()),
		)
	}
	return nil
}

// ImageURL exposes the object store's public location for an image.
func (s *ProductService) ImageURL(objectKey string) string {
	if objectKey == "" {
		return ""
	}
	return s.objects.URL(objectKey)
}

// transition applies a seller action that only changes status.
func (s *ProductService) transition(ctx context.Context, cmd ProductActionCommand, apply func(*domain.Product) error) (*domain.Product, error) {
	product, err := s.Get(ctx, cmd.ProductID)
	if err != nil {
		return nil, err
	}
	expected := product.Version

	if err := apply(product); err != nil {
		return nil, transitionError(err)
	}
	if err := s.products.Update(ctx, product, expected); err != nil {
		return nil, s.writeError(err)
	}
	return product, nil
}

// storeImages validates and writes uploads, returning the image rows to insert.
func (s *ProductService) storeImages(ctx context.Context, productID string, firstSlot int, uploads []ImageUpload, now time.Time) ([]domain.Image, error) {
	images := make([]domain.Image, 0, len(uploads))

	for i, upload := range uploads {
		slot := firstSlot + i
		if slot > domain.MaxImages {
			s.discardObjects(ctx, images)
			return nil, errs.Newf(errs.CodeImageLimitExceeded, "商品最多保留 %d 张图片", domain.MaxImages)
		}

		contentType, extension, err := detectImageType(upload)
		if err != nil {
			s.discardObjects(ctx, images)
			return nil, err
		}

		imageID := s.ids.NewImageID()
		key := objectKey(productID, imageID, extension)
		if err := s.objects.Put(ctx, key, contentType, upload.Data); err != nil {
			s.discardObjects(ctx, images)
			return nil, errs.Wrap(errs.CodeInternal, "图片保存失败", err)
		}

		images = append(images, domain.Image{
			ID:        imageID,
			ProductID: productID,
			ObjectKey: key,
			SortOrder: slot,
			CreatedAt: now,
		})
	}
	return images, nil
}

// discardObjects removes objects that will never be referenced by a row.
func (s *ProductService) discardObjects(ctx context.Context, images []domain.Image) {
	for _, image := range images {
		if err := s.objects.Delete(context.WithoutCancel(ctx), image.ObjectKey); err != nil {
			observability.LoggerWith(ctx, s.logger).Warn("orphaned image object",
				slog.String("object_key", image.ObjectKey),
				slog.String("error", err.Error()),
			)
		}
	}
}

// readError maps a repository read failure to a contract error.
func (s *ProductService) readError(err error, notFoundMessage string) error {
	if errors.Is(err, ErrNotFound) {
		return errs.Wrap(errs.CodeResourceNotFound, notFoundMessage, err)
	}
	return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
}

// writeError maps a repository write failure to a contract error.
func (s *ProductService) writeError(err error) error {
	switch {
	case errors.Is(err, ErrVersionConflict):
		return errs.Wrap(errs.CodeProductStateConflict, "商品状态已变化，请重试", err)
	case errors.Is(err, ErrNotFound):
		return errs.Wrap(errs.CodeResourceNotFound, "商品不存在", err)
	default:
		return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
}

// validationError maps a domain field error to VALIDATION_ERROR.
func validationError(err error) error {
	return errs.Wrap(errs.CodeValidation, err.Error(), err)
}

// transitionError maps a domain transition failure to the contract code that
// describes it: a permission problem, or a state that forbids the action.
func transitionError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotSeller):
		return errs.Wrap(errs.CodeForbidden, "无权执行该操作", err)
	case errors.Is(err, domain.ErrNotEditable),
		errors.Is(err, domain.ErrNotOnSale),
		errors.Is(err, domain.ErrNotOffShelf),
		errors.Is(err, domain.ErrNotReserved):
		return errs.Wrap(errs.CodeProductStateConflict, "当前商品状态不允许执行该操作", err)
	case errors.Is(err, domain.ErrImageLimitExceeded):
		return errs.Wrap(errs.CodeImageLimitExceeded, "商品图片数量已达上限", err)
	default:
		return validationError(err)
	}
}

// dedupe removes empty and repeated identifiers while preserving order.
func dedupe(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	unique := make([]string, 0, len(ids))
	for _, value := range ids {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}
