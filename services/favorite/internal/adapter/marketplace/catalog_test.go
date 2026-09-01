package marketplace_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	marketplaceadapter "github.com/KDZZZZZZ/short-term/services/favorite/internal/adapter/marketplace"
)

type fakeMarketplace struct {
	marketplacev1.MarketplaceServiceClient
	status marketplacev1.ProductStatus
	err    error
	actor  string
}

func (f *fakeMarketplace) GetProduct(ctx context.Context, req *marketplacev1.GetProductRequest, _ ...grpc.CallOption) (*marketplacev1.GetProductResponse, error) {
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		values := md.Get(grpcx.MetadataActorID)
		if len(values) > 0 {
			f.actor = values[0]
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	return &marketplacev1.GetProductResponse{Product: &marketplacev1.ProductDetail{
		Id: req.GetProductId(), SellerId: "u_seller", Status: f.status,
	}}, nil
}

func TestCatalogAcceptsProductsInEveryStatus(t *testing.T) {
	t.Parallel()

	statuses := []marketplacev1.ProductStatus{
		marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE,
		marketplacev1.ProductStatus_PRODUCT_STATUS_OFF_SHELF,
		marketplacev1.ProductStatus_PRODUCT_STATUS_RESERVED,
		marketplacev1.ProductStatus_PRODUCT_STATUS_SOLD,
	}
	for _, status := range statuses {
		status := status
		t.Run(status.String(), func(t *testing.T) {
			t.Parallel()
			client := &fakeMarketplace{status: status}
			catalog := marketplaceadapter.NewCatalog(client)
			product, err := catalog.Get(t.Context(), "u_buyer", "p_1")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if product.ID != "p_1" || product.SellerID != "u_seller" {
				t.Fatalf("product = %+v", product)
			}
			if client.actor != "u_buyer" {
				t.Fatalf("actor metadata = %q, want u_buyer", client.actor)
			}
		})
	}
}

func TestCatalogPreservesMarketplaceErrors(t *testing.T) {
	t.Parallel()

	want := errs.New(errs.CodeResourceNotFound, "商品不存在")
	catalog := marketplaceadapter.NewCatalog(&fakeMarketplace{err: want})
	_, err := catalog.Get(t.Context(), "u_1", "p_missing")
	if errs.CodeOf(err) != errs.CodeResourceNotFound {
		t.Fatalf("Get = %v, want RESOURCE_NOT_FOUND", err)
	}
}

func TestCatalogRejectsEmptyMarketplaceResponse(t *testing.T) {
	t.Parallel()

	client := &emptyMarketplace{}
	catalog := marketplaceadapter.NewCatalog(client)
	if _, err := catalog.Get(t.Context(), "u_1", "p_1"); err == nil {
		t.Fatal("Get accepted an empty product response")
	}
}

type emptyMarketplace struct {
	marketplacev1.MarketplaceServiceClient
}

func (e *emptyMarketplace) GetProduct(context.Context, *marketplacev1.GetProductRequest, ...grpc.CallOption) (*marketplacev1.GetProductResponse, error) {
	return &marketplacev1.GetProductResponse{}, nil
}
