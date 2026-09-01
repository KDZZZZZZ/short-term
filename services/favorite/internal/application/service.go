package application

import (
	"context"
	"errors"

	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/favorite/internal/domain"
)

// Service coordinates favorite relationships with Marketplace product facts.
type Service struct {
	repo     Repository
	products ProductCatalog
	clock    Clock
}

// NewService requires every dependency because add, list and mutations must
// never silently degrade into incomplete behavior.
func NewService(repo Repository, products ProductCatalog, clock Clock) (*Service, error) {
	if repo == nil || products == nil || clock == nil {
		return nil, errors.New("application: every Favorite Service dependency is required")
	}
	return &Service{repo: repo, products: products, clock: clock}, nil
}

// Add leaves the product in the favorited state. The repository's composite
// key makes repeated and concurrent requests idempotent without changing the
// original favorited_at value.
func (s *Service) Add(ctx context.Context, cmd FavoriteCommand) error {
	if err := validateCommand(cmd); err != nil {
		return err
	}

	product, err := s.products.Get(ctx, cmd.ActorID, cmd.ProductID)
	if err != nil {
		if errs.CodeOf(err) == errs.CodeResourceNotFound {
			return errs.New(errs.CodeResourceNotFound, "商品不存在")
		}
		return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	if product.ID == "" || product.SellerID == "" {
		return errs.New(errs.CodeInternal, "服务暂时不可用")
	}
	if product.ID != cmd.ProductID {
		return errs.New(errs.CodeInternal, "服务暂时不可用")
	}
	if product.SellerID == cmd.ActorID {
		return errs.New(errs.CodeSelfActionNotAllowed, "不能收藏自己发布的商品")
	}

	favorite, err := domain.New(cmd.ActorID, cmd.ProductID, s.clock.Now())
	if err != nil {
		return errs.Wrap(errs.CodeValidation, "收藏信息不合法", err)
	}
	if err := s.repo.Add(ctx, favorite); err != nil {
		return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return nil
}

// Remove leaves the product in the unfavorited state. It intentionally does
// not call Marketplace: absence of the relationship is already success.
func (s *Service) Remove(ctx context.Context, cmd FavoriteCommand) error {
	if err := validateCommand(cmd); err != nil {
		return err
	}
	if err := s.repo.Remove(ctx, cmd.ActorID, cmd.ProductID); err != nil {
		return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return nil
}

// List returns relationship facts only. Gateway performs one batch product
// lookup so every response carries Marketplace's current product status.
func (s *Service) List(ctx context.Context, query ListFavoritesQuery) (FavoritePage, error) {
	if query.ActorID == "" {
		return FavoritePage{}, errs.New(errs.CodeUnauthorized, "请先登录")
	}

	page := query.Page.Normalize()
	result, err := s.repo.List(ctx, query.ActorID, page)
	if err != nil {
		return FavoritePage{}, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return result, nil
}

// IsFavorited returns whether the composite relationship exists.
func (s *Service) IsFavorited(ctx context.Context, cmd FavoriteCommand) (bool, error) {
	if err := validateCommand(cmd); err != nil {
		return false, err
	}
	favorited, err := s.repo.IsFavorited(ctx, cmd.ActorID, cmd.ProductID)
	if err != nil {
		return false, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return favorited, nil
}

func validateCommand(cmd FavoriteCommand) error {
	if cmd.ActorID == "" {
		return errs.New(errs.CodeUnauthorized, "请先登录")
	}
	if err := domain.ValidateProductID(cmd.ProductID); err != nil {
		return errs.Wrap(errs.CodeValidation, "商品标识不合法", err)
	}
	return nil
}
