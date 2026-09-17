package handler

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/SPSingh09/zettabridge/internal/config"
	_ "github.com/SPSingh09/zettabridge/internal/platform/metrics"
)

func TestScrapeMetricsExposesZettabridgeSeries(t *testing.T) {
	app := fiber.New()
	h := &Handler{cfg: &config.Config{MetricsEnabled: true}}
	app.Get("/metrics", h.scrapeMetrics)

	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status want 200 got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"# HELP zettabridge_ingest_total",
		"# TYPE zettabridge_ingest_total",
		"# HELP zettabridge_trades_total",
		"# HELP zettabridge_dedup_hits_total",
		"# HELP zettabridge_billing_checkout_total",
		"# HELP zettabridge_billing_webhook_total",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in metrics body: %q", want, text[:min(len(text), 200)])
		}
	}
}

func TestScrapeMetricsDisabled(t *testing.T) {
	app := fiber.New()
	h := &Handler{cfg: &config.Config{MetricsEnabled: false}}
	app.Get("/metrics", h.scrapeMetrics)

	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status want 404 got %d", resp.StatusCode)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
