package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	authpkg "github.com/SPSingh09/zettabridge/internal/platform/security"
	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/platform/security/emailverify"
	"github.com/SPSingh09/zettabridge/internal/platform/security/invitetoken"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/platform/mailer"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/store"
)

const maxVerifyResendsPerHour = 3

// Handler handles authentication endpoints.
type Handler struct {
	*shared.Handler
	Mail mailer.Sender
}

// ── Package-level helpers ─────────────────────────────────────────────────────

func NormalizeAccountEmail(email string) (string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil {
		return "", err
	}
	return strings.ToLower(addr.Address), nil
}

func LoginBlockedByEmailVerification(cfg *config.Config, user *store.User) bool {
	return cfg.EmailVerificationRequired && user != nil && !user.EmailVerified()
}

func UserEmailVerifiedFields(user *store.User) (verified bool, verifiedAt interface{}) {
	if user == nil || user.EmailVerifiedAt == nil {
		return false, nil
	}
	return true, user.EmailVerifiedAt
}

// ── Handler methods ───────────────────────────────────────────────────────────

func (h *Handler) VerificationURL(rawToken string) string {
	return h.Cfg.AppPublicURL + "/v1/auth/verify-email?token=" + rawToken
}

func (h *Handler) IssueVerificationEmail(c *fiber.Ctx, userID, email string) error {
	if err := h.PG.InvalidatePendingEmailVerifications(c.Context(), userID); err != nil {
		return err
	}

	raw, hash, err := emailverify.Generate()
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	v := &store.EmailVerification{
		ID:        uuid.New().String(),
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: now.Add(h.Cfg.EmailVerificationTTL()),
		CreatedAt: now,
	}
	if err := h.PG.CreateEmailVerification(c.Context(), v); err != nil {
		return err
	}

	verifyURL := h.VerificationURL(raw)
	if err := h.Mail.SendVerification(c.Context(), email, verifyURL); err != nil {
		log.Printf("email_verify send failed user=%s: %v", userID, err)
		return fmt.Errorf("send verification email: %w", err)
	}
	return nil
}

func (h *Handler) Register(c *fiber.Ctx) error {
	var body struct {
		Email      string `json:"email"`
		Password   string `json:"password"`
		InviteCode string `json:"invite_code"`
	}
	if err := c.BodyParser(&body); err != nil || body.Email == "" || body.Password == "" {
		return shared.Fail(c, fiber.StatusBadRequest, "email and password required")
	}
	email, err := NormalizeAccountEmail(body.Email)
	if err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid email")
	}
	if err := authpkg.ValidatePassword(body.Password); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, err.Error())
	}

	var invite *store.PlatformInvite
	if h.Cfg.ClosedRegistration {
		if strings.TrimSpace(body.InviteCode) == "" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error":      "registration is invite-only",
				"error_code": "registration_closed",
			})
		}
		if !invitetoken.ValidateFormat(body.InviteCode) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error":      "invalid or expired invite code",
				"error_code": "invalid_invite",
			})
		}
		invite, err = h.PG.GetValidPlatformInviteByTokenHash(c.Context(), invitetoken.Hash(body.InviteCode))
		if err != nil {
			return shared.Fail(c, fiber.StatusInternalServerError, "invite lookup failed")
		}
		if invite == nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error":      "invalid or expired invite code",
				"error_code": "invalid_invite",
			})
		}
		if invite.Email != "" && !strings.EqualFold(invite.Email, email) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error":      "this invite is for a different email address",
				"error_code": "invite_email_mismatch",
			})
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not hash password")
	}

	user := &store.User{
		ID:           uuid.New().String(),
		Email:        email,
		PasswordHash: string(hash),
		Plan:         domain.PlanFree,
		Role:         domain.RoleUser,
		Status:       domain.StatusActive,
		CreatedAt:    time.Now().UTC(),
	}
	if err := h.PG.CreateUser(c.Context(), user); err != nil {
		return shared.Fail(c, fiber.StatusConflict, "email already registered")
	}

	if invite != nil {
		if err := h.PG.UsePlatformInvite(c.Context(), invite.ID, user.ID); err != nil {
			log.Printf("register: mark invite used failed invite=%s user=%s: %v", invite.ID, user.ID, err)
		}
	}

	if h.Cfg.EmailVerificationRequired {
		if err := h.IssueVerificationEmail(c, user.ID, user.Email); err != nil {
			log.Printf("register verification email failed user=%s: %v", user.ID, err)
		}
	} else {
		if err := h.PG.VerifyUserEmail(c.Context(), user.ID); err != nil {
			log.Printf("register auto-verify failed user=%s: %v", user.ID, err)
		}
	}

	resp := fiber.Map{
		"id":    user.ID,
		"email": user.Email,
	}
	if h.Cfg.EmailVerificationRequired {
		resp["email_verification_required"] = true
	}
	h.LogUserAudit(c, user.ID, "register", nil)
	return shared.Created(c, resp)
}

