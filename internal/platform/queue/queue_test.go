package queue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/domain"

	"github.com/SPSingh09/zettabridge/internal/algo"
	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/execution"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokerfactory"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/risk"
	"github.com/SPSingh09/zettabridge/internal/store"
)

type fakeStore struct {
	user             *store.User
	org              *store.Organization
	cred             *store.BrokerCredential
	paperAccount     *store.PaperAccount
	instrument       *store.Instrument
	marketProfile    *store.MarketProfile
	platformSettings map[string]string
	trades           []*store.Trade
}

func (f *fakeStore) GetOrgByID(_ context.Context, id string) (*store.Organization, error) {
	if f.org == nil || f.org.ID != id {
		return nil, errors.New("organization not found")
	}
	return f.org, nil
}

func (f *fakeStore) GetUserByID(_ context.Context, id string) (*store.User, error) {
	if f.user == nil || f.user.ID != id {
		return nil, errors.New("user not found")
	}
	return f.user, nil
}

func (f *fakeStore) GetBrokerCred(_ context.Context, id string) (*store.BrokerCredential, error) {
	if f.cred == nil || f.cred.ID != id {
		return nil, errors.New("credential not found")
	}
	return f.cred, nil
}

func (f *fakeStore) GetPaperAccount(_ context.Context, id string) (*store.PaperAccount, error) {
	if f.paperAccount == nil || f.paperAccount.ID != id {
		return nil, errors.New("paper account not found")
	}
	return f.paperAccount, nil
}

func (f *fakeStore) GetInstrument(_ context.Context, marketProfileCode, exchange, symbol string) (*store.Instrument, error) {
	if f.instrument == nil {
		return nil, nil
	}
	if f.instrument.MarketProfileCode != marketProfileCode || f.instrument.Exchange != exchange || f.instrument.Symbol != symbol {
		return nil, nil
	}
	return f.instrument, nil
}

func (f *fakeStore) GetMarketProfileByCode(_ context.Context, code string) (*store.MarketProfile, error) {
	if f.marketProfile == nil || f.marketProfile.Code != code {
		return nil, nil
	}
	return f.marketProfile, nil
}

func (f *fakeStore) GetPlatformSetting(_ context.Context, key string) (string, bool, error) {
	v, ok := f.platformSettings[key]
	return v, ok, nil
}

func (f *fakeStore) InsertTrade(_ context.Context, t *store.Trade) error {
	f.trades = append(f.trades, t)
	return nil
}

func (f *fakeStore) PersistTradeAudit(_ context.Context, t *store.Trade) error {
	for i, existing := range f.trades {
		if existing.ID == t.ID {
			f.trades[i] = t
			return nil
		}
	}
	f.trades = append(f.trades, t)
	return nil
}

func (f *fakeStore) FinalizeTrade(_ context.Context, t *store.Trade) error {
	for i, existing := range f.trades {
		if existing.ID == t.ID {
			f.trades[i] = t
			return nil
		}
	}
	return sql.ErrNoRows
}

func (f *fakeStore) IncrementUserLiveOrdersUsed(_ context.Context, _ string) error {
	return nil
}

func (f *fakeStore) InsertNotification(_ context.Context, _, _, _, _, _ string) {}

