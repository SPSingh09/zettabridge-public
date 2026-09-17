package marketdata

import (
	"context"
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/store"
)

// fakeStore is an in-memory Store for testing the sweep logic without a
// database, mirroring paperengine's fakeStore precedent.
type fakeStore struct {
	accounts    []*store.PaperAccount
	positions   map[string][]*store.PaperPosition // by account ID
	pnl         map[string]*store.PaperPnLSummary  // by account ID
	snapshots   []*store.PaperAccountSnapshot
	markUpdates int
	settings    map[string]string // platform_settings, empty map = nothing set
}

func (f *fakeStore) GetPlatformSetting(_ context.Context, key string) (string, bool, error) {
	v, ok := f.settings[key]
	return v, ok, nil
}

func (f *fakeStore) ListPaperAccountsWithOpenPositions(_ context.Context) ([]*store.PaperAccount, error) {
	return f.accounts, nil
}

func (f *fakeStore) ListPaperPositionsByAccount(_ context.Context, accountID string) ([]*store.PaperPosition, error) {
	return f.positions[accountID], nil
}

func (f *fakeStore) UpdatePaperPositionMark(_ context.Context, pos *store.PaperPosition) error {
	f.markUpdates++
	for _, p := range f.positions[pos.PaperAccountID] {
		if p.ID == pos.ID {
			p.LastPrice = pos.LastPrice
			p.UnrealizedPnL = pos.UnrealizedPnL
			p.UpdatedAt = pos.UpdatedAt
		}
	}
	return nil
}

func (f *fakeStore) GetPaperAccountPnL(_ context.Context, accountID string) (*store.PaperPnLSummary, error) {
	return f.pnl[accountID], nil
}

func (f *fakeStore) CreatePaperAccountSnapshot(_ context.Context, s *store.PaperAccountSnapshot) error {
	f.snapshots = append(f.snapshots, s)
	return nil
}

func (f *fakeStore) RecordPaperAccountEquitySnapshot(_ context.Context, accountID, _ string) error {
	summary := f.pnl[accountID]
	if summary == nil {
		return nil
	}
	return f.CreatePaperAccountSnapshot(context.Background(), &store.PaperAccountSnapshot{
		PaperAccountID: accountID,
		Equity:         summary.Equity,
		CashBalance:    summary.CashBalance,
		UnrealizedPnL:  summary.UnrealizedPnL,
		RealizedPnL:    summary.RealizedPnL,
	})
}

// fakeProvider returns a fixed quote map, or ErrNoQuote for unknown symbols.
type fakeProvider struct {
	quotes map[string]float64 // "exchange|symbol" -> LTP
}

func (p *fakeProvider) GetLTP(_ context.Context, sym Symbol) (*Quote, error) {
	if ltp, ok := p.quotes[sym.Exchange+"|"+sym.Symbol]; ok {
		return &Quote{Symbol: sym, LTP: ltp}, nil
	}
	return nil, ErrNoQuote
}

func (p *fakeProvider) GetBatchLTP(_ context.Context, symbols []Symbol) ([]Quote, error) {
	var out []Quote
	for _, s := range symbols {
		if ltp, ok := p.quotes[s.Exchange+"|"+s.Symbol]; ok {
			out = append(out, Quote{Symbol: s, LTP: ltp})
		}
	}
	if len(out) == 0 {
		return nil, ErrNoQuote
	}
	return out, nil
}

func TestSweep_SkipsAccountsWithNoOpenPositions(t *testing.T) {
	fs := &fakeStore{
		accounts: []*store.PaperAccount{{ID: "acc1", UserID: "u1"}},
		positions: map[string][]*store.PaperPosition{
			"acc1": {{ID: "p1", PaperAccountID: "acc1", Symbol: "SBIN", Exchange: "NSE", Quantity: 0}},
		},
	}
	registry := NewRegistry(ProviderSignal, map[string]Provider{ProviderSignal: SignalPriceProvider{}})
	job := New(fs, registry, time.Second)
	job.Sweep(context.Background())

	if len(fs.snapshots) != 0 {
		t.Fatalf("expected no snapshot for a flat account, got %d", len(fs.snapshots))
	}
	if fs.markUpdates != 0 {
		t.Fatalf("expected no mark updates for a flat account, got %d", fs.markUpdates)
	}
}

func TestSweep_SignalPriceProvider_LeavesPriceUnchangedButSnapshots(t *testing.T) {
	fs := &fakeStore{
		accounts: []*store.PaperAccount{{ID: "acc1", UserID: "u1"}},
		positions: map[string][]*store.PaperPosition{
			"acc1": {{ID: "p1", PaperAccountID: "acc1", Symbol: "SBIN", Exchange: "NSE", Quantity: 10, AvgEntryPrice: 820.5, LastPrice: 820.5}},
		},
		pnl: map[string]*store.PaperPnLSummary{
			"acc1": {PaperAccountID: "acc1", Equity: 100000, CashBalance: 91795, UnrealizedPnL: 0, RealizedPnL: 0},
		},
	}
	registry := NewRegistry(ProviderSignal, map[string]Provider{ProviderSignal: SignalPriceProvider{}})
	job := New(fs, registry, time.Second)
	job.Sweep(context.Background())

	if fs.markUpdates != 0 {
		t.Fatalf("SignalPriceProvider should never trigger a mark update, got %d", fs.markUpdates)
	}
	if len(fs.snapshots) != 1 {
		t.Fatalf("expected exactly one snapshot, got %d", len(fs.snapshots))
	}
	snap := fs.snapshots[0]
	if snap.Equity != 100000 || snap.PaperAccountID != "acc1" {
		t.Fatalf("snapshot mismatch: %+v", snap)
	}
}

