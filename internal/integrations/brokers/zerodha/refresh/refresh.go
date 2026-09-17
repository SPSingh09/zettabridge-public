// Package zerodharefresh runs a daily goroutine that notifies users to reconnect
// their Zerodha account before the 6:00 AM IST token expiry.
//
// Kite Connect access tokens expire at a fixed daily cutoff of 6:00 AM IST.
// We notify at 5:50 AM IST (00:20 UTC) to give users 10 minutes to reconnect.
package zerodharefresh

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/SPSingh09/zettabridge/internal/platform/mailer"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/integrations/telegram"
)

var ist = time.FixedZone("IST", 5*3600+30*60)

// Notifier sends daily Zerodha reconnect reminders.
type Notifier struct {
	pg           *store.PGStore
	mail         mailer.Sender
	tg           *telegram.Sender
	reconnectURL string // e.g. https://app.zettabridge.net/dashboard
}

func New(pg *store.PGStore, mail mailer.Sender, tg *telegram.Sender, reconnectURL string) *Notifier {
	return &Notifier{pg: pg, mail: mail, tg: tg, reconnectURL: reconnectURL}
}

// Start launches the background goroutine. It returns immediately.
// Cancel ctx to stop it (called on graceful shutdown).
func (n *Notifier) Start(ctx context.Context) {
	go n.run(ctx)
}

func (n *Notifier) run(ctx context.Context) {
	// Track the last date notifications were sent so we fire exactly once per day.
	var lastSentDate string

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			ist := t.In(ist)
			// Fire at 05:50 IST, once per calendar day.
			if ist.Hour() == 5 && ist.Minute() == 50 {
				today := ist.Format("2006-01-02")
				if today == lastSentDate {
					continue
				}
				lastSentDate = today
				n.notify(ctx)
			}
		}
	}
}

func (n *Notifier) notify(ctx context.Context) {
	contacts, err := n.pg.ListZerodhaCredentialContacts(ctx)
	if err != nil {
		log.Printf("zerodharefresh: query failed: %v", err)
		return
	}
	if len(contacts) == 0 {
		return
	}
	log.Printf("zerodharefresh: sending reconnect reminder to %d user(s)", len(contacts))
	for _, c := range contacts {
		if err := n.mail.SendZerodhaReconnect(ctx, c.Email, n.reconnectURL); err != nil {
			log.Printf("zerodharefresh: email to %s failed: %v", c.Email, err)
		}
		if c.TelegramChatID != "" {
			msg := fmt.Sprintf(
				"⚠️ <b>Reconnect Zerodha</b>\n\nYour Kite Connect access token expires at <b>6:00 AM IST</b>. You have ~10 minutes to reconnect.\n\n<a href=\"%s\">Reconnect now</a>",
				n.reconnectURL,
			)
			if err := n.tg.Send(ctx, c.TelegramChatID, msg); err != nil {
				log.Printf("zerodharefresh: telegram to chat %s failed: %v", c.TelegramChatID, err)
			}
		}
	}
}