func (f *fakeStore) HasActiveTradeWithSignalKey(_ context.Context, webhookID, signalKey string, windowSec int) (bool, error) {
	if signalKey == "" || windowSec <= 0 {
		return false, nil
	}
	cutoff := time.Now().UTC().Add(-time.Duration(windowSec) * time.Second)
	for _, tr := range f.trades {
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

type fakeRateLimiter struct {
	count     int64
	credCount int64
}

func (f *fakeRateLimiter) IncrRateLimit(_ context.Context, _ string) (int64, error) {
	return f.count, nil
}

func (f *fakeRateLimiter) IncrBrokerCredRateLimit(_ context.Context, _ string) (int64, error) {
	if f.credCount > 0 {
		return f.credCount, nil
	}
	return f.count, nil
}

type stubBroker struct {
	placeOrder  func(context.Context, *domain.PlaceRequest) (*domain.OrderResult, error)
	equity      float64
	equityErr   error
	placeCalled bool
}

func (s *stubBroker) PlaceOrder(ctx context.Context, req *domain.PlaceRequest) (*domain.OrderResult, error) {
	s.placeCalled = true
	if s.placeOrder != nil {
		return s.placeOrder(ctx, req)
	}
	return &domain.OrderResult{Status: domain.StatusSubmitted, OrderID: "STUB-1"}, nil
}

func (s *stubBroker) CancelOrder(_ context.Context, _ *domain.CancelRequest) error {
	return nil
}

func (s *stubBroker) GetAccountEquity(context.Context) (float64, error) {
	if s.equityErr != nil {
		return 0, s.equityErr
	}
	if s.equity > 0 {
		return s.equity, nil
	}
	return 10000, nil
}

func testCredKey(t *testing.T) []byte {
	t.Helper()
	return []byte("0123456789abcdef0123456789abcdef")
}

func newTestQueue(t *testing.T, fs *fakeStore, rl *fakeRateLimiter, brokerMode string, factory brokerFactory) *Queue {
	t.Helper()
	if fs == nil {
		fs = &fakeStore{}
	}
	if rl == nil {
		rl = &fakeRateLimiter{count: 1}
	}
	if brokerMode == "" {
		brokerMode = brokerfactory.ModeLive
	}
	if factory == nil {
		factory = brokerfactory.New
	}
	key := testCredKey(t)
	q := &Queue{
		jobs:       make(chan Job, 1),
		pg:         fs,
		redis:      rl,
		credKey:    key,
		brokerMode: brokerMode,
		infra:      livebrokers.DefaultInfra(),
		newBroker:  factory,
	}
	q.initOrchestrator(execution.DefaultEnabledAdapters(), execution.TransportConfig{Mode: execution.TransportLocal})
	return q
}

func baseJob(userID, credID string) Job {
	return Job{
		Webhook: &store.Webhook{
			ID:             "wh-1",
			UserID:         userID,
			BrokerCredID:   &credID,
			Symbol:         "RELIANCE",
			AllowedSymbols: []string{"RELIANCE"},
			LotSize:        1,
		},
		Signal: &store.SignalPayload{Action: "BUY", Symbol: "RELIANCE"},
	}
}

func baseUser(id string) *store.User {
	return &store.User{ID: id, Plan: plan.PlanPro}
}

func baseZerodhaCred(id string) *store.BrokerCredential {
	return &store.BrokerCredential{
		ID:             id,
		BrokerType:     "zerodha",
		EncryptedCreds: "api_key:access_token",
		AccountMode:    plan.AccountLive,
		Exchange:       "NSE",
		Product:        "MIS",
	}
}

func lastTrade(fs *fakeStore) *store.Trade {
	if len(fs.trades) == 0 {
		return nil
	}
	return fs.trades[len(fs.trades)-1]
}

func TestProcessFreeUserLiveRejected(t *testing.T) {
	fs := &fakeStore{
		user: &store.User{ID: "u1", Plan: plan.PlanFree},
		cred: baseZerodhaCred("c1"),
	}
	q := newTestQueue(t, fs, nil, brokerfactory.ModeLive, func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		t.Fatal("broker factory should not be called for free plan live webhook")
		return nil, nil
	})
	q.process(baseJob("u1", "c1"))

	tr := lastTrade(fs)
	if tr == nil || tr.Status != "rejected" {
		t.Fatalf("expected rejected trade, got %+v", tr)
	}
	if tr.ErrorCode != string(brokererr.CodeLiveNotAllowed) {
		t.Fatalf("error_code=%q want live_not_allowed", tr.ErrorCode)
	}
}

func TestProcessUnsupportedServerModeRejected(t *testing.T) {
	fs := &fakeStore{
		user: &store.User{ID: "u1", Plan: plan.PlanPro},
		cred: baseZerodhaCred("c1"),
	}
	q := newTestQueue(t, fs, nil, "paper", func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		t.Fatal("broker factory should not be called when server BROKER_MODE is unsupported")
		return nil, nil
	})
	q.process(baseJob("u1", "c1"))

	tr := lastTrade(fs)
	if tr == nil || tr.Status != "rejected" {
		t.Fatalf("expected rejected trade, got %+v", tr)
	}
	if tr.ErrorCode != string(brokererr.CodeLiveNotAllowed) {
		t.Fatalf("error_code=%q want live_not_allowed", tr.ErrorCode)
	}
}

