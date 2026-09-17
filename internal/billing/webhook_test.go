package billing_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stripe/stripe-go/v81"

	"github.com/SPSingh09/zettabridge/internal/billing"
	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

type webhookStoreStub struct {
	planStoreStub
	events         map[string]string
	billingSource  string
	stripeCustomer string
	stripeSub      string
	stripeIDs      struct {
		customer, subscription string
	}
}

func (s *webhookStoreStub) StripeWebhookEventExists(_ context.Context, eventID string) (bool, error) {
	_, ok := s.events[eventID]
	return ok, nil
}

func (s *webhookStoreStub) RecordStripeWebhookEvent(_ context.Context, eventID, eventType string) (bool, error) {
	if s.events == nil {
		s.events = map[string]string{}
	}
	if _, ok := s.events[eventID]; ok {
		return false, nil
	}
	s.events[eventID] = eventType
	return true, nil
}

func (s *webhookStoreStub) GetUserByStripeCustomerID(_ context.Context, customerID string) (*store.User, error) {
	if s.user != nil && s.stripeCustomer == customerID {
		u := *s.user
		u.BillingSource = s.billingSource
		u.StripeCustomerID = customerID
		u.StripeSubscriptionID = s.stripeSub
		return &u, nil
	}
	return nil, nil
}

func (s *webhookStoreStub) SetUserStripeIDs(_ context.Context, userID, customerID, subscriptionID string) error {
	s.stripeIDs.customer = customerID
	s.stripeIDs.subscription = subscriptionID
	return nil
}

func (s *webhookStoreStub) UpdateUserBillingSource(_ context.Context, userID, source string) error {
	s.billingSource = source
	return nil
}

func (s *webhookStoreStub) ClearUserStripeSubscription(_ context.Context, userID string) error {
	s.stripeSub = ""
	return nil
}

func checkoutSessionEvent(userID, customerID, subID, planName string) stripe.Event {
	raw, _ := json.Marshal(map[string]any{
		"id":                  "cs_test",
		"client_reference_id": userID,
		"customer":            customerID,
		"subscription":        subID,
		"metadata": map[string]string{
			"user_id": userID,
			"plan":    planName,
		},
	})
	return stripe.Event{
		ID:   "evt_checkout",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: raw},
	}
}

func subscriptionEvent(id, eventType, customerID, priceID, status string) stripe.Event {
	raw, _ := json.Marshal(map[string]any{
		"id":       id,
		"customer": customerID,
		"status":   status,
		"items": map[string]any{
			"data": []map[string]any{{
				"price": map[string]string{"id": priceID},
			}},
		},
	})
	return stripe.Event{
		ID:   "evt_" + eventType,
		Type: stripe.EventType(eventType),
		Data: &stripe.EventData{Raw: raw},
	}
}

func TestWebhookCheckoutCompletedUpgradesPro(t *testing.T) {
	stub := &webhookStoreStub{
		planStoreStub: planStoreStub{
			user: &store.User{ID: "u1", Plan: plan.PlanFree},
		},
	}
	proc := billing.NewWebhookProcessor(billingCfg(true), stub)

	if err := proc.Handle(context.Background(), checkoutSessionEvent("u1", "cus_1", "sub_1", plan.PlanPro)); err != nil {
		t.Fatal(err)
	}
	if stub.updated.Plan != plan.PlanPro {
		t.Fatalf("plan=%q", stub.updated.Plan)
	}
	if stub.billingSource != store.BillingSourceStripe {
		t.Fatalf("billing_source=%q", stub.billingSource)
	}
	if stub.stripeIDs.customer != "cus_1" || stub.stripeIDs.subscription != "sub_1" {
		t.Fatalf("stripe ids=%#v", stub.stripeIDs)
	}
	if stub.events["evt_checkout"] != "checkout.session.completed" {
		t.Fatal("event not recorded")
	}
}

func TestWebhookIdempotent(t *testing.T) {
	stub := &webhookStoreStub{
		planStoreStub: planStoreStub{
			user: &store.User{ID: "u1", Plan: plan.PlanFree},
		},
		events: map[string]string{"evt_checkout": "checkout.session.completed"},
	}
	proc := billing.NewWebhookProcessor(billingCfg(true), stub)

	if err := proc.Handle(context.Background(), checkoutSessionEvent("u1", "cus_1", "sub_1", plan.PlanPro)); err != nil {
		t.Fatal(err)
	}
	if stub.updated.Plan != "" {
		t.Fatal("expected duplicate event to skip apply")
	}
}

func TestWebhookSubscriptionUpdatedStripeOnly(t *testing.T) {
	stub := &webhookStoreStub{
		planStoreStub: planStoreStub{
			user: &store.User{ID: "u1", Plan: plan.PlanPro},
		},
		billingSource:  store.BillingSourceStripe,
		stripeCustomer: "cus_1",
	}
	proc := billing.NewWebhookProcessor(billingCfg(true), stub)

	ev := subscriptionEvent("sub_1", "customer.subscription.updated", "cus_1", "price_pro", "active")
	if err := proc.Handle(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if stub.updated.Plan != plan.PlanPro {
		t.Fatalf("update=%#v", stub.updated)
	}
}

func TestWebhookSubscriptionUpdatedSkipsAdmin(t *testing.T) {
	stub := &webhookStoreStub{
		planStoreStub: planStoreStub{
			user: &store.User{ID: "u1", Plan: plan.PlanPro},
		},
		billingSource:  store.BillingSourceAdmin,
		stripeCustomer: "cus_1",
	}
	proc := billing.NewWebhookProcessor(billingCfg(true), stub)

	ev := subscriptionEvent("sub_1", "customer.subscription.updated", "cus_1", "price_pro", "active")
	if err := proc.Handle(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if stub.updated.Plan != "" {
		t.Fatal("admin billing source should skip stripe updates")
	}
}

func TestWebhookSubscriptionDeletedDowngradesFree(t *testing.T) {
	stub := &webhookStoreStub{
		planStoreStub: planStoreStub{
			user: &store.User{ID: "u1", Plan: plan.PlanPro},
		},
		billingSource:  store.BillingSourceStripe,
		stripeCustomer: "cus_1",
		stripeSub:      "sub_1",
	}
	proc := billing.NewWebhookProcessor(billingCfg(true), stub)

	ev := subscriptionEvent("sub_1", "customer.subscription.deleted", "cus_1", "price_pro", "canceled")
	if err := proc.Handle(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if stub.updated.Plan != plan.PlanFree {
		t.Fatalf("plan=%q", stub.updated.Plan)
	}
	if stub.billingSource != store.BillingSourceFree {
		t.Fatalf("billing_source=%q", stub.billingSource)
	}
}

func TestWebhookPaymentFailedAcks(t *testing.T) {
	stub := &webhookStoreStub{}
	proc := billing.NewWebhookProcessor(&config.Config{}, stub)

	raw, _ := json.Marshal(map[string]string{
		"id":       "in_1",
		"customer": "cus_1",
	})
	ev := stripe.Event{
		ID:   "evt_invoice_failed",
		Type: "invoice.payment_failed",
		Data: &stripe.EventData{Raw: raw},
	}
	if err := proc.Handle(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if stub.events["evt_invoice_failed"] != "invoice.payment_failed" {
		t.Fatal("expected payment_failed event recorded")
	}
}
