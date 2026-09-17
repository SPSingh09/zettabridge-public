package paperengine

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// fakeStore is an in-memory Store for testing fill-rule math without a
// database — it applies outcomes the same way the real transactional
// store.PGStore.ExecutePaperFill does, just without locking or persistence.
type fakeStore struct {
	account   *store.PaperAccount
	positions map[string]*store.PaperPosition
	lastTrade *store.PaperTrade
}

func newFakeStore(cashBalance float64) *fakeStore {
	return &fakeStore{
		account:   &store.PaperAccount{ID: "acc1", UserID: "u1", CashBalance: cashBalance},
		positions: map[string]*store.PaperPosition{},
	}
}

func posKey(exchange, symbol, product string) string {
	return store.NormalizePaperExchange(exchange) + "|" + store.NormalizePaperSymbol(symbol) + "|" + product
}

func (f *fakeStore) ExecutePaperFill(_ context.Context, accountID, exchange, symbol, product string, fn store.PaperFillFunc) (*store.PaperFillOutcome, error) {
	if f.account == nil || f.account.ID != accountID {
		return nil, errors.New("paper account not found")
	}
	exchange = store.NormalizePaperExchange(exchange)
	symbol = store.NormalizePaperSymbol(symbol)
	key := posKey(exchange, symbol, product)
	outcome, err := fn(f.account, f.positions[key])
	if err != nil {
		return nil, err
	}
	if outcome.Position != nil {
		f.positions[key] = outcome.Position
	}
	if outcome.NewCashBalance != nil {
		f.account.CashBalance = *outcome.NewCashBalance
	}
	if outcome.Trade != nil {
		f.lastTrade = outcome.Trade
	}
	return outcome, nil
}

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 0.001 }

func TestEngineExecute_PositionLifecycle(t *testing.T) {
	fs := newFakeStore(100000)
	eng := New(fs)
	ctx := context.Background()

	req := func(side string, qty, price float64) domain.ExecutionRequest {
		return domain.ExecutionRequest{
			AccountID: "acc1", Symbol: "SBIN", Exchange: "NSE", Product: "MIS",
			Side: side, OrderType: string(domain.OrderTypeMarket), Quantity: qty, Price: price,
		}
	}

	// 1. BUY 10 @ 820.5 -> open long from flat
	res, err := eng.Execute(ctx, req("BUY", 10, 820.5))
	if err != nil || res.Status != string(domain.PaperOrderStatusFilled) {
		t.Fatalf("step1: %+v err=%v", res, err)
	}
	pos := fs.positions[posKey("NSE", "SBIN", "MIS")]
	if !almostEqual(pos.Quantity, 10) || !almostEqual(pos.AvgEntryPrice, 820.5) {
		t.Fatalf("step1 position: %+v", pos)
	}

	// 2. BUY 5 @ 830 -> add to long, weighted avg
	res, err = eng.Execute(ctx, req("BUY", 5, 830))
	if err != nil || res.Status != string(domain.PaperOrderStatusFilled) {
		t.Fatalf("step2: %+v err=%v", res, err)
	}
	pos = fs.positions[posKey("NSE", "SBIN", "MIS")]
	wantAvg := (820.5*10 + 830*5) / 15
	if !almostEqual(pos.Quantity, 15) || !almostEqual(pos.AvgEntryPrice, wantAvg) {
		t.Fatalf("step2 position: %+v want avg %v", pos, wantAvg)
	}

	// 3. SELL 5 @ 840 -> partial reduce long, realized pnl
	res, err = eng.Execute(ctx, req("SELL", 5, 840))
	if err != nil || res.Status != string(domain.PaperOrderStatusFilled) {
		t.Fatalf("step3: %+v err=%v", res, err)
	}
	wantPnL3 := (840 - wantAvg) * 5
	if !almostEqual(res.RealizedPnL, wantPnL3) {
		t.Fatalf("step3 realized pnl = %v want %v", res.RealizedPnL, wantPnL3)
	}

	// 4. SELL 15 @ 850 -> flips long(10) to short(-5)
	res, err = eng.Execute(ctx, req("SELL", 15, 850))
	if err != nil || res.Status != string(domain.PaperOrderStatusFilled) {
		t.Fatalf("step4: %+v err=%v", res, err)
	}
	wantPnL4 := (850 - wantAvg) * 10
	if !almostEqual(res.RealizedPnL, wantPnL4) {
		t.Fatalf("step4 realized pnl = %v want %v", res.RealizedPnL, wantPnL4)
	}
	pos = fs.positions[posKey("NSE", "SBIN", "MIS")]
	if !almostEqual(pos.Quantity, -5) || !almostEqual(pos.AvgEntryPrice, 850) {
		t.Fatalf("step4 position: %+v", pos)
	}

	// 5. BUY 3 @ 840 -> partial reduce short
	res, err = eng.Execute(ctx, req("BUY", 3, 840))
	if err != nil || res.Status != string(domain.PaperOrderStatusFilled) {
		t.Fatalf("step5: %+v err=%v", res, err)
	}
	wantPnL5 := (850.0 - 840) * 3
	if !almostEqual(res.RealizedPnL, wantPnL5) {
		t.Fatalf("step5 realized pnl = %v want %v", res.RealizedPnL, wantPnL5)
	}

	// 6. CLOSE @ 845 -> closes remaining short(-2) fully
	res, err = eng.Execute(ctx, req("CLOSE", 0, 845))
	if err != nil || res.Status != string(domain.PaperOrderStatusFilled) {
		t.Fatalf("step6: %+v err=%v", res, err)
	}
	wantPnL6 := (850.0 - 845) * 2
	if !almostEqual(res.RealizedPnL, wantPnL6) {
		t.Fatalf("step6 realized pnl = %v want %v", res.RealizedPnL, wantPnL6)
	}
	pos = fs.positions[posKey("NSE", "SBIN", "MIS")]
	if !almostEqual(pos.Quantity, 0) || !almostEqual(pos.AvgEntryPrice, 0) {
		t.Fatalf("step6 position: %+v", pos)
	}

	wantCash := 100000.0 - 820.5*10 - 830*5 + 840*5 + 850*15 - 840*3 - 845*2
	if !almostEqual(fs.account.CashBalance, wantCash) {
		t.Fatalf("final cash balance = %v want %v", fs.account.CashBalance, wantCash)
	}
}

