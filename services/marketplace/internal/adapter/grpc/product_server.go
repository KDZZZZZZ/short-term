// Package grpc exposes the Marketplace use cases over the internal gRPC
// contract in proto/shortterm/marketplace/v1.
package grpc

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// maxBatchProducts bounds one BatchGetProducts call. Favorite, conversation
// and trade lists complete one page at a time, and the largest public page is
// 100 rows.
const maxBatchProducts = 200

// Server adapts the Marketplace application services to the generated service
// interface.
type Server struct {
	products *application.ProductService
}

// NewServer builds the gRPC adapter.
func NewServer(products *application.ProductService) *Server {
	return &Server{products: products}
}

// CreateProduct publishes a listing.
func (s *Server) CreateProduct(ctx context.Context, req *marketplacev1.CreateProductRequest) (*marketplacev1.CreateProductResponse, error) {
	product, err := s.products.Create(ctx, application.CreateProductCommand{
		ActorID:     req.GetActorId(),
		Title:       req.GetTitle(),
		PriceMinor:  req.GetPriceMinor(),
		Category:    category(req.GetCategory()),
		Description: req.GetDescription(),
		Images:      uploads(req.GetImages()),
	})
	if err != nil {
		return nil, err
	}
	return &marketplacev1.CreateProductResponse{Product: s.productDetail(product)}, nil
}

// GetProduct returns one listing with its images.
func (s *Server) GetProduct(ctx context.Context, req *marketplacev1.GetProductRequest) (*marketplacev1.GetProductResponse, error) {
	product, err := s.products.Get(ctx, req.GetProductId())
	if err != nil {
		return nil, err
	}
	return &marketplacev1.GetProductResponse{Product: s.productDetail(product)}, nil
}

// ListProducts browses the public catalogue.
func (s *Server) ListProducts(ctx context.Context, req *marketplacev1.ListProductsRequest) (*marketplacev1.ListProductsResponse, error) {
	query := application.ListProductsQuery{
		Keyword: req.Keyword,
		Page:    application.Page{Number: req.GetPage(), Size: req.GetPageSize()},
	}
	if req.Category != nil {
		value := category(req.GetCategory())
		query.Category = &value
	}

	page, err := s.products.List(ctx, query)
	if err != nil {
		return nil, err
	}
	return &marketplacev1.ListProductsResponse{Page: s.productPage(page)}, nil
}

// ListUserProducts lists one seller's own products in any status.
func (s *Server) ListUserProducts(ctx context.Context, req *marketplacev1.ListUserProductsRequest) (*marketplacev1.ListUserProductsResponse, error) {
	query := application.ListUserProductsQuery{
		SellerID: req.GetSellerId(),
		Page:     application.Page{Number: req.GetPage(), Size: req.GetPageSize()},
	}
	if req.Status != nil {
		value := domainStatus(req.GetStatus())
		query.Status = &value
	}

	page, err := s.products.ListBySeller(ctx, query)
	if err != nil {
		return nil, err
	}
	return &marketplacev1.ListUserProductsResponse{Page: s.productPage(page)}, nil
}

// BatchGetProducts returns the listings that exist among the requested ids.
func (s *Server) BatchGetProducts(ctx context.Context, req *marketplacev1.BatchGetProductsRequest) (*marketplacev1.BatchGetProductsResponse, error) {
	if len(req.GetProductIds()) > maxBatchProducts {
		return nil, errs.Newf(errs.CodeValidation, "单次最多查询 %d 个商品", maxBatchProducts)
	}

	products, err := s.products.BatchGet(ctx, req.GetProductIds())
	if err != nil {
		return nil, err
	}

	summaries := make(map[string]*marketplacev1.ProductSummary, len(products))
	for id, product := range products {
		summaries[id] = s.productSummary(product)
	}
	return &marketplacev1.BatchGetProductsResponse{Products: summaries}, nil
}

