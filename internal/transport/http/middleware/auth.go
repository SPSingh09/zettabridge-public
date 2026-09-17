package middleware

import (
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	jwtware "github.com/gofiber/jwt/v3"
	"github.com/golang-jwt/jwt/v4"

	"github.com/SPSingh09/zettabridge/internal/store"
)

// Auth returns a Fiber middleware that validates Bearer JWT tokens.
func Auth(secret string) fiber.Handler {
	return jwtware.New(jwtware.Config{
		SigningKey: []byte(secret),
		Filter: func(c *fiber.Ctx) bool {
			// WebSocket trade stream authenticates in wsTradesUpgrade (?token= or Bearer).
			return c.Path() == "/v1/ws/trades"
		},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid or missing token",
			})
		},
	})
}

// UserID extracts the authenticated user's ID from the JWT claims.
func UserID(c *fiber.Ctx) string {
	token := c.Locals("user").(*jwt.Token)
	claims := token.Claims.(jwt.MapClaims)
	uid, _ := claims["sub"].(string)
	return uid
}

// UserRole extracts the platform role from JWT claims (defaults to user).
func UserRole(c *fiber.Ctx) string {
	token := c.Locals("user").(*jwt.Token)
	claims := token.Claims.(jwt.MapClaims)
	role, _ := claims["role"].(string)
	if role == "" {
		return "user"
	}
	return role
}

// BearerToken extracts the raw JWT from the Authorization header.
func BearerToken(c *fiber.Ctx) string {
	auth := c.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}

// CheckRevoked rejects JWTs that were logged out before natural expiry.
func CheckRevoked(redis *store.RedisStore) fiber.Handler {
	return func(c *fiber.Ctx) error {
		raw := BearerToken(c)
		if raw == "" {
			return c.Next()
		}
		revoked, err := redis.IsJWTRevoked(c.Context(), raw)
		if err != nil {
			log.Printf("jwt revoke check failed: %v", err)
			return c.Next()
		}
		if revoked {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "token revoked",
			})
		}
		return c.Next()
	}
}

// RequirePlatformAdmin rejects non-admin JWTs.
func RequirePlatformAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if UserRole(c) != "admin" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "platform admin access required",
			})
		}
		return c.Next()
	}
}
