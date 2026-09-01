package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	buyer  = "u_buyer"
	seller = "u_seller"
	other  = "u_other"
)

func newTradeFixture(t *testing.T) (*Product, *Trade) {
	t.Helper()

	product, err := NewProduct("p_1", seller, "机械键盘", 12000, CategoryDigital, "九成新", now)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	trade, err := NewTrade("t_1", product, buyer, nil, now)
	if err != nil {
		t.Fatalf("NewTrade: %v", err)
	}
	return product, trade
}

func TestNewTradeStartsPendingAndSnapshotsThePrice(t *testing.T) {
	t.Parallel()

	product, trade := newTradeFixture(t)

	if trade.Status != TradePending {
		t.Fatalf("Status = %s, want PENDING", trade.Status)
	}
	if trade.SellerID != seller || trade.BuyerID != buyer {
		t.Fatalf("parties = %s/%s", trade.BuyerID, trade.SellerID)
	}
	if trade.PriceSnapshotMinor != 12000 {
		t.Fatalf("PriceSnapshotMinor = %d, want 12000", trade.PriceSnapshotMinor)
	}

	// 后续修改价格不能改变已经达成的交易。
	newPrice := int64(9900)
	if err := product.Edit(seller, nil, &newPrice, nil, nil, now.Add(time.Hour)); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if trade.PriceSnapshotMinor != 12000 {
		t.Fatalf("the price snapshot followed the product to %d", trade.PriceSnapshotMinor)
	}
}

func TestNewTradeRejectsSelfPurchaseAndUntradableProducts(t *testing.T) {
	t.Parallel()

	product, _ := newTradeFixture(t)

	if _, err := NewTrade("t_2", product, seller, nil, now); !errors.Is(err, ErrSelfTrade) {
		t.Fatalf("buying own product = %v, want ErrSelfTrade", err)
	}

	for _, status := range []Status{StatusReserved, StatusSold, StatusOffShelf} {
		t.Run(string(status), func(t *testing.T) {
			blocked, err := NewProduct("p_2", seller, "别的", 100, CategoryOther, "d", now)
			if err != nil {
				t.Fatalf("NewProduct: %v", err)
			}
			blocked.Status = status

			if _, err := NewTrade("t_3", blocked, buyer, nil, now); !errors.Is(err, ErrNotOnSale) {
				t.Fatalf("trading a %s product = %v, want ErrNotOnSale", status, err)
			}
		})
	}
}

func TestAcceptIsSellerOnlyAndOnlyFromPending(t *testing.T) {
	t.Parallel()

	_, trade := newTradeFixture(t)

	if err := trade.Accept(buyer, now); !errors.Is(err, ErrNotTradeSeller) {
		t.Fatalf("accept by the buyer = %v, want ErrNotTradeSeller", err)
	}
	if err := trade.Accept(other, now); !errors.Is(err, ErrNotTradeSeller) {
		t.Fatalf("accept by a stranger = %v, want ErrNotTradeSeller", err)
	}
	if trade.Status != TradePending {
		t.Fatalf("a rejected accept changed the status to %s", trade.Status)
	}

	if err := trade.Accept(seller, now); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if trade.Status != TradeAccepted || trade.AcceptedAt == nil {
		t.Fatalf("Status/AcceptedAt = %s/%v", trade.Status, trade.AcceptedAt)
	}

	if err := trade.Accept(seller, now); !errors.Is(err, ErrTradeNotPending) {
		t.Fatalf("a second Accept = %v, want ErrTradeNotPending", err)
	}
}

func TestRejectIsSellerOnlyAndLeavesTheProductAlone(t *testing.T) {
	t.Parallel()

	product, trade := newTradeFixture(t)

	if err := trade.Reject(buyer, "不卖了", now); !errors.Is(err, ErrNotTradeSeller) {
		t.Fatalf("reject by the buyer = %v, want ErrNotTradeSeller", err)
	}
	if err := trade.Reject(seller, "不卖了", now); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	if trade.Status != TradeCancelled || trade.CancelledAt == nil {
		t.Fatalf("Status/CancelledAt = %s/%v", trade.Status, trade.CancelledAt)
	}
	if trade.CancelReason == nil || *trade.CancelReason != "不卖了" {
		t.Fatalf("CancelReason = %v", trade.CancelReason)
	}
	if product.Status != StatusOnSale {
		t.Fatalf("rejecting changed the product to %s", product.Status)
	}
}

func TestCancelPermissionsFollowTheState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		accept  bool
		actor   string
		wantErr error
	}{
		{name: "pending cancelled by the buyer", actor: buyer},
		{name: "pending cancelled by the seller", actor: seller, wantErr: ErrNotTradeBuyer},
		{name: "pending cancelled by a stranger", actor: other, wantErr: ErrNotTradeParty},
		{name: "accepted cancelled by the buyer", accept: true, actor: buyer},
		{name: "accepted cancelled by the seller", accept: true, actor: seller},
		{name: "accepted cancelled by a stranger", accept: true, actor: other, wantErr: ErrNotTradeParty},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, trade := newTradeFixture(t)
			if tt.accept {
				if err := trade.Accept(seller, now); err != nil {
					t.Fatalf("Accept: %v", err)
				}
			}
			before := trade.Status

			err := trade.Cancel(tt.actor, "临时有事", now)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Cancel = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && trade.Status != before {
				t.Fatalf("a rejected cancel changed the status to %s", trade.Status)
			}
			if tt.wantErr == nil && trade.Status != TradeCancelled {
				t.Fatalf("Status = %s, want CANCELLED", trade.Status)
			}
		})
	}
}

