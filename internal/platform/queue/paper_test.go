package queue

import (
	"context"
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/marketdata"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// fakePaperEngine records whether Execute was reached and what request it
// received — Phase 7's new checks should reject before ever calling this.
type fakePaperEngine struct {
	called  bool
	lastReq domain.ExecutionRequest
}

func (e *fakePaperEngine) Execute(_ context.Context, req domain.ExecutionRequest) (*domain.ExecutionResult, error) {
	e.called = true
	e.lastReq = req
	return &domain.ExecutionResult{OrderID: "PAPER-1", Status: string(domain.PaperOrderStatusFilled), FillPrice: req.Price}, nil
}

func basePaperAccount() *store.PaperAccount {
	return &store.PaperAccount{ID: "pa-1", UserID: "u1", MarketProfile: "indian_equity", Exchange: "NSE", DefaultProduct: "MIS", Status: "active"}
}

func paperTestUser() *store.User {
	return &store.User{ID: "u1", Plan: plan.PlanPro}
}

func activeInstrument() *store.Instrument {
	return &store.Instrument{MarketProfileCode: "indian_equity", Exchange: "NSE", Symbol: "SBIN", Active: true, TickSize: 0.05, LotSize: 1}
}

// alwaysOpenProfile has no trading_days restriction (24/7) — used when a
// test wants market hours to never block, without depending on wall-clock time.
func alwaysOpenProfile() *store.MarketProfile {
	return &store.MarketProfile{Code: "indian_equity", Name: "Indian Equity", Timezone: "UTC", TradingDays: nil}
}

// nseHoursProfile mirrors migration 039's seeded indian_equity schedule:
// Mon-Fri 09:15-15:30 IST. Used with a fixed `now` (not real wall-clock time)
// so tests are deterministic regardless of when the suite runs.
func nseHoursProfile() *store.MarketProfile {
	sched := store.TradingSchedule{
		"mon": {{Start: "09:15", End: "15:30"}},
		"tue": {{Start: "09:15", End: "15:30"}},
		"wed": {{Start: "09:15", End: "15:30"}},
		"thu": {{Start: "09:15", End: "15:30"}},
		"fri": {{Start: "09:15", End: "15:30"}},
	}
	return &store.MarketProfile{Code: "indian_equity", Name: "Indian Equity", Timezone: "Asia/Kolkata", TradingDays: &sched}
}

// A Sunday (outside any configured window) and a Wednesday during NSE hours,
// both in IST, for deterministic market-hours tests.
var (
	outsideMarketHours = time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)  // Sunday
	withinMarketHours  = time.Date(2026, 7, 8, 5, 0, 0, 0, time.UTC)   // Wednesday 10:30 IST (UTC+5:30)
)

func paperWebhook() *store.Webhook {
	acctID := "pa-1"
	return &store.Webhook{ID: "wh-1", UserID: "u1", PaperAccountID: &acctID, Symbol: "SBIN", Status: "active"}
}

func TestProcessPaperOrder_UnknownSymbolRejectedWithoutCallingEngine(t *testing.T) {
	fs := &fakeStore{paperAccount: basePaperAccount()} // no instrument configured
	engine := &fakePaperEngine{}
	q := newTestQueue(t, fs, nil, "", nil)
	q.SetPaperEngine(engine)

	trade := &store.Trade{ID: "t1"}
	q.processPaperOrder(context.Background(), paperWebhook(), &store.SignalPayload{Action: "BUY", Price: 100}, guard.TradeParams{Symbol: "SBIN", Lot: 10, Price: 100}, trade)

	if engine.called {
		t.Fatal("expected engine.Execute to never be called for an unknown symbol")
	}
	if trade.Status != "rejected" {
		t.Fatalf("expected rejected, got %q", trade.Status)
	}
	if got := lastTrade(fs); got == nil || got.Error == "" {
		t.Fatalf("expected a rejection reason, got %+v", got)
	}
}

