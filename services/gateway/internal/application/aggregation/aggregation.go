// Package aggregation 使用其他服务拥有的数据补全列表和详情响应。
//
// 每次补全都是批量调用。docs/software-design.md 第 3.3 节禁止每行执行一次 RPC；
// 从所属服务读取当前商品状态，而不是读取缓存副本，才能保证收藏、会话或交易投影
// 中的状态在响应时刻真实有效。
package aggregation

import (
	"context"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	favoritev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/favorite/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
)

// Aggregator 执行 REST 响应所需的跨服务数据补全。
type Aggregator struct {
	accounts  accountv1.AccountServiceClient
	products  marketplacev1.MarketplaceServiceClient
	favorites FavoriteChecker
}

// FavoriteChecker 返回当前用户是否收藏了某个商品。
type FavoriteChecker interface {
	IsFavorited(ctx context.Context, actorID, productID string) (bool, error)
}

// New 构造 Aggregator。
func New(accounts accountv1.AccountServiceClient, products marketplacev1.MarketplaceServiceClient, favorites FavoriteChecker) *Aggregator {
	return &Aggregator{accounts: accounts, products: products, favorites: favorites}
}

// Users 在一次调用中返回指定标识对应的公开资料。
// 已不存在的标识会直接从结果中省略。
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

// UserContacts 一次返回指定标识的昵称与联系方式。缺失的标识从结果中省略。
// 仅用于交易双方：交易读取端点只对交易方开放。
func (a *Aggregator) UserContacts(ctx context.Context, ids []string) (map[string]*accountv1.UserContact, error) {
	unique := dedupe(ids)
	if len(unique) == 0 {
		return map[string]*accountv1.UserContact{}, nil
	}

	resp, err := a.accounts.BatchGetUserContacts(ctx, &accountv1.BatchGetUserContactsRequest{UserIds: unique})
	if err != nil {
		return nil, err
	}
	return resp.GetUsers(), nil
}

// Products 在一次调用中返回每个商品的当前摘要。
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

// SellerContact 为商品详情响应返回一个卖家的联系方式资料。
// 它使用 GetUser，而 GetUser 的响应类型不包含学号字段。
func (a *Aggregator) SellerContact(ctx context.Context, sellerID string) (*accountv1.UserContact, error) {
	resp, err := a.accounts.GetUser(ctx, &accountv1.GetUserRequest{UserId: sellerID})
	if err != nil {
		return nil, err
	}
	return resp.GetUser(), nil
}

// AverageScores 一次返回指定用户作为卖家收到的评分平均值。
// 没有评分的用户不会出现在结果中，由调用方渲染为 null。
func (a *Aggregator) AverageScores(ctx context.Context, userIDs []string) (map[string]string, error) {
	unique := dedupe(userIDs)
	if len(unique) == 0 {
		return map[string]string{}, nil
	}

	resp, err := a.products.BatchGetUserAverageScores(ctx, &marketplacev1.BatchGetUserAverageScoresRequest{
		UserIds: unique,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetAverageScores(), nil
}

// SellerStats 返回用户作为卖家的公开统计：已完成交易数与在售商品数。
func (a *Aggregator) SellerStats(ctx context.Context, userID string) (*marketplacev1.GetUserStatsResponse, error) {
	return a.products.GetUserStats(ctx, &marketplacev1.GetUserStatsRequest{UserId: userID})
}

// IsFavorited 返回当前用户是否收藏了某个商品。匿名调用方永远不会被视为已收藏。
func (a *Aggregator) IsFavorited(ctx context.Context, actorID, productID string) (bool, error) {
	if actorID == "" || a.favorites == nil {
		return false, nil
	}
	return a.favorites.IsFavorited(ctx, actorID, productID)
}

// GRPCFavorites 通过 Favorite Service 检查收藏状态。
type GRPCFavorites struct {
	client favoritev1.FavoriteServiceClient
}

// NewGRPCFavorites 构造由 Favorite Service 支持的收藏检查器。
func NewGRPCFavorites(client favoritev1.FavoriteServiceClient) *GRPCFavorites {
	return &GRPCFavorites{client: client}
}

// IsFavorited 向 Favorite Service 查询收藏状态。
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

// dedupe 删除空标识和重复标识，同时保留原有顺序。
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
