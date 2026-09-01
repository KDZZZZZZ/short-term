// Package domain 保存 Marketplace 实体及其规则。商品状态只能通过此处声明的转换改变，
// 这是 docs/state-machines.md 的要求：不能通过通用 setter 或 PATCH 端点直接赋值状态。
package domain

import (
	"errors"
	"time"
	"unicode/utf8"
)

// Status 是商品生命周期状态。
type Status string

// docs/state-machines.md 中定义的商品状态。
const (
	StatusOnSale   Status = "ON_SALE"
	StatusReserved Status = "RESERVED"
	StatusSold     Status = "SOLD"
	StatusOffShelf Status = "OFF_SHELF"
)

// Valid 判断 s 是否为已知状态。
func (s Status) Valid() bool {
	switch s {
	case StatusOnSale, StatusReserved, StatusSold, StatusOffShelf:
		return true
	default:
		return false
	}
}

// Category 是商品分类。
type Category string

// 公开 ProductCategory schema 声明的分类。
const (
	CategoryTextbook Category = "TEXTBOOK"
	CategoryDigital  Category = "DIGITAL"
	CategoryLife     Category = "LIFE"
	CategoryOther    Category = "OTHER"
)

// Valid 判断 c 是否为已知分类。
func (c Category) Valid() bool {
	switch c {
	case CategoryTextbook, CategoryDigital, CategoryLife, CategoryOther:
		return true
	default:
		return false
	}
}

// 字段限制，与 openapi/components/schemas.yaml 保持一致。
const (
	// MaxTitleLength 是 ProductSummary/ProductDetail 的标题长度限制。
	MaxTitleLength = 100
	// MaxDescriptionLength 是 ProductDetail 的描述长度限制。
	MaxDescriptionLength = 2000
	// MaxPriceMinor 是 Price 模式可以表达的最大金额：
	// 以最小货币单位表示为 99999999.99。
	MaxPriceMinor = 9999999999
	// MaxImages 是商品可以携带的图片数量。
	MaxImages = 3
)

// 校验和状态转换错误。
var (
	ErrIDRequired         = errors.New("product id is required")
	ErrSellerRequired     = errors.New("seller id is required")
	ErrTitleLength        = errors.New("title must be 1-100 characters")
	ErrDescriptionLength  = errors.New("description must be 1-2000 characters")
	ErrPriceRange         = errors.New("price must be between 0 and 99999999.99")
	ErrCategoryUnknown    = errors.New("category is not one of TEXTBOOK, DIGITAL, LIFE, OTHER")
	ErrStatusUnknown      = errors.New("status is not a known product status")
	ErrNotSeller          = errors.New("only the seller may perform this action")
	ErrNotEditable        = errors.New("a product may only be edited while it is on sale or off shelf")
	ErrNotOnSale          = errors.New("the product is not on sale")
	ErrNotOffShelf        = errors.New("the product is not off shelf")
	ErrNotReserved        = errors.New("the product is not reserved")
	ErrImageLimitExceeded = errors.New("a product may hold at most three images")
	ErrImageSlotTaken     = errors.New("the image slot is already used")
	ErrImageNotFound      = errors.New("the product image does not exist")
)

// Image 是已存储的商品图片。对象键属于内部信息，公开 URL 由传输层推导，
// 因此迁移到其他对象存储不会改变领域层。
type Image struct {
	ID        string
	ProductID string
	ObjectKey string
	SortOrder int
	CreatedAt time.Time
}

