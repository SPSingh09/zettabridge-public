//go:build functional

package functionaltester

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v81/webhook"
)

type billingPlansResult struct {
	BillingEnabled bool `json:"billing_enabled"`
	Plans          []struct {
		Plan string `json:"plan"`
	} `json:"plans"`
	Checkout struct {
		CancelImmediate    bool   `json:"cancel_immediate"`
		SuccessURL         string `json:"success_url"`
	} `json:"checkout"`
}

type checkoutResult struct {
	URL string `json:"url"`
}

// TestBillingDisabled covers Phases A–D when BILLING_ENABLED=false (default docker-compose).
// Skipped when the billing overlay is active (make functional-test-billing).
func TestBillingDisabled(t *testing.T) {
	if os.Getenv("FUNCTIONAL_TEST_BILLING") == "1" {
		t.Skip("billing overlay enabled — use make functional-test for disabled-path coverage")
	}

	s := newState()
	s.registerOwner(t)
	s.loginOwner(t)

	if billingEnabledOnServer(s, t, s.ownerToken) {
		t.Skip("server has BILLING_ENABLED=true — use make functional-test-billing for enabled-path coverage")
	}

	t.Run("plans catalog", func(t *testing.T) {
		var out billingPlansResult
		callJSON(s, t, http.MethodGet, "/v1/billing/plans", s.ownerToken, nil, http.StatusOK, &out)
		if out.BillingEnabled {
			t.Fatal("expected billing_enabled false on default docker stack")
		}
		wantPlans := []string{"free", "paper", "pro", "pro_plus"}
		if len(out.Plans) != len(wantPlans) {
			t.Fatalf("plans want %d (%v) got %d", len(wantPlans), wantPlans, len(out.Plans))
		}
		got := make(map[string]bool, len(out.Plans))
		for _, p := range out.Plans {
			got[p.Plan] = true
		}
		for _, planName := range wantPlans {
			if !got[planName] {
				t.Fatalf("plans catalog missing %q", planName)
			}
		}
		if !out.Checkout.CancelImmediate {
			t.Fatal("expected cancel_immediate policy flag")
		}
		if !strings.Contains(out.Checkout.SuccessURL, "session_id=") {
			t.Fatalf("unexpected success_url: %q", out.Checkout.SuccessURL)
		}
	})

	t.Run("checkout and portal disabled", func(t *testing.T) {
		type checkoutReq struct {
			Plan string `json:"plan"`
		}
		callExpectStatus(s, t, http.MethodPost, "/v1/billing/checkout", s.ownerToken, checkoutReq{Plan: "pro"}, http.StatusServiceUnavailable)
		callExpectStatus(s, t, http.MethodPost, "/v1/billing/portal", s.ownerToken, nil, http.StatusServiceUnavailable)
	})

	t.Run("stripe webhook disabled", func(t *testing.T) {
		callExpectStatus(s, t, http.MethodPost, "/v1/webhooks/stripe", "", map[string]any{}, http.StatusServiceUnavailable)
	})

	t.Run("admin plan sets billing_source admin", func(t *testing.T) {
		s.loginAdmin(t)
		type planReq struct {
			Plan string `json:"plan"`
		}
		var out userSummary
		callJSON(s, t, http.MethodPut, "/v1/admin/users/"+s.ownerID+"/plan", s.adminToken, planReq{Plan: "pro"}, http.StatusOK, &out)
		if out.Plan != "pro" {
			t.Fatalf("plan want pro got %q", out.Plan)
		}
		if out.BillingSource != "admin" {
			t.Fatalf("billing_source want admin got %q", out.BillingSource)
		}
		var detail userSummary
		callJSON(s, t, http.MethodGet, "/v1/admin/users/"+s.ownerID, s.adminToken, nil, http.StatusOK, &detail)
		if detail.BillingSource != "admin" {
			t.Fatalf("admin GET billing_source want admin got %q", detail.BillingSource)
		}
		s.upgradeOwnerPlan(t, "free")
	})
}

