package handler

import (
	"context"
	"net/http"
	"strconv"
	"unicode/utf8"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/application/aggregation"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/mapper"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/middleware"
)

// IdempotencyKeyHeader carries a client-generated key that makes a retried
// command return the first result instead of acting twice.
const IdempotencyKeyHeader = "Idempotency-Key"

// Idempotency key limits, matching
// openapi/components/parameters.yaml#/IdempotencyKey.
const (
	minIdempotencyKeyLength = 16
	maxIdempotencyKeyLength = 128
)

// Trades serves the /trades endpoints and POST /products/{id}/trades.
type Trades struct {
	marketplace marketplacev1.MarketplaceServiceClient
	aggregator  *aggregation.Aggregator
	responder   Responder
}

// NewTrades builds the trade handler.
func NewTrades(marketplace marketplacev1.MarketplaceServiceClient, aggregator *aggregation.Aggregator, responder Responder) *Trades {
	return &Trades{marketplace: marketplace, aggregator: aggregator, responder: responder}
}

// Create handles POST /products/{productId}/trades.
func (h *Trades) Create(w http.ResponseWriter, r *http.Request) {
	var body dto.TradeCreateRequest
	if !decodeOptionalJSON(w, r, h.responder, &body) {
		return
	}

	var conversationID *string
	if body.ConversationID.Present && !body.ConversationID.IsNull() {
		value, err := body.ConversationID.String()
		length := utf8.RuneCountInString(value)
		if err != nil || length < 1 || length > 64 {
			h.responder.Fail(w, r, errs.CodeValidation, "conversation_id 必须是不超过 64 个字符的非空字符串或 null")
			return
		}
		conversationID = &value
	}

	idempotencyKey, ok := h.idempotencyKey(w, r)
	if !ok {
		return
	}

	resp, err := h.marketplace.CreateTrade(h.downstream(r), &marketplacev1.CreateTradeRequest{
		ActorId:               middleware.ActorID(r.Context()),
		ProductId:             r.PathValue("productId"),
		ConversationId:        conversationID,
		IdempotencyKey:        idempotencyKey,
		ConversationIdPresent: body.ConversationID.Present,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	status := http.StatusOK
	if resp.GetCreated() {
		// An idempotent replay of the first creation also carries Created=true,
		// so it reconstructs the original 201 rather than becoming create-or-get 200.
		status = http.StatusCreated
	}
	h.respondWithTrade(w, r, resp.GetTrade(), status)
}

// List handles GET /trades.
func (h *Trades) List(w http.ResponseWriter, r *http.Request) {
	role := r.URL.Query().Get("as")
	if role != "buyer" && role != "seller" {
		h.responder.Fail(w, r, errs.CodeValidation, "as 参数必须是 buyer 或 seller")
		return
	}

	page, size, ok := h.pagination(w, r)
	if !ok {
		return
	}

	req := &marketplacev1.ListTradesRequest{
		ActorId:  middleware.ActorID(r.Context()),
		AsBuyer:  role == "buyer",
		Page:     page,
		PageSize: size,
	}
	if raw := r.URL.Query().Get("status"); raw != "" {
		status, valid := mapper.ParseTradeStatus(raw)
		if !valid {
			h.responder.Fail(w, r, errs.CodeValidation, "交易状态不合法")
			return
		}
		req.Status = &status
	}

	resp, err := h.marketplace.ListTrades(h.downstream(r), req)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	users, err := h.aggregator.UserContacts(h.downstream(r), mapper.TradeParticipantIDs(resp.GetPage()))
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.OK(w, r, mapper.TradePage(resp.GetPage(), users))
}

// Get handles GET /trades/{tradeId}.
func (h *Trades) Get(w http.ResponseWriter, r *http.Request) {
	resp, err := h.marketplace.GetTrade(h.downstream(r), &marketplacev1.GetTradeRequest{
		ActorId: middleware.ActorID(r.Context()),
		TradeId: r.PathValue("tradeId"),
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.respondWithTrade(w, r, resp.GetTrade(), http.StatusOK)
}

// Accept handles POST /trades/{tradeId}/accept.
func (h *Trades) Accept(w http.ResponseWriter, r *http.Request) {
	idempotencyKey, ok := h.idempotencyKey(w, r)
	if !ok {
		return
	}

	resp, err := h.marketplace.AcceptTrade(h.downstream(r), &marketplacev1.AcceptTradeRequest{
		ActorId:        middleware.ActorID(r.Context()),
		TradeId:        r.PathValue("tradeId"),
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.respondWithTrade(w, r, resp.GetTrade(), http.StatusOK)
}

// Reject handles POST /trades/{tradeId}/reject.
func (h *Trades) Reject(w http.ResponseWriter, r *http.Request) {
	reason, idempotencyKey, ok := h.reasonRequest(w, r)
	if !ok {
		return
	}

	resp, err := h.marketplace.RejectTrade(h.downstream(r), &marketplacev1.RejectTradeRequest{
		ActorId:        middleware.ActorID(r.Context()),
		TradeId:        r.PathValue("tradeId"),
		Reason:         reason,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.respondWithTrade(w, r, resp.GetTrade(), http.StatusOK)
}

// Cancel handles POST /trades/{tradeId}/cancel.
func (h *Trades) Cancel(w http.ResponseWriter, r *http.Request) {
	reason, idempotencyKey, ok := h.reasonRequest(w, r)
	if !ok {
		return
	}

	resp, err := h.marketplace.CancelTrade(h.downstream(r), &marketplacev1.CancelTradeRequest{
		ActorId:        middleware.ActorID(r.Context()),
		TradeId:        r.PathValue("tradeId"),
		Reason:         reason,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.respondWithTrade(w, r, resp.GetTrade(), http.StatusOK)
}

// Confirm handles POST /trades/{tradeId}/confirm.
func (h *Trades) Confirm(w http.ResponseWriter, r *http.Request) {
	idempotencyKey, ok := h.idempotencyKey(w, r)
	if !ok {
		return
	}

	resp, err := h.marketplace.ConfirmTrade(h.downstream(r), &marketplacev1.ConfirmTradeRequest{
		ActorId:        middleware.ActorID(r.Context()),
		TradeId:        r.PathValue("tradeId"),
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.respondWithTrade(w, r, resp.GetTrade(), http.StatusOK)
}

// --- shared steps -----------------------------------------------------------

// respondWithTrade completes the two parties' identities and contacts and
// writes the trade. 交易双方的联系方式仅在此处（对交易方）公开。
func (h *Trades) respondWithTrade(w http.ResponseWriter, r *http.Request, trade *marketplacev1.Trade, status int) {
	contacts, err := h.aggregator.UserContacts(h.downstream(r), []string{trade.GetBuyerId(), trade.GetSellerId()})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.Success(w, r, status, mapper.Trade(trade, contacts))
}

// reasonRequest reads the required reason body and the optional key.
func (h *Trades) reasonRequest(w http.ResponseWriter, r *http.Request) (string, *string, bool) {
	var body dto.ReasonRequest
	if !decodeJSON(w, r, h.responder, &body) {
		return "", nil, false
	}
	if length := len([]rune(body.Reason)); length < 1 || length > 200 {
		h.responder.Fail(w, r, errs.CodeValidation, "原因长度必须为 1 至 200 个字符")
		return "", nil, false
	}

	idempotencyKey, ok := h.idempotencyKey(w, r)
	if !ok {
		return "", nil, false
	}
	return body.Reason, idempotencyKey, true
}

// idempotencyKey reads and validates the optional Idempotency-Key header.
//
// A malformed key is rejected rather than ignored: silently dropping it would
// turn a retry the client believes is safe into a second command.
func (h *Trades) idempotencyKey(w http.ResponseWriter, r *http.Request) (*string, bool) {
	raw := r.Header.Get(IdempotencyKeyHeader)
	if raw == "" {
		return nil, true
	}
	length := utf8.RuneCountInString(raw)
	if length < minIdempotencyKeyLength || length > maxIdempotencyKeyLength {
		h.responder.Fail(w, r, errs.CodeValidation, "Idempotency-Key 长度必须为 16 至 128 个字符")
		return nil, false
	}
	return &raw, true
}

// pagination reads the page and page_size query parameters.
func (h *Trades) pagination(w http.ResponseWriter, r *http.Request) (page, size int32, ok bool) {
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

func (h *Trades) intQuery(w http.ResponseWriter, r *http.Request, name string, fallback, minimum, maximum int32) (int32, bool) {
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

// downstream builds the context for an internal call.
func (h *Trades) downstream(r *http.Request) context.Context {
	ctx := grpcx.WithActor(r.Context(), middleware.ActorID(r.Context()))
	return grpcx.WithRequestID(ctx, middleware.RequestID(r.Context()))
}
