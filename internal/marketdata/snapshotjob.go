package marketdata

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// Store is the narrow persistence surface the snapshot job needs — mirrors
// the paperengine.Engine / queue.TradeStore precedent of depending on an
// interface, not the concrete *store.PGStore.
type Store interface {
	ListPaperAccountsWithOpenPositions(ctx context.Context) ([]*store.PaperAccount, error)
	ListPaperPositionsByAccount(ctx context.Context, accountID string) ([]*store.PaperPosition, error)
	UpdatePaperPositionMark(ctx context.Context, pos *store.PaperPosition) error
	GetPaperAccountPnL(ctx context.Context, accountID string) (*store.PaperPnLSummary, error)
	RecordPaperAccountEquitySnapshot(ctx context.Context, accountID, userID string) error
	GetPlatformSetting(ctx context.Context, key string) (value string, ok bool, err error)
}

// SnapshotJob periodically sweeps every paper account with an open position,
// refreshes last_price/unrealized_pnl from whichever provider is currently
// active (a no-op under SignalPriceProvider or an unconfigured real feed),
// and writes a paper_account_snapshots row so equity/P&L has a history over
// time. The active provider is re-read from platform_settings on every
// sweep, so an admin toggling it takes effect on the next tick — no restart.
// The sweep cadence itself is also admin-configurable the same way (see
// SettingKeyIntervalSec) — Interval is only the env-configured default used
// when no override is set.
type SnapshotJob struct {
	Store    Store
	Registry *Registry
	Interval time.Duration
	notify   MarkNotifier
}

// New returns a SnapshotJob. Call Start to begin the background ticker.
func New(s Store, registry *Registry, interval time.Duration) *SnapshotJob {
	return &SnapshotJob{Store: s, Registry: registry, Interval: interval}
}

// SetMarkNotifier wires optional real-time fan-out when LTP marks change.
func (j *SnapshotJob) SetMarkNotifier(n MarkNotifier) {
	if j != nil {
		j.notify = n
	}
}

// activeProvider resolves the currently configured provider from
// platform_settings, falling back to the registry's default when unset,
// unreadable, or unrecognized.
func (j *SnapshotJob) activeProvider(ctx context.Context) Provider {
	return j.Registry.ActiveProvider(ctx, j.Store)
}

// currentInterval resolves the currently configured sweep cadence from
// platform_settings, falling back to the env-configured Interval when unset,
// unreadable, or out of bounds.
func (j *SnapshotJob) currentInterval(ctx context.Context) time.Duration {
	v, ok, err := j.Store.GetPlatformSetting(ctx, SettingKeyIntervalSec)
	if err != nil {
		log.Printf("marketdata: read %s setting failed, using default: %v", SettingKeyIntervalSec, err)
	}
	if ok {
		if secs, err := strconv.Atoi(v); err == nil && secs >= MinSnapshotIntervalSec && secs <= MaxSnapshotIntervalSec {
			return time.Duration(secs) * time.Second
		}
	}
	return j.Interval
}

// Start runs the sweep on a self-rescheduling timer until ctx is cancelled.
// The interval is re-read before each wait, so an admin changing it takes
// effect starting with the next cycle — no restart. Non-blocking.
func (j *SnapshotJob) Start(ctx context.Context) {
	go func() {
		for {
			timer := time.NewTimer(j.currentInterval(ctx))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				j.Sweep(ctx)
			}
		}
	}()
}

// maxSymbolsPerBatch is FYERS's documented per-call quote limit (see
// fyers.Provider.GetBatchLTP). The sweep chunks its combined symbol set to
// this size instead of issuing one GetBatchLTP call per account — with N
// paper accounts open, a per-account call pattern fires N sequential FYERS
// requests within milliseconds of each other every tick, which trips
// FYERS's rate limit long before any single account's position count would.
const maxSymbolsPerBatch = 50

// Sweep runs one pass over every account with an open position. Exported so
// tests (and, if ever needed, an on-demand admin trigger) can call it directly
// without waiting for the ticker.
func (j *SnapshotJob) Sweep(ctx context.Context) {
	accounts, err := j.Store.ListPaperAccountsWithOpenPositions(ctx)
	if err != nil {
		log.Printf("marketdata: sweep: list accounts failed: %v", err)
		return
	}
	if len(accounts) == 0 {
		return
	}
	provider := j.activeProvider(ctx)

	type accountPositions struct {
		account *store.PaperAccount
		open    []*store.PaperPosition
	}
	var all []accountPositions
	uniqueSymbols := make(map[string]Symbol)
	for _, account := range accounts {
		positions, err := j.Store.ListPaperPositionsByAccount(ctx, account.ID)
		if err != nil {
			log.Printf("marketdata: sweep account %s: list positions failed: %v", account.ID, err)
			continue
		}
		var open []*store.PaperPosition
		for _, pos := range positions {
			if pos.Quantity == 0 {
				continue
			}
			open = append(open, pos)
			sym := Symbol{
				Exchange: store.NormalizePaperExchange(pos.Exchange),
				Symbol:   store.NormalizePaperSymbol(pos.Symbol),
			}
			uniqueSymbols[SymbolKey(sym.Exchange, sym.Symbol)] = sym
		}
		if len(open) > 0 {
			all = append(all, accountPositions{account: account, open: open})
		}
	}
	if len(all) == 0 {
		return
	}

	symbols := make([]Symbol, 0, len(uniqueSymbols))
	for _, s := range uniqueSymbols {
		symbols = append(symbols, s)
	}

	quoteByKey := make(map[string]float64, len(symbols))
	for i := 0; i < len(symbols); i += maxSymbolsPerBatch {
		end := i + maxSymbolsPerBatch
		if end > len(symbols) {
			end = len(symbols)
		}
		quotes, err := provider.GetBatchLTP(ctx, symbols[i:end])
		if err != nil && !errors.Is(err, ErrNoQuote) {
			log.Printf("marketdata: sweep: batch quote fetch failed (symbols %d-%d): %v", i, end, err)
		}
		for _, q := range quotes {
			quoteByKey[SymbolKey(q.Symbol.Exchange, q.Symbol.Symbol)] = q.LTP
		}
	}

	for _, ap := range all {
		j.applyMarks(ctx, ap.account, ap.open, quoteByKey)
	}
}

func (j *SnapshotJob) applyMarks(ctx context.Context, account *store.PaperAccount, open []*store.PaperPosition, quoteByKey map[string]float64) {
	now := time.Now().UTC()
	var updated []*store.PaperPosition
	for _, pos := range open {
		ltp, ok := quoteByKey[SymbolKey(pos.Exchange, pos.Symbol)]
		if !ok {
			continue // no new quote — keep the position's existing last_price/unrealized_pnl
		}
		pos.LastPrice = ltp
		pos.UnrealizedPnL = domain.UnrealizedPnL(pos.Quantity, pos.AvgEntryPrice, ltp)
		pos.UpdatedAt = now
		if err := j.Store.UpdatePaperPositionMark(ctx, pos); err != nil {
			log.Printf("marketdata: sweep account %s: update position %s failed: %v", account.ID, pos.ID, err)
			continue
		}
		updated = append(updated, pos)
	}

	if j.notify != nil && len(updated) > 0 {
		j.notify.OnPaperMarksUpdated(ctx, account, updated)
	}

	if err := j.Store.RecordPaperAccountEquitySnapshot(ctx, account.ID, account.UserID); err != nil {
		log.Printf("marketdata: sweep account %s: create snapshot failed: %v", account.ID, err)
	}
}