// TestBillingStripeWebhooks covers Phase C/D with signed webhook payloads (no Stripe CLI required).
// Requires server with BILLING_ENABLED=true — use docker-compose.billing.yml overlay.
func TestBillingStripeWebhooks(t *testing.T) {
	if os.Getenv("FUNCTIONAL_TEST_BILLING") != "1" {
		t.Skip("set FUNCTIONAL_TEST_BILLING=1 with billing overlay (make functional-test-billing)")
	}

	secret := os.Getenv("FUNCTIONAL_TEST_STRIPE_WEBHOOK_SECRET")
	if secret == "" {
		secret = "whsec_functional_test"
	}

	s := newState()
	s.registerOwner(t)
	s.loginOwner(t)

	t.Run("plans billing enabled", func(t *testing.T) {
		var out billingPlansResult
		callJSON(s, t, http.MethodGet, "/v1/billing/plans", s.ownerToken, nil, http.StatusOK, &out)
		if !out.BillingEnabled {
			t.Fatal("expected billing_enabled true — restart server with docker-compose.billing.yml")
		}
	})

	t.Run("webhook bad signature", func(t *testing.T) {
		status, _ := s.postStripeWebhook(t, []byte(`{"id":"evt_bad","object":"event","type":"ping"}`), "bad")
		if status != http.StatusBadRequest {
			t.Fatalf("bad signature: want 400 got %d", status)
		}
	})

	customerID := "cus_functional_" + uuid.New().String()[:8]
	subID := "sub_functional_" + uuid.New().String()[:8]

	t.Run("checkout.session.completed upgrades plan", func(t *testing.T) {
		evtID := "evt_checkout_" + uuid.New().String()[:8]
		body, sig := signedStripeEvent(t, secret, evtID, "checkout.session.completed", map[string]any{
			"id":                  "cs_functional_test",
			"client_reference_id": s.ownerID,
			"customer":            customerID,
			"subscription":        subID,
			"metadata": map[string]string{
				"user_id": s.ownerID,
				"plan":    "pro",
			},
		})
		status, raw := s.postStripeWebhook(t, body, sig)
		if status != http.StatusOK {
			t.Fatalf("checkout webhook: want 200 got %d body=%s", status, trimBody(raw))
		}

		s.loginAdmin(t)
		var user userSummary
		callJSON(s, t, http.MethodGet, "/v1/admin/users/"+s.ownerID, s.adminToken, nil, http.StatusOK, &user)
		if user.Plan != "pro" {
			t.Fatalf("plan after checkout webhook want pro got %q", user.Plan)
		}
		if user.BillingSource != "stripe" {
			t.Fatalf("billing_source want stripe got %q", user.BillingSource)
		}
	})

	t.Run("webhook idempotent", func(t *testing.T) {
		evtID := "evt_checkout_idem"
		body, sig := signedStripeEvent(t, secret, evtID, "checkout.session.completed", map[string]any{
			"id":                  "cs_functional_idem",
			"client_reference_id": s.ownerID,
			"customer":            customerID,
			"subscription":        subID,
			"metadata": map[string]string{
				"user_id": s.ownerID,
				"plan":    "pro",
			},
		})
		status, _ := s.postStripeWebhook(t, body, sig)
		if status != http.StatusOK {
			t.Fatalf("first delivery want 200 got %d", status)
		}
		status, _ = s.postStripeWebhook(t, body, sig)
		if status != http.StatusOK {
			t.Fatalf("duplicate delivery want 200 got %d", status)
		}
	})

	t.Run("subscription.deleted downgrades immediately", func(t *testing.T) {
		evtID := "evt_sub_del_" + uuid.New().String()[:8]
		body, sig := signedStripeEvent(t, secret, evtID, "customer.subscription.deleted", map[string]any{
			"id":       subID,
			"customer": customerID,
			"status":   "canceled",
		})
		status, raw := s.postStripeWebhook(t, body, sig)
		if status != http.StatusOK {
			t.Fatalf("deleted webhook: want 200 got %d body=%s", status, trimBody(raw))
		}

		s.loginAdmin(t)
		var user userSummary
		callJSON(s, t, http.MethodGet, "/v1/admin/users/"+s.ownerID, s.adminToken, nil, http.StatusOK, &user)
		if user.Plan != "free" {
			t.Fatalf("plan after delete want free got %q", user.Plan)
		}
		if user.BillingSource != "free" {
			t.Fatalf("billing_source want free got %q", user.BillingSource)
		}
	})

	t.Run("admin override ignores stripe webhook", func(t *testing.T) {
		s.loginAdmin(t)
		type planReq struct {
			Plan string `json:"plan"`
		}
		callJSON(s, t, http.MethodPut, "/v1/admin/users/"+s.ownerID+"/plan", s.adminToken, planReq{Plan: "pro"}, http.StatusOK, &userSummary{})

		evtID := "evt_sub_upd_" + uuid.New().String()[:8]
		body, sig := signedStripeEvent(t, secret, evtID, "customer.subscription.updated", map[string]any{
			"id":       subID,
			"customer": customerID,
			"status":   "active",
			"metadata": map[string]string{
				"plan": "free",
			},
		})
		status, _ := s.postStripeWebhook(t, body, sig)
		if status != http.StatusOK {
			t.Fatalf("subscription.updated webhook want 200 got %d", status)
		}

		var user userSummary
		callJSON(s, t, http.MethodGet, "/v1/admin/users/"+s.ownerID, s.adminToken, nil, http.StatusOK, &user)
		if user.Plan != "pro" {
			t.Fatalf("admin override: plan should stay pro, got %q", user.Plan)
		}
		if user.BillingSource != "admin" {
			t.Fatalf("billing_source want admin got %q", user.BillingSource)
		}
	})
}

