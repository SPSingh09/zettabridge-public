package integration

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokerfactory"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers/httpclient"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/platform/queue"
	"github.com/SPSingh09/zettabridge/internal/store"
)

type memStore struct {
	mu     sync.Mutex
	user   *store.User
	org    *store.Organization
	cred   *store.BrokerCredential
	trades []*store.Trade
}

func (m *memStore) GetUserByID(_ context.Context, id string) (*store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.user == nil || m.user.ID != id {
		return nil, errors.New("user not found")
	}
	return m.user, nil
}

func (m *memStore) GetBrokerCred(_ context.Context, id string) (*store.BrokerCredential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cred == nil || m.cred.ID != id {
		return nil, errors.New("credential not found")
	}
	return m.cred, nil
}

func (m *memStore) GetPaperAccount(_ context.Context, _ string) (*store.PaperAccount, error) {
	return nil, errors.New("paper account not found")
}

func (m *memStore) GetInstrument(_ context.Context, _, _, _ string) (*store.Instrument, error) {
	return nil, nil
}

func (m *memStore) GetMarketProfileByCode(_ context.Context, _ string) (*store.MarketProfile, error) {
	return nil, nil
}

func (m *memStore) GetPlatformSetting(_ context.Context, _ string) (string, bool, error) {
	return "", false, nil
}

func (m *memStore) InsertTrade(_ context.Context, t *store.Trade) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trades = append(m.trades, t)
	return nil
}

func (m *memStore) PersistTradeAudit(_ context.Context, t *store.Trade) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.trades {
		if existing.ID == t.ID {
			m.trades[i] = t
			return nil
		}
	}
	m.trades = append(m.trades, t)
	return nil
}

// FinalizeTrade mirrors PGStore.FinalizeTrade's update-by-ID semantics
// (async worker execution posting a result back for an already-inserted
// trade), rather than InsertTrade's append — so lastTrade() reflects the
// post-finalize state the same way a real DB row would.
func (m *memStore) FinalizeTrade(_ context.Context, t *store.Trade) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.trades {
		if existing.ID == t.ID {
			*existing = *t
			return nil
		}
	}
	m.trades = append(m.trades, t)
	return nil
}

func (m *memStore) IncrementUserLiveOrdersUsed(_ context.Context, _ string) error {
	return nil
}

func (m *memStore) InsertNotification(_ context.Context, _, _, _, _, _ string) {}

func (m *memStore) GetOrgByID(_ context.Context, id string) (*store.Organization, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.org == nil || m.org.ID != id {
		return nil, errors.New("organization not found")
	}
	return m.org, nil
}

func (m *memStore) HasActiveTradeWithSignalKey(_ context.Context, webhookID, signalKey string, windowSec int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if signalKey == "" || windowSec <= 0 {
		return false, nil
	}
	cutoff := time.Now().UTC().Add(-time.Duration(windowSec) * time.Second)
	for _, tr := range m.trades {
		if tr.WebhookID != webhookID || tr.SignalKey != signalKey {
			continue
		}
		if tr.Status == "rejected" {
			continue
		}
		if !tr.CreatedAt.Before(cutoff) {
			return true, nil
		}
	}
	return false, nil
}

func (m *memStore) lastTrade() *store.Trade {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.trades) == 0 {
		return nil
	}
	return m.trades[len(m.trades)-1]
}

type memRateLimiter struct {
	count int64
}

func (m *memRateLimiter) IncrRateLimit(_ context.Context, _ string) (int64, error) {
	return m.count, nil
}

func (m *memRateLimiter) IncrBrokerCredRateLimit(_ context.Context, _ string) (int64, error) {
	return m.count, nil
}

func testCredKey() []byte {
	return []byte("0123456789abcdef0123456789abcdef")
}

func liveInfra(zerodhaURL, mt5URL string) *livebrokers.Infra {
	return &livebrokers.Infra{
		HTTP: httpclient.New(time.Second, 0),
		URLs: livebrokers.URLs{
			ZerodhaLive: zerodhaURL,
			MT5Live:     mt5URL,
		},
	}
}

func baseJob(userID, credID, symbol string) queue.Job {
	return queue.Job{
		Webhook: &store.Webhook{
			ID:               "wh-live-1",
			UserID:           userID,
			BrokerCredID:     &credID,
			Symbol:           symbol,
			LotSize:          10,
			DefaultOrderType: "MARKET",
		},
		Signal: &store.SignalPayload{Action: "BUY"},
	}
}

