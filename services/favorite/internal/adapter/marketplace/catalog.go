// Package marketplace adapts Marketplace Service into Favorite's product fact
// source. Favorite never reads Marketplace tables directly.
package marketplace

import (
	"context"
	"fmt"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/favorite/internal/application"
)

// Catalog reads the minimum product projection needed by Favorite Service.
type Catalog struct {
	client marketplacev1.MarketplaceServiceClient
}

// NewCatalog constructs a Marketplace-backed product source.
func NewCatalog(client marketplacev1.MarketplaceServiceClient) *Catalog {
	return &Catalog{client: client}
}

var _ application.ProductCatalog = (*Catalog)(nil)

// Get accepts every Marketplace product status. Existence and seller identity
// are the only facts relevant to adding a favorite.
func (c *Catalog) Get(ctx context.Context, actorID, productID string) (application.Product, error) {
	resp, err := c.client.GetProduct(grpcx.WithActor(ctx, actorID), &marketplacev1.GetProductRequest{
		ProductId: productID,
	})
	if err != nil {
		return application.Product{}, err
	}

	product := resp.GetProduct()
	if product == nil || product.GetId() == "" || product.GetSellerId() == "" {
		return application.Product{}, fmt.Errorf("marketplace: empty product response")
	}
	return application.Product{ID: product.GetId(), SellerID: product.GetSellerId()}, nil
}
