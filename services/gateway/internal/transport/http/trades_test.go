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
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/handler"
)

const testTradeID = "t_01ARZ3NDEKTSV4RRFFQ69G5FAV"

func TestCreateTradeReturns201WithTheContractShape(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodPost, basePath+"/products/"+testProductID+"/trades", token, "")
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", status, body)
	}

	var envelope struct {
		Data struct {
			ID            string  `json:"id"`
			PriceSnapshot string  `json:"price_snapshot"`
			Status        string  `json:"status"`
			CancelReason  *string `json:"cancel_reason"`
			AcceptedAt    *string `json:"accepted_at"`
			Product       struct {
				Status string `json:"status"`
			} `json:"product"`
			Buyer struct {
				ID       string `json:"id"`
				Nickname string `json:"nickname"`
			} `json:"buyer"`
			Seller struct {
				ID string `json:"id"`
			} `json:"seller"`
		} `json:"data"`
	}
	decode(t, body, &envelope)

	if envelope.Data.PriceSnapshot != "123.45" {
		t.Fatalf("price_snapshot = %q, want a decimal string", envelope.Data.PriceSnapshot)
	}
	if envelope.Data.Status != "PENDING" || envelope.Data.Product.Status != "ON_SALE" {
		t.Fatalf("statuses = %q/%q", envelope.Data.Status, envelope.Data.Product.Status)
	}
	if envelope.Data.Buyer.Nickname == "" || envelope.Data.Seller.ID == "" {
		t.Fatalf("the trade parties were not completed: %s", body)
	}
	// Nullable fields must be present as null, not omitted.
	for _, field := range []string{"cancel_reason", "accepted_at", "completed_at", "cancelled_at", "conversation_id"} {
		if !strings.Contains(body, `"`+field+`"`) {
			t.Fatalf("response omits the required field %q: %s", field, body)
		}
	}
}

func TestCreateTradeAcceptsAnOptionalBodyAndForwardsTheConversation(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodPost, basePath+"/products/"+testProductID+"/trades", token,
		`{"conversation_id":"c_01ARZ3NDEKTSV4RRFFQ69G5FAV"}`)
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", status, body)
	}
	if marketplace.lastCreateTrade.GetConversationId() != "c_01ARZ3NDEKTSV4RRFFQ69G5FAV" {
		t.Fatalf("conversation_id = %q", marketplace.lastCreateTrade.GetConversationId())
	}
	if !marketplace.lastCreateTrade.GetConversationIdPresent() {
		t.Fatal("an explicit conversation_id was forwarded as omitted")
	}
	if marketplace.lastCreateTrade.GetActorId() != testActor {
		t.Fatalf("actor_id = %q, want the token subject", marketplace.lastCreateTrade.GetActorId())
	}
}

func TestCreateTradePreservesOmittedAndExplicitNullConversation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		wantPresent bool
	}{
		{name: "omitted body", body: "", wantPresent: false},
		{name: "empty object", body: `{}`, wantPresent: false},
		{name: "explicit null", body: `{"conversation_id":null}`, wantPresent: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			marketplace := &stubMarketplace{}
			server, token := newProductServer(t, &stubAccounts{}, marketplace)
			status, body := request(t, server, http.MethodPost,
				basePath+"/products/"+testProductID+"/trades", token, tt.body)
			if status != http.StatusCreated {
				t.Fatalf("status = %d, want 201: %s", status, body)
			}
			if got := marketplace.lastCreateTrade.GetConversationIdPresent(); got != tt.wantPresent {
				t.Fatalf("conversation_id_present = %v, want %v", got, tt.wantPresent)
			}
			if marketplace.lastCreateTrade.ConversationId != nil {
				t.Fatalf("conversation_id = %v, want nil", marketplace.lastCreateTrade.ConversationId)
			}
		})
	}
}