func TestProcessProUserMockModeUsesSimulatedBroker(t *testing.T) {
	fs := &fakeStore{
		user: baseUser("u1"),
		cred: baseZerodhaCred("c1"),
	}
	q := newTestQueue(t, fs, nil, brokerfactory.ModeMock, nil)
	q.process(baseJob("u1", "c1"))

	tr := lastTrade(fs)
	if tr == nil {
		t.Fatal("expected trade insert")
	}
	if tr.Status != domain.StatusSubmitted {
		t.Fatalf("status=%q want submitted", tr.Status)
	}
	if tr.BrokerOrder == "" || !strings.HasPrefix(tr.BrokerOrder, "mock-") {
		t.Fatalf("broker_order=%q want mock-* from simulated broker", tr.BrokerOrder)
	}
}

func TestProcessFactoryError(t *testing.T) {
	fs := &fakeStore{
		user: baseUser("u1"),
		cred: baseZerodhaCred("c1"),
	}
	q := newTestQueue(t, fs, nil, brokerfactory.ModeLive, func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		return nil, fmt.Errorf("live broker adapter: unsupported broker")
	})
	q.process(baseJob("u1", "c1"))

	tr := lastTrade(fs)
	if tr == nil {
		t.Fatal("expected trade insert")
	}
	if tr.Status != "rejected" {
		t.Fatalf("status=%q", tr.Status)
	}
	if tr.ErrorCode != string(brokererr.CodeInvalidCredentials) {
		t.Fatalf("error_code=%q", tr.ErrorCode)
	}
}

func TestProcessParseFailure(t *testing.T) {
	// Use ModeLive + AccountLive + paid plan so ResolveExecution returns live mode,
	// triggering the decrypt+parse path where four colon parts fail Zerodha format.
	fs := &fakeStore{
		user: baseUser("u1"),
		cred: &store.BrokerCredential{
			ID:             "c1",
			BrokerType:     "zerodha",
			EncryptedCreds: "a:b:c:d", // 4 parts — invalid for all zerodha modes
			AccountMode:    plan.AccountLive,
			Exchange:       "NSE",
			Product:        "MIS",
		},
	}
	q := newTestQueue(t, fs, nil, brokerfactory.ModeLive, nil)
	q.process(baseJob("u1", "c1"))

	tr := lastTrade(fs)
	if tr == nil || tr.Status != "rejected" {
		t.Fatalf("expected rejected trade, got %#v", tr)
	}
	if tr.ErrorCode != string(brokererr.CodeInvalidCredentials) {
		t.Fatalf("error_code=%q", tr.ErrorCode)
	}
}

func TestProcessMaporderFailure(t *testing.T) {
	fs := &fakeStore{
		user: baseUser("u1"),
		cred: baseZerodhaCred("c1"),
	}
	q := newTestQueue(t, fs, nil, brokerfactory.ModeLive, func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		return &stubBroker{}, nil
	})
	job := baseJob("u1", "c1")
	job.Signal.Symbol = "EURUSD"
	job.Webhook.AllowedSymbols = []string{"EURUSD"}
	q.process(job)

	tr := lastTrade(fs)
	if tr == nil || tr.Status != "rejected" {
		t.Fatalf("expected rejected trade, got %#v", tr)
	}
	if tr.ErrorCode != string(brokererr.CodeInvalidSymbol) {
		t.Fatalf("error_code=%q", tr.ErrorCode)
	}
}

func TestProcessPlaceOrderAuthFailed(t *testing.T) {
	fs := &fakeStore{
		user: baseUser("u1"),
		cred: baseZerodhaCred("c1"),
	}
	stub := &stubBroker{
		placeOrder: func(context.Context, *domain.PlaceRequest) (*domain.OrderResult, error) {
			return nil, brokererr.New(brokererr.CodeAuthFailed, brokererr.PublicMessage(brokererr.CodeAuthFailed))
		},
	}
	q := newTestQueue(t, fs, nil, brokerfactory.ModeLive, func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		return stub, nil
	})
	q.process(baseJob("u1", "c1"))

	tr := lastTrade(fs)
	if tr == nil || tr.Status != "rejected" {
		t.Fatalf("expected rejected trade, got %#v", tr)
	}
	if tr.ErrorCode != string(brokererr.CodeAuthFailed) {
		t.Fatalf("error_code=%q", tr.ErrorCode)
	}
}