func TestEngineExecute_ShortFromFlatAndFlipToLong(t *testing.T) {
	fs := newFakeStore(100000)
	eng := New(fs)
	ctx := context.Background()

	req := func(side string, qty, price float64) domain.ExecutionRequest {
		return domain.ExecutionRequest{
			AccountID: "acc1", Symbol: "INFY", Exchange: "NSE", Product: "MIS",
			Side: side, OrderType: string(domain.OrderTypeMarket), Quantity: qty, Price: price,
		}
	}

	res, err := eng.Execute(ctx, req("SELL", 10, 100))
	if err != nil || res.Status != string(domain.PaperOrderStatusFilled) {
		t.Fatalf("open short: %+v err=%v", res, err)
	}
	pos := fs.positions[posKey("NSE", "INFY", "MIS")]
	if !almostEqual(pos.Quantity, -10) || !almostEqual(pos.AvgEntryPrice, 100) {
		t.Fatalf("open short position: %+v", pos)
	}

	res, err = eng.Execute(ctx, req("BUY", 15, 90))
	if err != nil || res.Status != string(domain.PaperOrderStatusFilled) {
		t.Fatalf("flip to long: %+v err=%v", res, err)
	}
	wantPnL := (100.0 - 90) * 10
	if !almostEqual(res.RealizedPnL, wantPnL) {
		t.Fatalf("flip realized pnl = %v want %v", res.RealizedPnL, wantPnL)
	}
	pos = fs.positions[posKey("NSE", "INFY", "MIS")]
	if !almostEqual(pos.Quantity, 5) || !almostEqual(pos.AvgEntryPrice, 90) {
		t.Fatalf("post-flip position: %+v", pos)
	}
}