func TestCreateTradeReturns200ForAnExistingIntent(t *testing.T) {
	t.Parallel()

	server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{tradeExists: true})
	status, body := request(t, server, http.MethodPost,
		basePath+"/products/"+testProductID+"/trades", token, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
}

func TestIdempotencyKeyIsForwardedAndValidated(t *testing.T) {
	t.Parallel()

	validKey := "b8f0c1d2e3a4b5c6d7e8f9a0"

	t.Run("forwarded when valid", func(t *testing.T) {
		t.Parallel()

		marketplace := &stubMarketplace{}
		server, token := newProductServer(t, &stubAccounts{}, marketplace)

		status, body := requestWithHeaders(t, server, http.MethodPost,
			basePath+"/trades/"+testTradeID+"/accept", token, "",
			map[string]string{handler.IdempotencyKeyHeader: validKey})
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", status, body)
		}
		if marketplace.lastAccept.GetIdempotencyKey() != validKey {
			t.Fatalf("idempotency_key = %q, want it forwarded", marketplace.lastAccept.GetIdempotencyKey())
		}
	})

	t.Run("unicode length follows the OpenAPI character count", func(t *testing.T) {
		t.Parallel()

		marketplace := &stubMarketplace{}
		server, token := newProductServer(t, &stubAccounts{}, marketplace)
		unicodeKey := strings.Repeat("幂", 16)

		status, body := requestWithHeaders(t, server, http.MethodPost,
			basePath+"/trades/"+testTradeID+"/accept", token, "",
			map[string]string{handler.IdempotencyKeyHeader: unicodeKey})
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", status, body)
		}
		if marketplace.lastAccept.GetIdempotencyKey() != unicodeKey {
			t.Fatalf("idempotency_key = %q, want it forwarded", marketplace.lastAccept.GetIdempotencyKey())
		}
	})

	t.Run("absent when not sent", func(t *testing.T) {
		t.Parallel()

		marketplace := &stubMarketplace{}
		server, token := newProductServer(t, &stubAccounts{}, marketplace)

		if status, body := request(t, server, http.MethodPost, basePath+"/trades/"+testTradeID+"/accept", token, ""); status != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", status, body)
		}
		if marketplace.lastAccept.IdempotencyKey != nil {
			t.Fatal("an absent header became a key")
		}
	})

	t.Run("rejected when too short", func(t *testing.T) {
		t.Parallel()

		marketplace := &stubMarketplace{}
		server, token := newProductServer(t, &stubAccounts{}, marketplace)

		// Silently dropping a malformed key would turn a retry the client
		// believes is safe into a second command.
		status, body := requestWithHeaders(t, server, http.MethodPost,
			basePath+"/trades/"+testTradeID+"/accept", token, "",
			map[string]string{handler.IdempotencyKeyHeader: "short"})
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", status, body)
		}
		assertErrorCode(t, body, errs.CodeValidation)
		if marketplace.lastAccept != nil {
			t.Fatal("a rejected key still reached the Marketplace Service")
		}
	})
}

func TestReplayedCommandsKeepTheFirstStatusCode(t *testing.T) {
	t.Parallel()

	// A replay must be indistinguishable from the first response, so create
	// still answers 201 and accept still answers 200.
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{name: "create", method: http.MethodPost, path: "/products/" + testProductID + "/trades", wantStatus: http.StatusCreated},
		{name: "accept", method: http.MethodPost, path: "/trades/" + testTradeID + "/accept", wantStatus: http.StatusOK},
		{name: "confirm", method: http.MethodPost, path: "/trades/" + testTradeID + "/confirm", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{replayed: true})
			status, body := request(t, server, tt.method, basePath+tt.path, token, tt.body)

			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", status, tt.wantStatus, body)
			}
			// The replay flag is internal; it must not appear in the public body.
			if strings.Contains(body, "replayed") {
				t.Fatalf("the public response leaked the replay flag: %s", body)
			}
		})
	}
}

