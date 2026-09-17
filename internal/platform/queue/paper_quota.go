package queue

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

type paperQuotaNotifyLimiter interface {
	TryClaimPaperQuotaNotify(ctx context.Context, userID string) (bool, error)
}

type paperOrderCounter interface {
	CountPaperOrdersByUserInMonth(ctx context.Context, userID string, month time.Time) (int, error)
}

// enforcePaperTradeQuota rejects when the user's plan monthly paper trade cap is exhausted.
func (q *Queue) enforcePaperTradeQuota(ctx context.Context, user *store.User) (rejectReason string, quotaJustHit bool) {
	if user == nil || q.pg == nil {
		return "", false
	}
	if plan.PaperTradesUnlimited(user.Plan) {
		return "", false
	}
	counter, ok := q.pg.(paperOrderCounter)
	if !ok {
		return "", false
	}
	used, err := counter.CountPaperOrdersByUserInMonth(ctx, user.ID, time.Now().UTC())
	if err != nil {
		log.Printf("worker: paper quota count failed user=%s: %v", user.ID, err)
		return "", false
	}
	limits := plan.LimitsFor(user.Plan)
	if err := plan.CanPlacePaperTrade(user.Plan, used); err != nil {
		msg := fmt.Sprintf(
			"monthly paper trade limit reached (%d/%d on %s plan) — upgrade your plan or wait until next month",
			used, limits.MaxPaperTradesPerMonth, user.Plan,
		)
		return msg, used >= limits.MaxPaperTradesPerMonth
	}
	return "", false
}

func (q *Queue) notifyPaperQuotaExceeded(ctx context.Context, user *store.User) {
	if user == nil {
		return
	}
	limits := plan.LimitsFor(user.Plan)
	if limits.MaxPaperTradesPerMonth == 0 {
		return
	}

	shouldNotify := true
	if nl, ok := q.redis.(paperQuotaNotifyLimiter); ok {
		claimed, err := nl.TryClaimPaperQuotaNotify(ctx, user.ID)
		if err != nil {
			log.Printf("worker: paper quota notify dedupe failed user=%s: %v", user.ID, err)
		} else if !claimed {
			shouldNotify = false
		}
	}
	if !shouldNotify {
		return
	}

	upgradeURL := q.billingUpgradeURL
	if upgradeURL == "" {
		upgradeURL = "/billing"
	}
	title := "Paper trade limit reached"
	body := fmt.Sprintf(
		"You've used all %d paper trades on your Free plan this month. Upgrade for unlimited paper trading or wait until next month.",
		limits.MaxPaperTradesPerMonth,
	)
	q.pg.InsertNotification(ctx, user.ID, "paper_quota_exceeded", title, body, "")

	if q.mailer != nil && user.Email != "" {
		if err := q.mailer.SendPaperTradeQuotaExceeded(ctx, user.Email, upgradeURL, limits.MaxPaperTradesPerMonth); err != nil {
			log.Printf("worker: paper quota email failed user=%s: %v", user.ID, err)
		}
	}
	if q.tg != nil && user.TelegramChatID != "" {
		msg := fmt.Sprintf(
			"⚠️ <b>Paper trade limit reached</b>\n\nYou've used all %d paper trades on your Free plan this month.\n<a href=\"%s\">Upgrade your plan</a> or wait until next month.",
			limits.MaxPaperTradesPerMonth, upgradeURL,
		)
		if err := q.tg.Send(ctx, user.TelegramChatID, msg); err != nil {
			log.Printf("worker: paper quota telegram failed user=%s: %v", user.ID, err)
		}
	}
}