func (h *Handler) Login(c *fiber.Ctx) error {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}

	email, err := NormalizeAccountEmail(body.Email)
	if err != nil {
		return shared.Fail(c, fiber.StatusUnauthorized, "invalid credentials")
	}

	user, err := h.PG.GetUserByEmail(c.Context(), email)
	if err != nil || user == nil {
		return shared.Fail(c, fiber.StatusUnauthorized, "invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)); err != nil {
		return shared.Fail(c, fiber.StatusUnauthorized, "invalid credentials")
	}
	if user.Status == domain.StatusSuspended {
		return shared.Fail(c, fiber.StatusForbidden, "account suspended")
	}
	if LoginBlockedByEmailVerification(h.Cfg, user) {
		return shared.Fail(c, fiber.StatusForbidden, "email not verified")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  user.ID,
		"role": user.Role,
		"exp":  time.Now().Add(5 * time.Hour).Unix(),
		"jti":  uuid.New().String(),
	})
	signed, err := token.SignedString([]byte(h.Cfg.JWTSecret))
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not issue tokens")
	}
	resp := fiber.Map{"token": signed, "expires_in": 18000}
	if mem, err := h.PG.GetActiveOrgMembershipByUser(c.Context(), user.ID); err == nil && mem != nil {
		resp["org_id"] = mem.OrgID
		resp["org_role"] = mem.Role
	}
	h.LogUserAudit(c, user.ID, "login", nil)
	return shared.Ok(c, resp)
}

func (h *Handler) Logout(c *fiber.Ctx) error {
	raw := middleware.BearerToken(c)
	if raw == "" {
		return shared.Fail(c, fiber.StatusUnauthorized, "invalid or missing token")
	}

	token := c.Locals("user").(*jwt.Token)
	claims := token.Claims.(jwt.MapClaims)
	exp, ok := claims["exp"].(float64)
	if !ok {
		return shared.Fail(c, fiber.StatusUnauthorized, "invalid or missing token")
	}

	ttl := time.Until(time.Unix(int64(exp), 0))
	if ttl > 0 {
		if err := h.Redis.RevokeJWT(c.Context(), raw, ttl); err != nil {
			return shared.Fail(c, fiber.StatusInternalServerError, "logout failed")
		}
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) GetMe(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	user, err := h.LoadUserWithOrgContext(c, uid)
	if err != nil || user == nil {
		return shared.Fail(c, fiber.StatusNotFound, "user not found")
	}
	verified, verifiedAt := UserEmailVerifiedFields(user)
	data := fiber.Map{
		"id":             user.ID,
		"email":          user.Email,
		"plan":           plan.EffectivePlan(user.Plan),
		"role":           user.Role,
		"status":         user.Status,
		"email_verified": verified,
		"created_at":     user.CreatedAt,
	}
	if verifiedAt != nil {
		data["email_verified_at"] = verifiedAt
	}
	h.EnrichMePlanFields(c, data, user)
	hasPaused, _ := h.PG.HasAutoPausedItems(c.Context(), uid)
	data["has_auto_paused_items"] = hasPaused
	return shared.Ok(c, data)
}

func (h *Handler) VerifyEmail(c *fiber.Ctx) error {
	frontendBase := h.Cfg.FrontendURL + "/email-verified"

	raw := strings.TrimSpace(c.Query("token"))
	if !emailverify.ValidateFormat(raw) {
		return c.Redirect(frontendBase+"?error=invalid_token", fiber.StatusFound)
	}

	v, err := h.PG.GetPendingEmailVerificationByHash(c.Context(), emailverify.Hash(raw))
	if err != nil {
		return c.Redirect(frontendBase+"?error=server_error", fiber.StatusFound)
	}
	if v == nil {
		return c.Redirect(frontendBase+"?error=invalid_token", fiber.StatusFound)
	}
	if v.ConsumedAt != nil {
		return c.Redirect(frontendBase+"?error=link_superseded", fiber.StatusFound)
	}
	if time.Now().After(v.ExpiresAt) {
		return c.Redirect(frontendBase+"?error=expired_token", fiber.StatusFound)
	}

	if err := h.PG.ConsumeEmailVerification(c.Context(), v.ID, v.UserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Redirect(frontendBase+"?error=expired_token", fiber.StatusFound)
		}
		return c.Redirect(frontendBase+"?error=server_error", fiber.StatusFound)
	}

	user, err := h.PG.GetUserByID(c.Context(), v.UserID)
	if err != nil || user == nil {
		return c.Redirect(frontendBase+"?error=server_error", fiber.StatusFound)
	}

	return c.Redirect(frontendBase+"?email="+url.QueryEscape(user.Email), fiber.StatusFound)
}

func (h *Handler) ResendVerification(c *fiber.Ctx) error {
	var body struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	email, err := NormalizeAccountEmail(body.Email)
	if err != nil {
		return shared.Ok(c, fiber.Map{"message": "if the account exists and is unverified, a verification email was sent"})
	}

	count, err := h.Redis.IncrVerifyResendLimit(c.Context(), email)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "rate limit check failed")
	}
	if count > maxVerifyResendsPerHour {
		return shared.Fail(c, fiber.StatusTooManyRequests, "too many verification emails sent; try again later")
	}

	user, err := h.PG.GetUserByEmail(c.Context(), email)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if user != nil && !user.EmailVerified() {
		if err := h.IssueVerificationEmail(c, user.ID, user.Email); err != nil {
			log.Printf("resend verification failed user=%s: %v", user.ID, err)
			return shared.Fail(c, fiber.StatusInternalServerError, "could not send verification email; please try again later")
		}
	}

	return shared.Ok(c, fiber.Map{"message": "if the account exists and is unverified, a verification email was sent"})
}
