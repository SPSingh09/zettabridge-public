package publisher

import (
	"context"
	"log"
	"time"

	"github.com/SPSingh09/zettabridge/internal/platform/metrics"
	"github.com/SPSingh09/zettabridge/internal/store"
)

const defaultExpiryInterval = 1 * time.Minute

// ExpiryStore is the persistence surface the expiry job needs.
type ExpiryStore interface {
	ExpireStalePublisherOrders(ctx context.Context) ([]string, error)
	GetTradeByID(ctx context.Context, id string) (*store.Trade, error)
}

// TradeNotifier pushes updated trade rows to connected dashboard clients.
type TradeNotifier interface {
	OnTradeInserted(ctx context.Context, trade *store.Trade)
}

// ExpiryJob periodically marks overdue publisher handoffs as expired.
type ExpiryJob struct {
	pg       ExpiryStore
	notify   TradeNotifier
	interval time.Duration
}

func NewExpiryJob(pg ExpiryStore, notify TradeNotifier) *ExpiryJob {
	return &ExpiryJob{pg: pg, notify: notify, interval: defaultExpiryInterval}
}

// Start launches the background goroutine. Cancel ctx to stop it.
func (j *ExpiryJob) Start(ctx context.Context) {
	if j == nil || j.pg == nil {
		return
	}
	go j.run(ctx)
}

func (j *ExpiryJob) run(ctx context.Context) {
	j.tick(ctx)
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.tick(ctx)
		}
	}
}

func (j *ExpiryJob) tick(ctx context.Context) {
	tradeIDs, err := j.pg.ExpireStalePublisherOrders(ctx)
	if err != nil {
		log.Printf("publisher expiry: sweep failed: %v", err)
		return
	}
	if len(tradeIDs) == 0 {
		return
	}
	log.Printf("publisher expiry: marked %d handoff(s) as expired", len(tradeIDs))
	for _, tradeID := range tradeIDs {
		trade, err := j.pg.GetTradeByID(ctx, tradeID)
		if err != nil || trade == nil {
			continue
		}
		metrics.RecordTrade(trade.Status, trade.ErrorCode, "zerodha")
		if j.notify != nil {
			j.notify.OnTradeInserted(ctx, trade)
		}
	}
}
