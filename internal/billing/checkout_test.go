package billing_test

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/billing"
	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

type checkoutStoreStub struct {
	planStoreStub
	membership *store.OrgMember
	customerID string
}

func (s *checkoutStoreStub) GetUserActiveOrgMembership(_ context.Context, userID string) (*store.OrgMember, error) {
	if s.membership != nil && s.membership.UserID == userID {
		return s.membership, nil
	}
	return nil, nil
}

func (s *checkoutStoreStub) SetUserStripeCustomerID(_ context.Context, userID, customerID string) error {
	s.customerID = customerID
	return nil
}

type stripeStub struct {
	checkoutURL  string
	portalURL    string
	customerID   string
	lastCheckout billing.CheckoutSessionParams
}

func (s *stripeStub) CreateCustomer(email, userID string) (string, error) {
	s.customerID = "cus_test"
	return s.customerID, nil
}

func (s *stripeStub) CreateCheckoutSession(in billing.CheckoutSessionParams) (string, error) {
	s.lastCheckout = in
	if s.checkoutURL == "" {
		return "https://checkout.stripe.com/test", nil
	}
	return s.checkoutURL, nil
}

func (s *stripeStub) CreatePortalSession(customerID, returnURL string) (string, error) {
	if s.portalURL == "" {
		return "https://billing.stripe.com/portal/test", nil
	}
	return s.portalURL, nil
}

func billingCfg(enabled bool) *config.Config {
	return &config.Config{
		AppPublicURL:                 "http://localhost:8080",
		BillingEnabled:               enabled,
		EmailVerificationRequired:    false,
		StripePricePro: "price_pro",
	}
}

func TestCreateCheckoutDisabled(t *testing.T) {
	svc := billing.NewService(billingCfg(false), &checkoutStoreStub{}, &stripeStub{})
	_, err := svc.CreateCheckout(context.Background(), "u1", billing.CheckoutRequest{Plan: plan.PlanPro})
	if !errors.Is(err, billing.ErrBillingDisabled) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateCheckoutPro(t *testing.T) {
	now := time.Now().UTC()
	storeStub := &checkoutStoreStub{
		planStoreStub: planStoreStub{
			user: &store.User{ID: "u1", Email: "a@example.com", Plan: plan.PlanFree, EmailVerifiedAt: &now},
		},
	}
	stripe := &stripeStub{}
	svc := billing.NewService(billingCfg(true), storeStub, stripe)

	url, err := svc.CreateCheckout(context.Background(), "u1", billing.CheckoutRequest{Plan: plan.PlanPro})
	if err != nil {
		t.Fatal(err)
	}
	if url == "" || stripe.lastCheckout.PriceID != "price_pro" || stripe.lastCheckout.UserID != "u1" {
		t.Fatalf("url=%q checkout=%#v", url, stripe.lastCheckout)
	}
	if storeStub.customerID != "cus_test" {
		t.Fatalf("customer not saved: %q", storeStub.customerID)
	}
}

func TestCreateCheckoutOrgMemberForbidden(t *testing.T) {
	storeStub := &checkoutStoreStub{
		planStoreStub: planStoreStub{
			user: &store.User{ID: "u1", Email: "a@example.com", Plan: plan.PlanFree},
		},
		membership: &store.OrgMember{UserID: "u1", Role: domain.RoleMember, Status: domain.StatusActive},
	}
	svc := billing.NewService(billingCfg(true), storeStub, &stripeStub{})
	_, err := svc.CreateCheckout(context.Background(), "u1", billing.CheckoutRequest{Plan: plan.PlanPro})
	if !errors.Is(err, billing.ErrOrgMemberCheckout) {
		t.Fatalf("got %v", err)
	}
}

func TestCreatePortalRequiresCustomer(t *testing.T) {
	storeStub := &checkoutStoreStub{
		planStoreStub: planStoreStub{user: &store.User{ID: "u1", Email: "a@example.com"}},
	}
	svc := billing.NewService(billingCfg(true), storeStub, &stripeStub{})
	_, err := svc.CreatePortal(context.Background(), "u1")
	if !errors.Is(err, billing.ErrNoStripeCustomer) {
		t.Fatalf("got %v", err)
	}
}

func TestCreatePortalOK(t *testing.T) {
	storeStub := &checkoutStoreStub{
		planStoreStub: planStoreStub{
			user: &store.User{ID: "u1", Email: "a@example.com", StripeCustomerID: "cus_existing"},
		},
	}
	svc := billing.NewService(billingCfg(true), storeStub, &stripeStub{})
	url, err := svc.CreatePortal(context.Background(), "u1")
	if err != nil || url == "" {
		t.Fatalf("url=%q err=%v", url, err)
	}
}