func TestEngineExecute_Rejections(t *testing.T) {
	fs := newFakeStore(100000)
	eng := New(fs)
	ctx := context.Background()

	cases := []struct {
		name string
		req  domain.ExecutionRequest
		want string
	}{
		{
			name: "market order missing price",
			req: domain.ExecutionRequest{
				AccountID: "acc1", Symbol: "TCS", Exchange: "NSE", Product: "MIS",
				Side: string(domain.SideBuy), OrderType: string(domain.OrderTypeMarket), Quantity: 10,
			},
			want: "price is required for paper MARKET order until live data feed is enabled",
		},
		{
			name: "limit order missing price",
			req: domain.ExecutionRequest{
				AccountID: "acc1", Symbol: "TCS", Exchange: "NSE", Product: "MIS",
				Side: string(domain.SideBuy), OrderType: string(domain.OrderTypeLimit), Quantity: 10,
			},
			want: "price is required for paper LIMIT order in v1",
		},
		{
			name: "close with no position",
			req: domain.ExecutionRequest{
				AccountID: "acc1", Symbol: "TCS", Exchange: "NSE", Product: "MIS",
				Side: string(domain.SideClose), OrderType: string(domain.OrderTypeMarket), Price: 100,
			},
			want: "No open position to close",
		},
		{
			name: "zero quantity",
			req: domain.ExecutionRequest{
				AccountID: "acc1", Symbol: "TCS", Exchange: "NSE", Product: "MIS",
				Side: string(domain.SideBuy), OrderType: string(domain.OrderTypeMarket), Quantity: 0, Price: 100,
			},
			want: "quantity must be greater than zero",
		},
		{
			name: "price not a tick size multiple",
			req: domain.ExecutionRequest{
				AccountID: "acc1", Symbol: "TCS", Exchange: "NSE", Product: "MIS",
				Side: string(domain.SideBuy), OrderType: string(domain.OrderTypeMarket), Quantity: 10, Price: 820.53,
				TickSize: 0.05,
			},
			want: "price 820.5300 is not a multiple of tick size 0.0500",
		},
		{
			name: "quantity not a lot size multiple",
			req: domain.ExecutionRequest{
				AccountID: "acc1", Symbol: "TCS", Exchange: "NSE", Product: "MIS",
				Side: string(domain.SideBuy), OrderType: string(domain.OrderTypeMarket), Quantity: 10, Price: 820.50,
				LotSize: 25,
			},
			want: "quantity 10.0000 is not a multiple of lot size 25.0000",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := eng.Execute(ctx, tc.req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Status != string(domain.PaperOrderStatusRejected) {
				t.Fatalf("expected rejected, got status=%s", res.Status)
			}
			if res.Reason != tc.want {
				t.Fatalf("reason = %q want %q", res.Reason, tc.want)
			}
		})
	}
}

func TestEngineExecute_TickAndLotSizeCompliantOrdersFill(t *testing.T) {
	fs := newFakeStore(100000)
	eng := New(fs)
	ctx := context.Background()

	// Exact multiples of both tick and lot size should fill normally.
	res, err := eng.Execute(ctx, domain.ExecutionRequest{
		AccountID: "acc1", Symbol: "NIFTY", Exchange: "NSE", Product: "MIS",
		Side: string(domain.SideBuy), OrderType: string(domain.OrderTypeMarket),
		Quantity: 50, Price: 22150.05, TickSize: 0.05, LotSize: 25,
	})
	if err != nil || res.Status != string(domain.PaperOrderStatusFilled) {
		t.Fatalf("expected fill, got %+v err=%v", res, err)
	}
}

