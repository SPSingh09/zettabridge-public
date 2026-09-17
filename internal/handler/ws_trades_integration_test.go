package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"

	"github.com/SPSingh09/zettabridge/internal/platform/security"
	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func newWSTestApp(t *testing.T, secret string) *fiber.App {
	t.Helper()
	cfg := &config.Config{JWTSecret: secret}
	h := &Handler{cfg: cfg, redis: store.NewRedis("redis://127.0.0.1:6379")}

	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Use(cors.New(cors.Config{
		AllowOrigins: "http://localhost:3000",
		AllowHeaders: "Origin, Content-Type, Authorization",
		AllowMethods: "GET, POST, PUT, PATCH, DELETE",
	}))

	// Mirror production: /v1 group JWT middleware must not block ?token= on WS.
	app.Get("/v1/ws/trades", h.wsTradesUpgrade, websocket.New(func(conn *websocket.Conn) {
		_ = conn.Close()
	}))
	protected := app.Group("/v1", middleware.Auth(secret))
	protected.Get("/me", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })

	return app
}

func TestWSTradesUpgradeQueryToken(t *testing.T) {
	secret := "dev-secret-change-in-production-min-32-chars"
	token, _, err := security.IssueAccessToken(secret, "user-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	app := newWSTestApp(t, secret)

	req := httptest.NewRequest(http.MethodGet, "/v1/ws/trades?token="+token, nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		body := make([]byte, 256)
		n, _ := resp.Body.Read(body)
		t.Fatalf("query token upgrade want 101 got %d body=%q", resp.StatusCode, string(body[:n]))
	}
}

func TestWSTradesUpgradeBearerToken(t *testing.T) {
	secret := "dev-secret-change-in-production-min-32-chars"
	token, _, err := security.IssueAccessToken(secret, "user-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	app := newWSTestApp(t, secret)

	req := httptest.NewRequest(http.MethodGet, "/v1/ws/trades", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		body := make([]byte, 256)
		n, _ := resp.Body.Read(body)
		t.Fatalf("bearer upgrade want 101 got %d body=%q", resp.StatusCode, string(body[:n]))
	}
}
