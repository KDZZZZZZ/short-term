// Package aggregation completes list and detail responses with data owned by
// other services.
//
// Every completion is a batch call. docs/software-design.md section 3.3
// forbids one RPC per row, and reading the current product status from the
// owning service rather than a cached copy is what makes the status in a
// favorite, conversation or trade projection true at the moment of the reply.
package aggregation

import (
	"context"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	favoritev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/favorite/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
)

// Aggregator performs the cross-service completions the REST responses need.
type Aggregator struct {
	accounts  accountv1.AccountServiceClient
	products  marketplacev1.MarketplaceServiceClient
	favorites FavoriteChecker
}

// FavoriteChecker reports whether the acting user has favorited a product.
type FavoriteChecker interface {
	IsFavorited(ctx context.Context, actorID, productID string) (bool, error)
}

// New builds an Aggregator.
func New(accounts accountv1.AccountServiceClient, products marketplacev1.MarketplaceServiceClient, favorites FavoriteChecker) *Aggregator {
	return &Aggregator{accounts: accounts, products: products, favorites: favorites}
}

// Users returns the public profiles for the given identifiers in one call.
// Identifiers that no longer exist are simply absent from the result.
func (a *Aggregator) Users(ctx context.Context, ids []string) (map[string]*accountv1.UserPublic, error) {
	unique := dedupe(ids)
	if len(unique) == 0 {
		return map[string]*accountv1.UserPublic{}, nil
	}

	resp, err := a.accounts.BatchGetUsers(ctx, &accountv1.BatchGetUsersRequest{UserIds: unique})
	if err != nil {
		return nil, err
	}
	return resp.GetUsers(), nil
}

// Products returns the current summary of each product in one call.
func (a *Aggregator) Products(ctx context.Context, ids []string) (map[string]*marketplacev1.ProductSummary, error) {
	unique := dedupe(ids)
	if len(unique) == 0 {
		return map[string]*marketplacev1.ProductSummary{}, nil
	}

	resp, err := a.products.BatchGetProducts(ctx, &marketplacev1.BatchGetProductsRequest{ProductIds: unique})
	if err != nil {
		return nil, err
	}
	return resp.GetProducts(), nil
}

// SellerContact returns one seller's contact profile for the product detail
// response. It uses GetUser, whose response type has no student number field.
func (a *Aggregator) SellerContact(ctx context.Context, sellerID string) (*accountv1.UserContact, error) {
	resp, err := a.accounts.GetUser(ctx, &accountv1.GetUserRequest{UserId: sellerID})
	if err != nil {
		return nil, err
	}
	return resp.GetUser(), nil
}

// IsFavorited reports whether the acting user favorited a product. An
// anonymous caller is never favorited.
func (a *Aggregator) IsFavorited(ctx context.Context, actorID, productID string) (bool, error) {
	if actorID == "" || a.favorites == nil {
		return false, nil
	}
	return a.favorites.IsFavorited(ctx, actorID, productID)
}

// GRPCFavorites checks favorites through the Favorite Service.
type GRPCFavorites struct {
	client favoritev1.FavoriteServiceClient
}

// NewGRPCFavorites builds a favorite checker backed by the Favorite Service.
func NewGRPCFavorites(client favoritev1.FavoriteServiceClient) *GRPCFavorites {
	return &GRPCFavorites{client: client}
}

// IsFavorited asks the Favorite Service.
func (f *GRPCFavorites) IsFavorited(ctx context.Context, actorID, productID string) (bool, error) {
	resp, err := f.client.IsFavorited(grpcx.WithActor(ctx, actorID), &favoritev1.IsFavoritedRequest{
		ActorId:   actorID,
		ProductId: productID,
	})
	if err != nil {
		return false, err
	}
	return resp.GetFavorited(), nil
}

// dedupe removes empty and repeated identifiers while preserving order.
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
