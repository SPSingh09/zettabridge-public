package marketdata

import (
	"context"

	"github.com/SPSingh09/zettabridge/internal/store"
)

// MarkNotifier receives paper position mark updates (LTP / unrealized P&L).
type MarkNotifier interface {
	OnPaperMarksUpdated(ctx context.Context, account *store.PaperAccount, positions []*store.PaperPosition)
}
