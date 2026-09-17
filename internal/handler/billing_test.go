package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestGetBillingPlans(t *testing.T) {
	cfg := &config.Config{AppPublicURL: "http://localhost:8080", BillingEnabled: false}
	h := New(&store.PGStore{}, nil, nil, cfg, nil, nil)

	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Get("/v1/billing/plans", h.getBillingPlans)

	req := httptest.NewRequest("GET", "/v1/billing/plans", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}

	var env struct {
		Data struct {
			Plans []struct {
				Plan string `json:"plan"`
			} `json:"plans"`
			Checkout struct {
				CancelImmediate bool `json:"cancel_immediate"`
			} `json:"checkout"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data.Plans) != 4 {
		t.Fatalf("expected 4 plans, got %d", len(env.Data.Plans))
	}
	if !env.Data.Checkout.CancelImmediate {
		t.Fatal("expected cancel_immediate checkout policy flag")
	}
}

func TestPostBillingCheckoutDisabled(t *testing.T) {
	cfg := &config.Config{AppPublicURL: "http://localhost:8080", BillingEnabled: false}
	h := New(&store.PGStore{}, nil, nil, cfg, nil, nil)

	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Post("/v1/billing/checkout", h.postBillingCheckout)

	req := httptest.NewRequest("POST", "/v1/billing/checkout", strings.NewReader(`{"plan":"pro"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}
