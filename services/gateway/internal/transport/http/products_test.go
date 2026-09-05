package http_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/auth"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/application/aggregation"
	gatewayhttp "github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/handler"
)

const testProductID = "p_01ARZ3NDEKTSV4RRFFQ69G5FAV"

// newProductServer 将真实路由器与下游桩服务连接起来，因此断言针对的是 Gateway
// 必须实现的公开 HTTP 契约。
func newProductServer(t *testing.T, accounts *stubAccounts, marketplace *stubMarketplace) (*httptest.Server, string) {
	t.Helper()

	verifier, err := auth.NewVerifier(tokenConfig(), nil)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	issuer, err := auth.NewIssuer(tokenConfig(), nil, func() string { return "jti" })
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	token, _, err := issuer.Issue(testActor)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := gatewayhttp.NewResponder(logger)
	// 这个仅覆盖商品 HTTP 契约的夹具不需要收藏状态，因此省略 checker；
	// 生产接线始终使用真实 Favorite Service。
	aggregator := aggregation.New(accounts, marketplace, nil)

	router := gatewayhttp.NewRouter(gatewayhttp.RouterOptions{
		BasePath:     basePath,
		Verifier:     verifier,
		MaxBodyBytes: 1 << 20,
		Logger:       logger,
		Ready:        func(context.Context) error { return nil },
		Auth:         handler.NewAuth(accounts, responder),
		Users:        handler.NewUsers(accounts, responder),
		Products:     handler.NewProducts(marketplace, accounts, aggregator, responder),
		Trades:       handler.NewTrades(marketplace, aggregator, responder),
		Comments:     handler.NewComments(marketplace, aggregator, responder),
	})

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server, token
}

func TestListProductsRendersTheContractShape(t *testing.T) {
	t.Parallel()

	server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{})

	status, body := request(t, server, http.MethodGet, basePath+"/products?page=1&page_size=20", token, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}

	var envelope struct {
		Data struct {
			Items []struct {
				ID       string  `json:"id"`
				Price    string  `json:"price"`
				Category string  `json:"category"`
				Status   string  `json:"status"`
				CoverURL *string `json:"cover_url"`
				Seller   struct {
					ID       string `json:"id"`
					Nickname string `json:"nickname"`
				} `json:"seller"`
			} `json:"items"`
			Page     int32 `json:"page"`
			PageSize int32 `json:"page_size"`
			Total    int64 `json:"total"`
		} `json:"data"`
	}
	decode(t, body, &envelope)

	if len(envelope.Data.Items) != 1 {
		t.Fatalf("items = %d, want 1: %s", len(envelope.Data.Items), body)
	}
	item := envelope.Data.Items[0]
	// 12345 个最小货币单位必须渲染为十进制字符串，不能是数字。
	if item.Price != "123.45" {
		t.Fatalf("price = %q, want \"123.45\"", item.Price)
	}
	if item.Status != "ON_SALE" || item.Category != "DIGITAL" {
		t.Fatalf("status/category = %q/%q", item.Status, item.Category)
	}
	if item.Seller.Nickname == "" {
		t.Fatal("the seller identity was not completed")
	}
	if envelope.Data.Total != 1 {
		t.Fatalf("total = %d, want 1", envelope.Data.Total)
	}
	if strings.Contains(body, "student_no") {
		t.Fatalf("a product list disclosed a student number: %s", body)
	}
}

func TestListProductsCompletesSellersInOneBatchCall(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{listItems: 25}
	accounts := &stubAccounts{}
	server, token := newProductServer(t, accounts, marketplace)

	status, body := request(t, server, http.MethodGet, basePath+"/products?page_size=100", token, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}

	// 整页只调用一次，绝不按行调用。
	if accounts.batchCalls != 1 {
		t.Fatalf("BatchGetUsers was called %d times, want exactly 1", accounts.batchCalls)
	}
	if accounts.lastBatchSize > 2 {
		t.Fatalf("the batch carried %d ids, want the distinct sellers only", accounts.lastBatchSize)
	}
}

