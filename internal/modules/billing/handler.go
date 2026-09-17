package billing

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/stripe/stripe-go/v81/webhook"

	billingpkg "github.com/SPSingh09/zettabridge/internal/billing"
	"github.com/SPSingh09/zettabridge/internal/platform/metrics"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/domain"
)

// Handler handles billing endpoints.
type Handler struct {
	*shared.Handler
	Billing        *billingpkg.Service
	StripeWebhooks *billingpkg.WebhookProcessor
}

func (h *Handler) GetBillingPlans(c *fiber.Ctx) error {
	return shared.Ok(c, billingpkg.CatalogFromConfig(h.Cfg))
}

func (h *Handler) PostBillingCheckout(c *fiber.Ctx) error {
	if h.Billing == nil {
		return shared.Fail(c, fiber.StatusServiceUnavailable, billingpkg.ErrBillingDisabled.Error())
	}

	var body billingpkg.CheckoutRequest
	if err := c.BodyParser(&body); err != nil || body.Plan == "" {
		return shared.Fail(c, fiber.StatusBadRequest, "plan is required")
	}

	url, err := h.Billing.CreateCheckout(c.Context(), middleware.UserID(c), body)
	if err != nil {
		return BillingError(c, err)
	}
	metrics.RecordBillingCheckout()
	return shared.Ok(c, fiber.Map{"url": url})
}

func (h *Handler) PostBillingPortal(c *fiber.Ctx) error {
	if h.Billing == nil {
		return shared.Fail(c, fiber.StatusServiceUnavailable, billingpkg.ErrBillingDisabled.Error())
	}

	url, err := h.Billing.CreatePortal(c.Context(), middleware.UserID(c))
	if err != nil {
		return BillingError(c, err)
	}
	return shared.Ok(c, fiber.Map{"url": url})
}

func (h *Handler) PostStripeWebhook(c *fiber.Ctx) error {
	if !h.Cfg.BillingEnabled || h.StripeWebhooks == nil {
		metrics.RecordBillingWebhook(metrics.BillingWebhookDisabled)
		return shared.Fail(c, fiber.StatusServiceUnavailable, "billing is not enabled")
	}

	sig := c.Get("Stripe-Signature")
	if sig == "" {
		metrics.RecordBillingWebhook(metrics.BillingWebhookSignatureInvalid)
		return shared.Fail(c, fiber.StatusBadRequest, "missing Stripe-Signature header")
	}

	event, err := webhook.ConstructEventWithOptions(c.Body(), sig, h.Cfg.StripeWebhookSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		metrics.RecordBillingWebhook(metrics.BillingWebhookSignatureInvalid)
		return shared.Fail(c, fiber.StatusBadRequest, "invalid webhook signature")
	}

	if err := h.StripeWebhooks.Handle(c.Context(), event); err != nil {
		log.Printf("stripe webhook %s (%s): %v", event.ID, event.Type, err)
		metrics.RecordBillingWebhook(metrics.BillingWebhookError)
		return shared.Fail(c, fiber.StatusInternalServerError, "webhook processing failed")
	}
	metrics.RecordBillingWebhook(metrics.BillingWebhookOK)
	return c.SendStatus(fiber.StatusOK)
}

// BillingError maps billing package errors to HTTP responses.
func BillingError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, billingpkg.ErrBillingDisabled):
		return shared.Fail(c, fiber.StatusServiceUnavailable, err.Error())
	case errors.Is(err, billingpkg.ErrEmailNotVerified):
		return shared.Fail(c, fiber.StatusForbidden, err.Error())
	case errors.Is(err, billingpkg.ErrOrgMemberCheckout):
		return shared.Fail(c, fiber.StatusForbidden, err.Error())
	case errors.Is(err, billingpkg.ErrInvalidCheckoutPlan),
		errors.Is(err, billingpkg.ErrInvalidPlan):
		return shared.Fail(c, fiber.StatusBadRequest, err.Error())
	case billingpkg.SeatLimitConflict(err):
		return shared.Fail(c, fiber.StatusConflict, domain.ErrSeatLimitTooLow.Error())
	case errors.Is(err, billingpkg.ErrNoStripeCustomer):
		return shared.Fail(c, fiber.StatusNotFound, err.Error())
	case errors.Is(err, billingpkg.ErrUserNotFound):
		return shared.Fail(c, fiber.StatusNotFound, err.Error())
	default:
		return shared.Fail(c, fiber.StatusInternalServerError, "billing request failed")
	}
}