func TestTradeActionsMapDomainErrorsToTheContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   errs.Code
	}{
		{name: "participant has the wrong role", err: errs.New(errs.CodeForbidden, "无权执行该操作"), wantStatus: http.StatusForbidden, wantCode: errs.CodeForbidden},
		{name: "not a party", err: errs.New(errs.CodeResourceNotFound, "交易不存在"), wantStatus: http.StatusNotFound, wantCode: errs.CodeResourceNotFound},
		{name: "trade already moved on", err: errs.New(errs.CodeTradeStateConflict, "当前交易状态不允许执行该操作"), wantStatus: http.StatusConflict, wantCode: errs.CodeTradeStateConflict},
		{name: "product unavailable", err: errs.New(errs.CodeProductNotAvailable, "商品当前不可交易"), wantStatus: http.StatusConflict, wantCode: errs.CodeProductNotAvailable},
		{name: "missing trade", err: errs.New(errs.CodeResourceNotFound, "交易不存在"), wantStatus: http.StatusNotFound, wantCode: errs.CodeResourceNotFound},
		{name: "conversation mismatch", err: errs.New(errs.CodeConversationMismatch, "会话不匹配"), wantStatus: http.StatusConflict, wantCode: errs.CodeConversationMismatch},
		{name: "buying your own product", err: errs.New(errs.CodeSelfActionNotAllowed, "不能购买自己发布的商品"), wantStatus: http.StatusConflict, wantCode: errs.CodeSelfActionNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{tradeErr: tt.err})
			status, body := request(t, server, http.MethodPost, basePath+"/trades/"+testTradeID+"/accept", token, "")

			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", status, tt.wantStatus, body)
			}
			assertErrorCode(t, body, tt.wantCode)
		})
	}
}