// Product 是商品的聚合根。
type Product struct {
	ID          string
	SellerID    string
	Title       string
	PriceMinor  int64
	Category    Category
	Description string
	Status      Status
	// Version 每次变更都会递增，用于在商品行上检测乐观并发冲突。
	Version   int64
	Images    []Image
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewProduct 创建处于 ON_SALE 状态的商品。
func NewProduct(id, sellerID, title string, priceMinor int64, category Category, description string, now time.Time) (*Product, error) {
	if id == "" {
		return nil, ErrIDRequired
	}
	if sellerID == "" {
		return nil, ErrSellerRequired
	}
	if err := ValidateTitle(title); err != nil {
		return nil, err
	}
	if err := ValidatePrice(priceMinor); err != nil {
		return nil, err
	}
	if err := ValidateDescription(description); err != nil {
		return nil, err
	}
	if !category.Valid() {
		return nil, ErrCategoryUnknown
	}

	return &Product{
		ID:          id,
		SellerID:    sellerID,
		Title:       title,
		PriceMinor:  priceMinor,
		Category:    category,
		Description: description,
		Status:      StatusOnSale,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// ValidateTitle 强制执行公开标题限制。
func ValidateTitle(title string) error {
	length := utf8.RuneCountInString(title)
	if length < 1 || length > MaxTitleLength {
		return ErrTitleLength
	}
	return nil
}

// ValidateDescription 强制执行公开描述限制。
func ValidateDescription(description string) error {
	length := utf8.RuneCountInString(description)
	if length < 1 || length > MaxDescriptionLength {
		return ErrDescriptionLength
	}
	return nil
}

// ValidatePrice 强制执行公开 Price 模式可以表达的范围。
func ValidatePrice(priceMinor int64) error {
	if priceMinor < 0 || priceMinor > MaxPriceMinor {
		return ErrPriceRange
	}
	return nil
}

// IsSeller 判断 actorID 是否发布了该商品。
func (p *Product) IsSeller(actorID string) bool { return actorID != "" && actorID == p.SellerID }

// Edit 修改描述字段。只有非 nil 参数对应的字段会改变，状态永远不是其中之一：
// 已预留或已售出的商品属于进行中的交易，不能在交易对方操作下移动状态。
func (p *Product) Edit(actorID string, title *string, priceMinor *int64, category *Category, description *string, now time.Time) error {
	if err := p.RequireContentMutation(actorID); err != nil {
		return err
	}

	if title != nil {
		if err := ValidateTitle(*title); err != nil {
			return err
		}
	}
	if priceMinor != nil {
		if err := ValidatePrice(*priceMinor); err != nil {
			return err
		}
	}
	if description != nil {
		if err := ValidateDescription(*description); err != nil {
			return err
		}
	}
	if category != nil && !category.Valid() {
		return ErrCategoryUnknown
	}

	if title != nil {
		p.Title = *title
	}
	if priceMinor != nil {
		p.PriceMinor = *priceMinor
	}
	if category != nil {
		p.Category = *category
	}
	if description != nil {
		p.Description = *description
	}
	p.touch(now)
	return nil
}

// RequireContentMutation 检查商品字段或图片是否可由 actorID 修改。
// 是否存在 PENDING 意向由持有 Product 锁的应用事务额外检查。
func (p *Product) RequireContentMutation(actorID string) error {
	if !p.IsSeller(actorID) {
		return ErrNotSeller
	}
	if p.Status != StatusOnSale && p.Status != StatusOffShelf {
		return ErrNotEditable
	}
	return nil
}

// AppendImages 按上传顺序追加图片，并维持连续的 1..N 排序。
func (p *Product) AppendImages(actorID string, images []Image, now time.Time) error {
	if err := p.RequireContentMutation(actorID); err != nil {
		return err
	}
	if len(images) == 0 || len(p.Images)+len(images) > MaxImages {
		return ErrImageLimitExceeded
	}

	firstOrder := len(p.Images) + 1
	for i := range images {
		images[i].ProductID = p.ID
		images[i].SortOrder = firstOrder + i
		p.Images = append(p.Images, images[i])
	}
	p.touch(now)
	return nil
}

// RemoveImage 删除指定图片，保持其余图片相对顺序并重新编号为连续的 1..N。
func (p *Product) RemoveImage(actorID, imageID string, now time.Time) (string, error) {
	if err := p.RequireContentMutation(actorID); err != nil {
		return "", err
	}

	index := -1
	for i := range p.Images {
		if p.Images[i].ID == imageID {
			index = i
			break
		}
	}
	if index < 0 {
		return "", ErrImageNotFound
	}

	objectKey := p.Images[index].ObjectKey
	p.Images = append(p.Images[:index], p.Images[index+1:]...)
	for i := range p.Images {
		p.Images[i].SortOrder = i + 1
	}
	p.touch(now)
	return objectKey, nil
}

// OffShelf 执行 ON_SALE -> OFF_SHELF。
func (p *Product) OffShelf(actorID string, now time.Time) error {
	if !p.IsSeller(actorID) {
		return ErrNotSeller
	}
	if p.Status != StatusOnSale {
		return ErrNotOnSale
	}
	p.Status = StatusOffShelf
	p.touch(now)
	return nil
}

// Relist 执行 OFF_SHELF -> ON_SALE。
func (p *Product) Relist(actorID string, now time.Time) error {
	if !p.IsSeller(actorID) {
		return ErrNotSeller
	}
	if p.Status != StatusOffShelf {
		return ErrNotOffShelf
	}
	p.Status = StatusOnSale
	p.touch(now)
	return nil
}

// Reserve 在卖家接受交易时执行 ON_SALE -> RESERVED。
// 它由交易状态机驱动，绝不由商品端点驱动。
func (p *Product) Reserve(now time.Time) error {
	if p.Status != StatusOnSale {
		return ErrNotOnSale
	}
	p.Status = StatusReserved
	p.touch(now)
	return nil
}

// Release 在已接受交易取消时执行 RESERVED -> ON_SALE。
func (p *Product) Release(now time.Time) error {
	if p.Status != StatusReserved {
		return ErrNotReserved
	}
	p.Status = StatusOnSale
	p.touch(now)
	return nil
}

// MarkSold 在双方确认时执行 RESERVED -> SOLD。SOLD 是终态。
func (p *Product) MarkSold(now time.Time) error {
	if p.Status != StatusReserved {
		return ErrNotReserved
	}
	p.Status = StatusSold
	p.touch(now)
	return nil
}

// Tradable 判断是否可以为该商品创建或接受新交易。
// RESERVED、SOLD 和 OFF_SHELF 商品不可交易。
func (p *Product) Tradable() bool { return p.Status == StatusOnSale }

// NextImageSlot 返回最小的空闲 sort_order；商品图片已达上限时返回错误。
func (p *Product) NextImageSlot() (int, error) {
	used := make(map[int]bool, len(p.Images))
	for _, image := range p.Images {
		used[image.SortOrder] = true
	}
	for slot := 1; slot <= MaxImages; slot++ {
		if !used[slot] {
			return slot, nil
		}
	}
	return 0, ErrImageLimitExceeded
}

// CoverObjectKey 返回排序最小图片的对象键，即商品封面；
// 商品没有图片时返回空字符串。
func (p *Product) CoverObjectKey() string {
	cover := ""
	lowest := MaxImages + 1
	for _, image := range p.Images {
		if image.SortOrder < lowest {
			lowest = image.SortOrder
			cover = image.ObjectKey
		}
	}
	return cover
}

// touch 记录一次变更：更新时间戳并递增版本，这样可以检测读取过旧数据行的并发写入方。
func (p *Product) touch(now time.Time) {
	p.Version++
	p.UpdatedAt = now
}
