package store

import (
	"errors"
	"regexp"
	"strconv"
	"testing"

	"github.com/lib/pq"
)

// insertTradeSQL mirrors the INSERT in PGStore.InsertTrade — keep in sync.
const insertTradeSQL = `
INSERT INTO trades
   (id, user_id, webhook_id, webhook_label, signal, symbol, lot_size, broker_order, signal_key, comment, algo_id, status, fill_price, order_type, product, sl_price, tp_price, error, error_code, created_at, updated_at, broker_responded_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,
         CASE WHEN $12 = 'queued' THEN NULL ELSE $21::timestamptz END)`

func TestInsertTradeSQLBindCount(t *testing.T) {
	re := regexp.MustCompile(`\$(\d+)`)
	max := 0
	for _, m := range re.FindAllStringSubmatch(insertTradeSQL, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatal(err)
		}
		if n > max {
			max = n
		}
	}
	const wantArgs = 21
	if max != wantArgs {
		t.Fatalf("InsertTrade SQL max placeholder $%d, Go must pass %d args", max, wantArgs)
	}
}

func TestIsTradeDuplicateKey(t *testing.T) {
	dup := &pq.Error{Code: "23505"}
	if !isTradeDuplicateKey(dup) {
		t.Fatal("expected duplicate key detection")
	}
	if isTradeDuplicateKey(errors.New("other")) {
		t.Fatal("expected false for generic error")
	}
}
