package http_test

import (
	"net/http"
	"strings"
	"testing"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
)

func TestGetUserProfileReturnsThePublicShape(t *testing.T) {
	t.Parallel()

	marketplace := &stubMarketplace{
		averageScores: map[string]string{"u_seller": "4.50"},
		userStats:     &marketplacev1.GetUserStatsResponse{CompletedTrades: 2, OnSaleProducts: 3},
	}
	server, token := newProductServer(t, &stubAccounts{}, marketplace)

	status, body := request(t, server, http.MethodGet, basePath+"/users/u_seller", token, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}

	var envelope struct {
		Data struct {
			ID                   string  `json:"id"`
			Nickname             string  `json:"nickname"`
			AverageScore         *string `json:"average_score"`
			CompletedTradesCount int64   `json:"completed_trades_count"`
			OnSaleProductsCount  int64   `json:"on_sale_products_count"`
		} `json:"data"`
	}
	decode(t, body, &envelope)

	if envelope.Data.ID != "u_seller" || envelope.Data.Nickname == "" {
		t.Fatalf("profile is incomplete: %s", body)
	}
	if envelope.Data.AverageScore == nil || *envelope.Data.AverageScore != "4.50" {
		t.Fatalf("average_score missing: %s", body)
	}
	if envelope.Data.CompletedTradesCount != 2 || envelope.Data.OnSaleProductsCount != 3 {
		t.Fatalf("stats = %d/%d, want 2/3", envelope.Data.CompletedTradesCount, envelope.Data.OnSaleProductsCount)
	}
	if strings.Contains(body, "student_no") || strings.Contains(body, "wechat") {
		t.Fatalf("the public profile must not leak student numbers or contacts: %s", body)
	}
}

func TestGetUserProfileExposesNullAverageWithoutScores(t *testing.T) {
	t.Parallel()

	server, token := newProductServer(t, &stubAccounts{}, &stubMarketplace{})

	status, body := request(t, server, http.MethodGet, basePath+"/users/u_nobody_rated", token, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
	if !strings.Contains(body, `"average_score":null`) {
		t.Fatalf("average_score must be present as null: %s", body)
	}
}

func TestGetUserProfileRejectsAnUnknownUser(t *testing.T) {
	t.Parallel()

	server, token := newProductServer(t, &stubAccounts{missingUser: true}, &stubMarketplace{})

	status, body := request(t, server, http.MethodGet, basePath+"/users/u_missing", token, "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", status, body)
	}
	if !strings.Contains(body, "RESOURCE_NOT_FOUND") {
		t.Fatalf("error code missing: %s", body)
	}
}
