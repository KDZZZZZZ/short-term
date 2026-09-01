// Package grpc exposes Favorite Service use cases through the internal proto
// contract in proto/shortterm/favorite/v1.
package grpc

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	favoritev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/favorite/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/favorite/internal/application"
)

// Server maps generated messages to Favorite application use cases.
type Server struct {
	favoritev1.UnimplementedFavoriteServiceServer

	app *application.Service
}

// NewServer constructs the gRPC adapter.
func NewServer(app *application.Service) *Server { return &Server{app: app} }

// AddFavorite leaves the relationship present.
func (s *Server) AddFavorite(ctx context.Context, req *favoritev1.AddFavoriteRequest) (*favoritev1.AddFavoriteResponse, error) {
	if err := requireActor(ctx, req.GetActorId()); err != nil {
		return nil, err
	}
	if err := s.app.Add(ctx, application.FavoriteCommand{
		ActorID: req.GetActorId(), ProductID: req.GetProductId(),
	}); err != nil {
		return nil, err
	}
	return &favoritev1.AddFavoriteResponse{}, nil
}

// RemoveFavorite leaves the relationship absent.
func (s *Server) RemoveFavorite(ctx context.Context, req *favoritev1.RemoveFavoriteRequest) (*favoritev1.RemoveFavoriteResponse, error) {
	if err := requireActor(ctx, req.GetActorId()); err != nil {
		return nil, err
	}
	if err := s.app.Remove(ctx, application.FavoriteCommand{
		ActorID: req.GetActorId(), ProductID: req.GetProductId(),
	}); err != nil {
		return nil, err
	}
	return &favoritev1.RemoveFavoriteResponse{}, nil
}

// ListFavorites returns relationship facts in deterministic order.
func (s *Server) ListFavorites(ctx context.Context, req *favoritev1.ListFavoritesRequest) (*favoritev1.ListFavoritesResponse, error) {
	if err := requireActor(ctx, req.GetActorId()); err != nil {
		return nil, err
	}
	page, err := s.app.List(ctx, application.ListFavoritesQuery{
		ActorID: req.GetActorId(),
		Page:    application.Page{Number: req.GetPage(), Size: req.GetPageSize()},
	})
	if err != nil {
		return nil, err
	}

	items := make([]*favoritev1.FavoriteItem, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, &favoritev1.FavoriteItem{
			ProductId:   item.ProductID,
			FavoritedAt: timestamppb.New(item.FavoritedAt),
		})
	}
	return &favoritev1.ListFavoritesResponse{Page: &favoritev1.FavoritePage{
		Items: items, Page: page.Page, PageSize: page.Size, Total: page.Total,
	}}, nil
}

// IsFavorited reports whether the composite relationship exists.
func (s *Server) IsFavorited(ctx context.Context, req *favoritev1.IsFavoritedRequest) (*favoritev1.IsFavoritedResponse, error) {
	if err := requireActor(ctx, req.GetActorId()); err != nil {
		return nil, err
	}
	favorited, err := s.app.IsFavorited(ctx, application.FavoriteCommand{
		ActorID: req.GetActorId(), ProductID: req.GetProductId(),
	})
	if err != nil {
		return nil, err
	}
	return &favoritev1.IsFavoritedResponse{Favorited: favorited}, nil
}

// requireActor prevents a caller from asking Favorite Service to act on a
// different user's relationship set.
func requireActor(ctx context.Context, actorID string) error {
	if actorID == "" {
		return errs.New(errs.CodeUnauthorized, "请先登录")
	}
	if authenticated := grpcx.ActorID(ctx); authenticated != "" && authenticated != actorID {
		return errs.New(errs.CodeForbidden, "无权执行该操作")
	}
	return nil
}
