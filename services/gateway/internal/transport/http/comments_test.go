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

const testCommentID = "cm_01ARZ3NDEKTSV4RRFFQ69G5FAV"

// commentFixture 构造下游返回的评论表示。
func commentFixture() *marketplacev1.ProductComment {
	return &marketplacev1.ProductComment{
		Id:        testCommentID,
		ProductId: testProductID,
		UserId:    testActor,
		Content:   "很划算，成色如描述",
		CreatedAt: timestamppb.New(time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)),
	}
}

// CreateProductComment 和 ListProductComments 的桩方法放在评论测试文件中，
// 使评论相关行为与商品桩的其他部分保持独立。
func (s *stubMarketplace) CreateProductComment(_ context.Context, req *marketplacev1.CreateProductCommentRequest, _ ...grpc.CallOption) (*marketplacev1.CreateProductCommentResponse, error) {
	s.lastCreateComment = req
	if s.createCommentErr != nil {
		return nil, s.createCommentErr
	}
	return s.createCommentResp, nil
}

func (s *stubMarketplace) ListProductComments(_ context.Context, req *marketplacev1.ListProductCommentsRequest, _ ...grpc.CallOption) (*marketplacev1.ListProductCommentsResponse, error) {
	s.lastListComments = req
	if s.listCommentsErr != nil {
		return nil, s.listCommentsErr
	}
	return s.listCommentsResp, nil
}

func TestCreateCommentReturns201WithTheContractShape(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{createCommentResp: &marketplacev1.CreateProductCommentResponse{Comment: commentFixture()}}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodPost,
		basePath+"/products/"+testProductID+"/comments", token, `{"content":"很划算，成色如描述"}`)
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", status, body)
	}

	var envelope struct {
		Data struct {
			ID        string `json:"id"`
			ProductID string `json:"product_id"`
			User      struct {
				ID       string `json:"id"`
				Nickname string `json:"nickname"`
			} `json:"user"`
			Content   string `json:"content"`
			CreatedAt string `json:"created_at"`
		} `json:"data"`
	}
	decode(t, body, &envelope)

	if envelope.Data.ID != testCommentID || envelope.Data.ProductID != testProductID {
		t.Fatalf("identifiers = %s/%s, want %s", envelope.Data.ID, envelope.Data.ProductID, testCommentID)
	}
	if envelope.Data.User.Nickname == "" {
		t.Fatalf("the commenter identity was not completed: %s", body)
	}
	if envelope.Data.Content != "很划算，成色如描述" || envelope.Data.CreatedAt == "" {
		t.Fatalf("content or created_at missing: %s", body)
	}
	if marketplace.lastCreateComment.GetActorId() != testActor {
		t.Fatalf("actor_id = %q, want the token subject", marketplace.lastCreateComment.GetActorId())
	}
	if marketplace.lastCreateComment.GetProductId() != testProductID {
		t.Fatalf("product_id = %q, want the path product", marketplace.lastCreateComment.GetProductId())
	}
	if marketplace.lastCreateComment.GetContent() != "很划算，成色如描述" {
		t.Fatalf("content = %q, want the request body", marketplace.lastCreateComment.GetContent())
	}
}

func TestCreateCommentValidatesTheContentBody(t *testing.T) {
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
				basePath+"/products/"+testProductID+"/comments", token, tt.body)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", status, body)
			}
			if !strings.Contains(body, "VALIDATION_ERROR") {
				t.Fatalf("error code missing: %s", body)
			}
			if marketplace.lastCreateComment != nil {
				t.Fatal("an invalid body was forwarded to the marketplace service")
			}
		})
	}
}

func TestCreateCommentMapsMissingProductTo404(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{createCommentErr: errs.New(errs.CodeResourceNotFound, "商品不存在")}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodPost,
		basePath+"/products/p_missing/comments", token, `{"content":"不错"}`)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", status, body)
	}
	if !strings.Contains(body, "RESOURCE_NOT_FOUND") {
		t.Fatalf("error code missing: %s", body)
	}
}

func TestListProductCommentsRendersTheContractShape(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{listCommentsResp: &marketplacev1.ListProductCommentsResponse{
		Page: &marketplacev1.ProductCommentPage{
			Items: []*marketplacev1.ProductComment{commentFixture()}, Page: 2, PageSize: 20, Total: 1,
		},
	}}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodGet,
		basePath+"/products/"+testProductID+"/comments?page=2&page_size=20", token, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}

	var envelope struct {
		Data struct {
			Items []struct {
				ID        string `json:"id"`
				ProductID string `json:"product_id"`
				User      struct {
					ID       string `json:"id"`
					Nickname string `json:"nickname"`
				} `json:"user"`
				Content   string `json:"content"`
				CreatedAt string `json:"created_at"`
			} `json:"items"`
			Page     int32 `json:"page"`
			PageSize int32 `json:"page_size"`
			Total    int64 `json:"total"`
		} `json:"data"`
	}
	decode(t, body, &envelope)

	if len(envelope.Data.Items) != 1 || envelope.Data.Total != 1 {
		t.Fatalf("page = %+v, want one comment", envelope.Data)
	}
	item := envelope.Data.Items[0]
	if item.ID != testCommentID || item.User.Nickname == "" || item.Content == "" || item.CreatedAt == "" {
		t.Fatalf("comment item is incomplete: %s", body)
	}
	if marketplace.lastListComments.GetPage() != 2 {
		t.Fatalf("page = %d, want the query parameter forwarded", marketplace.lastListComments.GetPage())
	}
}

func TestListProductCommentsMapsNotFound(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{listCommentsErr: errs.New(errs.CodeResourceNotFound, "商品不存在")}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodGet,
		basePath+"/products/p_missing/comments", token, "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", status, body)
	}
	if !strings.Contains(body, "RESOURCE_NOT_FOUND") {
		t.Fatalf("error code missing: %s", body)
	}
}