func TestEngineExecute_CloseIgnoresLotSize(t *testing.T) {
	fs := newFakeStore(100000)
	eng := New(fs)
	ctx := context.Background()

	// Open a position whose quantity (10) is not a lot-size (25) multiple —
	// realistic if lot_size was configured/changed after the position was
	// opened. CLOSE must still be allowed to flatten it; only new order
	// quantities are lot-size-checked, not the engine-computed close amount.
	if _, err := eng.Execute(ctx, domain.ExecutionRequest{
		AccountID: "acc1", Symbol: "NIFTY", Exchange: "NSE", Product: "MIS",
		Side: string(domain.SideBuy), OrderType: string(domain.OrderTypeMarket),
		Quantity: 10, Price: 22150, TickSize: 0.05, // no LotSize set on this open, simulating pre-existing position
	}); err != nil {
		t.Fatalf("setup buy failed: %v", err)
	}

	res, err := eng.Execute(ctx, domain.ExecutionRequest{
		AccountID: "acc1", Symbol: "NIFTY", Exchange: "NSE", Product: "MIS",
		Side: string(domain.SideClose), OrderType: string(domain.OrderTypeMarket),
		Price: 22200, TickSize: 0.05, LotSize: 25,
	})
	if err != nil || res.Status != string(domain.PaperOrderStatusFilled) {
		t.Fatalf("expected CLOSE to fill despite non-lot-multiple quantity, got %+v err=%v", res, err)
	}
}

func TestEngineExecute_SlippageWorsensFillPrice(t *testing.T) {
	fs := newFakeStore(100000)
	eng := New(fs)
	ctx := context.Background()

	// 50 bps (0.5%) slippage on a BUY should fill *higher* than the signal
	// price: 100 * 1.005 = 100.5.
	res, err := eng.Execute(ctx, domain.ExecutionRequest{
		AccountID: "acc1", Symbol: "SBIN", Exchange: "NSE", Product: "MIS",
		Side: string(domain.SideBuy), OrderType: string(domain.OrderTypeMarket),
		Quantity: 10, Price: 100, SlippageBps: 50,
	})
	if err != nil || res.Status != string(domain.PaperOrderStatusFilled) {
		t.Fatalf("expected fill, got %+v err=%v", res, err)
	}
	if !almostEqual(res.FillPrice, 100.5) {
		t.Fatalf("expected fill price 100.5 (100 + 50bps), got %v", res.FillPrice)
	}

	// A SELL should fill *lower*: 100 * 0.995 = 99.5.
	res2, err := eng.Execute(ctx, domain.ExecutionRequest{
		AccountID: "acc1", Symbol: "SBIN", Exchange: "NSE", Product: "MIS",
		Side: string(domain.SideSell), OrderType: string(domain.OrderTypeMarket),
		Quantity: 10, Price: 100, SlippageBps: 50,
	})
	if err != nil || res2.Status != string(domain.PaperOrderStatusFilled) {
		t.Fatalf("expected fill, got %+v err=%v", res2, err)
	}
	if !almostEqual(res2.FillPrice, 99.5) {
		t.Fatalf("expected fill price 99.5 (100 - 50bps), got %v", res2.FillPrice)
	}
}

func TestEngineExecute_FeeBpsDeductedFromCashAndRecordedOnTrade(t *testing.T) {
	fs := newFakeStore(100000)
	eng := New(fs)
	ctx := context.Background()

	// BUY 10 @ 100, fee_bps=10 (0.1%): notional=1000, fee=1000*10/10000=1.0.
	// cash_balance should drop by exactly 1000 (the buy) + 1.0 (the fee) = 1001.
	res, err := eng.Execute(ctx, domain.ExecutionRequest{
		AccountID: "acc1", Symbol: "SBIN", Exchange: "NSE", Product: "MIS",
		Side: string(domain.SideBuy), OrderType: string(domain.OrderTypeMarket),
		Quantity: 10, Price: 100, FeeBps: 10,
	})
	if err != nil || res.Status != string(domain.PaperOrderStatusFilled) {
		t.Fatalf("expected fill, got %+v err=%v", res, err)
	}
	if !almostEqual(fs.account.CashBalance, 100000-1001) {
		t.Fatalf("expected cash_balance %v, got %v", 100000-1001, fs.account.CashBalance)
	}
	if fs.lastTrade == nil || !almostEqual(fs.lastTrade.Fees, 1.0) {
		t.Fatalf("expected trade.Fees 1.0, got %+v", fs.lastTrade)
	}
	// realized_pnl stays gross (0 on an opening buy) — fees are tracked
	// separately, not folded into realized_pnl.
	if !almostEqual(fs.lastTrade.RealizedPnL, 0) {
		t.Fatalf("expected realized_pnl 0 on an opening buy, got %v", fs.lastTrade.RealizedPnL)
	}

	// Now close it at the same price with the same fee_bps: realized_pnl
	// should be 0 (no price movement), but the fee is charged again on exit.
	res2, err := eng.Execute(ctx, domain.ExecutionRequest{
		AccountID: "acc1", Symbol: "SBIN", Exchange: "NSE", Product: "MIS",
		Side: string(domain.SideClose), OrderType: string(domain.OrderTypeMarket),
		Price: 100, FeeBps: 10,
	})
	if err != nil || res2.Status != string(domain.PaperOrderStatusFilled) {
		t.Fatalf("expected close to fill, got %+v err=%v", res2, err)
	}
	if !almostEqual(fs.lastTrade.Fees, 1.0) {
		t.Fatalf("expected exit trade.Fees 1.0, got %v", fs.lastTrade.Fees)
	}
	if !almostEqual(fs.account.CashBalance, 100000-1001-1.0+1000) {
		t.Fatalf("expected cash_balance %v after round-trip, got %v", 100000-1001-1.0+1000, fs.account.CashBalance)
	}
}

