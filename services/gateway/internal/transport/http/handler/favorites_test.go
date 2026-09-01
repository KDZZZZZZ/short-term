package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	favoritev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/favorite/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/application/aggregation"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/handler"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/middleware"
)

const (
	testUserID    = "u_favorite_owner"
	testRequestID = "req_favorite_test"
)

type responseCapture struct {
	status int
	data   any
	code   errs.Code
	err    error
}

func (r *responseCapture) OK(_ http.ResponseWriter, _ *http.Request, data any) {
	r.status, r.data = http.StatusOK, data
}
func (r *responseCapture) Created(_ http.ResponseWriter, _ *http.Request, data any) {
	r.status, r.data = http.StatusCreated, data
}
func (r *responseCapture) Success(_ http.ResponseWriter, _ *http.Request, status int, data any) {
	r.status, r.data = status, data
}
func (r *responseCapture) Empty(_ http.ResponseWriter, _ *http.Request) {
	r.status, r.data = http.StatusOK, struct{}{}
}
func (r *responseCapture) Error(_ http.ResponseWriter, _ *http.Request, err error) {
	r.code, r.err = errs.CodeOf(err), err
}
func (r *responseCapture) Fail(_ http.ResponseWriter, _ *http.Request, code errs.Code, _ string) {
	r.code = code
}

type favoriteClient struct {
	favoritev1.FavoriteServiceClient
	items           []*favoritev1.FavoriteItem
	total           int64
	err             error
	listCalls       int
	lastList        *favoritev1.ListFavoritesRequest
	lastAdd         *favoritev1.AddFavoriteRequest
	lastRemove      *favoritev1.RemoveFavoriteRequest
	metadataActor   string
	metadataRequest string
}

func (f *favoriteClient) capture(ctx context.Context) {
	md, _ := metadata.FromOutgoingContext(ctx)
	if values := md.Get(grpcx.MetadataActorID); len(values) > 0 {
		f.metadataActor = values[0]
	}
	if values := md.Get(grpcx.MetadataRequestID); len(values) > 0 {
		f.metadataRequest = values[0]
	}
}

func (f *favoriteClient) ListFavorites(ctx context.Context, req *favoritev1.ListFavoritesRequest, _ ...grpc.CallOption) (*favoritev1.ListFavoritesResponse, error) {
	f.capture(ctx)
	f.listCalls++
	f.lastList = req
	if f.err != nil {
		return nil, f.err
	}
	return &favoritev1.ListFavoritesResponse{Page: &favoritev1.FavoritePage{
		Items: f.items, Page: req.GetPage(), PageSize: req.GetPageSize(), Total: f.total,
	}}, nil
}

func (f *favoriteClient) AddFavorite(ctx context.Context, req *favoritev1.AddFavoriteRequest, _ ...grpc.CallOption) (*favoritev1.AddFavoriteResponse, error) {
	f.capture(ctx)
	f.lastAdd = req
	if f.err != nil {
		return nil, f.err
	}
	return &favoritev1.AddFavoriteResponse{}, nil
}

func (f *favoriteClient) RemoveFavorite(ctx context.Context, req *favoritev1.RemoveFavoriteRequest, _ ...grpc.CallOption) (*favoritev1.RemoveFavoriteResponse, error) {
	f.capture(ctx)
	f.lastRemove = req
	if f.err != nil {
		return nil, f.err
	}
	return &favoritev1.RemoveFavoriteResponse{}, nil
}

type marketplaceClient struct {
	marketplacev1.MarketplaceServiceClient
	products map[string]*marketplacev1.ProductSummary
	calls    int
	ids      []string
}

func (m *marketplaceClient) BatchGetProducts(_ context.Context, req *marketplacev1.BatchGetProductsRequest, _ ...grpc.CallOption) (*marketplacev1.BatchGetProductsResponse, error) {
	m.calls++
	m.ids = append([]string(nil), req.GetProductIds()...)
	return &marketplacev1.BatchGetProductsResponse{Products: m.products}, nil
}

type accountClient struct {
	accountv1.AccountServiceClient
	users map[string]*accountv1.UserPublic
	calls int
	ids   []string
}

