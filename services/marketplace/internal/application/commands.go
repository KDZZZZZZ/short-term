package application

import "github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"

// 分页限制，与 openapi/components/parameters.yaml 一致。
const (
	// DefaultPageSize 是 page_size 默认值。
	DefaultPageSize int32 = 20
	// MaxPageSize 是 page_size 最大值。
	MaxPageSize int32 = 100
)

// CreateProductCommand 发布商品。
type CreateProductCommand struct {
	ActorID     string
	Title       string
	PriceMinor  int64
	Category    domain.Category
	Description string
	Images      []ImageUpload
}

// UpdateProductCommand 编辑商品。nil 字段保持不变。
type UpdateProductCommand struct {
	ActorID     string
	ProductID   string
	Title       *string
	PriceMinor  *int64
	Category    *domain.Category
	Description *string
}

// ListProductsQuery 浏览公开商品目录。
type ListProductsQuery struct {
	Keyword  *string
	Category *domain.Category
	Page     Page
}

// ListUserProductsQuery 列出某个卖家的商品。
type ListUserProductsQuery struct {
	SellerID string
	Status   *domain.Status
	Page     Page
}

// AddImagesCommand 向商品追加图片。
type AddImagesCommand struct {
	ActorID   string
	ProductID string
	Images    []ImageUpload
}

// DeleteImageCommand 从商品中删除一张图片。
type DeleteImageCommand struct {
	ActorID   string
	ProductID string
	ImageID   string
}

// ProductActionCommand 是只需要商品信息的卖家操作。
type ProductActionCommand struct {
	ActorID   string
	ProductID string
}

// normalize 将分页请求限制在公开契约允许的范围内，使省略参数的调用方仍能得到
// 文档规定的默认值。
func (p Page) normalize() Page {
	if p.Number < 1 {
		p.Number = 1
	}
	if p.Size < 1 {
		p.Size = DefaultPageSize
	}
	if p.Size > MaxPageSize {
		p.Size = MaxPageSize
	}
	return p
}

// Offset 是该页需要跳过的行数。
func (p Page) Offset() int32 { return (p.Number - 1) * p.Size }
