package handler

import (
	"bytes"
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stripe/stripe-go/v81/webhook"

	"github.com/SPSingh09/zettabridge/internal/billing"
	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestPostStripeWebhookDisabled(t *testing.T) {
	cfg := &config.Config{BillingEnabled: false}
	h := New(&store.PGStore{}, nil, nil, cfg, nil, nil)

	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Post("/v1/webhooks/stripe", h.postStripeWebhook)

	req := httptest.NewRequest("POST", "/v1/webhooks/stripe", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestPostStripeWebhookBadSignature(t *testing.T) {
	cfg := &config.Config{
		BillingEnabled:      true,
		StripeWebhookSecret: "whsec_test",
	}
	h := New(&store.PGStore{}, nil, nil, cfg, nil, nil)

	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Post("/v1/webhooks/stripe", h.postStripeWebhook)

	req := httptest.NewRequest("POST", "/v1/webhooks/stripe", nil)
	req.Header.Set("Stripe-Signature", "bad")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestPostStripeWebhookValidSignatureUnhandledEvent(t *testing.T) {
	const secret = "whsec_test"
	cfg := &config.Config{
		BillingEnabled:      true,
		StripeWebhookSecret: secret,
	}
	h := New(&store.PGStore{}, nil, nil, cfg, nil, nil)
	h.stripeWebhooks = billing.NewWebhookProcessor(cfg, noopWebhookStore{})

	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Post("/v1/webhooks/stripe", h.postStripeWebhook)

	payload := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: []byte(`{"id":"evt_test","object":"event","type":"ping"}`),
		Secret:  secret,
	})

	req := httptest.NewRequest("POST", "/v1/webhooks/stripe", bytes.NewReader(payload.Payload))
	req.Header.Set("Stripe-Signature", payload.Header)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

// noopWebhookStore satisfies billing.WebhookStore for signature-only handler tests.
type noopWebhookStore struct{}

func (noopWebhookStore) StripeWebhookEventExists(context.Context, string) (bool, error) {
	return false, nil
}
func (noopWebhookStore) RecordStripeWebhookEvent(context.Context, string, string) (bool, error) {
	return true, nil
}
func (noopWebhookStore) GetUserByStripeCustomerID(context.Context, string) (*store.User, error) {
	return nil, nil
}
func (noopWebhookStore) SetUserStripeIDs(context.Context, string, string, string) error { return nil }
func (noopWebhookStore) UpdateUserBillingSource(context.Context, string, string) error  { return nil }
func (noopWebhookStore) ClearUserStripeSubscription(context.Context, string) error     { return nil }
func (noopWebhookStore) GetUserByID(context.Context, string) (*store.User, error)     { return nil, nil }
func (noopWebhookStore) GetOrgByOwnerUserID(context.Context, string) (*store.Organization, error) {
	return nil, nil
}
func (noopWebhookStore) CountSeatsUsed(context.Context, string) (int, error)         { return 0, nil }
func (noopWebhookStore) UpdateUserPlan(context.Context, string, string) error          { return nil }
func (noopWebhookStore) UpdateUserOrgsEnabled(context.Context, string, bool) error     { return nil }
func (noopWebhookStore) SetOrgStatusForOwner(context.Context, string, string) error    { return nil }
func (noopWebhookStore) SyncOrgSeatLimitForOwner(context.Context, string, int) error       { return nil }
func (noopWebhookStore) PauseAllWebhooksByUser(context.Context, string) error               { return nil }
func (noopWebhookStore) PauseAllCredentialsByUser(context.Context, string) error            { return nil }
func (noopWebhookStore) ClearAutoPausedWebhooksByUser(context.Context, string) error        { return nil }
func (noopWebhookStore) ClearAutoPausedCredentialsByUser(context.Context, string) error     { return nil }
