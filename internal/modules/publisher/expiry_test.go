package publisher

import (
	"context"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/store"
)

type fakeExpiryStore struct {
	tradeIDs []string
	trade    *store.Trade
}

func (f *fakeExpiryStore) ExpireStalePublisherOrders(_ context.Context) ([]string, error) {
	return f.tradeIDs, nil
}

func (f *fakeExpiryStore) GetTradeByID(_ context.Context, id string) (*store.Trade, error) {
	if f.trade != nil && f.trade.ID == id {
		return f.trade, nil
	}
	return nil, nil
}

type fakeNotifier struct {
	trades []*store.Trade
}

func (f *fakeNotifier) OnTradeInserted(_ context.Context, trade *store.Trade) {
	f.trades = append(f.trades, trade)
}

func TestExpiryJobTickNotifiesUpdatedTrades(t *testing.T) {
	trade := &store.Trade{
		ID:        "trade-1",
		Status:    "rejected",
		ErrorCode: store.PublisherHandoffExpiredCode,
		Error:     store.PublisherHandoffExpiredMsg,
	}
	notifier := &fakeNotifier{}
	job := &ExpiryJob{
		pg: &fakeExpiryStore{
			tradeIDs: []string{"trade-1"},
			trade:    trade,
		},
		notify: notifier,
	}
	job.tick(context.Background())
	if len(notifier.trades) != 1 {
		t.Fatalf("notified %d trades, want 1", len(notifier.trades))
	}
	if notifier.trades[0].ErrorCode != store.PublisherHandoffExpiredCode {
		t.Fatalf("error_code=%q", notifier.trades[0].ErrorCode)
	}
}
