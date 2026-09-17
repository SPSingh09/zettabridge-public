package config

import "strings"

// PortalReturnURL is where Stripe Customer Portal sends the user after managing billing.
func (c *Config) PortalReturnURL() string {
	if c == nil {
		return ""
	}
	if v := strings.TrimSpace(c.BillingPortalReturnURL); v != "" {
		return v
	}
	if v := strings.TrimSpace(c.BillingCancelURL); v != "" {
		return v
	}
	return strings.TrimRight(c.AppPublicURL, "/") + "/billing/cancel"
}
