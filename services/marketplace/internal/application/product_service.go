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

// ProductService 实现商品用例。
type ProductService struct {
	products ProductRepository
	objects  ObjectStore
	ids      IDGenerator
	clock    Clock
	logger   *slog.Logger
}

// NewProductService 组装商品用例。
func NewProductService(products ProductRepository, objects ObjectStore, ids IDGenerator, clock Clock, logger *slog.Logger) (*ProductService, error) {
	if products == nil || objects == nil || ids == nil || clock == nil {
		return nil, errors.New("application: every product dependency is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ProductService{products: products, objects: objects, ids: ids, clock: clock, logger: logger}, nil
}

// Create 发布商品，并存储随商品上传的图片。
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

	// 图片在写入数据行前先完成校验和存储，因此被拒绝的上传不会留下商品。
	// 如果商品插入随后失败，这些已写入但未被引用的对象会在这里删除。
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

// Get 返回一个商品及其图片。
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

// BatchGet 返回指定 id 中存在的商品。
//
// 这是避免收藏、会话和交易列表按行执行 RPC 的批量调用
// （docs/software-design.md 第 3.3 节），也使这些投影携带当前商品状态，而不是过期副本。
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

// List 浏览公开商品目录。
//
// 公开列表只返回 ON_SALE 商品。是否同时显示 RESERVED 商品是一个尚未决定的产品问题
// （docs/software-design.md 第 11.3 节）；在契约变更前，筛选条件固定在这里，
// 而不是由调用方传入。
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

// ListBySeller 按任意状态列出某个卖家的商品。
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

// Update 编辑商品的描述字段。
func (s *ProductService) Update(ctx context.Context, cmd UpdateProductCommand) (*domain.Product, error) {
	return s.mutate(ctx, cmd.ProductID, func(product *domain.Product, hasPending bool) error {
		if err := mutableProduct(product, cmd.ActorID, hasPending); err != nil {
			return err
		}
		if err := product.Edit(cmd.ActorID, cmd.Title, cmd.PriceMinor, cmd.Category, cmd.Description, s.clock.Now()); err != nil {
			return transitionError(err)
		}
		return nil
	})
}

// OffShelf 执行卖家的 ON_SALE -> OFF_SHELF 操作。
func (s *ProductService) OffShelf(ctx context.Context, cmd ProductActionCommand) (*domain.Product, error) {
	return s.mutate(ctx, cmd.ProductID, func(product *domain.Product, hasPending bool) error {
		if !product.IsSeller(cmd.ActorID) {
			return transitionError(domain.ErrNotSeller)
		}
		if product.Status != domain.StatusOnSale {
			return errs.New(errs.CodeProductStateConflict, "当前商品状态不允许下架")
		}
		if hasPending {
			return errs.New(errs.CodeTradeStateConflict, "商品存在待处理购买意向，请先逐笔拒绝")
		}
		if err := product.OffShelf(cmd.ActorID, s.clock.Now()); err != nil {
			return transitionError(err)
		}
		return nil
	})
}

// Relist 执行卖家的 OFF_SHELF -> ON_SALE 操作。
func (s *ProductService) Relist(ctx context.Context, cmd ProductActionCommand) (*domain.Product, error) {
	return s.mutate(ctx, cmd.ProductID, func(product *domain.Product, _ bool) error {
		if err := product.Relist(cmd.ActorID, s.clock.Now()); err != nil {
			return transitionError(err)
		}
		return nil
	})
}

// AddImages 向现有商品追加图片。
func (s *ProductService) AddImages(ctx context.Context, cmd AddImagesCommand) ([]domain.Image, error) {
	if len(cmd.Images) == 0 {
		return nil, errs.New(errs.CodeValidation, "请至少上传一张图片")
	}
	if len(cmd.Images) > domain.MaxImages {
		return nil, errs.Newf(errs.CodeImageLimitExceeded, "商品最多保留 %d 张图片", domain.MaxImages)
	}

	now := s.clock.Now()
	stored, err := s.storeImages(ctx, cmd.ProductID, 1, cmd.Images, now)
	if err != nil {
		return nil, err
	}

	product, err := s.mutate(ctx, cmd.ProductID, func(product *domain.Product, hasPending bool) error {
		if err := mutableProduct(product, cmd.ActorID, hasPending); err != nil {
			return err
		}
		if err := product.AppendImages(cmd.ActorID, stored, now); err != nil {
			return transitionError(err)
		}
		return nil
	})
	if err != nil {
		s.discardObjects(ctx, stored)
		return nil, err
	}
	return product.Images, nil
}

// DeleteImage 从商品中删除一张图片。
func (s *ProductService) DeleteImage(ctx context.Context, cmd DeleteImageCommand) error {
	var key string
	_, err := s.mutate(ctx, cmd.ProductID, func(product *domain.Product, hasPending bool) error {
		if err := mutableProduct(product, cmd.ActorID, hasPending); err != nil {
			return err
		}
		removed, err := product.RemoveImage(cmd.ActorID, cmd.ImageID, s.clock.Now())
		if err != nil {
			if !errors.Is(err, domain.ErrImageNotFound) {
				return transitionError(err)
			}
			return err
		}
		key = removed
		return nil
	})
	if err != nil {
		return err
	}

	// 数据行已经删除，因此图片不再属于该商品。对象删除失败会留下可由清理流程重试的
	// 孤立对象；不能因为它而将已完成的删除变成卖家看到的错误。
	if err := s.objects.Delete(ctx, key); err != nil {
		observability.LoggerWith(ctx, s.logger).Warn("orphaned image object",
			slog.String("object_key", key),
			slog.String("error", err.Error()),
		)
	}
	return nil
}

// ImageURL 返回图片在对象存储中的公开位置。
func (s *ProductService) ImageURL(objectKey string) string {
	if objectKey == "" {
		return ""
	}
	return s.objects.URL(objectKey)
}

// mutate serializes every product content/state mutation with purchase-intent
// creation by taking the same Product row lock in PostgreSQL.
func (s *ProductService) mutate(ctx context.Context, productID string, apply ProductMutation) (*domain.Product, error) {
	product, err := s.products.Mutate(ctx, productID, apply)
	if err == nil {
		return product, nil
	}
	if errs.CodeOf(err) != errs.CodeInternal {
		return nil, err
	}
	if errors.Is(err, ErrNotFound) {
		return nil, errs.Wrap(errs.CodeResourceNotFound, "商品不存在", err)
	}
	if errors.Is(err, domain.ErrImageNotFound) {
		return nil, errs.Wrap(errs.CodeResourceNotFound, "图片不存在", err)
	}
	if errors.Is(err, ErrVersionConflict) {
		return nil, errs.Wrap(errs.CodeProductStateConflict, "商品状态已变化，请重试", err)
	}
	return nil, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
}

// mutableProduct enforces the latest contract in privacy-safe order: ownership,
// terminal product state, then the presence of a PENDING purchase intent.
func mutableProduct(product *domain.Product, actorID string, hasPending bool) error {
	if err := product.RequireContentMutation(actorID); err != nil {
		return transitionError(err)
	}
	if hasPending {
		return errs.New(errs.CodeTradeStateConflict, "商品存在待处理购买意向，暂不能修改")
	}
	return nil
}

// storeImages 校验并写入上传内容，返回待插入的图片行。
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

// discardObjects 删除永远不会被数据行引用的对象。
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

// readError 将仓储读取失败映射为契约错误。
func (s *ProductService) readError(err error, notFoundMessage string) error {
	if errors.Is(err, ErrNotFound) {
		return errs.Wrap(errs.CodeResourceNotFound, notFoundMessage, err)
	}
	return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
}

// writeError 将仓储写入失败映射为契约错误。
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

// validationError 将领域字段错误映射为 VALIDATION_ERROR。
func validationError(err error) error {
	return errs.Wrap(errs.CodeValidation, err.Error(), err)
}

// transitionError 将领域状态转换失败映射为描述该失败的契约错误码：
// 谁可以执行操作，或当前状态允许什么操作。
func transitionError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotSeller),
		errors.Is(err, domain.ErrNotTradeSeller),
		errors.Is(err, domain.ErrNotTradeBuyer),
		errors.Is(err, domain.ErrNotTradeParty):
		return errs.Wrap(errs.CodeForbidden, "无权执行该操作", err)
	case errors.Is(err, domain.ErrTradeNotPending),
		errors.Is(err, domain.ErrTradeNotAccepted):
		return errs.Wrap(errs.CodeTradeStateConflict, "当前交易状态不允许执行该操作", err)
	case errors.Is(err, domain.ErrNotOnSale),
		errors.Is(err, domain.ErrNotReserved):
		return errs.Wrap(errs.CodeProductNotAvailable, "商品当前不可交易", err)
	case errors.Is(err, domain.ErrNotEditable),
		errors.Is(err, domain.ErrNotOffShelf):
		return errs.Wrap(errs.CodeProductStateConflict, "当前商品状态不允许执行该操作", err)
	case errors.Is(err, domain.ErrImageLimitExceeded):
		return errs.Wrap(errs.CodeImageLimitExceeded, "商品图片数量已达上限", err)
	case errors.Is(err, domain.ErrCancelReasonLength):
		return errs.Wrap(errs.CodeValidation, "原因长度必须为 1 至 200 个字符", err)
	default:
		return validationError(err)
	}
}

// dedupe 删除空标识和重复标识，同时保留原有顺序。
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
