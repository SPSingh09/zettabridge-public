// Package fillpoll polls Kite Connect for submitted Zerodha OAuth orders and
// transitions trades to filled/rejected/cancelled once Kite reports a terminal status.
package fillpoll

import (
	"context"
	"log"
	"time"

	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
	credenc "github.com/SPSingh09/zettabridge/internal/platform/crypto"
	"github.com/SPSingh09/zettabridge/internal/platform/metrics"
	"github.com/SPSingh09/zettabridge/internal/store"
)

const defaultMaxAge = 48 * time.Hour

// Store is the persistence surface the fill poll job needs.
type Store interface {
	ListPendingZerodhaOAuthFills(ctx context.Context, maxAge time.Duration, limit int) ([]store.PendingFillTrade, error)
	GetBrokerCred(ctx context.Context, id string) (*store.BrokerCredential, error)
	UpdateTradeFillOutcome(ctx context.Context, id, status string, fillPrice float64, errMsg string) error
}

// TradeNotifier pushes trade updates to connected clients.
type TradeNotifier interface {
	OnTradeInserted(ctx context.Context, trade *store.Trade)
}

// Job polls Kite for terminal order status on submitted OAuth trades.
type Job struct {
	store    Store
	infra    *livebrokers.Infra
	credKey  []byte
	notify   TradeNotifier
	interval time.Duration
	maxAge   time.Duration
	limit    int
}

// New returns a fill poll job. interval <= 0 disables Start().
func New(s Store, infra *livebrokers.Infra, credKey []byte, notify TradeNotifier, interval time.Duration) *Job {
	return &Job{
		store:    s,
		infra:    infra,
		credKey:  credKey,
		notify:   notify,
		interval: interval,
		maxAge:   defaultMaxAge,
		limit:    500,
	}
}

// Start runs the poll loop until ctx is cancelled. Non-blocking.
func (j *Job) Start(ctx context.Context) {
	if j == nil || j.interval <= 0 {
		return
	}
	go func() {
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
	}()
}

// Tick runs one poll sweep. Exported for tests.
func (j *Job) Tick(ctx context.Context) {
	j.tick(ctx)
}

func (j *Job) tick(ctx context.Context) {
	pending, err := j.store.ListPendingZerodhaOAuthFills(ctx, j.maxAge, j.limit)
	if err != nil {
		log.Printf("zerodhafill: list pending trades failed: %v", err)
		return
	}
	if len(pending) == 0 {
		return
	}

	byCred := make(map[string][]store.PendingFillTrade)
	for _, item := range pending {
		byCred[item.BrokerCredID] = append(byCred[item.BrokerCredID], item)
	}

	for credID, trades := range byCred {
		j.pollCredential(ctx, credID, trades)
	}
}

func (j *Job) pollCredential(ctx context.Context, credID string, trades []store.PendingFillTrade) {
	cred, err := j.store.GetBrokerCred(ctx, credID)
	if err != nil || cred == nil {
		log.Printf("zerodhafill: credential %s lookup failed: %v", credID, err)
		return
	}

	plaintext, err := credenc.PlaintextOrDecrypt(cred.EncryptedCreds, j.credKey, cred.ID)
	if err != nil {
		log.Printf("zerodhafill: decrypt cred %s failed: %v", credID, err)
		return
	}
	parsed, err := brokercreds.Parse(cred.BrokerType, plaintext)
	if err != nil {
		log.Printf("zerodhafill: parse cred %s failed: %v", credID, err)
		return
	}

	broker, err := livebrokers.NewWithParsed(cred, parsed, j.infra, nil)
	if err != nil {
		log.Printf("zerodhafill: broker init cred %s failed: %v", credID, err)
		return
	}
	zb, ok := broker.(*livebrokers.ZerodhaBroker)
	if !ok {
		return
	}

	book, err := zb.FetchTodayOrders(ctx)
	if err != nil {
		if brokererr.CodeOf(err) == brokererr.CodeAuthFailed {
			log.Printf("zerodhafill: Kite auth failed for cred %s — user may need to reconnect", credID)
		} else {
			log.Printf("zerodhafill: get orders cred %s failed: %v", credID, err)
		}
		return
	}

	for _, item := range trades {
		orderID := item.Trade.BrokerOrder
		if snap, found := book[orderID]; found {
			j.applySnapshot(ctx, item.Trade, snap)
			continue
		}
		snap, err := zb.FetchOrderHistory(ctx, orderID)
		if err != nil {
			if brokererr.CodeOf(err) != brokererr.CodeOrderNotFound {
				log.Printf("zerodhafill: order history cred=%s order=%s failed: %v", credID, orderID, err)
			}
			continue
		}
		j.applySnapshot(ctx, item.Trade, *snap)
	}
}

func (j *Job) applySnapshot(ctx context.Context, trade store.Trade, snap livebrokers.KiteOrderSnapshot) {
	newStatus, fillPrice, errMsg, ok := livebrokers.ApplyKiteSnapshot(snap)
	if !ok || newStatus == trade.Status {
		return
	}

	if err := j.store.UpdateTradeFillOutcome(ctx, trade.ID, newStatus, fillPrice, errMsg); err != nil {
		log.Printf("zerodhafill: update trade %s failed: %v", trade.ID, err)
		return
	}

	trade.Status = newStatus
	trade.FillPrice = fillPrice
	if errMsg != "" {
		trade.Error = errMsg
	}
	metrics.RecordTrade(trade.Status, trade.ErrorCode, "zerodha")
	if j.notify != nil {
		j.notify.OnTradeInserted(ctx, &trade)
	}
	log.Printf("zerodhafill: trade %s order %s -> %s fill=%.2f", trade.ID, trade.BrokerOrder, newStatus, fillPrice)
}