// UpdateProduct edits a listing's descriptive fields.
func (s *Server) UpdateProduct(ctx context.Context, req *marketplacev1.UpdateProductRequest) (*marketplacev1.UpdateProductResponse, error) {
	cmd := application.UpdateProductCommand{
		ActorID:     req.GetActorId(),
		ProductID:   req.GetProductId(),
		Title:       req.Title,
		PriceMinor:  req.PriceMinor,
		Description: req.Description,
	}
	if req.Category != nil {
		value := category(req.GetCategory())
		cmd.Category = &value
	}

	product, err := s.products.Update(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return &marketplacev1.UpdateProductResponse{Product: s.productDetail(product)}, nil
}

// AddProductImages appends images to a listing.
func (s *Server) AddProductImages(ctx context.Context, req *marketplacev1.AddProductImagesRequest) (*marketplacev1.AddProductImagesResponse, error) {
	images, err := s.products.AddImages(ctx, application.AddImagesCommand{
		ActorID:   req.GetActorId(),
		ProductID: req.GetProductId(),
		Images:    uploads(req.GetImages()),
	})
	if err != nil {
		return nil, err
	}
	return &marketplacev1.AddProductImagesResponse{Images: s.images(images)}, nil
}

// DeleteProductImage removes one image from a listing.
func (s *Server) DeleteProductImage(ctx context.Context, req *marketplacev1.DeleteProductImageRequest) (*marketplacev1.DeleteProductImageResponse, error) {
	err := s.products.DeleteImage(ctx, application.DeleteImageCommand{
		ActorID:   req.GetActorId(),
		ProductID: req.GetProductId(),
		ImageID:   req.GetImageId(),
	})
	if err != nil {
		return nil, err
	}
	return &marketplacev1.DeleteProductImageResponse{}, nil
}

// OffShelfProduct performs the seller's ON_SALE -> OFF_SHELF action.
func (s *Server) OffShelfProduct(ctx context.Context, req *marketplacev1.OffShelfProductRequest) (*marketplacev1.OffShelfProductResponse, error) {
	product, err := s.products.OffShelf(ctx, application.ProductActionCommand{
		ActorID:   req.GetActorId(),
		ProductID: req.GetProductId(),
	})
	if err != nil {
		return nil, err
	}
	return &marketplacev1.OffShelfProductResponse{Product: s.productDetail(product)}, nil
}

// RelistProduct performs the seller's OFF_SHELF -> ON_SALE action.
func (s *Server) RelistProduct(ctx context.Context, req *marketplacev1.RelistProductRequest) (*marketplacev1.RelistProductResponse, error) {
	product, err := s.products.Relist(ctx, application.ProductActionCommand{
		ActorID:   req.GetActorId(),
		ProductID: req.GetProductId(),
	})
	if err != nil {
		return nil, err
	}
	return &marketplacev1.RelistProductResponse{Product: s.productDetail(product)}, nil
}

// --- mapping ----------------------------------------------------------------

func (s *Server) productDetail(product *domain.Product) *marketplacev1.ProductDetail {
	return &marketplacev1.ProductDetail{
		Id:          product.ID,
		Title:       product.Title,
		PriceMinor:  product.PriceMinor,
		Category:    categoryProto(product.Category),
		Description: product.Description,
		Status:      statusProto(product.Status),
		Images:      s.images(product.Images),
		SellerId:    product.SellerID,
		CreatedAt:   timestamppb.New(product.CreatedAt),
		UpdatedAt:   timestamppb.New(product.UpdatedAt),
	}
}

func (s *Server) productSummary(product *domain.Product) *marketplacev1.ProductSummary {
	summary := &marketplacev1.ProductSummary{
		Id:         product.ID,
		Title:      product.Title,
		PriceMinor: product.PriceMinor,
		Category:   categoryProto(product.Category),
		Status:     statusProto(product.Status),
		SellerId:   product.SellerID,
		CreatedAt:  timestamppb.New(product.CreatedAt),
	}
	if key := product.CoverObjectKey(); key != "" {
		url := s.products.ImageURL(key)
		summary.CoverUrl = &url
	}
	return summary
}

func (s *Server) productPage(page application.ProductPage) *marketplacev1.ProductPage {
	items := make([]*marketplacev1.ProductSummary, 0, len(page.Items))
	for _, product := range page.Items {
		items = append(items, s.productSummary(product))
	}
	return &marketplacev1.ProductPage{
		Items:    items,
		Page:     page.Page,
		PageSize: page.Size,
		Total:    page.Total,
	}
}

func (s *Server) images(images []domain.Image) []*marketplacev1.ProductImage {
	mapped := make([]*marketplacev1.ProductImage, 0, len(images))
	for _, image := range images {
		mapped = append(mapped, &marketplacev1.ProductImage{
			Id:        image.ID,
			Url:       s.products.ImageURL(image.ObjectKey),
			SortOrder: int32(image.SortOrder),
			CreatedAt: timestamppb.New(image.CreatedAt),
		})
	}
	return mapped
}

func uploads(src []*marketplacev1.ImageUpload) []application.ImageUpload {
	mapped := make([]application.ImageUpload, 0, len(src))
	for _, upload := range src {
		mapped = append(mapped, application.ImageUpload{
			Data:        upload.GetData(),
			ContentType: upload.GetContentType(),
		})
	}
	return mapped
}

// category maps the wire enum to the domain value. UNSPECIFIED maps to an
// empty category, which domain validation rejects: an unset enum is a client
// error, not a silent default.
func category(value marketplacev1.ProductCategory) domain.Category {
	switch value {
	case marketplacev1.ProductCategory_PRODUCT_CATEGORY_TEXTBOOK:
		return domain.CategoryTextbook
	case marketplacev1.ProductCategory_PRODUCT_CATEGORY_DIGITAL:
		return domain.CategoryDigital
	case marketplacev1.ProductCategory_PRODUCT_CATEGORY_LIFE:
		return domain.CategoryLife
	case marketplacev1.ProductCategory_PRODUCT_CATEGORY_OTHER:
		return domain.CategoryOther
	default:
		return ""
	}
}

func categoryProto(value domain.Category) marketplacev1.ProductCategory {
	switch value {
	case domain.CategoryTextbook:
		return marketplacev1.ProductCategory_PRODUCT_CATEGORY_TEXTBOOK
	case domain.CategoryDigital:
		return marketplacev1.ProductCategory_PRODUCT_CATEGORY_DIGITAL
	case domain.CategoryLife:
		return marketplacev1.ProductCategory_PRODUCT_CATEGORY_LIFE
	case domain.CategoryOther:
		return marketplacev1.ProductCategory_PRODUCT_CATEGORY_OTHER
	default:
		return marketplacev1.ProductCategory_PRODUCT_CATEGORY_UNSPECIFIED
	}
}

func domainStatus(value marketplacev1.ProductStatus) domain.Status {
	switch value {
	case marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE:
		return domain.StatusOnSale
	case marketplacev1.ProductStatus_PRODUCT_STATUS_RESERVED:
		return domain.StatusReserved
	case marketplacev1.ProductStatus_PRODUCT_STATUS_SOLD:
		return domain.StatusSold
	case marketplacev1.ProductStatus_PRODUCT_STATUS_OFF_SHELF:
		return domain.StatusOffShelf
	default:
		return ""
	}
}

func statusProto(value domain.Status) marketplacev1.ProductStatus {
	switch value {
	case domain.StatusOnSale:
		return marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE
	case domain.StatusReserved:
		return marketplacev1.ProductStatus_PRODUCT_STATUS_RESERVED
	case domain.StatusSold:
		return marketplacev1.ProductStatus_PRODUCT_STATUS_SOLD
	case domain.StatusOffShelf:
		return marketplacev1.ProductStatus_PRODUCT_STATUS_OFF_SHELF
	default:
		return marketplacev1.ProductStatus_PRODUCT_STATUS_UNSPECIFIED
	}
}