func TestLiveQueueZerodhaBuy(t *testing.T) {
	var gotAuth string
	var gotOrderType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/orders/regular" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		_ = r.ParseForm()
		gotOrderType = r.FormValue("order_type")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]string{"order_id": "Z-LIVE-99"},
		})
	}))
	defer srv.Close()

	fs := &memStore{
		user: &store.User{ID: "u1", Plan: plan.PlanPro},
		cred: &store.BrokerCredential{
			ID:             "c1",
			BrokerType:     "zerodha",
			EncryptedCreds: "apikey:mysecret:access",
			AccountMode:    plan.AccountLive,
			Exchange:       "NSE",
			Product:        "MIS",
		},
	}
	q := queue.NewForTest(fs, &memRateLimiter{count: 1}, testCredKey(), brokerfactory.ModeLive, liveInfra(srv.URL, ""))
	q.ProcessJobSync(baseJob("u1", "c1", "RELIANCE"))

	tr := fs.lastTrade()
	if tr == nil {
		t.Fatal("expected trade")
	}
	if tr.Status != domain.StatusSubmitted {
		t.Fatalf("status=%q want submitted", tr.Status)
	}
	if tr.BrokerOrder != "Z-LIVE-99" {
		t.Fatalf("broker_order=%q", tr.BrokerOrder)
	}
	if gotAuth != "token apikey:access" { // api_key:access_token from 3-field credential
		t.Fatalf("Authorization=%q", gotAuth)
	}
	if gotOrderType != "MARKET" {
		t.Fatalf("order_type=%q", gotOrderType)
	}
}

// TestLiveQueueZerodhaBuyMarketWithSL covers the bracket/cover-order path
// (SL points present routes through Kite's /orders/co instead of
// /orders/regular) end to end, through the real maporder.Build resolution
// — not a hand-built domain.PlaceRequest — to confirm a MARKET-configured
// webhook still places a MARKET cover order rather than silently falling
// back to LIMIT when a stop-loss is set (the most common real trading
// signal shape).
func TestLiveQueueZerodhaBuyMarketWithSL(t *testing.T) {
	var gotOrderType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/quote":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"NSE:RELIANCE": map[string]interface{}{"last_price": 2500.0},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/orders/co":
			_ = r.ParseForm()
			gotOrderType = r.FormValue("order_type")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]string{"order_id": "ZCO-LIVE-1"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	fs := &memStore{
		user: &store.User{ID: "u1", Plan: plan.PlanPro},
		cred: &store.BrokerCredential{
			ID:             "c1",
			BrokerType:     "zerodha",
			EncryptedCreds: "apikey:mysecret:access",
			AccountMode:    plan.AccountLive,
			Exchange:       "NSE",
			Product:        "MIS",
		},
	}
	q := queue.NewForTest(fs, &memRateLimiter{count: 1}, testCredKey(), brokerfactory.ModeLive, liveInfra(srv.URL, ""))
	job := baseJob("u1", "c1", "RELIANCE")
	job.Signal.SLPts = 20
	q.ProcessJobSync(job)

	tr := fs.lastTrade()
	if tr == nil || tr.Status != domain.StatusSubmitted {
		t.Fatalf("expected submitted trade, got %+v", tr)
	}
	if gotOrderType != "MARKET" {
		t.Fatalf("order_type=%q want MARKET", gotOrderType)
	}
}

