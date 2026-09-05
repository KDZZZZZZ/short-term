package http_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
)

const testTradeReviewID = "tr_01ARZ3NDEKTSV4RRFFQ69G5FAV"

// tradeReviewFixture 构造下游返回的买家评价表示。
func tradeReviewFixture() *marketplacev1.TradeReview {
	return &marketplacev1.TradeReview{
		Id:        testTradeReviewID,
		TradeId:   testTradeID,
		ProductId: testProductID,
		BuyerId:   testActor,
		Content:   "交易愉快，卖家很耐心",
		CreatedAt: timestamppb.New(time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)),
	}
}

// CreateTradeReview 和 BatchGetProductTradeReviews 的桩方法放在评价测试
// 文件中，使评价相关行为与商品桩的其他部分保持独立。
func (s *stubMarketplace) CreateTradeReview(_ context.Context, req *marketplacev1.CreateTradeReviewRequest, _ ...grpc.CallOption) (*marketplacev1.CreateTradeReviewResponse, error) {
	s.lastCreateReview = req
	if s.createReviewErr != nil {
		return nil, s.createReviewErr
	}
	return s.createReviewResp, nil
}

func (s *stubMarketplace) BatchGetProductTradeReviews(_ context.Context, req *marketplacev1.BatchGetProductTradeReviewsRequest, _ ...grpc.CallOption) (*marketplacev1.BatchGetProductTradeReviewsResponse, error) {
	s.lastBatchReviews = req
	if s.batchReviewsErr != nil {
		return nil, s.batchReviewsErr
	}
	reviews := make(map[string]*marketplacev1.TradeReview, len(s.tradeReviews))
	for id, review := range s.tradeReviews {
		reviews[id] = review
	}
	return &marketplacev1.BatchGetProductTradeReviewsResponse{Reviews: reviews}, nil
}

func TestCreateTradeReviewReturns201WithTheContractShape(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{createReviewResp: &marketplacev1.CreateTradeReviewResponse{Review: tradeReviewFixture()}}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodPost,
		basePath+"/trades/"+testTradeID+"/review", token, `{"content":"交易愉快，卖家很耐心"}`)
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", status, body)
	}

	var envelope struct {
		Data struct {
			ID        string `json:"id"`
			TradeID   string `json:"trade_id"`
			ProductID string `json:"product_id"`
			Buyer     struct {
				ID       string `json:"id"`
				Nickname string `json:"nickname"`
			} `json:"buyer"`
			Content   string `json:"content"`
			CreatedAt string `json:"created_at"`
		} `json:"data"`
	}
	decode(t, body, &envelope)

	if envelope.Data.ID != testTradeReviewID || envelope.Data.TradeID != testTradeID {
		t.Fatalf("identifiers = %s/%s, want %s/%s", envelope.Data.ID, envelope.Data.TradeID, testTradeReviewID, testTradeID)
	}
	if envelope.Data.Buyer.Nickname == "" {
		t.Fatalf("the buyer identity was not completed: %s", body)
	}
	if marketplace.lastCreateReview.GetActorId() != testActor {
		t.Fatalf("actor_id = %q, want the token subject", marketplace.lastCreateReview.GetActorId())
	}
	if marketplace.lastCreateReview.GetTradeId() != testTradeID {
		t.Fatalf("trade_id = %q, want the path trade", marketplace.lastCreateReview.GetTradeId())
	}
}

func TestCreateTradeReviewValidatesTheContentBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "empty content", body: `{"content":""}`},
		{name: "too long content", body: `{"content":"` + strings.Repeat("好", 501) + `"}`},
		{name: "missing content", body: `{}`},
		{name: "unknown field", body: `{"content":"不错","rating":5}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			marketplace := &stubMarketplace{}
			server, token := newProductServer(t, &stubAccounts{}, marketplace)
			status, body := request(t, server, http.MethodPost,
				basePath+"/trades/"+testTradeID+"/review", token, tt.body)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", status, body)
			}
			if !strings.Contains(body, "VALIDATION_ERROR") {
				t.Fatalf("error code missing: %s", body)
			}
			if marketplace.lastCreateReview != nil {
				t.Fatal("an invalid body was forwarded to the marketplace service")
			}
		})
	}
}

func TestCreateTradeReviewMapsDownstreamErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		downstream error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "duplicate review",
			downstream: errs.New(errs.CodeTradeReviewExists, "该交易已经收到买家评价"),
			wantStatus: http.StatusConflict,
			wantCode:   "TRADE_REVIEW_ALREADY_EXISTS",
		},
		{
			name:       "trade not completed",
			downstream: errs.New(errs.CodeForbidden, "交易完成后才能发布评价"),
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "hidden trade",
			downstream: errs.New(errs.CodeResourceNotFound, "交易不存在"),
			wantStatus: http.StatusNotFound,
			wantCode:   "RESOURCE_NOT_FOUND",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			marketplace := &stubMarketplace{createReviewErr: tt.downstream}
			server, token := newProductServer(t, &stubAccounts{}, marketplace)
			status, body := request(t, server, http.MethodPost,
				basePath+"/trades/"+testTradeID+"/review", token, `{"content":"不错"}`)
			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", status, tt.wantStatus, body)
			}
			if !strings.Contains(body, tt.wantCode) {
				t.Fatalf("error code %q missing: %s", tt.wantCode, body)
			}
		})
	}
}

func TestProductDetailIncludesBuyerReviewForSoldProducts(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{
		sold:         true,
		tradeReviews: map[string]*marketplacev1.TradeReview{testProductID: tradeReviewFixture()},
	}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodGet, basePath+"/products/"+testProductID, token, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}

	var envelope struct {
		Data struct {
			Status      string `json:"status"`
			BuyerReview *struct {
				ID    string `json:"id"`
				Buyer struct {
					Nickname string `json:"nickname"`
				} `json:"buyer"`
				Content string `json:"content"`
			} `json:"buyer_review"`
		} `json:"data"`
	}
	decode(t, body, &envelope)

	if envelope.Data.Status != "SOLD" {
		t.Fatalf("status = %q, want SOLD", envelope.Data.Status)
	}
	if envelope.Data.BuyerReview == nil || envelope.Data.BuyerReview.ID != testTradeReviewID {
		t.Fatalf("buyer_review missing: %s", body)
	}
	if envelope.Data.BuyerReview.Buyer.Nickname == "" || envelope.Data.BuyerReview.Content == "" {
		t.Fatalf("buyer_review is incomplete: %s", body)
	}
}

func TestProductDetailExposesNullBuyerReviewWithoutOne(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{sold: true}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodGet, basePath+"/products/"+testProductID, token, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
	if !strings.Contains(body, `"buyer_review":null`) {
		t.Fatalf("buyer_review must be present as null: %s", body)
	}
}

func TestListMyProductsEmbedsBuyerReviews(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{
		tradeReviews: map[string]*marketplacev1.TradeReview{testProductID: tradeReviewFixture()},
	}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodGet, basePath+"/users/me/products", token, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}

	var envelope struct {
		Data struct {
			Items []struct {
				ID          string `json:"id"`
				BuyerReview *struct {
					ID    string `json:"id"`
					Buyer struct {
						Nickname string `json:"nickname"`
					} `json:"buyer"`
				} `json:"buyer_review"`
			} `json:"items"`
		} `json:"data"`
	}
	decode(t, body, &envelope)

	if len(envelope.Data.Items) == 0 {
		t.Fatalf("my products page is empty: %s", body)
	}
	item := envelope.Data.Items[0]
	if item.ID != testProductID {
		t.Fatalf("item id = %q, want %q", item.ID, testProductID)
	}
	if item.BuyerReview == nil || item.BuyerReview.ID != testTradeReviewID || item.BuyerReview.Buyer.Nickname == "" {
		t.Fatalf("buyer_review is missing or incomplete: %s", body)
	}
	if got := marketplace.lastBatchReviews.GetProductIds(); len(got) != 1 || got[0] != testProductID {
		t.Fatalf("batch product ids = %v, want the page products", got)
	}
}

func TestListByUserReturnsPublicSellerProducts(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodGet, basePath+"/users/u_seller/products", token, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}

	req := marketplace.lastListUser
	if req == nil {
		t.Fatal("ListUserProducts was not called")
	}
	if req.GetSellerId() != "u_seller" {
		t.Fatalf("seller_id = %q, want the path user", req.GetSellerId())
	}
	statuses := req.GetStatuses()
	if len(statuses) != 2 ||
		statuses[0] != marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE ||
		statuses[1] != marketplacev1.ProductStatus_PRODUCT_STATUS_SOLD {
		t.Fatalf("statuses = %v, want ON_SALE and SOLD", statuses)
	}
}

func TestListByUserRejectsAnUnknownUser(t *testing.T) {
	t.Parallel()

	server, token := newProductServer(t, &stubAccounts{missingUser: true}, &stubMarketplace{})

	status, body := request(t, server, http.MethodGet, basePath+"/users/u_missing/products", token, "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", status, body)
	}
	if !strings.Contains(body, "RESOURCE_NOT_FOUND") {
		t.Fatalf("error code missing: %s", body)
	}
}
