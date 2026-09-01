package mapper

import (
	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
)

// accountUserPublic 是聚合层提供的批量资料结构。
type accountUserPublic = accountv1.UserPublic

// lookupSeller 渲染卖家身份；账户不存在时回退到占位值。
// 列表不能因为某个卖家被删除而失败，且契约要求该字段必须存在。
func lookupSeller(sellerID string, sellers map[string]*accountUserPublic) dto.UserPublic {
	return UserPublic(sellerID, sellers[sellerID])
}

// productCategories 在线路枚举和公开字符串枚举之间进行映射。
var productCategories = map[marketplacev1.ProductCategory]string{
	marketplacev1.ProductCategory_PRODUCT_CATEGORY_TEXTBOOK: "TEXTBOOK",
	marketplacev1.ProductCategory_PRODUCT_CATEGORY_DIGITAL:  "DIGITAL",
	marketplacev1.ProductCategory_PRODUCT_CATEGORY_LIFE:     "LIFE",
	marketplacev1.ProductCategory_PRODUCT_CATEGORY_OTHER:    "OTHER",
}

// productStatuses 在线路枚举和公开字符串枚举之间进行映射。
var productStatuses = map[marketplacev1.ProductStatus]string{
	marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE:   "ON_SALE",
	marketplacev1.ProductStatus_PRODUCT_STATUS_RESERVED:  "RESERVED",
	marketplacev1.ProductStatus_PRODUCT_STATUS_SOLD:      "SOLD",
	marketplacev1.ProductStatus_PRODUCT_STATUS_OFF_SHELF: "OFF_SHELF",
}

// ProductCategory 渲染公开分类字符串。
func ProductCategory(value marketplacev1.ProductCategory) string {
	return productCategories[value]
}

// ParseProductCategory 将公开分类字符串转换为线路枚举，并返回该值是否由契约定义。
func ParseProductCategory(value string) (marketplacev1.ProductCategory, bool) {
	for enum, name := range productCategories {
		if name == value {
			return enum, true
		}
	}
	return marketplacev1.ProductCategory_PRODUCT_CATEGORY_UNSPECIFIED, false
}

// ProductStatus 渲染公开状态字符串。
func ProductStatus(value marketplacev1.ProductStatus) string {
	return productStatuses[value]
}

// ParseProductStatus 将公开状态字符串转换为线路枚举。
func ParseProductStatus(value string) (marketplacev1.ProductStatus, bool) {
	for enum, name := range productStatuses {
		if name == value {
			return enum, true
		}
	}
	return marketplacev1.ProductStatus_PRODUCT_STATUS_UNSPECIFIED, false
}

// ProductSummary 映射一行列表数据，并使用调用方已经执行的批量资料查询补全卖家。
func ProductSummary(src *marketplacev1.ProductSummary, sellers map[string]*accountUserPublic) dto.ProductSummary {
	return dto.ProductSummary{
		ID:        src.GetId(),
		Title:     src.GetTitle(),
		Price:     FormatPrice(src.GetPriceMinor()),
		Category:  ProductCategory(src.GetCategory()),
		CoverURL:  src.CoverUrl,
		Status:    ProductStatus(src.GetStatus()),
		Seller:    lookupSeller(src.GetSellerId(), sellers),
		CreatedAt: Timestamp(src.GetCreatedAt()),
	}
}

// ProductDetail 映射详情响应。is_favorited 和卖家联系方式由调用方解析，
// 因为调用方知道请求是否已通过认证。
func ProductDetail(src *marketplacev1.ProductDetail, seller dto.SellerContact, isFavorited bool) dto.ProductDetail {
	return dto.ProductDetail{
		ID:          src.GetId(),
		Title:       src.GetTitle(),
		Price:       FormatPrice(src.GetPriceMinor()),
		Category:    ProductCategory(src.GetCategory()),
		Description: src.GetDescription(),
		Status:      ProductStatus(src.GetStatus()),
		Images:      ProductImages(src.GetImages()),
		Seller:      seller,
		IsFavorited: isFavorited,
		CreatedAt:   Timestamp(src.GetCreatedAt()),
		UpdatedAt:   Timestamp(src.GetUpdatedAt()),
	}
}

// ProductImages 映射图片列表，并始终返回非 nil 切片，使 JSON 数组不会为 null。
func ProductImages(src []*marketplacev1.ProductImage) []dto.ProductImage {
	images := make([]dto.ProductImage, 0, len(src))
	for _, image := range src {
		images = append(images, dto.ProductImage{
			ID:        image.GetId(),
			URL:       image.GetUrl(),
			SortOrder: image.GetSortOrder(),
			CreatedAt: Timestamp(image.GetCreatedAt()),
		})
	}
	return images
}

// ProductPage 映射一页摘要。
func ProductPage(src *marketplacev1.ProductPage, sellers map[string]*accountUserPublic) dto.ProductPage {
	items := make([]dto.ProductSummary, 0, len(src.GetItems()))
	for _, item := range src.GetItems() {
		items = append(items, ProductSummary(item, sellers))
	}
	return dto.ProductPage{
		Items:    items,
		Page:     src.GetPage(),
		PageSize: src.GetPageSize(),
		Total:    src.GetTotal(),
	}
}
