package invites

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/SPSingh09/zettabridge/internal/platform/security/invitetoken"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/platform/mailer"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/integrations/telegram"
)

// Handler handles org invite endpoints.
type Handler struct {
	*shared.Handler
	Mail mailer.Sender
	TG   *telegram.Sender
}

func (h *Handler) CreateOrgInvite(c *fiber.Ctx) error {
	orgID := c.Params("id")
	actorID := middleware.UserID(c)

	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	email, err := NormalizeInviteEmail(body.Email)
	if err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid email")
	}
	if body.Role == "" {
		body.Role = domain.RoleMember
	}
	if body.Role == domain.RoleOwner {
		return shared.Fail(c, fiber.StatusBadRequest, domain.ErrCannotInviteOwner.Error())
	}
	if !domain.IsInvitableRole(body.Role) {
		return shared.Fail(c, fiber.StatusBadRequest, domain.ErrInvalidRole.Error())
	}

	org, err := h.PG.GetOrgByID(c.Context(), orgID)
	if err != nil || org == nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}

	seatsUsed, err := h.PG.CountSeatsUsed(c.Context(), orgID)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "seat lookup failed")
	}
	if seatsUsed >= org.SeatLimit {
		return shared.Fail(c, fiber.StatusConflict, domain.ErrSeatLimitReached.Error())
	}

	if existing, err := h.PG.GetOrgMemberByEmail(c.Context(), orgID, email); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	} else if existing != nil {
		return shared.Fail(c, fiber.StatusConflict, domain.ErrAlreadyMember.Error())
	}

	if user, err := h.PG.GetUserByEmail(c.Context(), email); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	} else if user != nil {
		inOrg, err := h.PG.UserInAnyOrg(c.Context(), user.ID)
		if err != nil {
			return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
		}
		if inOrg {
			return shared.Fail(c, fiber.StatusConflict, domain.ErrUserInOtherOrg.Error())
		}
		if user.Status != domain.StatusActive {
			return shared.Fail(c, fiber.StatusConflict, "invited account is not active")
		}
	}

	rawToken, tokenHash, err := invitetoken.Generate()
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not generate invite token")
	}

	now := time.Now().UTC()
	inv := &store.OrgInvite{
		ID:        uuid.New().String(),
		OrgID:     orgID,
		Email:     email,
		Role:      body.Role,
		TokenHash: tokenHash,
		ExpiresAt: now.Add(time.Duration(h.Cfg.OrgInviteTTLDays) * 24 * time.Hour),
		InvitedBy: actorID,
		CreatedAt: now,
	}
	if err := h.PG.CreateOrgInvite(c.Context(), inv); err != nil {
		if strings.Contains(err.Error(), "idx_org_invites_pending_unique") ||
			strings.Contains(err.Error(), "duplicate key") {
			return shared.Fail(c, fiber.StatusConflict, domain.ErrPendingInviteExists.Error())
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not create invite")
	}

	inviteURL := h.Cfg.FrontendURL + "/invites/" + rawToken
	if err := h.Mail.SendOrgInvite(c.Context(), email, org.Name, inviteURL); err != nil {
		log.Printf("org_invite: email failed invite=%s to=%s: %v", inv.ID, email, err)
	}

	// If the invitee already has a ZettaBridge account with Telegram connected,
	// also deliver the invite via Telegram for faster visibility.
	if existingUser, _ := h.PG.GetUserByEmail(c.Context(), email); existingUser != nil && existingUser.TelegramChatID != "" {
		msg := fmt.Sprintf(
			"📩 <b>You've been invited to %s</b>\n\nYou have been invited to join the <b>%s</b> workspace on ZettaBridge.\n\n<a href=\"%s\">Accept invitation</a>\n\n<i>This link expires in 7 days.</i>",
			org.Name, org.Name, inviteURL,
		)
		if err := h.TG.Send(c.Context(), existingUser.TelegramChatID, msg); err != nil {
			log.Printf("org_invite: telegram failed invite=%s chat=%s: %v", inv.ID, existingUser.TelegramChatID, err)
		}
	}

	return shared.Created(c, fiber.Map{
		"id":         inv.ID,
		"email":      inv.Email,
		"role":       inv.Role,
		"expires_at": inv.ExpiresAt,
		"token":      rawToken,
	})
}

func (h *Handler) ListOrgInvites(c *fiber.Ctx) error {
	orgID := c.Params("id")
	invites, err := h.PG.ListPendingOrgInvites(c.Context(), orgID)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not list invites")
	}
	if invites == nil {
		invites = []*store.OrgInvite{}
	}

	items := make([]fiber.Map, 0, len(invites))
	for _, inv := range invites {
		items = append(items, fiber.Map{
			"id":         inv.ID,
			"email":      inv.Email,
			"role":       inv.Role,
			"status":     "pending",
			"expires_at": inv.ExpiresAt,
			"invited_by": inv.InvitedBy,
			"created_at": inv.CreatedAt,
		})
	}
	return shared.Ok(c, items)
}

