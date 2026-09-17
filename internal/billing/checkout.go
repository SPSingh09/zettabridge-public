package billing

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"context"

	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// CheckoutStore is the persistence surface for Stripe checkout and portal.
type CheckoutStore interface {
	PlanStore
	GetUserActiveOrgMembership(ctx context.Context, userID string) (*store.OrgMember, error)
	SetUserStripeCustomerID(ctx context.Context, userID, customerID string) error
}

// StripeGateway creates Stripe customers and hosted sessions.
type StripeGateway interface {
	CreateCustomer(email, userID string) (customerID string, err error)
	CreateCheckoutSession(in CheckoutSessionParams) (url string, err error)
	CreatePortalSession(customerID, returnURL string) (url string, err error)
}

// CheckoutSessionParams configures a Stripe Checkout subscription session.
type CheckoutSessionParams struct {
	CustomerID string
	PriceID    string
	UserID     string
	Plan       string
	SuccessURL string
	CancelURL  string
}

// CheckoutRequest is the body for POST /v1/billing/checkout.
type CheckoutRequest struct {
	Plan string `json:"plan"`
}

// Service orchestrates Stripe checkout and customer portal sessions.
type Service struct {
	cfg     *config.Config
	pg      CheckoutStore
	stripe  StripeGateway
	applier *Applier
}

func NewService(cfg *config.Config, pg CheckoutStore, stripe StripeGateway) *Service {
	return &Service{
		cfg:     cfg,
		pg:      pg,
		stripe:  stripe,
		applier: NewApplier(pg),
	}
}

func (s *Service) CreateCheckout(ctx context.Context, userID string, req CheckoutRequest) (string, error) {
	if !s.cfg.BillingEnabled {
		return "", ErrBillingDisabled
	}
	if req.Plan == plan.PlanFree {
		return "", ErrInvalidCheckoutPlan
	}

	user, err := s.pg.GetUserByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if user == nil {
		return "", ErrUserNotFound
	}
	if s.cfg.EmailVerificationRequired && !user.EmailVerified() {
		return "", ErrEmailNotVerified
	}
	if err := s.guardOrgCheckout(ctx, userID); err != nil {
		return "", err
	}

	if err := s.applier.Validate(ctx, Change{UserID: userID, Plan: req.Plan}); err != nil {
		return "", err
	}

	priceID := PriceIDForOffer(s.cfg, req.Plan)
	if priceID == "" {
		return "", ErrInvalidCheckoutPlan
	}

	customerID := user.StripeCustomerID
	if customerID == "" {
		customerID, err = s.stripe.CreateCustomer(user.Email, userID)
		if err != nil {
			return "", err
		}
		if err := s.pg.SetUserStripeCustomerID(ctx, userID, customerID); err != nil {
			return "", err
		}
	}

	urls := CheckoutURLsFromConfig(s.cfg)
	return s.stripe.CreateCheckoutSession(CheckoutSessionParams{
		CustomerID: customerID,
		PriceID:    priceID,
		UserID:     userID,
		Plan:       req.Plan,
		SuccessURL: urls.SuccessURL,
		CancelURL:  urls.CancelURL,
	})
}

func (s *Service) CreatePortal(ctx context.Context, userID string) (string, error) {
	if !s.cfg.BillingEnabled {
		return "", ErrBillingDisabled
	}

	user, err := s.pg.GetUserByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if user == nil {
		return "", ErrUserNotFound
	}
	if err := s.guardOrgCheckout(ctx, userID); err != nil {
		return "", err
	}
	if user.StripeCustomerID == "" {
		return "", ErrNoStripeCustomer
	}

	return s.stripe.CreatePortalSession(user.StripeCustomerID, s.cfg.PortalReturnURL())
}

func (s *Service) guardOrgCheckout(ctx context.Context, userID string) error {
	mem, err := s.pg.GetUserActiveOrgMembership(ctx, userID)
	if err != nil {
		return err
	}
	if mem != nil && mem.Role != domain.RoleOwner {
		return ErrOrgMemberCheckout
	}
	return nil
}
