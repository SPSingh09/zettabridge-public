package billing

import (
	"context"
	"database/sql"
	"errors"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// PlanStore is the persistence surface required to apply subscription plans.
type PlanStore interface {
	GetUserByID(ctx context.Context, id string) (*store.User, error)
	UpdateUserPlan(ctx context.Context, id, planName string) error
	PauseAllWebhooksByUser(ctx context.Context, userID string) error
	PauseAllCredentialsByUser(ctx context.Context, userID string) error
	ClearAutoPausedWebhooksByUser(ctx context.Context, userID string) error
	ClearAutoPausedCredentialsByUser(ctx context.Context, userID string) error
}

// Applier validates and persists plan changes (admin override and Stripe webhooks).
type Applier struct {
	pg PlanStore
}

func NewApplier(pg PlanStore) *Applier {
	return &Applier{pg: pg}
}

// Change describes a target subscription plan for a user.
type Change struct {
	UserID string
	Plan   string
}

// Validate checks plan change rules without persisting (used before Stripe checkout).
func (a *Applier) Validate(ctx context.Context, ch Change) error {
	if !domain.IsValidPlan(ch.Plan) {
		return ErrInvalidPlan
	}

	current, err := a.pg.GetUserByID(ctx, ch.UserID)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrUserNotFound
	}
	return nil
}

func planRank(p string) int {
	switch p {
	case plan.PlanFree:
		return 0
	case plan.PlanPaper:
		return 1
	case plan.PlanPro:
		return 2
	case plan.PlanProPlus:
		return 3
	default:
		return 0
	}
}

// Apply updates user plan and pauses or clears auto-paused resources on tier changes.
func (a *Applier) Apply(ctx context.Context, ch Change) error {
	if err := a.Validate(ctx, ch); err != nil {
		return err
	}

	before, err := a.pg.GetUserByID(ctx, ch.UserID)
	if err != nil {
		return err
	}
	isDowngrade := planRank(ch.Plan) < planRank(before.Plan)
	isUpgrade := planRank(ch.Plan) > planRank(before.Plan)

	if err := a.pg.UpdateUserPlan(ctx, ch.UserID, ch.Plan); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}

	if isDowngrade {
		_ = a.pg.PauseAllWebhooksByUser(ctx, ch.UserID)
		_ = a.pg.PauseAllCredentialsByUser(ctx, ch.UserID)
	} else if isUpgrade {
		_ = a.pg.ClearAutoPausedWebhooksByUser(ctx, ch.UserID)
		_ = a.pg.ClearAutoPausedCredentialsByUser(ctx, ch.UserID)
	}

	return nil
}

// SeatLimitConflict reports whether err is a seat-limit validation failure.
func SeatLimitConflict(err error) bool {
	return errors.Is(err, ErrSeatLimitTooLow) || errors.Is(err, domain.ErrSeatLimitTooLow)
}
