package billing

import "errors"

// Product policy decisions for Stripe integration (PR 4B.1).
const (
	// CancelImmediate downgrades plan when subscription is deleted (no end-of-period grace).
	CancelImmediate = true
)

// CheckoutSessionIDPlaceholder is substituted by Stripe in success URLs.
const CheckoutSessionIDPlaceholder = "{CHECKOUT_SESSION_ID}"

var (
	ErrUserNotFound        = errors.New("user not found")
	ErrInvalidPlan         = errors.New("invalid plan")
	ErrSeatLimitTooLow     = errors.New("seat limit below current org membership")
	ErrBillingDisabled     = errors.New("billing is not enabled")
	ErrEmailNotVerified    = errors.New("email verification required before checkout")
	ErrOrgMemberCheckout   = errors.New("org members cannot manage billing; contact the org owner")
	ErrInvalidCheckoutPlan = errors.New("plan is not available for checkout")
	ErrNoStripeCustomer    = errors.New("no billing account; complete checkout first")
)