func (h *Handler) RevokeOrgInvite(c *fiber.Ctx) error {
	orgID := c.Params("id")
	inviteID := c.Params("inviteId")
	if _, err := uuid.Parse(inviteID); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid invite id")
	}

	if err := h.PG.RevokeOrgInvite(c.Context(), orgID, inviteID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return shared.Fail(c, fiber.StatusNotFound, domain.ErrInviteNotFound.Error())
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not revoke invite")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) PreviewInvite(c *fiber.Ctx) error {
	raw := c.Params("token")
	if !invitetoken.ValidateFormat(raw) {
		return shared.Fail(c, fiber.StatusNotFound, domain.ErrInvalidInvite.Error())
	}

	inv, err := h.PG.GetPendingOrgInviteByTokenHash(c.Context(), invitetoken.Hash(raw))
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if inv == nil || inv.AcceptedAt != nil || inv.RevokedAt != nil || time.Now().After(inv.ExpiresAt) {
		return shared.Fail(c, fiber.StatusNotFound, domain.ErrInvalidInvite.Error())
	}

	org, err := h.PG.GetOrgByID(c.Context(), inv.OrgID)
	if err != nil || org == nil || org.Status != domain.OrgStatusActive {
		return shared.Fail(c, fiber.StatusNotFound, domain.ErrInvalidInvite.Error())
	}

	return shared.Ok(c, store.OrgInvitePreview{
		OrgName:   org.Name,
		Email:     inv.Email,
		Role:      inv.Role,
		ExpiresAt: inv.ExpiresAt,
	})
}

func (h *Handler) AcceptInvite(c *fiber.Ctx) error {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	if !invitetoken.ValidateFormat(body.Token) {
		return shared.Fail(c, fiber.StatusBadRequest, domain.ErrInvalidInvite.Error())
	}
	if utf8.RuneCountInString(body.Password) < 8 {
		return shared.Fail(c, fiber.StatusBadRequest, "password must be at least 8 characters")
	}

	inv, err := h.PG.GetPendingOrgInviteByTokenHash(c.Context(), invitetoken.Hash(body.Token))
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if inv == nil || inv.AcceptedAt != nil || inv.RevokedAt != nil || time.Now().After(inv.ExpiresAt) {
		return shared.Fail(c, fiber.StatusBadRequest, domain.ErrInvalidInvite.Error())
	}

	existing, err := h.PG.GetUserByEmail(c.Context(), inv.Email)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}

	var user *store.User
	if existing != nil {
		if existing.Status != domain.StatusActive {
			return shared.Fail(c, fiber.StatusForbidden, "account suspended")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(existing.PasswordHash), []byte(body.Password)); err != nil {
			return shared.Fail(c, fiber.StatusUnauthorized, "invalid credentials")
		}
		inOrg, err := h.PG.UserInAnyOrg(c.Context(), existing.ID)
		if err != nil {
			return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
		}
		if inOrg {
			return shared.Fail(c, fiber.StatusConflict, domain.ErrUserInOtherOrg.Error())
		}
		user = existing
	} else {
		hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil {
			return shared.Fail(c, fiber.StatusInternalServerError, "could not hash password")
		}
		user = &store.User{
			ID:           uuid.New().String(),
			Email:        inv.Email,
			PasswordHash: string(hash),
			Plan:         domain.PlanFree,
			Role:         domain.RoleUser,
			Status:       domain.StatusActive,
			OrgsEnabled:  false,
			CreatedAt:    time.Now().UTC(),
		}
	}

	member := &store.OrgMember{
		ID:       uuid.New().String(),
		UserID:   user.ID,
		Role:     inv.Role,
		Status:   domain.StatusActive,
		JoinedAt: time.Now().UTC(),
	}

	tokens, err := h.PG.AcceptOrgInvite(c.Context(), inv.ID, user, member)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return shared.Fail(c, fiber.StatusBadRequest, domain.ErrInvalidInvite.Error())
		}
		if strings.Contains(err.Error(), "seat limit") {
			return shared.Fail(c, fiber.StatusConflict, domain.ErrSeatLimitReached.Error())
		}
		if strings.Contains(err.Error(), "already belongs") {
			return shared.Fail(c, fiber.StatusConflict, domain.ErrUserInOtherOrg.Error())
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not accept invite")
	}
	h.InvalidateWebhookTokens(c.Context(), tokens)

	// Pause the new member's solo webhooks and credentials so they don't run
	// while the user operates within the household org. They will be unpaused
	// automatically if the member is ever removed from the org.
	if err := h.PG.PauseAllWebhooksByUser(c.Context(), user.ID); err != nil {
		log.Printf("acceptInvite: pause webhooks user=%s: %v", user.ID, err)
	}
	if err := h.PG.PauseAllCredentialsByUser(c.Context(), user.ID); err != nil {
		log.Printf("acceptInvite: pause credentials user=%s: %v", user.ID, err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  user.ID,
		"role": user.Role,
		"exp":  time.Now().Add(24 * time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(h.Cfg.JWTSecret))
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not sign token")
	}

	return shared.Ok(c, fiber.Map{
		"token":      signed,
		"expires_in": 86400,
		"org_id":     member.OrgID,
		"org_role":   member.Role,
		"user_id":    user.ID,
		"email":      user.Email,
	})
}

// NormalizeInviteEmail parses and lower-cases an email address.
func NormalizeInviteEmail(email string) (string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil {
		return "", err
	}
	return strings.ToLower(addr.Address), nil
}
