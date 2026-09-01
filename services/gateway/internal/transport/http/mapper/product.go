package mapper

import (
	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
)

// accountUserPublic is the batch profile shape the aggregation layer supplies.
type accountUserPublic = accountv1.UserPublic

// lookupSeller renders a seller identity, falling back to a placeholder when
// the account no longer exists. A list must not fail because one seller was
// removed, and the contract requires the field to be present.
func lookupSeller(sellerID string, sellers map[string]*accountUserPublic) dto.UserPublic {
	return UserPublic(sellerID, sellers[sellerID])
}

// productCategories maps between the wire enum and the public string enum.
var productCategories = map[marketplacev1.ProductCategory]string{
	marketplacev1.ProductCategory_PRODUCT_CATEGORY_TEXTBOOK: "TEXTBOOK",
	marketplacev1.ProductCategory_PRODUCT_CATEGORY_DIGITAL:  "DIGITAL",
	marketplacev1.ProductCategory_PRODUCT_CATEGORY_LIFE:     "LIFE",
	marketplacev1.ProductCategory_PRODUCT_CATEGORY_OTHER:    "OTHER",
}

// productStatuses maps between the wire enum and the public string enum.
var productStatuses = map[marketplacev1.ProductStatus]string{
	marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE:   "ON_SALE",
	marketplacev1.ProductStatus_PRODUCT_STATUS_RESERVED:  "RESERVED",
	marketplacev1.ProductStatus_PRODUCT_STATUS_SOLD:      "SOLD",
	marketplacev1.ProductStatus_PRODUCT_STATUS_OFF_SHELF: "OFF_SHELF",
}

// ProductCategory renders the public category string.
func ProductCategory(value marketplacev1.ProductCategory) string {
	return productCategories[value]
}

// ParseProductCategory converts a public category string to the wire enum,
// reporting whether it is one the contract defines.
func ParseProductCategory(value string) (marketplacev1.ProductCategory, bool) {
	for enum, name := range productCategories {
		if name == value {
			return enum, true
		}
	}
	return marketplacev1.ProductCategory_PRODUCT_CATEGORY_UNSPECIFIED, false
}

// ProductStatus renders the public status string.
func ProductStatus(value marketplacev1.ProductStatus) string {
	return productStatuses[value]
}

// ParseProductStatus converts a public status string to the wire enum.
func ParseProductStatus(value string) (marketplacev1.ProductStatus, bool) {
	for enum, name := range productStatuses {
		if name == value {
			return enum, true
		}
	}
	return marketplacev1.ProductStatus_PRODUCT_STATUS_UNSPECIFIED, false
}

// ProductSummary maps one list row, completing the seller from the batch
// profile lookup the caller already performed.
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

// ProductDetail maps the detail response. is_favorited and the seller contact
// are resolved by the caller, which knows whether the request was authenticated.
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

// ProductImages maps an image list, always producing a non-nil slice so the
// JSON array is never null.
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

// ProductPage maps a page of summaries.
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
