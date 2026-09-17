package fillpoll

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers/httpclient"
	"github.com/SPSingh09/zettabridge/internal/store"
)

type fakeStore struct {
	pending []store.PendingFillTrade
	cred    *store.BrokerCredential
	updates []struct {
		id, status string
		fill       float64
		errMsg     string
	}
}

func (f *fakeStore) ListPendingZerodhaOAuthFills(ctx context.Context, maxAge time.Duration, limit int) ([]store.PendingFillTrade, error) {
	return f.pending, nil
}

func (f *fakeStore) GetBrokerCred(ctx context.Context, id string) (*store.BrokerCredential, error) {
	return f.cred, nil
}

func (f *fakeStore) UpdateTradeFillOutcome(ctx context.Context, id, status string, fillPrice float64, errMsg string) error {
	f.updates = append(f.updates, struct {
		id, status string
		fill       float64
		errMsg     string
	}{id, status, fillPrice, errMsg})
	return nil
}

type fakeNotify struct {
	trades []*store.Trade
}

func (f *fakeNotify) OnTradeInserted(ctx context.Context, trade *store.Trade) {
	f.trades = append(f.trades, trade)
}

func TestJobPollsTodayOrderBook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orders" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"order_id": "KITE-1", "status": "COMPLETE", "average_price": 2500.25},
			},
		})
	}))
	defer srv.Close()

	fs := &fakeStore{
		pending: []store.PendingFillTrade{{
			BrokerCredID: "cred-1",
			Trade: store.Trade{
				ID:          "trade-1",
				BrokerOrder: "KITE-1",
				Status:      domain.StatusSubmitted,
				WebhookID:   "wh-1",
			},
		}},
		cred: &store.BrokerCredential{
			ID:              "cred-1",
			BrokerType:      "zerodha",
			ExecutionMode:   "user_api_oauth",
			AccountMode:     "live",
			EncryptedCreds:  "apikey:secret:token",
		},
	}
	notify := &fakeNotify{}
	infra := &livebrokers.Infra{
		HTTP: httpclient.New(time.Second, 0),
		URLs: livebrokers.URLs{ZerodhaLive: srv.URL},
	}
	job := New(fs, infra, []byte("00000000000000000000000000000000"), notify, time.Minute)
	job.Tick(context.Background())

	if len(fs.updates) != 1 {
		t.Fatalf("updates=%d want 1", len(fs.updates))
	}
	if fs.updates[0].status != domain.StatusFilled || fs.updates[0].fill != 2500.25 {
		t.Fatalf("update=%+v", fs.updates[0])
	}
	if len(notify.trades) != 1 || notify.trades[0].Status != domain.StatusFilled {
		t.Fatalf("notify=%+v", notify.trades)
	}
}

func TestJobFallsBackToOrderHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/orders":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []any{}})
		case "/orders/KITE-2":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{"order_id": "KITE-2", "status": "REJECTED", "status_message": "insufficient funds"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	fs := &fakeStore{
		pending: []store.PendingFillTrade{{
			BrokerCredID: "cred-1",
			Trade: store.Trade{
				ID:          "trade-2",
				BrokerOrder: "KITE-2",
				Status:      domain.StatusSubmitted,
			},
		}},
		cred: &store.BrokerCredential{
			ID:             "cred-1",
			BrokerType:     "zerodha",
			ExecutionMode:  "user_api_oauth",
			AccountMode:    "live",
			EncryptedCreds: "apikey:secret:token",
		},
	}
	infra := &livebrokers.Infra{
		HTTP: httpclient.New(time.Second, 0),
		URLs: livebrokers.URLs{ZerodhaLive: srv.URL},
	}
	job := New(fs, infra, []byte("00000000000000000000000000000000"), nil, time.Minute)
	job.Tick(context.Background())

	if len(fs.updates) != 1 || fs.updates[0].status != domain.StatusRejected {
		t.Fatalf("updates=%+v", fs.updates)
	}
	if fs.updates[0].errMsg != "insufficient funds" {
		t.Fatalf("errMsg=%q", fs.updates[0].errMsg)
	}
}