// TestBillingStripeCheckoutOptional runs a real Stripe Checkout session when test keys are configured.
func TestBillingStripeCheckoutOptional(t *testing.T) {
	if os.Getenv("FUNCTIONAL_TEST_BILLING_CHECKOUT") != "1" {
		t.Skip("set FUNCTIONAL_TEST_BILLING_CHECKOUT=1 with real sk_test_* and price IDs (see docs/testing.md)")
	}
	if os.Getenv("FUNCTIONAL_TEST_BILLING") != "1" {
		t.Skip("requires FUNCTIONAL_TEST_BILLING=1 and billing-enabled server")
	}

	s := newState()
	s.registerOwner(t)
	s.loginOwner(t)

	type checkoutReq struct {
		Plan string `json:"plan"`
	}
	var out checkoutResult
	callJSON(s, t, http.MethodPost, "/v1/billing/checkout", s.ownerToken, checkoutReq{Plan: "pro"}, http.StatusOK, &out)
	if out.URL == "" || !strings.HasPrefix(out.URL, "https://checkout.stripe.com/") {
		t.Fatalf("unexpected checkout url: %q", out.URL)
	}
	t.Logf("complete checkout in browser: %s", out.URL)
	t.Log("then confirm GET /v1/admin/users/:id shows plan=pro billing_source=stripe")
}

func signedStripeEvent(t *testing.T, secret, eventID, eventType string, dataObject map[string]any) ([]byte, string) {
	t.Helper()
	dataRaw, err := json.Marshal(dataObject)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"id":     eventID,
		"object": "event",
		"type":   eventType,
		"data": map[string]any{
			"object": json.RawMessage(dataRaw),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  secret,
	})
	return signed.Payload, signed.Header
}

func (s *state) postStripeWebhook(t *testing.T, body []byte, sigHeader string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, s.baseURL+"/v1/webhooks/stripe", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", sigHeader)
	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, raw
}

func billingEnabledOnServer(s *state, t *testing.T, token string) bool {
	t.Helper()
	var out billingPlansResult
	status, raw := s.request(t, http.MethodGet, "/v1/billing/plans", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /v1/billing/plans: want 200 got %d body=%s", status, trimBody(raw))
	}
	if err := decodeEnvelope(raw, &out); err != nil {
		t.Fatalf("decode billing plans: %v", err)
	}
	return out.BillingEnabled
}
