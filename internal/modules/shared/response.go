package shared

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/SPSingh09/zettabridge/internal/plan"
)

func Fail(c *fiber.Ctx, status int, msg string) error {
	_ = c.Status(status).JSON(fiber.Map{"error": msg})
	return fiber.NewError(status, msg)
}

func Ok(c *fiber.Ctx, data interface{}) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": data})
}

func Created(c *fiber.Ctx, data interface{}) error {
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": data})
}

func PlanError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, plan.ErrLiveNotAllowed),
		errors.Is(err, plan.ErrWebhookLimitReached),
		errors.Is(err, plan.ErrBrokerLimitReached),
		errors.Is(err, plan.ErrPaperAccountLimitReached),
		errors.Is(err, plan.ErrAuditLogsNotAllowed),
		errors.Is(err, plan.ErrAdvancedWebhookGuardsNotAllowed):
		return Fail(c, fiber.StatusForbidden, err.Error())
	case errors.Is(err, plan.ErrInvalidAccountMode):
		return Fail(c, fiber.StatusBadRequest, err.Error())
	default:
		return Fail(c, fiber.StatusBadRequest, err.Error())
	}
}
