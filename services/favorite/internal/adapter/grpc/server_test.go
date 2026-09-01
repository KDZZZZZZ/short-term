package grpc_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"

	favoritev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/favorite/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	grpcadapter "github.com/KDZZZZZZ/short-term/services/favorite/internal/adapter/grpc"
	"github.com/KDZZZZZZ/short-term/services/favorite/internal/application"
	"github.com/KDZZZZZZ/short-term/services/favorite/internal/domain"
)

type clock struct{ at time.Time }

func (c clock) Now() time.Time { return c.at }

type catalog struct{ product application.Product }

func (c catalog) Get(context.Context, string, string) (application.Product, error) {
	return c.product, nil
}

type repository struct{ items map[string]domain.Favorite }

func (r *repository) Add(_ context.Context, item domain.Favorite) error {
	key := item.UserID + "/" + item.ProductID
	if _, ok := r.items[key]; !ok {
		r.items[key] = item
	}
	return nil
}

func (r *repository) Remove(_ context.Context, userID, productID string) error {
	delete(r.items, userID+"/"+productID)
	return nil
}

func (r *repository) List(_ context.Context, userID string, page application.Page) (application.FavoritePage, error) {
	items := make([]domain.Favorite, 0, len(r.items))
	for _, item := range r.items {
		if item.UserID == userID {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ProductID > items[j].ProductID })
	return application.FavoritePage{Items: items, Page: page.Number, Size: page.Size, Total: int64(len(items))}, nil
}

func (r *repository) IsFavorited(_ context.Context, userID, productID string) (bool, error) {
	_, ok := r.items[userID+"/"+productID]
	return ok, nil
}

func newServer(t *testing.T) *grpcadapter.Server {
	t.Helper()
	app, err := application.NewService(
		&repository{items: make(map[string]domain.Favorite)},
		catalog{product: application.Product{ID: "p_1", SellerID: "u_seller"}},
		clock{at: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)},
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return grpcadapter.NewServer(app)
}

func actorContext(actorID string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(grpcx.MetadataActorID, actorID))
}

func TestFavoriteRPCFlow(t *testing.T) {
	t.Parallel()

	server := newServer(t)
	ctx := actorContext("u_buyer")
	if _, err := server.AddFavorite(ctx, &favoritev1.AddFavoriteRequest{ActorId: "u_buyer", ProductId: "p_1"}); err != nil {
		t.Fatalf("AddFavorite: %v", err)
	}
	// A second call is an idempotent success.
	if _, err := server.AddFavorite(ctx, &favoritev1.AddFavoriteRequest{ActorId: "u_buyer", ProductId: "p_1"}); err != nil {
		t.Fatalf("AddFavorite replay: %v", err)
	}

	check, err := server.IsFavorited(ctx, &favoritev1.IsFavoritedRequest{ActorId: "u_buyer", ProductId: "p_1"})
	if err != nil || !check.GetFavorited() {
		t.Fatalf("IsFavorited = %+v, %v; want true", check, err)
	}

	list, err := server.ListFavorites(ctx, &favoritev1.ListFavoritesRequest{ActorId: "u_buyer"})
	if err != nil {
		t.Fatalf("ListFavorites: %v", err)
	}
	if list.GetPage().GetPage() != 1 || list.GetPage().GetPageSize() != application.DefaultPageSize {
		t.Fatalf("page = %+v, want defaults", list.GetPage())
	}
	if len(list.GetPage().GetItems()) != 1 || list.GetPage().GetItems()[0].GetProductId() != "p_1" {
		t.Fatalf("items = %+v", list.GetPage().GetItems())
	}
	if err := list.GetPage().GetItems()[0].GetFavoritedAt().CheckValid(); err != nil {
		t.Fatalf("favorited_at: %v", err)
	}

	if _, err := server.RemoveFavorite(ctx, &favoritev1.RemoveFavoriteRequest{ActorId: "u_buyer", ProductId: "p_1"}); err != nil {
		t.Fatalf("RemoveFavorite: %v", err)
	}
	if _, err := server.RemoveFavorite(ctx, &favoritev1.RemoveFavoriteRequest{ActorId: "u_buyer", ProductId: "p_1"}); err != nil {
		t.Fatalf("RemoveFavorite replay: %v", err)
	}
	check, err = server.IsFavorited(ctx, &favoritev1.IsFavoritedRequest{ActorId: "u_buyer", ProductId: "p_1"})
	if err != nil || check.GetFavorited() {
		t.Fatalf("IsFavorited after remove = %+v, %v; want false", check, err)
	}
}

func TestFavoriteRPCRejectsActorImpersonation(t *testing.T) {
	t.Parallel()

	server := newServer(t)
	_, err := server.AddFavorite(actorContext("u_authenticated"), &favoritev1.AddFavoriteRequest{
		ActorId: "u_other", ProductId: "p_1",
	})
	if errs.CodeOf(err) != errs.CodeForbidden {
		t.Fatalf("AddFavorite = %v, want FORBIDDEN", err)
	}
}

func TestFavoriteRPCRequiresAuthentication(t *testing.T) {
	t.Parallel()

	server := newServer(t)
	_, err := server.ListFavorites(context.Background(), &favoritev1.ListFavoritesRequest{})
	if errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("ListFavorites = %v, want UNAUTHORIZED", err)
	}
}