func TestProcessMockHappyPath(t *testing.T) {
	fs := &fakeStore{
		user: baseUser("u1"),
		cred: baseZerodhaCred("c1"),
	}
	stub := &stubBroker{}
	q := newTestQueue(t, fs, nil, brokerfactory.ModeLive, func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		return stub, nil
	})
	q.process(baseJob("u1", "c1"))

	tr := lastTrade(fs)
	if tr == nil {
		t.Fatal("expected trade insert")
	}
	if tr.Status != domain.StatusSubmitted {
		t.Fatalf("status=%q", tr.Status)
	}
	if tr.BrokerOrder == "" {
		t.Fatal("expected broker order id")
	}
	if tr.ErrorCode != "" {
		t.Fatalf("unexpected error_code=%q", tr.ErrorCode)
	}
	if !stub.placeCalled {
		t.Fatal("expected PlaceOrder call")
	}
}

func TestProcessRateLimitExceeded(t *testing.T) {
	fs := &fakeStore{
		user: &store.User{ID: "u1", Plan: plan.PlanFree},
		cred: baseZerodhaCred("c1"),
	}
	stub := &stubBroker{}
	q := newTestQueue(t, fs, &fakeRateLimiter{count: 2}, brokerfactory.ModeLive, func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		return stub, nil
	})
	q.process(baseJob("u1", "c1"))

	tr := lastTrade(fs)
	if tr == nil || tr.Status != "rejected" {
		t.Fatalf("expected rejected trade, got %#v", tr)
	}
	if tr.ErrorCode != string(brokererr.CodeRateLimited) {
		t.Fatalf("error_code=%q want rate_limited", tr.ErrorCode)
	}
	if stub.placeCalled {
		t.Fatal("broker should not be called when rate limited")
	}
}

func TestProcessEnterpriseUserTenOrdersAllowed(t *testing.T) {
	fs := &fakeStore{
		user: baseUser("u1"),
		cred: baseZerodhaCred("c1"),
	}
	stub := &stubBroker{}
	q := newTestQueue(t, fs, &fakeRateLimiter{count: 10}, brokerfactory.ModeLive, func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		return stub, nil
	})
	q.process(baseJob("u1", "c1"))
	if !stub.placeCalled {
		t.Fatal("expected PlaceOrder at 10/s for enterprise user")
	}
}

func TestProcessEnterpriseUserEleventhOrderRejected(t *testing.T) {
	fs := &fakeStore{
		user: baseUser("u1"),
		cred: baseZerodhaCred("c1"),
	}
	stub := &stubBroker{}
	q := newTestQueue(t, fs, &fakeRateLimiter{count: 11}, brokerfactory.ModeLive, func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		return stub, nil
	})
	q.process(baseJob("u1", "c1"))
	tr := lastTrade(fs)
	if tr == nil || tr.ErrorCode != string(brokererr.CodeRateLimited) {
		t.Fatalf("expected rate_limited at 11th order, got %#v", tr)
	}
	if stub.placeCalled {
		t.Fatal("broker should not be called when rate limited")
	}
}

func TestProcessComplianceSuspendedUser(t *testing.T) {
	fs := &fakeStore{
		user: &store.User{ID: "u1", Plan: plan.PlanPro, Status: "suspended"},
		cred: baseZerodhaCred("c1"),
	}
	stub := &stubBroker{}
	q := newTestQueue(t, fs, nil, brokerfactory.ModeLive, func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		return stub, nil
	})
	q.process(baseJob("u1", "c1"))

	tr := lastTrade(fs)
	if tr == nil || tr.Status != "rejected" {
		t.Fatalf("expected rejected trade, got %#v", tr)
	}
	if tr.ErrorCode != string(brokererr.CodeAccountSuspended) {
		t.Fatalf("error_code=%q", tr.ErrorCode)
	}
	if stub.placeCalled {
		t.Fatal("broker should not be called for suspended user")
	}
}