func TestRejectAndCancelRequireAReason(t *testing.T) {
	t.Parallel()

	server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{})

	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "reject without a body", path: "/reject", body: ""},
		{name: "reject with an empty reason", path: "/reject", body: `{"reason":""}`},
		{name: "cancel with a too long reason", path: "/cancel", body: `{"reason":"` + strings.Repeat("字", 201) + `"}`},
		{name: "cancel with an unknown field", path: "/cancel", body: `{"reason":"x","status":"CANCELLED"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, body := request(t, server, http.MethodPost, basePath+"/trades/"+testTradeID+tt.path, token, tt.body)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", status, body)
			}
			assertErrorCode(t, body, errs.CodeValidation)
		})
	}
}

func TestListTradesRequiresTheRoleParameter(t *testing.T) {
	t.Parallel()

	server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{})

	for _, query := range []string{"", "?as=", "?as=owner", "?as=buyer&status=UNKNOWN", "?as=buyer&page=0"} {
		t.Run("query="+query, func(t *testing.T) {
			t.Parallel()

			status, body := request(t, server, http.MethodGet, basePath+"/trades"+query, token, "")
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", status, body)
			}
			assertErrorCode(t, body, errs.CodeValidation)
		})
	}
}

func TestListTradesForwardsTheRoleAndCompletesPartiesInOneCall(t *testing.T) {
	t.Parallel()

	accounts := &stubAccounts{}
	marketplace := &stubMarketplace{tradeItems: 10}
	server, token := newProductServer(t, accounts, marketplace)

	status, body := request(t, server, http.MethodGet, basePath+"/trades?as=seller&status=ACCEPTED", token, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}

	if marketplace.lastListTrades.GetAsBuyer() {
		t.Fatal("as=seller was forwarded as a buyer query")
	}
	if marketplace.lastListTrades.GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_ACCEPTED {
		t.Fatalf("status filter = %s", marketplace.lastListTrades.GetStatus())
	}
	if accounts.batchCalls != 1 {
		t.Fatalf("BatchGetUsers was called %d times, want exactly 1 for the page", accounts.batchCalls)
	}
}

// --- stub trade methods -----------------------------------------------------

func (s *stubMarketplace) trade() *marketplacev1.Trade {
	cover := "https://media.example.test/products/" + testProductID + "/cover.png"
	return &marketplacev1.Trade{
		Id: testTradeID,
		Product: &marketplacev1.TradeProduct{
			Id:       testProductID,
			Title:    "机械键盘",
			CoverUrl: &cover,
			Status:   marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE,
		},
		BuyerId:            testActor,
		SellerId:           "u_second_seller",
		PriceSnapshotMinor: 12345,
		Status:             marketplacev1.TradeStatus_TRADE_STATUS_PENDING,
		CreatedAt:          timestamppb.New(time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)),
		UpdatedAt:          timestamppb.New(time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)),
	}
}

func (s *stubMarketplace) CreateTrade(_ context.Context, req *marketplacev1.CreateTradeRequest, _ ...grpc.CallOption) (*marketplacev1.CreateTradeResponse, error) {
	s.lastCreateTrade = req
	if s.tradeErr != nil {
		return nil, s.tradeErr
	}
	return &marketplacev1.CreateTradeResponse{
		Trade: s.trade(), Replayed: s.replayed, Created: !s.tradeExists,
	}, nil
}

func (s *stubMarketplace) AcceptTrade(_ context.Context, req *marketplacev1.AcceptTradeRequest, _ ...grpc.CallOption) (*marketplacev1.AcceptTradeResponse, error) {
	s.lastAccept = req
	if s.tradeErr != nil {
		return nil, s.tradeErr
	}
	return &marketplacev1.AcceptTradeResponse{Trade: s.trade(), Replayed: s.replayed}, nil
}

func (s *stubMarketplace) RejectTrade(_ context.Context, _ *marketplacev1.RejectTradeRequest, _ ...grpc.CallOption) (*marketplacev1.RejectTradeResponse, error) {
	if s.tradeErr != nil {
		return nil, s.tradeErr
	}
	return &marketplacev1.RejectTradeResponse{Trade: s.trade(), Replayed: s.replayed}, nil
}

func (s *stubMarketplace) CancelTrade(_ context.Context, _ *marketplacev1.CancelTradeRequest, _ ...grpc.CallOption) (*marketplacev1.CancelTradeResponse, error) {
	if s.tradeErr != nil {
		return nil, s.tradeErr
	}
	return &marketplacev1.CancelTradeResponse{Trade: s.trade(), Replayed: s.replayed}, nil
}

func (s *stubMarketplace) ConfirmTrade(_ context.Context, _ *marketplacev1.ConfirmTradeRequest, _ ...grpc.CallOption) (*marketplacev1.ConfirmTradeResponse, error) {
	if s.tradeErr != nil {
		return nil, s.tradeErr
	}
	return &marketplacev1.ConfirmTradeResponse{Trade: s.trade(), Replayed: s.replayed}, nil
}

func (s *stubMarketplace) GetTrade(_ context.Context, _ *marketplacev1.GetTradeRequest, _ ...grpc.CallOption) (*marketplacev1.GetTradeResponse, error) {
	if s.tradeErr != nil {
		return nil, s.tradeErr
	}
	return &marketplacev1.GetTradeResponse{Trade: s.trade()}, nil
}

func (s *stubMarketplace) ListTrades(_ context.Context, req *marketplacev1.ListTradesRequest, _ ...grpc.CallOption) (*marketplacev1.ListTradesResponse, error) {
	s.lastListTrades = req
	if s.tradeErr != nil {
		return nil, s.tradeErr
	}

	count := s.tradeItems
	if count == 0 {
		count = 1
	}
	items := make([]*marketplacev1.Trade, 0, count)
	for range count {
		items = append(items, s.trade())
	}
	return &marketplacev1.ListTradesResponse{Page: &marketplacev1.TradePage{
		Items: items, Page: 1, PageSize: 20, Total: int64(count),
	}}, nil
}