func TestListProductsRejectsInvalidQueryParameters(t *testing.T) {
	t.Parallel()

	server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{})

	tests := []struct {
		name  string
		query string
	}{
		{name: "page zero", query: "?page=0"},
		{name: "negative page", query: "?page=-1"},
		{name: "page is not a number", query: "?page=first"},
		{name: "page size above the maximum", query: "?page_size=101"},
		{name: "page size zero", query: "?page_size=0"},
		{name: "unknown category", query: "?category=BOOKS"},
		{name: "blank keyword", query: "?keyword=%20%20"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, body := request(t, server, http.MethodGet, basePath+"/products"+tt.query, token, "")
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", status, body)
			}
			assertErrorCode(t, body, errs.CodeValidation)
		})
	}
}

func TestProductEndpointsRequireAuthentication(t *testing.T) {
	t.Parallel()

	server, _ := newProductServer(t, &stubAccounts{}, &stubMarketplace{})

	// openapi/openapi.yaml 全局应用 bearerAuth；只有注册和登录例外。
	for _, path := range []string{"/products", "/products/" + testProductID, "/users/me/products"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			status, body := requestRaw(t, server, http.MethodGet, basePath+path, "", "")
			if status != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401: %s", status, body)
			}
		})
	}
}

func TestGetProductReturnsSellerContactWithoutTheStudentNumber(t *testing.T) {
	t.Parallel()

	server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{})

	status, body := request(t, server, http.MethodGet, basePath+"/products/"+testProductID, token, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}

	var envelope struct {
		Data struct {
			Status      string `json:"status"`
			Price       string `json:"price"`
			IsFavorited bool   `json:"is_favorited"`
			Images      []struct {
				URL       string `json:"url"`
				SortOrder int32  `json:"sort_order"`
			} `json:"images"`
			Seller struct {
				ID       string  `json:"id"`
				Nickname string  `json:"nickname"`
				Wechat   *string `json:"wechat"`
				QQ       *string `json:"qq"`
			} `json:"seller"`
		} `json:"data"`
	}
	decode(t, body, &envelope)

	if envelope.Data.Seller.Wechat == nil || *envelope.Data.Seller.Wechat == "" {
		t.Fatal("the detail response must carry the seller contact")
	}
	if strings.Contains(body, "student_no") || strings.Contains(body, "20260001") {
		t.Fatalf("the product detail disclosed the seller student number: %s", body)
	}
	// Favorite Service 尚未接入，因此标记存在且为 false，而不是缺失。
	if envelope.Data.IsFavorited {
		t.Fatal("is_favorited should be false while the Favorite Service is not wired")
	}
	if len(envelope.Data.Images) != 1 || envelope.Data.Images[0].SortOrder != 1 {
		t.Fatalf("images = %+v", envelope.Data.Images)
	}
}

func TestGetProductPropagatesNotFound(t *testing.T) {
	t.Parallel()

	server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{
		getErr: errs.New(errs.CodeResourceNotFound, "商品不存在"),
	})

	status, body := request(t, server, http.MethodGet, basePath+"/products/"+testProductID, token, "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", status, body)
	}
	assertErrorCode(t, body, errs.CodeResourceNotFound)
}

func TestCreateProductRequiresAPublishedContact(t *testing.T) {
	t.Parallel()

	server, token := newProductServer(t, &stubAccounts{withoutContact: true}, &stubMarketplace{})
	body, contentType := productForm(t, "机械键盘", "123.45", "DIGITAL", "九成新", 0)

	status, response := upload(t, server, http.MethodPost, basePath+"/products", token, contentType, body)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", status, response)
	}
	assertErrorCode(t, response, errs.CodeContactRequired)
}

func TestCreateProductAcceptsAMultipartListing(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)
	body, contentType := productForm(t, "机械键盘", "123.45", "DIGITAL", "九成新", 2)

	status, response := upload(t, server, http.MethodPost, basePath+"/products", token, contentType, body)
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", status, response)
	}

	req := marketplace.lastCreate
	if req == nil {
		t.Fatal("CreateProduct was not called")
	}
	if req.GetActorId() != testActor {
		t.Fatalf("actor_id = %q, want the token subject", req.GetActorId())
	}
	// 十进制字符串必须以最小货币单位的形式传到服务。
	if req.GetPriceMinor() != 12345 {
		t.Fatalf("price_minor = %d, want 12345", req.GetPriceMinor())
	}
	if req.GetCategory() != marketplacev1.ProductCategory_PRODUCT_CATEGORY_DIGITAL {
		t.Fatalf("category = %s", req.GetCategory())
	}
	if len(req.GetImages()) != 2 {
		t.Fatalf("images = %d, want 2", len(req.GetImages()))
	}
	for _, image := range req.GetImages() {
		if len(image.GetData()) == 0 {
			t.Fatal("an uploaded file arrived empty")
		}
	}
}

func TestCreateProductRejectsInvalidFormFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		title    string
		price    string
		category string
		wantCode errs.Code
	}{
		{name: "price is not decimal", title: "t", price: "12.345", category: "DIGITAL", wantCode: errs.CodeValidation},
		{name: "price is negative", title: "t", price: "-1", category: "DIGITAL", wantCode: errs.CodeValidation},
		{name: "price is missing", title: "t", price: "", category: "DIGITAL", wantCode: errs.CodeValidation},
		{name: "category is unknown", title: "t", price: "1.00", category: "BOOKS", wantCode: errs.CodeValidation},
		{name: "category is missing", title: "t", price: "1.00", category: "", wantCode: errs.CodeValidation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{})
			body, contentType := productForm(t, tt.title, tt.price, tt.category, "描述", 0)

			status, response := upload(t, server, http.MethodPost, basePath+"/products", token, contentType, body)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", status, response)
			}
			assertErrorCode(t, response, tt.wantCode)
		})
	}
}

func TestCreateProductEnforcesTheImageLimits(t *testing.T) {
	t.Parallel()

	t.Run("too many files", func(t *testing.T) {
		t.Parallel()

		server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{})
		body, contentType := productForm(t, "机械键盘", "1.00", "DIGITAL", "描述", 4)

		status, response := upload(t, server, http.MethodPost, basePath+"/products", token, contentType, body)
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", status, response)
		}
		assertErrorCode(t, response, errs.CodeImageLimitExceeded)
	})

	t.Run("file over five mebibytes", func(t *testing.T) {
		t.Parallel()

		server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{})
		body, contentType := oversizedProductForm(t)

		status, response := upload(t, server, http.MethodPost, basePath+"/products", token, contentType, body)
		if status != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413: %s", status, response)
		}
		assertErrorCode(t, response, errs.CodePayloadTooLarge)
	})
}

func TestCreateProductRejectsANonMultipartBody(t *testing.T) {
	t.Parallel()

	server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{})

	status, body := request(t, server, http.MethodPost, basePath+"/products", token, `{"title":"x"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", status, body)
	}
	assertErrorCode(t, body, errs.CodeValidation)
}

func TestUpdateProductValidatesTheBody(t *testing.T) {
	t.Parallel()

	server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{})

	tests := []struct {
		name string
		body string
	}{
		{name: "empty object", body: `{}`},
		{name: "unknown field", body: `{"status":"SOLD"}`},
		{name: "bad price", body: `{"price":"12.345"}`},
		{name: "unknown category", body: `{"category":"BOOKS"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, response := request(t, server, http.MethodPatch, basePath+"/products/"+testProductID, token, tt.body)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", status, response)
			}
			assertErrorCode(t, response, errs.CodeValidation)
		})
	}
}

