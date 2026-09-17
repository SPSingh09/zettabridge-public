package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

const serviceTokenHeader = "X-ZB-Service-Token"

// RequireServiceToken rejects requests without a matching service token.
func RequireServiceToken(expected string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if strings.TrimSpace(expected) == "" {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "service token not configured",
			})
		}
		got := strings.TrimSpace(c.Get(serviceTokenHeader))
		if got == "" || got != expected {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid service token",
			})
		}
		return c.Next()
	}
}
