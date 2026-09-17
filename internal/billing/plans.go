package billing

import (
	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/plan"
)

// Catalog is returned by GET /v1/billing/plans.
type Catalog struct {
	BillingEnabled bool         `json:"billing_enabled"`
	Checkout       CheckoutURLs `json:"checkout"`
	Plans          []PlanOffer  `json:"plans"`
}

// PlanOffer describes a purchasable or default tier.
// PriceLabel is optional and comes from deployment config (never hardcoded here).
type PlanOffer struct {
	Plan                   string `json:"plan"`
	Name                   string `json:"name"`
	PriceLabel             string `json:"price_label,omitempty"`
	MaxPaperAccounts       int    `json:"max_paper_accounts"`
	MaxPaperWebhooks       int    `json:"max_paper_webhooks"`
	MaxLiveWebhooks        int    `json:"max_live_webhooks"`
	MaxBrokers             int    `json:"max_brokers"`
	MaxPaperTradesPerMonth *int   `json:"max_paper_trades_per_month,omitempty"`
	MaxWebhooks            int    `json:"max_webhooks"` // combined paper + live (legacy)
	OrdersPerSec           *int   `json:"orders_per_sec"`
	LiveTrading             bool   `json:"live_trading"`
	AuditLogs               bool   `json:"audit_logs"`
	MultiProductCredentials bool   `json:"multi_product_credentials"`
}

// CatalogFromConfig builds the public plan matrix for billing UI and API clients.
func CatalogFromConfig(cfg *config.Config) Catalog {
	names := map[string]string{
		plan.PlanFree:    "Free",
		plan.PlanPaper:   "Paper",
		plan.PlanPro:     "Pro",
		plan.PlanProPlus: "Pro Plus",
	}
	offers := make([]PlanOffer, 0, len(plan.ValidPlans))
	for _, p := range plan.ValidPlans {
		offers = append(offers, offerFromLimits(p, names[p], cfg.PlanPriceLabel(p)))
	}
	return Catalog{
		BillingEnabled: cfg.BillingEnabled,
		Checkout:       CheckoutURLsFromConfig(cfg),
		Plans:          offers,
	}
}

func offerFromLimits(planName, name, priceLabel string) PlanOffer {
	limits := plan.LimitsFor(planName)
	v := limits.OrdersPerSec
	offer := PlanOffer{
		Plan:             planName,
		Name:             name,
		PriceLabel:       priceLabel,
		MaxPaperAccounts: limits.MaxPaperAccounts,
		MaxPaperWebhooks: limits.MaxPaperWebhooks,
		MaxLiveWebhooks:  limits.MaxLiveWebhooks,
		MaxBrokers:       limits.MaxBrokers,
		MaxWebhooks:      limits.MaxPaperWebhooks + limits.MaxLiveWebhooks,
		OrdersPerSec:     &v,
		LiveTrading:             limits.LiveAllowed,
		AuditLogs:               limits.AuditLogs,
		MultiProductCredentials: limits.MultiProductCredentials,
	}
	if limits.MaxPaperTradesPerMonth > 0 {
		offer.MaxPaperTradesPerMonth = &limits.MaxPaperTradesPerMonth
	}
	return offer
}

// PriceIDForOffer maps a plan to its configured Stripe price ID (empty when unset).
func PriceIDForOffer(cfg *config.Config, planName string) string {
	if cfg == nil {
		return ""
	}
	switch planName {
	case plan.PlanPaper:
		return cfg.StripePricePaper
	case plan.PlanPro:
		return cfg.StripePricePro
	case plan.PlanProPlus:
		return cfg.StripePriceProPlus
	default:
		return ""
	}
}

// PlanFromStripePriceID resolves a Stripe price ID to a plan name.
func PlanFromStripePriceID(cfg *config.Config, priceID string) (planName string, ok bool) {
	if cfg == nil || priceID == "" {
		return "", false
	}
	switch priceID {
	case cfg.StripePricePaper:
		return plan.PlanPaper, true
	case cfg.StripePricePro:
		return plan.PlanPro, true
	case cfg.StripePriceProPlus:
		return plan.PlanProPlus, true
	default:
		return "", false
	}
}
