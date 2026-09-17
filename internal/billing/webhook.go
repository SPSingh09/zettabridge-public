package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/stripe/stripe-go/v81"

	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// WebhookStore is the persistence surface for Stripe webhook processing.
type WebhookStore interface {
	PlanStore
	StripeWebhookEventExists(ctx context.Context, eventID string) (bool, error)
	RecordStripeWebhookEvent(ctx context.Context, eventID, eventType string) (bool, error)
	GetUserByStripeCustomerID(ctx context.Context, customerID string) (*store.User, error)
	SetUserStripeIDs(ctx context.Context, userID, customerID, subscriptionID string) error
	UpdateUserBillingSource(ctx context.Context, userID, source string) error
	ClearUserStripeSubscription(ctx context.Context, userID string) error
}

// WebhookProcessor applies Stripe subscription events to user plans.
type WebhookProcessor struct {
	cfg     *config.Config
	pg      WebhookStore
	applier *Applier
}

func NewWebhookProcessor(cfg *config.Config, pg WebhookStore) *WebhookProcessor {
	return &WebhookProcessor{
		cfg:     cfg,
		pg:      pg,
		applier: NewApplier(pg),
	}
}

// Handle processes a verified Stripe webhook event with idempotency.
func (p *WebhookProcessor) Handle(ctx context.Context, event stripe.Event) error {
	exists, err := p.pg.StripeWebhookEventExists(ctx, event.ID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	switch string(event.Type) {
	case "checkout.session.completed":
		err = p.handleCheckoutCompleted(ctx, event)
	case "customer.subscription.updated":
		err = p.handleSubscriptionUpdated(ctx, event)
	case "customer.subscription.deleted":
		err = p.handleSubscriptionDeleted(ctx, event)
	case "invoice.payment_failed":
		err = p.handlePaymentFailed(event)
	default:
		return nil
	}
	if err != nil {
		return err
	}

	_, err = p.pg.RecordStripeWebhookEvent(ctx, event.ID, string(event.Type))
	return err
}

func (p *WebhookProcessor) handleCheckoutCompleted(ctx context.Context, event stripe.Event) error {
	var sess stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
		return fmt.Errorf("parse checkout session: %w", err)
	}

	userID := sess.ClientReferenceID
	if userID == "" && sess.Metadata != nil {
		userID = sess.Metadata["user_id"]
	}
	if userID == "" {
		log.Printf("stripe webhook checkout.session.completed: missing user reference session=%s", sess.ID)
		return nil
	}

	planName := ""
	if sess.Metadata != nil {
		planName = sess.Metadata["plan"]
	}
	if planName == "" {
		var ok bool
		planName, ok = planFromCheckoutLineItems(p.cfg, &sess)
		if !ok {
			return fmt.Errorf("unknown plan for checkout session %s", sess.ID)
		}
	}

	if err := p.applier.Apply(ctx, Change{UserID: userID, Plan: planName}); err != nil {
		return err
	}

	customerID := stripeResourceID(sess.Customer)
	subID := stripeResourceID(sess.Subscription)
	if customerID != "" {
		if err := p.pg.SetUserStripeIDs(ctx, userID, customerID, subID); err != nil {
			return err
		}
	}
	return p.pg.UpdateUserBillingSource(ctx, userID, store.BillingSourceStripe)
}

func (p *WebhookProcessor) handleSubscriptionUpdated(ctx context.Context, event stripe.Event) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
		return fmt.Errorf("parse subscription: %w", err)
	}

	user, err := p.userForStripeCustomer(ctx, sub.Customer)
	if err != nil {
		return err
	}
	if user == nil || user.BillingSource != store.BillingSourceStripe {
		return nil
	}
	if !subscriptionGrantsAccess(sub.Status) {
		return nil
	}

	planName, ok := planFromSubscription(p.cfg, &sub)
	if !ok {
		return fmt.Errorf("unknown price on subscription %s", sub.ID)
	}

	if err := p.applier.Apply(ctx, Change{UserID: user.ID, Plan: planName}); err != nil {
		return err
	}

	customerID := stripeResourceID(sub.Customer)
	return p.pg.SetUserStripeIDs(ctx, user.ID, customerID, sub.ID)
}

func (p *WebhookProcessor) handleSubscriptionDeleted(ctx context.Context, event stripe.Event) error {
	if !CancelImmediate {
		return nil
	}

	var sub stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
		return fmt.Errorf("parse subscription: %w", err)
	}

	user, err := p.userForStripeCustomer(ctx, sub.Customer)
	if err != nil {
		return err
	}
	if user == nil || user.BillingSource != store.BillingSourceStripe {
		return nil
	}

	if err := p.applier.Apply(ctx, Change{UserID: user.ID, Plan: plan.PlanFree}); err != nil {
		return err
	}
	if err := p.pg.ClearUserStripeSubscription(ctx, user.ID); err != nil {
		return err
	}
	return p.pg.UpdateUserBillingSource(ctx, user.ID, store.BillingSourceFree)
}

func (p *WebhookProcessor) handlePaymentFailed(event stripe.Event) error {
	var inv stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
		return fmt.Errorf("parse invoice: %w", err)
	}
	log.Printf("stripe invoice.payment_failed: invoice=%s customer=%s",
		inv.ID, stripeResourceID(inv.Customer))
	return nil
}

func (p *WebhookProcessor) userForStripeCustomer(ctx context.Context, customer any) (*store.User, error) {
	customerID := stripeResourceID(customer)
	if customerID == "" {
		return nil, nil
	}
	user, err := p.pg.GetUserByStripeCustomerID(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		log.Printf("stripe webhook: user not found for customer=%s", customerID)
	}
	return user, nil
}

func subscriptionGrantsAccess(status stripe.SubscriptionStatus) bool {
	switch status {
	case stripe.SubscriptionStatusActive, stripe.SubscriptionStatusTrialing:
		return true
	default:
		return false
	}
}

func planFromSubscription(cfg *config.Config, sub *stripe.Subscription) (planName string, ok bool) {
	if sub.Metadata != nil {
		if p := sub.Metadata["plan"]; p != "" {
			return p, true
		}
	}
	if sub.Items != nil && len(sub.Items.Data) > 0 && sub.Items.Data[0].Price != nil {
		return PlanFromStripePriceID(cfg, sub.Items.Data[0].Price.ID)
	}
	return "", false
}

func planFromCheckoutLineItems(cfg *config.Config, sess *stripe.CheckoutSession) (planName string, ok bool) {
	if sess.LineItems == nil || len(sess.LineItems.Data) == 0 {
		return "", false
	}
	item := sess.LineItems.Data[0]
	if item.Price == nil {
		return "", false
	}
	return PlanFromStripePriceID(cfg, item.Price.ID)
}

func stripeResourceID(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case *stripe.Customer:
		if x != nil {
			return x.ID
		}
	case stripe.Customer:
		return x.ID
	case *stripe.Subscription:
		if x != nil {
			return x.ID
		}
	case stripe.Subscription:
		return x.ID
	case *stripe.Invoice:
		if x != nil {
			return stripeResourceID(x.Customer)
		}
	case stripe.Invoice:
		return stripeResourceID(x.Customer)
	}
	return ""
}
