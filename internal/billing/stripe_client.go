package billing

import (
	"fmt"

	"github.com/stripe/stripe-go/v81"
	portalsession "github.com/stripe/stripe-go/v81/billingportal/session"
	checkoutsession "github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/customer"
)

// StripeClient implements StripeGateway using the official Stripe SDK.
type StripeClient struct{}

func NewStripeClient(secretKey string) *StripeClient {
	stripe.Key = secretKey
	return &StripeClient{}
}

func (c *StripeClient) CreateCustomer(email, userID string) (string, error) {
	params := &stripe.CustomerParams{
		Email: stripe.String(email),
	}
	params.AddMetadata("user_id", userID)
	cust, err := customer.New(params)
	if err != nil {
		return "", fmt.Errorf("stripe create customer: %w", err)
	}
	return cust.ID, nil
}

func (c *StripeClient) CreateCheckoutSession(in CheckoutSessionParams) (string, error) {
	metadata := map[string]string{
		"user_id": in.UserID,
		"plan":    in.Plan,
	}

	params := &stripe.CheckoutSessionParams{
		Mode:              stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		Customer:          stripe.String(in.CustomerID),
		ClientReferenceID: stripe.String(in.UserID),
		SuccessURL:        stripe.String(in.SuccessURL),
		CancelURL:         stripe.String(in.CancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(in.PriceID),
				Quantity: stripe.Int64(1),
			},
		},
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata:        metadata,
			TrialPeriodDays: stripe.Int64(14),
		},
	}
	for k, v := range metadata {
		params.AddMetadata(k, v)
	}

	sess, err := checkoutsession.New(params)
	if err != nil {
		return "", fmt.Errorf("stripe checkout session: %w", err)
	}
	if sess.URL == "" {
		return "", fmt.Errorf("stripe checkout session: empty url")
	}
	return sess.URL, nil
}

func (c *StripeClient) CreatePortalSession(customerID, returnURL string) (string, error) {
	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(customerID),
		ReturnURL: stripe.String(returnURL),
	}
	sess, err := portalsession.New(params)
	if err != nil {
		return "", fmt.Errorf("stripe portal session: %w", err)
	}
	if sess.URL == "" {
		return "", fmt.Errorf("stripe portal session: empty url")
	}
	return sess.URL, nil
}