func TestEngineExecute_ZeroBpsMeansNoFrictionByDefault(t *testing.T) {
	fs := newFakeStore(100000)
	eng := New(fs)
	ctx := context.Background()

	// SlippageBps/FeeBps default to Go's zero value when unset (matching a
	// market profile that never opted in) — fill price and cash math must be
	// identical to the pre-fee-model behavior.
	res, err := eng.Execute(ctx, domain.ExecutionRequest{
		AccountID: "acc1", Symbol: "SBIN", Exchange: "NSE", Product: "MIS",
		Side: string(domain.SideBuy), OrderType: string(domain.OrderTypeMarket),
		Quantity: 10, Price: 100,
	})
	if err != nil || res.Status != string(domain.PaperOrderStatusFilled) {
		t.Fatalf("expected fill, got %+v err=%v", res, err)
	}
	if !almostEqual(res.FillPrice, 100) {
		t.Fatalf("expected fill price 100 (unchanged), got %v", res.FillPrice)
	}
	if !almostEqual(fs.account.CashBalance, 100000-1000) {
		t.Fatalf("expected cash_balance %v, got %v", 100000-1000, fs.account.CashBalance)
	}
	if fs.lastTrade == nil || fs.lastTrade.Fees != 0 {
		t.Fatalf("expected trade.Fees 0, got %+v", fs.lastTrade)
	}
}

func TestEngineExecute_MixedCaseSymbolAccumulates(t *testing.T) {
	fs := newFakeStore(100000)
	eng := New(fs)
	ctx := context.Background()

	req := func(symbol string, qty float64) domain.ExecutionRequest {
		return domain.ExecutionRequest{
			AccountID: "acc1", Symbol: symbol, Exchange: "nse", Product: "MIS",
			Side: string(domain.SideBuy), OrderType: string(domain.OrderTypeMarket), Quantity: qty, Price: 100,
		}
	}

	if _, err := eng.Execute(ctx, req("sbin", 1)); err != nil {
		t.Fatalf("first buy: %v", err)
	}
	if _, err := eng.Execute(ctx, req("SBIN", 2)); err != nil {
		t.Fatalf("second buy: %v", err)
	}
	pos := fs.positions[posKey("NSE", "SBIN", "MIS")]
	if pos == nil || !almostEqual(pos.Quantity, 3) {
		t.Fatalf("expected qty 3 on one position, got %+v", pos)
	}
}

func TestEngineExecute_AccountNotFound(t *testing.T) {
	fs := newFakeStore(100000)
	eng := New(fs)
	ctx := context.Background()

	_, err := eng.Execute(ctx, domain.ExecutionRequest{
		AccountID: "does-not-exist", Symbol: "TCS", Exchange: "NSE", Product: "MIS",
		Side: string(domain.SideBuy), OrderType: string(domain.OrderTypeMarket), Quantity: 10, Price: 100,
	})
	if err == nil {
		t.Fatal("expected an error for an unknown account, got nil")
	}
}