func (a *accountClient) BatchGetUsers(_ context.Context, req *accountv1.BatchGetUsersRequest, _ ...grpc.CallOption) (*accountv1.BatchGetUsersResponse, error) {
	a.calls++
	a.ids = append([]string(nil), req.GetUserIds()...)
	return &accountv1.BatchGetUsersResponse{Users: a.users}, nil
}

func request(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	ctx := middleware.WithActorID(req.Context(), testUserID)
	ctx = middleware.WithRequestID(ctx, testRequestID)
	return req.WithContext(ctx)
}

func product(id, seller string, status marketplacev1.ProductStatus) *marketplacev1.ProductSummary {
	return &marketplacev1.ProductSummary{
		Id: id, Title: "商品 " + id, PriceMinor: 1250,
		Category: marketplacev1.ProductCategory_PRODUCT_CATEGORY_LIFE,
		Status:   status, SellerId: seller,
		CreatedAt: timestamppb.New(time.Date(2026, 9, 1, 7, 0, 0, 0, time.UTC)),
	}
}

func TestFavoritesListUsesOneProductBatchAndReturnsCurrentStatuses(t *testing.T) {
	t.Parallel()

	favoritedAt := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	favorites := &favoriteClient{items: []*favoritev1.FavoriteItem{
		{ProductId: "p_reserved", FavoritedAt: timestamppb.New(favoritedAt)},
		{ProductId: "p_off_shelf", FavoritedAt: timestamppb.New(favoritedAt.Add(-time.Hour))},
	}, total: 2}
	marketplace := &marketplaceClient{products: map[string]*marketplacev1.ProductSummary{
		"p_reserved":  product("p_reserved", "u_seller_1", marketplacev1.ProductStatus_PRODUCT_STATUS_RESERVED),
		"p_off_shelf": product("p_off_shelf", "u_seller_2", marketplacev1.ProductStatus_PRODUCT_STATUS_OFF_SHELF),
	}}
	accounts := &accountClient{users: map[string]*accountv1.UserPublic{
		"u_seller_1": {Id: "u_seller_1", Nickname: "卖家一"},
		"u_seller_2": {Id: "u_seller_2", Nickname: "卖家二"},
	}}
	responder := &responseCapture{}
	aggregator := aggregation.New(accounts, marketplace, nil)
	h := handler.NewFavorites(favorites, aggregator, responder)

	h.List(httptest.NewRecorder(), request(http.MethodGet, "/favorites?page=2&page_size=10"))
	if responder.code != "" || responder.status != http.StatusOK {
		t.Fatalf("response = status %d code %s error %v", responder.status, responder.code, responder.err)
	}
	page, ok := responder.data.(dto.FavoritePage)
	if !ok {
		t.Fatalf("data type = %T, want dto.FavoritePage", responder.data)
	}
	if page.Page != 2 || page.PageSize != 10 || page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("page = %+v", page)
	}
	if page.Items[0].Product.Status != "RESERVED" || page.Items[1].Product.Status != "OFF_SHELF" {
		t.Fatalf("current statuses = %q, %q", page.Items[0].Product.Status, page.Items[1].Product.Status)
	}
	if page.Items[0].FavoritedAt != favoritedAt.Format(time.RFC3339) {
		t.Fatalf("favorited_at = %q", page.Items[0].FavoritedAt)
	}
	if favorites.listCalls != 1 || marketplace.calls != 1 || accounts.calls != 1 {
		t.Fatalf("calls favorite/marketplace/account = %d/%d/%d, want 1/1/1", favorites.listCalls, marketplace.calls, accounts.calls)
	}
	if len(marketplace.ids) != 2 || marketplace.ids[0] != "p_reserved" || marketplace.ids[1] != "p_off_shelf" {
		t.Fatalf("BatchGetProducts ids = %v", marketplace.ids)
	}
	if favorites.lastList.GetActorId() != testUserID || favorites.metadataActor != testUserID || favorites.metadataRequest != testRequestID {
		t.Fatalf("downstream identity request=%+v actor=%q request_id=%q", favorites.lastList, favorites.metadataActor, favorites.metadataRequest)
	}
}