func TestProcessPaperOrder_PausedAccountRejectedWithoutCallingEngine(t *testing.T) {
	acc := basePaperAccount()
	acc.Status = "paused"
	fs := &fakeStore{paperAccount: acc, instrument: activeInstrument()}
	engine := &fakePaperEngine{}
	q := newTestQueue(t, fs, nil, "", nil)
	q.SetPaperEngine(engine)

	trade := &store.Trade{ID: "t1"}
	q.processPaperOrder(context.Background(), paperWebhook(), &store.SignalPayload{Action: "BUY", Price: 100}, guard.TradeParams{Symbol: "SBIN", Lot: 10, Price: 100}, trade)

	if engine.called {
		t.Fatal("expected engine.Execute to never be called for a paused account")
	}
	if trade.Status != "rejected" {
		t.Fatalf("expected rejected, got %q", trade.Status)
	}
	if got := lastTrade(fs); got == nil || got.Error != "paper account is paused" {
		t.Fatalf("expected 'paper account is paused' rejection reason, got %+v", got)
	}
}

func TestProcessPaperOrder_InactiveInstrumentRejected(t *testing.T) {
	instr := activeInstrument()
	instr.Active = false
	fs := &fakeStore{paperAccount: basePaperAccount(), instrument: instr}
	engine := &fakePaperEngine{}
	q := newTestQueue(t, fs, nil, "", nil)
	q.SetPaperEngine(engine)

	trade := &store.Trade{ID: "t1"}
	q.processPaperOrder(context.Background(), paperWebhook(), &store.SignalPayload{Action: "BUY", Price: 100}, guard.TradeParams{Symbol: "SBIN", Lot: 10, Price: 100}, trade)

	if engine.called {
		t.Fatal("expected engine.Execute to never be called for an inactive instrument")
	}
	if trade.Status != "rejected" {
		t.Fatalf("expected rejected, got %q", trade.Status)
	}
}

func TestCheckPaperMarketHoursAt_OutsideHoursRejected(t *testing.T) {
	fs := &fakeStore{
		marketProfile:    nseHoursProfile(),
		platformSettings: map[string]string{guard.MarketHoursEnforcementSettingKey: "true"},
	}
	q := newTestQueue(t, fs, nil, "", nil)

	reason := q.checkPaperMarketHoursAt(context.Background(), basePaperAccount(), outsideMarketHours)
	if reason == "" {
		t.Fatal("expected a rejection reason outside trading hours, got none")
	}
}

func TestCheckPaperMarketHoursAt_WithinHoursAllowed(t *testing.T) {
	fs := &fakeStore{
		marketProfile:    nseHoursProfile(),
		platformSettings: map[string]string{guard.MarketHoursEnforcementSettingKey: "true"},
	}
	q := newTestQueue(t, fs, nil, "", nil)

	reason := q.checkPaperMarketHoursAt(context.Background(), basePaperAccount(), withinMarketHours)
	if reason != "" {
		t.Fatalf("expected no rejection within trading hours, got %q", reason)
	}
}

func TestCheckPaperMarketHoursAt_DefaultEnabledWhenSettingUnset(t *testing.T) {
	// No platformSettings entry at all — enforcement must default to ON.
	fs := &fakeStore{marketProfile: nseHoursProfile()}
	q := newTestQueue(t, fs, nil, "", nil)

	reason := q.checkPaperMarketHoursAt(context.Background(), basePaperAccount(), outsideMarketHours)
	if reason == "" {
		t.Fatal("expected market-hours enforcement to default to enabled when the platform setting is unset")
	}
}

func TestCheckPaperMarketHoursAt_DisabledBypassesSchedule(t *testing.T) {
	fs := &fakeStore{
		marketProfile:    nseHoursProfile(),
		platformSettings: map[string]string{guard.MarketHoursEnforcementSettingKey: "false"},
	}
	q := newTestQueue(t, fs, nil, "", nil)

	reason := q.checkPaperMarketHoursAt(context.Background(), basePaperAccount(), outsideMarketHours)
	if reason != "" {
		t.Fatalf("expected enforcement disabled to bypass the schedule entirely, got %q", reason)
	}
}

