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

const testReviewID = "rv_01ARZ3NDEKTSV4RRFFQ69G5FAV"

// reviewFixture 构造下游返回的评论表示。
func reviewFixture() *marketplacev1.Review {
	return &marketplacev1.Review{
		Id:        testReviewID,
		ProductId: testProductID,
		BuyerId:   testActor,
		Comment:   "很划算，成色如描述",
		CreatedAt: timestamppb.New(time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)),
	}
}

// CreateReview 和 ListProductReviews 的桩方法放在评论测试文件中，
// 使评论相关行为与商品桩的其他部分保持独立。
func (s *stubMarketplace) CreateReview(_ context.Context, req *marketplacev1.CreateReviewRequest, _ ...grpc.CallOption) (*marketplacev1.CreateReviewResponse, error) {
	s.lastCreateReview = req
	if s.createReviewErr != nil {
		return nil, s.createReviewErr
	}
	return s.createReviewResp, nil
}

func (s *stubMarketplace) ListProductReviews(_ context.Context, req *marketplacev1.ListProductReviewsRequest, _ ...grpc.CallOption) (*marketplacev1.ListProductReviewsResponse, error) {
	s.lastListReviews = req
	if s.listReviewsErr != nil {
		return nil, s.listReviewsErr
	}
	return s.listReviewsResp, nil
}

func TestCreateReviewReturns201WithTheContractShape(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{createReviewResp: &marketplacev1.CreateReviewResponse{Review: reviewFixture()}}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodPost,
		basePath+"/products/"+testProductID+"/reviews", token, `{"comment":"很划算，成色如描述"}`)
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", status, body)
	}

	var envelope struct {
		Data struct {
			ID        string `json:"id"`
			ProductID string `json:"product_id"`
			Buyer     struct {
				ID       string `json:"id"`
				Nickname string `json:"nickname"`
			} `json:"buyer"`
			Comment   string `json:"comment"`
			CreatedAt string `json:"created_at"`
		} `json:"data"`
	}
	decode(t, body, &envelope)

	if envelope.Data.ID != testReviewID || envelope.Data.ProductID != testProductID {
		t.Fatalf("identifiers = %s/%s, want %s", envelope.Data.ID, envelope.Data.ProductID, testReviewID)
	}
	if envelope.Data.Buyer.Nickname == "" {
		t.Fatalf("the buyer identity was not completed: %s", body)
	}
	if envelope.Data.Comment != "很划算，成色如描述" || envelope.Data.CreatedAt == "" {
		t.Fatalf("comment or created_at missing: %s", body)
	}
	if marketplace.lastCreateReview.GetActorId() != testActor {
		t.Fatalf("actor_id = %q, want the token subject", marketplace.lastCreateReview.GetActorId())
	}
	if marketplace.lastCreateReview.GetProductId() != testProductID {
		t.Fatalf("product_id = %q, want the path product", marketplace.lastCreateReview.GetProductId())
	}
	if marketplace.lastCreateReview.GetComment() != "很划算，成色如描述" {
		t.Fatalf("comment = %q, want the request body", marketplace.lastCreateReview.GetComment())
	}
}

func TestCreateReviewValidatesTheCommentBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "empty comment", body: `{"comment":""}`},
		{name: "too long comment", body: `{"comment":"` + strings.Repeat("好", 501) + `"}`},
		{name: "missing comment", body: `{}`},
		{name: "unknown field", body: `{"comment":"不错","rating":5}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			marketplace := &stubMarketplace{}
			server, token := newProductServer(t, &stubAccounts{}, marketplace)
			status, body := request(t, server, http.MethodPost,
				basePath+"/products/"+testProductID+"/reviews", token, tt.body)
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

func TestCreateReviewMapsDownstreamErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		downstream error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "duplicate review",
			downstream: errs.New(errs.CodeReviewAlreadyExists, "已经评论过该商品"),
			wantStatus: http.StatusConflict,
			wantCode:   "REVIEW_ALREADY_EXISTS",
		},
		{
			name:       "no completed trade",
			downstream: errs.New(errs.CodeForbidden, "只有完成交易的买家可以评论该商品"),
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "missing product",
			downstream: errs.New(errs.CodeResourceNotFound, "商品不存在"),
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
				basePath+"/products/"+testProductID+"/reviews", token, `{"comment":"不错"}`)
			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", status, tt.wantStatus, body)
			}
			if !strings.Contains(body, tt.wantCode) {
				t.Fatalf("error code %q missing: %s", tt.wantCode, body)
			}
		})
	}
}

func TestListProductReviewsRendersTheContractShape(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{listReviewsResp: &marketplacev1.ListProductReviewsResponse{
		Page: &marketplacev1.ReviewPage{
			Items: []*marketplacev1.Review{reviewFixture()}, Page: 2, PageSize: 20, Total: 1,
		},
	}}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodGet,
		basePath+"/products/"+testProductID+"/reviews?page=2&page_size=20", token, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}

	var envelope struct {
		Data struct {
			Items []struct {
				ID        string `json:"id"`
				ProductID string `json:"product_id"`
				Buyer     struct {
					ID       string `json:"id"`
					Nickname string `json:"nickname"`
				} `json:"buyer"`
				Comment   string `json:"comment"`
				CreatedAt string `json:"created_at"`
			} `json:"items"`
			Page     int32 `json:"page"`
			PageSize int32 `json:"page_size"`
			Total    int64 `json:"total"`
		} `json:"data"`
	}
	decode(t, body, &envelope)

	if len(envelope.Data.Items) != 1 || envelope.Data.Total != 1 {
		t.Fatalf("page = %+v, want one review", envelope.Data)
	}
	item := envelope.Data.Items[0]
	if item.ID != testReviewID || item.Buyer.Nickname == "" || item.Comment == "" || item.CreatedAt == "" {
		t.Fatalf("review item is incomplete: %s", body)
	}
	if marketplace.lastListReviews.GetPage() != 2 {
		t.Fatalf("page = %d, want the query parameter forwarded", marketplace.lastListReviews.GetPage())
	}
}

func TestListProductReviewsMapsNotFound(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{listReviewsErr: errs.New(errs.CodeResourceNotFound, "商品不存在")}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodGet,
		basePath+"/products/p_missing/reviews", token, "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", status, body)
	}
	if !strings.Contains(body, "RESOURCE_NOT_FOUND") {
		t.Fatalf("error code missing: %s", body)
	}
}
