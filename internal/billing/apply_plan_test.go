package billing_test

import (
	"context"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/billing"
	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

type planStoreStub struct {
	user    *store.User
	updated billing.Change
}

func (s *planStoreStub) GetUserByID(_ context.Context, id string) (*store.User, error) {
	if s.user != nil && s.user.ID == id {
		return s.user, nil
	}
	return nil, nil
}

func (s *planStoreStub) UpdateUserPlan(_ context.Context, id, planName string) error {
	s.updated = billing.Change{UserID: id, Plan: planName}
	return nil
}

func (s *planStoreStub) PauseAllWebhooksByUser(_ context.Context, _ string) error    { return nil }
func (s *planStoreStub) PauseAllCredentialsByUser(_ context.Context, _ string) error { return nil }
func (s *planStoreStub) ClearAutoPausedWebhooksByUser(_ context.Context, _ string) error {
	return nil
}
func (s *planStoreStub) ClearAutoPausedCredentialsByUser(_ context.Context, _ string) error {
	return nil
}

func TestApplierApplyPro(t *testing.T) {
	stub := &planStoreStub{
		user: &store.User{ID: "u1", Plan: plan.PlanFree},
	}
	applier := billing.NewApplier(stub)
	if err := applier.Apply(context.Background(), billing.Change{
		UserID: "u1", Plan: plan.PlanPro,
	}); err != nil {
		t.Fatal(err)
	}
	if stub.updated.Plan != plan.PlanPro {
		t.Fatalf("unexpected update: %#v", stub.updated)
	}
}

func TestApplierDowngradeToFree(t *testing.T) {
	stub := &planStoreStub{
		user: &store.User{ID: "u1", Plan: plan.PlanPro},
	}
	applier := billing.NewApplier(stub)
	if err := applier.Apply(context.Background(), billing.Change{
		UserID: "u1", Plan: plan.PlanFree,
	}); err != nil {
		t.Fatal(err)
	}
	if stub.updated.Plan != plan.PlanFree {
		t.Fatalf("unexpected update: %#v", stub.updated)
	}
}

func TestCatalogFromConfig(t *testing.T) {
	cfg := &config.Config{
		AppPublicURL:   "http://localhost:8080",
		BillingEnabled: false,
	}
	cat := billing.CatalogFromConfig(cfg)
	if len(cat.Plans) != 4 {
		t.Fatalf("expected 4 plans, got %d", len(cat.Plans))
	}
	if cat.Plans[0].PriceLabel != "" {
		t.Fatal("expected empty price labels by default")
	}
	if !cat.Checkout.CancelImmediate {
		t.Fatal("expected cancel_immediate policy flag enabled")
	}
	if cat.Checkout.SuccessURL != "http://localhost:8080/billing/success?session_id={CHECKOUT_SESSION_ID}" {
		t.Fatalf("unexpected success url: %s", cat.Checkout.SuccessURL)
	}
}

func TestPlanFromStripePriceID(t *testing.T) {
	cfg := &config.Config{
		StripePricePaper:   "price_paper",
		StripePricePro:     "price_pro",
		StripePriceProPlus: "price_pro_plus",
	}
	for _, tc := range []struct {
		price string
		plan  string
	}{
		{"price_paper", plan.PlanPaper},
		{"price_pro", plan.PlanPro},
		{"price_pro_plus", plan.PlanProPlus},
	} {
		planName, ok := billing.PlanFromStripePriceID(cfg, tc.price)
		if !ok || planName != tc.plan {
			t.Fatalf("price %q => %q ok=%v", tc.price, planName, ok)
		}
	}
}