func TestProcessDedupSkipsDuplicatePlaceOrder(t *testing.T) {
	fs := &fakeStore{
		user: baseUser("u1"),
		cred: baseZerodhaCred("c1"),
	}
	stub := &stubBroker{}
	q := newTestQueue(t, fs, nil, brokerfactory.ModeLive, func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		return stub, nil
	})

	job := baseJob("u1", "c1")
	job.Webhook.DedupWindowSec = 300
	job.Signal = &store.SignalPayload{Action: "BUY", Comment: "dedup-test"}
	job.SignalKey = "fixed-signal-key"
	job.TradeID = "trade-dup-1"

	q.process(job)
	if !stub.placeCalled {
		t.Fatal("expected first PlaceOrder")
	}
	stub.placeCalled = false

	job2 := baseJob("u1", "c1")
	job2.Webhook.DedupWindowSec = 300
	job2.Signal = &store.SignalPayload{Action: "BUY", Comment: "dedup-test"}
	job2.SignalKey = "fixed-signal-key"
	job2.TradeID = "trade-dup-2"
	q.process(job2)
	if stub.placeCalled {
		t.Fatal("duplicate signal should skip PlaceOrder")
	}
	if len(fs.trades) != 2 {
		t.Fatalf("expected 2 trades (original + duplicate rejected), got %d", len(fs.trades))
	}
	if fs.trades[1].ErrorCode != "duplicate_suppressed" {
		t.Fatalf("duplicate trade code=%q", fs.trades[1].ErrorCode)
	}
}

func TestProcessDedupAllowsDifferentComment(t *testing.T) {
	fs := &fakeStore{
		user: baseUser("u1"),
		cred: baseZerodhaCred("c1"),
	}
	stub := &stubBroker{}
	q := newTestQueue(t, fs, nil, brokerfactory.ModeLive, func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		return stub, nil
	})

	job1 := baseJob("u1", "c1")
	job1.Webhook.DedupWindowSec = 300
	job1.Signal = &store.SignalPayload{Action: "BUY", Comment: "a"}
	job1.SignalKey = "key-a"
	q.process(job1)

	job2 := baseJob("u1", "c1")
	job2.Webhook.DedupWindowSec = 300
	job2.Signal = &store.SignalPayload{Action: "BUY", Comment: "b"}
	job2.SignalKey = "key-b"
	q.process(job2)

	if len(fs.trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(fs.trades))
	}
}

func TestProcessStoresSignalKeyAndComment(t *testing.T) {
	fs := &fakeStore{
		user: baseUser("u1"),
		cred: baseZerodhaCred("c1"),
	}
	q := newTestQueue(t, fs, nil, brokerfactory.ModeLive, func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		return &stubBroker{}, nil
	})
	job := baseJob("u1", "c1")
	job.Signal = &store.SignalPayload{Action: "BUY", Comment: "tv-alert"}
	job.SignalKey = "sk-1"
	q.process(job)

	tr := lastTrade(fs)
	if tr == nil {
		t.Fatal("expected trade")
	}
	if tr.SignalKey != "sk-1" {
		t.Fatalf("signal_key=%q", tr.SignalKey)
	}
	if tr.Comment != "tv-alert" {
		t.Fatalf("comment=%q", tr.Comment)
	}
}

func TestProcessRejectsMissingAlgoID(t *testing.T) {
	fs := &fakeStore{
		user: baseUser("u1"),
		cred: &store.BrokerCredential{
			ID:             "c1",
			BrokerType:     "zerodha",
			EncryptedCreds: "api_key:access_token",
			AccountMode:    plan.AccountLive,
			Exchange:       "NSE",
			Product:        "MIS",
		},
	}
	q := newTestQueue(t, fs, nil, brokerfactory.ModeLive, func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		return &stubBroker{}, nil
	})
	q.algoPolicy = algo.Policy{Required: true}
	q.initOrchestrator(execution.DefaultEnabledAdapters(), execution.TransportConfig{Mode: execution.TransportLocal})

	q.process(baseJob("u1", "c1"))

	tr := lastTrade(fs)
	if tr == nil || tr.Status != "rejected" {
		t.Fatalf("expected rejected trade, got %+v", tr)
	}
	if tr.ErrorCode != string(brokererr.CodeAlgoIDRequired) {
		t.Fatalf("error_code=%q", tr.ErrorCode)
	}
}