func TestCancelIsRejectedOnTerminalTrades(t *testing.T) {
	t.Parallel()

	for _, status := range []TradeStatus{TradeCompleted, TradeCancelled} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			_, trade := newTradeFixture(t)
			trade.Status = status

			if err := trade.Cancel(buyer, "算了", now); !errors.Is(err, ErrTradeNotPending) {
				t.Fatalf("cancelling a %s trade = %v, want a state error", status, err)
			}
			if trade.Status != status {
				t.Fatalf("a rejected cancel changed the status to %s", trade.Status)
			}
		})
	}
}

func TestCancelReasonIsValidated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		reason string
	}{
		{name: "empty", reason: ""},
		{name: "too long", reason: strings.Repeat("字", 201)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, trade := newTradeFixture(t)
			if err := trade.Cancel(buyer, tt.reason, now); !errors.Is(err, ErrCancelReasonLength) {
				t.Fatalf("Cancel = %v, want ErrCancelReasonLength", err)
			}
			if trade.Status != TradePending {
				t.Fatal("a rejected reason still cancelled the trade")
			}
		})
	}

	// 200 个字符是文档规定的上限，必须被接受。
	_, trade := newTradeFixture(t)
	if err := trade.Cancel(buyer, strings.Repeat("字", 200), now); err != nil {
		t.Fatalf("Cancel with a 200-character reason: %v", err)
	}
}

func TestConfirmCompletesOnlyWhenBothPartiesHaveConfirmed(t *testing.T) {
	t.Parallel()

	_, trade := newTradeFixture(t)
	if err := trade.Accept(seller, now); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	completed, err := trade.Confirm(buyer, now)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if completed {
		t.Fatal("one confirmation completed the trade")
	}
	if trade.Status != TradeAccepted {
		t.Fatalf("Status = %s, want ACCEPTED after one confirmation", trade.Status)
	}
	if !trade.BuyerConfirmed() || trade.SellerConfirmed() {
		t.Fatalf("confirmations = %v/%v", trade.BuyerConfirmed(), trade.SellerConfirmed())
	}

	completed, err = trade.Confirm(seller, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if !completed {
		t.Fatal("the second confirmation did not complete the trade")
	}
	if trade.Status != TradeCompleted || trade.CompletedAt == nil {
		t.Fatalf("Status/CompletedAt = %s/%v", trade.Status, trade.CompletedAt)
	}
}

func TestRepeatedConfirmationIsIdempotent(t *testing.T) {
	t.Parallel()

	_, trade := newTradeFixture(t)
	if err := trade.Accept(seller, now); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if _, err := trade.Confirm(buyer, now); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	firstConfirmation := *trade.BuyerConfirmedAt

	completed, err := trade.Confirm(buyer, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("a repeated Confirm = %v, want nil", err)
	}
	if completed {
		t.Fatal("the buyer confirming twice completed the trade")
	}
	if !trade.BuyerConfirmedAt.Equal(firstConfirmation) {
		t.Fatal("a repeated confirmation moved the recorded timestamp")
	}
	if trade.Status != TradeAccepted {
		t.Fatalf("Status = %s, want ACCEPTED", trade.Status)
	}
}

func TestConfirmRequiresAnAcceptedTradeAndAParty(t *testing.T) {
	t.Parallel()

	_, pending := newTradeFixture(t)
	if _, err := pending.Confirm(buyer, now); !errors.Is(err, ErrTradeNotAccepted) {
		t.Fatalf("confirming a pending trade = %v, want ErrTradeNotAccepted", err)
	}

	_, accepted := newTradeFixture(t)
	if err := accepted.Accept(seller, now); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if _, err := accepted.Confirm(other, now); !errors.Is(err, ErrNotTradeParty) {
		t.Fatalf("confirming as a stranger = %v, want ErrNotTradeParty", err)
	}
	if accepted.BuyerConfirmed() || accepted.SellerConfirmed() {
		t.Fatal("a rejected confirmation was recorded")
	}
}

func TestSystemCancelOnlyAppliesToPendingTrades(t *testing.T) {
	t.Parallel()

	_, trade := newTradeFixture(t)
	if err := trade.SystemCancel("卖家已接受其他交易", now); err != nil {
		t.Fatalf("SystemCancel: %v", err)
	}
	if trade.Status != TradeCancelled {
		t.Fatalf("Status = %s, want CANCELLED", trade.Status)
	}

	_, accepted := newTradeFixture(t)
	if err := accepted.Accept(seller, now); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if err := accepted.SystemCancel("x", now); !errors.Is(err, ErrTradeNotPending) {
		t.Fatalf("SystemCancel on an accepted trade = %v, want ErrTradeNotPending", err)
	}
}

func TestOnlyThePartiesCanBeIdentified(t *testing.T) {
	t.Parallel()

	_, trade := newTradeFixture(t)

	if !trade.IsBuyer(buyer) || !trade.IsSeller(seller) {
		t.Fatal("the parties are not recognised")
	}
	if trade.IsParty(other) || trade.IsParty("") {
		t.Fatal("a stranger or an empty actor was treated as a party")
	}
}
