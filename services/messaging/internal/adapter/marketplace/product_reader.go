// Package marketplace adapts Marketplace product facts for conversation creation.
package marketplace

import (
	"context"
	"fmt"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/application"
)

type ProductReader struct {
	client marketplacev1.MarketplaceServiceClient
}

func NewProductReader(client marketplacev1.MarketplaceServiceClient) *ProductReader {
	return &ProductReader{client: client}
}

var _ application.ProductReader = (*ProductReader)(nil)

func (r *ProductReader) Get(ctx context.Context, productID string) (application.Product, error) {
	ctx = grpcx.WithActor(ctx, grpcx.ActorID(ctx))
	ctx = grpcx.WithRequestID(ctx, grpcx.RequestID(ctx))
	resp, err := r.client.GetProduct(ctx, &marketplacev1.GetProductRequest{ProductId: productID})
	if err != nil {
		if errs.CodeOf(err) == errs.CodeResourceNotFound {
			return application.Product{}, errs.New(errs.CodeResourceNotFound, "商品不存在")
		}
		return application.Product{}, fmt.Errorf("marketplace: get product: %w", err)
	}
	product := resp.GetProduct()
	if product == nil || product.GetId() != productID || product.GetSellerId() == "" {
		return application.Product{}, fmt.Errorf("marketplace: empty product response")
	}
	return application.Product{ID: product.GetId(), SellerID: product.GetSellerId()}, nil
}
