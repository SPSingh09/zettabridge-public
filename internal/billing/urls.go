package billing

import (
	"strings"

	"github.com/SPSingh09/zettabridge/internal/config"
)

const (
	defaultSuccessPath = "/billing/success?session_id=" + CheckoutSessionIDPlaceholder
	defaultCancelPath  = "/billing/cancel"
)

// CheckoutURLs holds redirect targets for Stripe Checkout (phase B).
type CheckoutURLs struct {
	SuccessURL            string `json:"success_url"`
	CancelURL             string `json:"cancel_url"`
	CancelImmediate       bool   `json:"cancel_immediate"`
	SuccessURLPlaceholder string `json:"success_url_placeholder"`
}

// CheckoutURLsFromConfig builds checkout redirect URLs from runtime config.
func CheckoutURLsFromConfig(cfg *config.Config) CheckoutURLs {
	success := strings.TrimSpace(cfg.BillingSuccessURL)
	if success == "" {
		success = cfg.AppPublicURL + defaultSuccessPath
	}
	cancel := strings.TrimSpace(cfg.BillingCancelURL)
	if cancel == "" {
		cancel = cfg.AppPublicURL + defaultCancelPath
	}
	return CheckoutURLs{
		SuccessURL:            success,
		CancelURL:             cancel,
		CancelImmediate:       CancelImmediate,
		SuccessURLPlaceholder: CheckoutSessionIDPlaceholder,
	}
}
