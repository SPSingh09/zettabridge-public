package queue

import (
	"context"
	"fmt"
	"log"

	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func (q *Queue) dispatchAlerts(ctx context.Context, trade *store.Trade) {
	user, err := q.pg.GetUserByID(ctx, trade.UserID)
	if err != nil || user == nil {
		return
	}
	if plan.CanReceiveNotifications(user.Plan) != nil {
		return
	}

	title, body := notificationText(trade)
	q.pg.InsertNotification(ctx, user.ID, trade.Status, title, body, trade.ID)

	if q.tg != nil && user.TelegramChatID != "" {
		msg := telegramText(trade)
		if err := q.tg.Send(ctx, user.TelegramChatID, msg); err != nil {
			log.Printf("telegram: send failed user=%s: %v", user.ID, err)
		}
	}
}

func notificationText(t *store.Trade) (title, body string) {
	label := t.WebhookLabel
	if label == "" {
		label = t.WebhookID
	}
	switch t.Status {
	case "filled":
		title = fmt.Sprintf("Order Filled — %s", t.Symbol)
		body = fmt.Sprintf("%s | %s | Lot: %g | Fill: %g | Webhook: %s",
			t.Signal, t.OrderType, t.LotSize, t.FillPrice, label)
	case "submitted":
		title = fmt.Sprintf("Order Submitted — %s", t.Symbol)
		body = fmt.Sprintf("%s | %s | Lot: %g | Webhook: %s",
			t.Signal, t.OrderType, t.LotSize, label)
	case "rejected":
		title = fmt.Sprintf("Order Rejected — %s", t.Symbol)
		body = fmt.Sprintf("%s | Reason: %s | Webhook: %s",
			t.Signal, t.Error, label)
	case "cancelled":
		title = fmt.Sprintf("Order Cancelled — %s", t.Symbol)
		body = fmt.Sprintf("%s | Webhook: %s", t.Signal, label)
	default:
		title = fmt.Sprintf("Trade Update — %s", t.Symbol)
		body = fmt.Sprintf("%s | Status: %s | Webhook: %s", t.Signal, t.Status, label)
	}
	return title, body
}

func telegramText(t *store.Trade) string {
	label := t.WebhookLabel
	if label == "" {
		label = t.WebhookID
	}
	switch t.Status {
	case "filled":
		return fmt.Sprintf(
			"✅ <b>Order Filled</b> — %s\nWebhook: %s\n%s | Lot: %g | Fill: %g\n<code>ID: %s</code>",
			t.Symbol, label, t.Signal, t.LotSize, t.FillPrice, t.ID)
	case "submitted":
		return fmt.Sprintf(
			"📤 <b>Order Submitted</b> — %s\nWebhook: %s\n%s | Lot: %g\n<code>ID: %s</code>",
			t.Symbol, label, t.Signal, t.LotSize, t.ID)
	case "rejected":
		return fmt.Sprintf(
			"❌ <b>Order Rejected</b> — %s\nWebhook: %s\n%s | Reason: %s\n<code>ID: %s</code>",
			t.Symbol, label, t.Signal, t.Error, t.ID)
	case "cancelled":
		return fmt.Sprintf(
			"🚫 <b>Order Cancelled</b> — %s\nWebhook: %s\n%s\n<code>ID: %s</code>",
			t.Symbol, label, t.Signal, t.ID)
	default:
		return fmt.Sprintf(
			"ℹ️ <b>Trade Update</b> — %s\nWebhook: %s\n%s | Status: %s\n<code>ID: %s</code>",
			t.Symbol, label, t.Signal, t.Status, t.ID)
	}
}
