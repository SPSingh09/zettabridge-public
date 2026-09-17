package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestMiddlewareBearerOrQuery(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString(middlewareBearerOrQuery(c))
	})

	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1In0.sig"
	req := httptest.NewRequest("GET", "/test?token="+jwt, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 512)
	n, _ := resp.Body.Read(body)
	got := string(body[:n])
	if got != jwt {
		t.Fatalf("query token got %q want %q", got, jwt)
	}

	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("Authorization", "Bearer "+jwt)
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatal(err)
	}
	n, _ = resp2.Body.Read(body)
	got = string(body[:n])
	if got != jwt {
		t.Fatalf("bearer token got %q want %q", got, jwt)
	}
}

func TestMiddlewareBearerOrQueryWebSocketUpgrade(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString(middlewareBearerOrQuery(c))
	})

	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1In0.sig"
	req := httptest.NewRequest("GET", "/test?token="+jwt, nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 512)
	n, _ := resp.Body.Read(body)
	got := string(body[:n])
	if got != jwt {
		t.Fatalf("ws upgrade query token got %q want %q", got, jwt)
	}
}
