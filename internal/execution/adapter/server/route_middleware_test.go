package server

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
)

// Regression: OAuth callbacks must stay public when registered alongside
// service-token-protected order routes. Use a narrow group prefix — a /v1
// group would blanket-match every /v1/* route in current Fiber.
func TestPublicRouteAfterServiceTokenGroup(t *testing.T) {
	app := fiber.New()
	orders := app.Group("/v1/orders")
	orders.Use(middleware.RequireServiceToken("secret"))
	orders.Post("/", func(c *fiber.Ctx) error { return c.SendString("orders") })

	app.Get("/v1/credentials/zerodha/callback", func(c *fiber.Ctx) error {
		return c.SendString("oauth-ok")
	})

	req := httptest.NewRequest("GET", "/v1/credentials/zerodha/callback", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusOK || string(body) != "oauth-ok" {
		t.Fatalf("expected public oauth callback, got status=%d body=%q", resp.StatusCode, body)
	}
}

func TestZerodhaOAuthCallbackPublicAfterNewApp(t *testing.T) {
	app := NewApp(Config{
		Name:         "zerodha",
		Port:         "8092",
		Executor:     &LiveExecutor{Live: nil},
		ServiceToken: "secret",
		MountPublic: func(app *fiber.App) {
			app.Get("/v1/credentials/zerodha/callback", func(c *fiber.Ctx) error {
				return c.SendString("oauth-ok")
			})
		},
	})

	req := httptest.NewRequest("GET", "/v1/credentials/zerodha/callback?status=success&request_token=x&state=y", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusOK || string(body) != "oauth-ok" {
		t.Fatalf("expected public oauth callback via NewApp, got status=%d body=%q", resp.StatusCode, body)
	}
}

func TestOrdersRequireServiceToken(t *testing.T) {
	app := NewApp(Config{
		Name:         "test",
		Port:         "8092",
		Executor:     &LiveExecutor{Live: nil},
		ServiceToken: "secret",
	})

	req := httptest.NewRequest("POST", "/v1/orders", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusUnauthorized || string(body) != `{"error":"invalid service token"}` {
		t.Fatalf("expected service token rejection, got status=%d body=%q", resp.StatusCode, body)
	}
}