func TestSweep_RealProvider_UpdatesMarkAndSnapshot(t *testing.T) {
	fs := &fakeStore{
		accounts: []*store.PaperAccount{{ID: "acc1", UserID: "u1"}},
		positions: map[string][]*store.PaperPosition{
			"acc1": {{ID: "p1", PaperAccountID: "acc1", Symbol: "SBIN", Exchange: "NSE", Quantity: 10, AvgEntryPrice: 820.5, LastPrice: 820.5}},
		},
		pnl: map[string]*store.PaperPnLSummary{
			// Reflects what GetPaperAccountPnL would recompute after the mark update below.
			"acc1": {PaperAccountID: "acc1", Equity: 100050, CashBalance: 91795, UnrealizedPnL: 50, RealizedPnL: 0},
		},
	}
	provider := &fakeProvider{quotes: map[string]float64{"NSE|SBIN": 825.5}}
	registry := NewRegistry(ProviderSignal, map[string]Provider{ProviderSignal: SignalPriceProvider{}, "fake": provider})
	fs.settings = map[string]string{SettingKeyProvider: "fake"}
	job := New(fs, registry, time.Second)
	job.Sweep(context.Background())

	if fs.markUpdates != 1 {
		t.Fatalf("expected exactly one mark update, got %d", fs.markUpdates)
	}
	pos := fs.positions["acc1"][0]
	if pos.LastPrice != 825.5 {
		t.Fatalf("expected last_price 825.5, got %v", pos.LastPrice)
	}
	wantUnrealized := (825.5 - 820.5) * 10
	if pos.UnrealizedPnL != wantUnrealized {
		t.Fatalf("expected unrealized_pnl %v, got %v", wantUnrealized, pos.UnrealizedPnL)
	}
	if len(fs.snapshots) != 1 || fs.snapshots[0].UnrealizedPnL != 50 {
		t.Fatalf("expected one snapshot with unrealized_pnl 50, got %+v", fs.snapshots)
	}
}

// TestSweep_RuntimeProviderSwitch verifies the admin-configurable toggle:
// changing the platform_settings row between two sweeps switches providers
// on the *same* job instance — no restart, no re-construction.
func TestSweep_RuntimeProviderSwitch(t *testing.T) {
	fs := &fakeStore{
		accounts: []*store.PaperAccount{{ID: "acc1", UserID: "u1"}},
		positions: map[string][]*store.PaperPosition{
			"acc1": {{ID: "p1", PaperAccountID: "acc1", Symbol: "SBIN", Exchange: "NSE", Quantity: 10, AvgEntryPrice: 820.5, LastPrice: 820.5}},
		},
		pnl: map[string]*store.PaperPnLSummary{
			"acc1": {PaperAccountID: "acc1", Equity: 100000, CashBalance: 91795},
		},
		settings: map[string]string{},
	}
	fake := &fakeProvider{quotes: map[string]float64{"NSE|SBIN": 825.5}}
	registry := NewRegistry(ProviderSignal, map[string]Provider{ProviderSignal: SignalPriceProvider{}, "fake": fake})
	job := New(fs, registry, time.Second)

	// Setting unset -> falls back to SignalPriceProvider -> no mark update.
	job.Sweep(context.Background())
	if fs.markUpdates != 0 {
		t.Fatalf("expected no mark update before the setting is toggled, got %d", fs.markUpdates)
	}

	// Admin flips the setting between sweeps (simulating the PUT endpoint) —
	// same job instance, no restart.
	fs.settings[SettingKeyProvider] = "fake"
	fs.pnl["acc1"] = &store.PaperPnLSummary{PaperAccountID: "acc1", Equity: 100050, UnrealizedPnL: 50}

	job.Sweep(context.Background())
	if fs.markUpdates != 1 {
		t.Fatalf("expected exactly one mark update after switching to the fake provider, got %d", fs.markUpdates)
	}
	if fs.positions["acc1"][0].LastPrice != 825.5 {
		t.Fatalf("expected last_price 825.5 after switching providers, got %v", fs.positions["acc1"][0].LastPrice)
	}
}

// TestCurrentInterval covers the admin-configurable sweep cadence: unset
// falls back to the env-configured default, an in-bounds override wins, and
// an out-of-bounds value (e.g. a bad manual platform_settings edit) is
// ignored rather than producing a too-fast or stuck-forever ticker.
func TestCurrentInterval(t *testing.T) {
	registry := NewRegistry(ProviderSignal, map[string]Provider{ProviderSignal: SignalPriceProvider{}})

	fs := &fakeStore{settings: map[string]string{}}
	job := New(fs, registry, 45*time.Second)
	if got := job.currentInterval(context.Background()); got != 45*time.Second {
		t.Fatalf("expected default 45s when unset, got %v", got)
	}

	fs.settings[SettingKeyIntervalSec] = "20"
	if got := job.currentInterval(context.Background()); got != 20*time.Second {
		t.Fatalf("expected override 20s, got %v", got)
	}

	fs.settings[SettingKeyIntervalSec] = "1" // below MinSnapshotIntervalSec
	if got := job.currentInterval(context.Background()); got != 45*time.Second {
		t.Fatalf("expected fallback to default for out-of-bounds value, got %v", got)
	}

	fs.settings[SettingKeyIntervalSec] = "not-a-number"
	if got := job.currentInterval(context.Background()); got != 45*time.Second {
		t.Fatalf("expected fallback to default for unparsable value, got %v", got)
	}
}