func TestProcessPaperOrder_MarketHoursEnforcementDisabled_ProceedsRegardlessOfSchedule(t *testing.T) {
	fs := &fakeStore{
		user:          paperTestUser(),
		paperAccount:  basePaperAccount(),
		instrument:    activeInstrument(),
		marketProfile: nseHoursProfile(),
		platformSettings: map[string]string{
			guard.MarketHoursEnforcementSettingKey: "false",
		},
	}
	engine := &fakePaperEngine{}
	q := newTestQueue(t, fs, nil, "", nil)
	q.SetPaperEngine(engine)

	trade := &store.Trade{ID: "t1"}
	q.processPaperOrder(context.Background(), paperWebhook(), &store.SignalPayload{Action: "BUY", Price: 100}, guard.TradeParams{Symbol: "SBIN", Lot: 10, Price: 100}, trade)

	if !engine.called {
		t.Fatal("expected engine.Execute to be called when enforcement is disabled")
	}
	if trade.Status != "filled" {
		t.Fatalf("expected filled, got %q", trade.Status)
	}
}

func TestProcessPaperOrder_HappyPath_PassesInstrumentTickAndLotSizeToEngine(t *testing.T) {
	fs := &fakeStore{
		user:          paperTestUser(),
		paperAccount:  basePaperAccount(),
		instrument:    activeInstrument(),
		marketProfile: alwaysOpenProfile(),
	}
	engine := &fakePaperEngine{}
	q := newTestQueue(t, fs, nil, "", nil)
	q.SetPaperEngine(engine)

	trade := &store.Trade{ID: "t1"}
	q.processPaperOrder(context.Background(), paperWebhook(), &store.SignalPayload{Action: "BUY", Price: 100}, guard.TradeParams{Symbol: "SBIN", Lot: 10, Price: 100}, trade)

	if !engine.called {
		t.Fatal("expected engine.Execute to be called")
	}
	if engine.lastReq.TickSize != 0.05 || engine.lastReq.LotSize != 1 {
		t.Fatalf("expected instrument tick/lot size to be passed through, got %+v", engine.lastReq)
	}
	if engine.lastReq.UserID != "u1" {
		t.Fatalf("expected webhook user id on execution request, got %q", engine.lastReq.UserID)
	}
	if trade.Status != "filled" {
		t.Fatalf("expected filled, got %q", trade.Status)
	}
}

type paperTestQuoteProvider struct {
	ltp float64
}

func (p paperTestQuoteProvider) GetLTP(_ context.Context, _ marketdata.Symbol) (*marketdata.Quote, error) {
	return &marketdata.Quote{LTP: p.ltp}, nil
}

func (p paperTestQuoteProvider) GetBatchLTP(ctx context.Context, symbols []marketdata.Symbol) ([]marketdata.Quote, error) {
	q, err := p.GetLTP(ctx, symbols[0])
	if err != nil {
		return nil, err
	}
	return []marketdata.Quote{*q}, nil
}

func TestProcessPaperOrder_MarketOrderWithoutPrice_UsesLiveFeedLTP(t *testing.T) {
	fs := &fakeStore{
		user:          paperTestUser(),
		paperAccount:  basePaperAccount(),
		instrument:    activeInstrument(),
		marketProfile: alwaysOpenProfile(),
		platformSettings: map[string]string{
			marketdata.SettingKeyProvider: marketdata.ProviderFyers,
		},
	}
	engine := &fakePaperEngine{}
	q := newTestQueue(t, fs, nil, "", nil)
	q.SetPaperEngine(engine)
	q.SetMarketDataRegistry(marketdata.NewRegistry(marketdata.ProviderSignal, map[string]marketdata.Provider{
		marketdata.ProviderSignal: marketdata.SignalPriceProvider{},
		marketdata.ProviderFyers:  paperTestQuoteProvider{ltp: 842.25},
	}))

	wh := paperWebhook()
	wh.DefaultOrderType = "MARKET"
	trade := &store.Trade{ID: "t1"}
	q.processPaperOrder(
		context.Background(),
		wh,
		&store.SignalPayload{Action: "SELL", Lot: 1},
		guard.TradeParams{Symbol: "SBIN", Lot: 1, Price: 0},
		trade,
	)

	if !engine.called {
		t.Fatal("expected engine.Execute to be called")
	}
	if engine.lastReq.Price != 842.25 {
		t.Fatalf("expected fill price from live feed, got %v", engine.lastReq.Price)
	}
	if trade.Status != "filled" {
		t.Fatalf("expected filled, got %q (%s)", trade.Status, trade.Error)
	}
}