func TestProcessTagsLiveIndianOrder(t *testing.T) {
	var gotAlgoID string
	fs := &fakeStore{
		user: baseUser("u1"),
		cred: &store.BrokerCredential{
			ID:             "c1",
			BrokerType:     "zerodha",
			EncryptedCreds: "api_key:access_token",
			AccountMode:    plan.AccountLive,
			Exchange:       "NSE",
			Product:        "MIS",
			AlgoID:         "LIVEALGO1",
		},
	}
	stub := &stubBroker{}
	q := newTestQueue(t, fs, nil, brokerfactory.ModeLive, func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		return stub, nil
	})
	q.algoPolicy = algo.Policy{Required: true}
	q.initOrchestrator(execution.DefaultEnabledAdapters(), execution.TransportConfig{Mode: execution.TransportLocal})
	stub.placeOrder = func(_ context.Context, req *domain.PlaceRequest) (*domain.OrderResult, error) {
		gotAlgoID = req.AlgoID
		return &domain.OrderResult{Status: domain.StatusSubmitted, OrderID: "Z1"}, nil
	}

	q.process(baseJob("u1", "c1"))

	if gotAlgoID != "LIVEALGO1" {
		t.Fatalf("PlaceRequest.AlgoID=%q", gotAlgoID)
	}
	tr := lastTrade(fs)
	if tr == nil || tr.AlgoID != "LIVEALGO1" {
		t.Fatalf("trade algo_id=%q status=%q", tr.AlgoID, tr.Status)
	}
}

func TestResolveLotSizeEquityFallback(t *testing.T) {
	q := &Queue{}
	stub := &stubBroker{equityErr: errors.New("equity unavailable")}

	lot, err := q.resolveLotSize(context.Background(), stub, "zerodha", 0, 2.5, 1.0, 10)
	if err != nil {
		t.Fatalf("resolveLotSize: %v", err)
	}
	if lot != 2.5 {
		t.Fatalf("lot=%v want fixed lot fallback 2.5", lot)
	}
}

func TestResolveLotSizeEquityFallbackMinLot(t *testing.T) {
	q := &Queue{}
	stub := &stubBroker{equityErr: errors.New("equity unavailable")}

	lot, err := q.resolveLotSize(context.Background(), stub, "zerodha", 0, 0, 1.0, 10)
	if err != nil {
		t.Fatalf("resolveLotSize: %v", err)
	}
	if lot != risk.MinQtyForBroker("zerodha") {
		t.Fatalf("lot=%v want min qty %v for zerodha", lot, risk.MinQtyForBroker("zerodha"))
	}
}

func TestResolveLotSizeFixedLotWins(t *testing.T) {
	q := &Queue{}
	stub := &stubBroker{}

	lot, err := q.resolveLotSize(context.Background(), stub, "zerodha", 3.5, 1, 0, 0)
	if err != nil {
		t.Fatalf("resolveLotSize: %v", err)
	}
	if lot != 1 {
		t.Fatalf("lot=%v want webhook fixed lot 1", lot)
	}
}

func TestProcessPendingConfirmationStoredCorrectly(t *testing.T) {
	// When the broker returns pending_confirmation (publisher mode handoff),
	// the worker must persist the trade with that exact status and store
	// the publisher order ID in broker_order.
	const publisherOrderID = "pub-order-uuid-123"
	cred := baseZerodhaCred("c1")
	cred.ExecutionMode = string(domain.ExecutionModePublisher)
	fs := &fakeStore{
		user: baseUser("u1"),
		cred: cred,
	}
	factory := func(_ string, _ *livebrokers.Infra, _ *store.BrokerCredential, _ brokercreds.Parsed) (domain.Broker, error) {
		return &stubBroker{
			placeOrder: func(_ context.Context, req *domain.PlaceRequest) (*domain.OrderResult, error) {
				if req.AlgoID != "" {
					t.Fatalf("publisher PlaceRequest.AlgoID=%q, want empty", req.AlgoID)
				}
				return &domain.OrderResult{
					Status:  domain.StatusPendingConfirmation,
					OrderID: publisherOrderID,
					Handoff: &domain.HandoffResult{
						Provider: "zerodha",
						Method:   "POST_FORM",
						Action:   "https://kite.zerodha.com/connect/basket",
						Fields:   map[string]string{"api_key": "k", "data": "[]"},
					},
				}, nil
			},
		}, nil
	}
	q := newTestQueue(t, fs, nil, brokerfactory.ModeLive, factory)
	q.algoPolicy = algo.Policy{Required: true}
	q.process(baseJob("u1", "c1"))

	tr := lastTrade(fs)
	if tr == nil {
		t.Fatal("no trade inserted")
	}
	if tr.Status != domain.StatusPendingConfirmation {
		t.Fatalf("trade.Status=%q want %q", tr.Status, domain.StatusPendingConfirmation)
	}
	if tr.BrokerOrder != publisherOrderID {
		t.Fatalf("trade.BrokerOrder=%q want %q", tr.BrokerOrder, publisherOrderID)
	}
}
