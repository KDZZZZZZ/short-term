package handler

import (
	"context"
	"net/http"
	"strconv"

	favoritev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/favorite/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/application/aggregation"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/mapper"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/middleware"
)

// Favorites implements the public /favorites endpoints.
type Favorites struct {
	favorites  favoritev1.FavoriteServiceClient
	aggregator *aggregation.Aggregator
	responder  Responder
}

// NewFavorites constructs the Favorite HTTP handler.
func NewFavorites(favorites favoritev1.FavoriteServiceClient, aggregator *aggregation.Aggregator, responder Responder) *Favorites {
	return &Favorites{favorites: favorites, aggregator: aggregator, responder: responder}
}

// List loads relationship facts first, then uses one Marketplace batch and one
// Account batch to assemble current ProductSummary values without N+1 calls.
func (h *Favorites) List(w http.ResponseWriter, r *http.Request) {
	page, size, ok := h.pagination(w, r)
	if !ok {
		return
	}
	actorID := middleware.ActorID(r.Context())
	ctx := h.downstream(r)
	resp, err := h.favorites.ListFavorites(ctx, &favoritev1.ListFavoritesRequest{
		ActorId: actorID, Page: page, PageSize: size,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	relationPage := resp.GetPage()
	if relationPage == nil {
		h.responder.Error(w, r, errs.New(errs.CodeInternal, "服务暂时不可用"))
		return
	}

	productIDs := make([]string, 0, len(relationPage.GetItems()))
	for _, item := range relationPage.GetItems() {
		productIDs = append(productIDs, item.GetProductId())
	}
	products, err := h.aggregator.Products(ctx, productIDs)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	sellerIDs := make([]string, 0, len(products))
	for _, productID := range productIDs {
		product := products[productID]
		if product == nil {
			h.responder.Error(w, r, errs.New(errs.CodeInternal, "服务暂时不可用"))
			return
		}
		sellerIDs = append(sellerIDs, product.GetSellerId())
	}
	sellers, err := h.aggregator.Users(ctx, sellerIDs)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	mapped, err := mapper.FavoritePage(relationPage, products, sellers)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	h.responder.OK(w, r, mapped)
}

// Add handles an idempotent PUT /favorites/{productId}.
func (h *Favorites) Add(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.ActorID(r.Context())
	_, err := h.favorites.AddFavorite(h.downstream(r), &favoritev1.AddFavoriteRequest{
		ActorId: actorID, ProductId: r.PathValue("productId"),
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	h.responder.Empty(w, r)
}

// Remove handles an idempotent DELETE /favorites/{productId}.
func (h *Favorites) Remove(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.ActorID(r.Context())
	_, err := h.favorites.RemoveFavorite(h.downstream(r), &favoritev1.RemoveFavoriteRequest{
		ActorId: actorID, ProductId: r.PathValue("productId"),
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	h.responder.Empty(w, r)
}

func (h *Favorites) pagination(w http.ResponseWriter, r *http.Request) (page, size int32, ok bool) {
	page, ok = h.intQuery(w, r, "page", 1, 1, 0)
	if !ok {
		return 0, 0, false
	}
	size, ok = h.intQuery(w, r, "page_size", 20, 1, 100)
	if !ok {
		return 0, 0, false
	}
	return page, size, true
}

func (h *Favorites) intQuery(w http.ResponseWriter, r *http.Request, name string, fallback, minimum, maximum int32) (int32, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || int32(value) < minimum || (maximum > 0 && int32(value) > maximum) {
		h.responder.Fail(w, r, errs.CodeValidation, name+" 参数不合法")
		return 0, false
	}
	return int32(value), true
}

func (h *Favorites) downstream(r *http.Request) context.Context {
	ctx := grpcx.WithActor(r.Context(), middleware.ActorID(r.Context()))
	return grpcx.WithRequestID(ctx, middleware.RequestID(r.Context()))
}