// TestLiveQueueZerodhaBuyLimit is TestLiveQueueZerodhaBuy's mirror image —
// a webhook configured for LIMIT must place a LIMIT order, through the same
// real maporder.Build resolution, not just MARKET (which the other test
// already covers). Together these two lock in that wh.DefaultOrderType is
// the sole, correctly-respected source of truth for OAuth execution mode.
func TestLiveQueueZerodhaBuyLimit(t *testing.T) {
	var gotOrderType, gotPrice string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/orders/regular" {
			http.NotFound(w, r)
			return
		}
		_ = r.ParseForm()
		gotOrderType = r.FormValue("order_type")
		gotPrice = r.FormValue("price")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]string{"order_id": "Z-LIVE-LIMIT-1"},
		})
	}))
	defer srv.Close()

	fs := &memStore{
		user: &store.User{ID: "u1", Plan: plan.PlanPro},
		cred: &store.BrokerCredential{
			ID:             "c1",
			BrokerType:     "zerodha",
			EncryptedCreds: "apikey:mysecret:access",
			AccountMode:    plan.AccountLive,
			Exchange:       "NSE",
			Product:        "MIS",
		},
	}
	q := queue.NewForTest(fs, &memRateLimiter{count: 1}, testCredKey(), brokerfactory.ModeLive, liveInfra(srv.URL, ""))
	job := baseJob("u1", "c1", "RELIANCE")
	job.Webhook.DefaultOrderType = "LIMIT"
	job.Signal.Price = 2500.50
	q.ProcessJobSync(job)

	tr := fs.lastTrade()
	if tr == nil || tr.Status != domain.StatusSubmitted {
		t.Fatalf("expected submitted trade, got %+v", tr)
	}
	if gotOrderType != "LIMIT" {
		t.Fatalf("order_type=%q want LIMIT", gotOrderType)
	}
	if gotPrice != "2500.50" {
		t.Fatalf("price=%q want 2500.50", gotPrice)
	}
}

func TestLiveQueueMT5Buy(t *testing.T) {
	var gotAuthToken string
	var gotActionType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/users/current/accounts/acct-1/trade" {
			http.NotFound(w, r)
			return
		}
		gotAuthToken = r.Header.Get("auth-token")
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotActionType, _ = body["actionType"].(string)
		_ = json.NewEncoder(w).Encode(map[string]string{"orderId": "MT5-LIVE-42"})
	}))
	defer srv.Close()

	fs := &memStore{
		user: &store.User{ID: "u1", Plan: plan.PlanPro},
		cred: &store.BrokerCredential{
			ID:             "c1",
			BrokerType:     "mt5_cloud",
			EncryptedCreds: "tok:acct-1",
			AccountMode:    plan.AccountLive,
		},
	}
	q := queue.NewForTest(fs, &memRateLimiter{count: 1}, testCredKey(), brokerfactory.ModeLive, liveInfra("", srv.URL))
	job := baseJob("u1", "c1", "EURUSD")
	job.Webhook.LotSize = 0.1
	q.ProcessJobSync(job)

	tr := fs.lastTrade()
	if tr == nil {
		t.Fatal("expected trade")
	}
	if tr.Status != domain.StatusSubmitted {
		t.Fatalf("status=%q", tr.Status)
	}
	if tr.BrokerOrder != "MT5-LIVE-42" {
		t.Fatalf("broker_order=%q", tr.BrokerOrder)
	}
	if gotAuthToken != "tok" {
		t.Fatalf("auth-token=%q", gotAuthToken)
	}
	if gotActionType != "ORDER_TYPE_BUY" {
		t.Fatalf("actionType=%q", gotActionType)
	}
}

func TestLiveQueueMT5CloseSymbol(t *testing.T) {
	var gotActionType string
	var gotSymbol string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/users/current/accounts/acct-1/trade" {
			http.NotFound(w, r)
			return
		}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotActionType, _ = body["actionType"].(string)
		gotSymbol, _ = body["symbol"].(string)
		_ = json.NewEncoder(w).Encode(map[string]string{"orderId": "MT5-CLOSE-1"})
	}))
	defer srv.Close()

	fs := &memStore{
		user: &store.User{ID: "u1", Plan: plan.PlanPro},
		cred: &store.BrokerCredential{
			ID:             "c1",
			BrokerType:     "mt5_cloud",
			EncryptedCreds: "tok:acct-1",
			AccountMode:    plan.AccountLive,
		},
	}
	q := queue.NewForTest(fs, &memRateLimiter{count: 1}, testCredKey(), brokerfactory.ModeLive, liveInfra("", srv.URL))
	job := baseJob("u1", "c1", "EURUSD")
	job.Signal.Action = "CLOSE"
	q.ProcessJobSync(job)

	tr := fs.lastTrade()
	if tr == nil {
		t.Fatal("expected trade")
	}
	if tr.Status != domain.StatusSubmitted {
		t.Fatalf("status=%q", tr.Status)
	}
	if tr.BrokerOrder != "MT5-CLOSE-1" {
		t.Fatalf("broker_order=%q", tr.BrokerOrder)
	}
	if gotActionType != "POSITIONS_CLOSE_SYMBOL" {
		t.Fatalf("actionType=%q", gotActionType)
	}
	if gotSymbol != "EURUSD" {
		t.Fatalf("symbol=%q", gotSymbol)
	}
}
