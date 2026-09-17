// Package missquareoff runs a background job that auto-exits all open MIS paper
// positions between 15:15 and 15:30 IST on weekdays — mirroring Indian
// exchange intraday square-off.
package missquareoff

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/marketdata"
	"github.com/SPSingh09/zettabridge/internal/paperengine"
	"github.com/SPSingh09/zettabridge/internal/store"
)

var ist = time.FixedZone("IST", 5*3600+30*60)

const (
	windowStartMin = 15*60 + 15 // 15:15 IST
	windowEndMin   = 15*60 + 30 // 15:30 IST
)

// Job periodically squares off open MIS paper positions during the exchange
// auto square-off window.
type Job struct {
	pg     *store.PGStore
	engine domain.ExecutionDestination
	md     *marketdata.Registry
}

func New(pg *store.PGStore, engine domain.ExecutionDestination, md *marketdata.Registry) *Job {
	return &Job{pg: pg, engine: engine, md: md}
}

// Start launches the background goroutine. Cancel ctx to stop it.
func (j *Job) Start(ctx context.Context) {
	go j.run(ctx)
}

func (j *Job) run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			if InWindow(t.In(ist)) {
				j.squareOffAll(ctx)
			}
		}
	}
}

// InWindow reports whether ist falls inside the weekday MIS square-off window.
func InWindow(istTime time.Time) bool {
	switch istTime.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	}
	mins := istTime.Hour()*60 + istTime.Minute()
	return mins >= windowStartMin && mins <= windowEndMin
}

func (j *Job) squareOffAll(ctx context.Context) {
	rows, err := j.pg.ListOpenMISPaperPositions(ctx)
	if err != nil {
		log.Printf("missquareoff: list open MIS positions failed: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	log.Printf("missquareoff: squaring off %d open MIS position(s)", len(rows))
	var closed, failed int
	for _, row := range rows {
		if row == nil || row.Position == nil || row.Account == nil {
			continue
		}
		if row.Account.Status == "closed" {
			continue
		}
		signalID := uuid.New().String()
		result, err := paperengine.FlattenPosition(
			ctx, j.engine, j.md, j.pg, row.Account, row.Position, signalID, false,
		)
		if err != nil {
			failed++
			log.Printf("missquareoff: close failed account=%s symbol=%s: %v",
				row.Account.ID, row.Position.Symbol, err)
			continue
		}
		if result == nil || result.Status != string(domain.PaperOrderStatusFilled) {
			failed++
			reason := ""
			if result != nil {
				reason = result.Reason
			}
			log.Printf("missquareoff: close rejected account=%s symbol=%s: %s",
				row.Account.ID, row.Position.Symbol, reason)
			continue
		}
		closed++
		log.Printf("missquareoff: closed account=%s symbol=%s fill=%.2f",
			row.Account.ID, row.Position.Symbol, result.FillPrice)
	}
	if closed > 0 || failed > 0 {
		log.Printf("missquareoff: done closed=%d failed=%d", closed, failed)
	}
}
