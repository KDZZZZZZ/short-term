package mapper

import (
	"fmt"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	favoritev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/favorite/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
)

// FavoritePage combines relationship facts with one batch of current product
// facts. A missing product is a cross-service integrity failure, not a reason
// to fabricate a ProductSummary or silently change the page total.
func FavoritePage(
	src *favoritev1.FavoritePage,
	products map[string]*marketplacev1.ProductSummary,
	sellers map[string]*accountv1.UserPublic,
) (dto.FavoritePage, error) {
	items := make([]dto.FavoriteItem, 0, len(src.GetItems()))
	for _, relation := range src.GetItems() {
		product := products[relation.GetProductId()]
		if product == nil {
			return dto.FavoritePage{}, fmt.Errorf("favorite product %q is missing from Marketplace", relation.GetProductId())
		}
		items = append(items, dto.FavoriteItem{
			Product:     ProductSummary(product, sellers),
			FavoritedAt: Timestamp(relation.GetFavoritedAt()),
		})
	}
	return dto.FavoritePage{
		Items: items, Page: src.GetPage(), PageSize: src.GetPageSize(), Total: src.GetTotal(),
	}, nil
}