func TestUpdateProductCannotAssignStatus(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	// 契约的 ProductUpdateRequest 设置 additionalProperties: false，且没有 status
	// 属性，因此尝试设置 status 必须被拒绝，不能静默忽略。
	status, body := request(t, server, http.MethodPatch, basePath+"/products/"+testProductID, token,
		`{"title":"新标题","status":"SOLD"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", status, body)
	}
	if marketplace.lastUpdate != nil {
		t.Fatal("a rejected body still reached the Marketplace Service")
	}
}

func TestProductActionsMapDownstreamStateConflicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		err        error
		wantStatus int
		wantCode   errs.Code
	}{
		{
			name: "off shelf a reserved product", method: http.MethodPost, path: "/off-shelf",
			err:        errs.New(errs.CodeProductStateConflict, "当前商品状态不允许执行该操作"),
			wantStatus: http.StatusConflict, wantCode: errs.CodeProductStateConflict,
		},
		{
			name: "relist by a non-seller", method: http.MethodPost, path: "/relist",
			err:        errs.New(errs.CodeForbidden, "无权执行该操作"),
			wantStatus: http.StatusForbidden, wantCode: errs.CodeForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{actionErr: tt.err})
			status, body := request(t, server, tt.method, basePath+"/products/"+testProductID+tt.path, token, "")

			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", status, tt.wantStatus, body)
			}
			assertErrorCode(t, body, tt.wantCode)
		})
	}
}

func TestListMineFiltersByStatus(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodGet, basePath+"/users/me/products?status=OFF_SHELF", token, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}

	req := marketplace.lastListUser
	if req == nil {
		t.Fatal("ListUserProducts was not called")
	}
	if req.GetSellerId() != testActor {
		t.Fatalf("seller_id = %q, want the token subject", req.GetSellerId())
	}
	if req.GetStatus() != marketplacev1.ProductStatus_PRODUCT_STATUS_OFF_SHELF {
		t.Fatalf("status filter = %s, want OFF_SHELF", req.GetStatus())
	}
}

func TestListMineRejectsAnUnknownStatus(t *testing.T) {
	t.Parallel()

	server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{})

	status, body := request(t, server, http.MethodGet, basePath+"/users/me/products?status=PENDING", token, "")
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", status, body)
	}
	assertErrorCode(t, body, errs.CodeValidation)
}

// --- multipart 辅助函数 ----------------------------------------------------

func productForm(t *testing.T, title, price, category, description string, images int) (*bytes.Buffer, string) {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for name, value := range map[string]string{
		"title": title, "price": price, "category": category, "description": description,
	} {
		if value == "" {
			continue
		}
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("write field %s: %v", name, err)
		}
	}
	for range images {
		part, err := writer.CreateFormFile("images", "photo.png")
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write(smallPNG(t)); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return &buf, writer.FormDataContentType()
}

func oversizedProductForm(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for name, value := range map[string]string{
		"title": "机械键盘", "price": "1.00", "category": "DIGITAL", "description": "描述",
	} {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("write field %s: %v", name, err)
		}
	}
	part, err := writer.CreateFormFile("images", "huge.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(bytes.Repeat([]byte{0x89}, handler.MaxImageBytes+1)); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return &buf, writer.FormDataContentType()
}

func upload(t *testing.T, server *httptest.Server, method, path, token, contentType string, body *bytes.Buffer) (int, string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, server.URL+path, body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(payload)
}

func smallPNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// --- Marketplace 桩实现 -----------------------------------------------------

// stubMarketplace 代替 Marketplace Service。该服务自身的行为由基于真实数据库和
// 对象存储的集成测试覆盖。
type stubMarketplace struct {
	marketplacev1.MarketplaceServiceClient

	listItems    int
	tradeItems   int
	getErr       error
	actionErr    error
	tradeErr     error
	replayed     bool
	tradeExists  bool
	lastCreate   *marketplacev1.CreateProductRequest
	lastUpdate   *marketplacev1.UpdateProductRequest
	lastListUser *marketplacev1.ListUserProductsRequest

	lastCreateTrade *marketplacev1.CreateTradeRequest
	lastAccept      *marketplacev1.AcceptTradeRequest
	lastListTrades  *marketplacev1.ListTradesRequest

	createCommentResp *marketplacev1.CreateProductCommentResponse
	createCommentErr  error
	listCommentsResp  *marketplacev1.ListProductCommentsResponse
	listCommentsErr   error
	lastCreateComment *marketplacev1.CreateProductCommentRequest
	lastListComments  *marketplacev1.ListProductCommentsRequest
}

func (s *stubMarketplace) summary(id, sellerID string) *marketplacev1.ProductSummary {
	cover := "https://media.example.test/products/" + id + "/cover.png"
	return &marketplacev1.ProductSummary{
		Id:         id,
		Title:      "机械键盘",
		PriceMinor: 12345,
		Category:   marketplacev1.ProductCategory_PRODUCT_CATEGORY_DIGITAL,
		CoverUrl:   &cover,
		Status:     marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE,
		SellerId:   sellerID,
		CreatedAt:  timestamppb.New(time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)),
	}
}

func (s *stubMarketplace) detail() *marketplacev1.ProductDetail {
	return &marketplacev1.ProductDetail{
		Id:          testProductID,
		Title:       "机械键盘",
		PriceMinor:  12345,
		Category:    marketplacev1.ProductCategory_PRODUCT_CATEGORY_DIGITAL,
		Description: "九成新",
		Status:      marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE,
		Images: []*marketplacev1.ProductImage{{
			Id:        "img_1",
			Url:       "https://media.example.test/products/" + testProductID + "/img_1.png",
			SortOrder: 1,
			CreatedAt: timestamppb.New(time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)),
		}},
		SellerId:  testActor,
		CreatedAt: timestamppb.New(time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)),
		UpdatedAt: timestamppb.New(time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)),
	}
}

func (s *stubMarketplace) page() *marketplacev1.ProductPage {
	count := s.listItems
	if count == 0 {
		count = 1
	}
	items := make([]*marketplacev1.ProductSummary, 0, count)
	for i := range count {
		// 页面中重复出现两个不同卖家，这样测试可以证明批量调用携带了不同的标识，
		// 而不是每行调用一次。
		sellerID := testActor
		if i%2 == 1 {
			sellerID = "u_second_seller"
		}
		items = append(items, s.summary(testProductID, sellerID))
	}
	return &marketplacev1.ProductPage{Items: items, Page: 1, PageSize: 20, Total: int64(count)}
}

func (s *stubMarketplace) CreateProduct(_ context.Context, req *marketplacev1.CreateProductRequest, _ ...grpc.CallOption) (*marketplacev1.CreateProductResponse, error) {
	s.lastCreate = req
	return &marketplacev1.CreateProductResponse{Product: s.detail()}, nil
}

func (s *stubMarketplace) GetProduct(_ context.Context, _ *marketplacev1.GetProductRequest, _ ...grpc.CallOption) (*marketplacev1.GetProductResponse, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return &marketplacev1.GetProductResponse{Product: s.detail()}, nil
}

func (s *stubMarketplace) ListProducts(_ context.Context, _ *marketplacev1.ListProductsRequest, _ ...grpc.CallOption) (*marketplacev1.ListProductsResponse, error) {
	return &marketplacev1.ListProductsResponse{Page: s.page()}, nil
}

func (s *stubMarketplace) ListUserProducts(_ context.Context, req *marketplacev1.ListUserProductsRequest, _ ...grpc.CallOption) (*marketplacev1.ListUserProductsResponse, error) {
	s.lastListUser = req
	return &marketplacev1.ListUserProductsResponse{Page: s.page()}, nil
}

func (s *stubMarketplace) BatchGetProducts(_ context.Context, req *marketplacev1.BatchGetProductsRequest, _ ...grpc.CallOption) (*marketplacev1.BatchGetProductsResponse, error) {
	products := make(map[string]*marketplacev1.ProductSummary, len(req.GetProductIds()))
	for _, id := range req.GetProductIds() {
		products[id] = s.summary(id, testActor)
	}
	return &marketplacev1.BatchGetProductsResponse{Products: products}, nil
}

func (s *stubMarketplace) UpdateProduct(_ context.Context, req *marketplacev1.UpdateProductRequest, _ ...grpc.CallOption) (*marketplacev1.UpdateProductResponse, error) {
	s.lastUpdate = req
	return &marketplacev1.UpdateProductResponse{Product: s.detail()}, nil
}

func (s *stubMarketplace) OffShelfProduct(_ context.Context, _ *marketplacev1.OffShelfProductRequest, _ ...grpc.CallOption) (*marketplacev1.OffShelfProductResponse, error) {
	if s.actionErr != nil {
		return nil, s.actionErr
	}
	return &marketplacev1.OffShelfProductResponse{Product: s.detail()}, nil
}

func (s *stubMarketplace) RelistProduct(_ context.Context, _ *marketplacev1.RelistProductRequest, _ ...grpc.CallOption) (*marketplacev1.RelistProductResponse, error) {
	if s.actionErr != nil {
		return nil, s.actionErr
	}
	return &marketplacev1.RelistProductResponse{Product: s.detail()}, nil
}

func (s *stubMarketplace) AddProductImages(_ context.Context, _ *marketplacev1.AddProductImagesRequest, _ ...grpc.CallOption) (*marketplacev1.AddProductImagesResponse, error) {
	return &marketplacev1.AddProductImagesResponse{Images: s.detail().GetImages()}, nil
}

func (s *stubMarketplace) DeleteProductImage(_ context.Context, _ *marketplacev1.DeleteProductImageRequest, _ ...grpc.CallOption) (*marketplacev1.DeleteProductImageResponse, error) {
	return &marketplacev1.DeleteProductImageResponse{}, nil
}

var _ accountv1.AccountServiceClient = (*stubAccounts)(nil)