func TestFavoritesListEmptyPageAvoidsAggregationRPCs(t *testing.T) {
	t.Parallel()

	favorites := &favoriteClient{}
	marketplace := &marketplaceClient{}
	accounts := &accountClient{}
	responder := &responseCapture{}
	h := handler.NewFavorites(favorites, aggregation.New(accounts, marketplace, nil), responder)
	h.List(httptest.NewRecorder(), request(http.MethodGet, "/favorites"))
	if responder.status != http.StatusOK {
		t.Fatalf("status = %d code = %s", responder.status, responder.code)
	}
	page := responder.data.(dto.FavoritePage)
	if page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("items = %#v, want non-nil empty slice", page.Items)
	}
	if marketplace.calls != 0 || accounts.calls != 0 {
		t.Fatalf("empty page made aggregation calls %d/%d", marketplace.calls, accounts.calls)
	}
}

func TestFavoritesListRejectsInvalidPaginationBeforeRPC(t *testing.T) {
	t.Parallel()

	favorites := &favoriteClient{}
	responder := &responseCapture{}
	h := handler.NewFavorites(favorites, aggregation.New(&accountClient{}, &marketplaceClient{}, nil), responder)
	h.List(httptest.NewRecorder(), request(http.MethodGet, "/favorites?page_size=101"))
	if responder.code != errs.CodeValidation || favorites.listCalls != 0 {
		t.Fatalf("code = %s list calls = %d, want validation before RPC", responder.code, favorites.listCalls)
	}
}

func TestFavoritesListFailsWhenRelationshipProductIsMissing(t *testing.T) {
	t.Parallel()

	favorites := &favoriteClient{items: []*favoritev1.FavoriteItem{{
		ProductId: "p_missing", FavoritedAt: timestamppb.Now(),
	}}, total: 1}
	responder := &responseCapture{}
	h := handler.NewFavorites(favorites, aggregation.New(&accountClient{}, &marketplaceClient{products: map[string]*marketplacev1.ProductSummary{}}, nil), responder)
	h.List(httptest.NewRecorder(), request(http.MethodGet, "/favorites"))
	if responder.code != errs.CodeInternal {
		t.Fatalf("code = %s, want INTERNAL_ERROR", responder.code)
	}
}

func TestFavoritesMutationsForwardActorAndRemainEmpty200(t *testing.T) {
	t.Parallel()

	favorites := &favoriteClient{}
	responder := &responseCapture{}
	h := handler.NewFavorites(favorites, aggregation.New(&accountClient{}, &marketplaceClient{}, nil), responder)

	add := request(http.MethodPut, "/favorites/p_1")
	add.SetPathValue("productId", "p_1")
	h.Add(httptest.NewRecorder(), add)
	if responder.status != http.StatusOK || favorites.lastAdd.GetActorId() != testUserID || favorites.lastAdd.GetProductId() != "p_1" {
		t.Fatalf("add response/request = %d %+v", responder.status, favorites.lastAdd)
	}

	remove := request(http.MethodDelete, "/favorites/p_1")
	remove.SetPathValue("productId", "p_1")
	h.Remove(httptest.NewRecorder(), remove)
	if responder.status != http.StatusOK || favorites.lastRemove.GetActorId() != testUserID || favorites.lastRemove.GetProductId() != "p_1" {
		t.Fatalf("remove response/request = %d %+v", responder.status, favorites.lastRemove)
	}
}

func TestFavoritesAddPreservesSelfActionError(t *testing.T) {
	t.Parallel()

	favorites := &favoriteClient{err: errs.New(errs.CodeSelfActionNotAllowed, "不能收藏自己发布的商品")}
	responder := &responseCapture{}
	h := handler.NewFavorites(favorites, aggregation.New(&accountClient{}, &marketplaceClient{}, nil), responder)
	req := request(http.MethodPut, "/favorites/p_own")
	req.SetPathValue("productId", "p_own")
	h.Add(httptest.NewRecorder(), req)
	if responder.code != errs.CodeSelfActionNotAllowed {
		t.Fatalf("code = %s, want SELF_ACTION_NOT_ALLOWED", responder.code)
	}
}
