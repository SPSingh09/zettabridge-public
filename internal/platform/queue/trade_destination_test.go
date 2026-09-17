package queue

import (
	"context"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokerfactory"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestResolveWebhookProductPaperDefault(t *testing.T) {
	fs := &fakeStore{paperAccount: basePaperAccount()}
	q := newTestQueue(t, fs, nil, "", nil)

	wh := paperWebhook()
	params := guard.TradeParams{Symbol: "SBIN", Lot: 1}
	if err := q.resolveWebhookProduct(context.Background(), wh, &params); err != nil {
		t.Fatal(err)
	}
	if params.Product != "MIS" {
		t.Fatalf("product=%q want MIS from paper account default", params.Product)
	}
}

func TestResolveWebhookProductMultiProductRequiresSignal(t *testing.T) {
	fs := &fakeStore{
		cred: &store.BrokerCredential{
			ID:         "c1",
			BrokerType: "zerodha",
			Product:    "MIS,CNC",
		},
	}
	q := newTestQueue(t, fs, nil, "", nil)

	wh := &store.Webhook{BrokerCredID: strPtr("c1")}
	params := guard.TradeParams{Symbol: "RELIANCE", Lot: 1}
	if err := q.resolveWebhookProduct(context.Background(), wh, &params); err == nil {
		t.Fatal("expected error when product omitted on multi-product credential")
	}

	params = guard.TradeParams{Symbol: "RELIANCE", Lot: 1, Product: "CNC"}
	if err := q.resolveWebhookProduct(context.Background(), wh, &params); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.Product != "CNC" {
		t.Fatalf("product=%q want CNC", params.Product)
	}
}

func TestProcessPaperWebhookEndToEnd(t *testing.T) {
	engine := &fakePaperEngine{}
	fs := &fakeStore{
		user:          paperTestUser(),
		paperAccount:  basePaperAccount(),
		instrument:    activeInstrument(),
		marketProfile: alwaysOpenProfile(),
	}
	q := newTestQueue(t, fs, nil, "", nil)
	q.SetPaperEngine(engine)

	job := Job{
		Webhook: paperWebhook(),
		Signal:  &store.SignalPayload{Action: "BUY", Symbol: "SBIN", Lot: 10, Price: 100},
		TradeID: "paper-trade-1",
	}
	q.process(job)

	if len(fs.trades) == 0 {
		t.Fatal("expected trade row to be persisted")
	}
	tr := fs.trades[0]
	if tr.ID != "paper-trade-1" {
		t.Fatalf("trade id=%q", tr.ID)
	}
	if tr.Status != "filled" {
		t.Fatalf("status=%q want filled error=%q code=%q", tr.Status, tr.Error, tr.ErrorCode)
	}
	if tr.Product != "MIS" {
		t.Fatalf("product=%q want MIS on trade row", tr.Product)
	}
	if !engine.called {
		t.Fatal("expected paper engine Execute to be called")
	}
	if engine.lastReq.Product != "MIS" {
		t.Fatalf("execution product=%q want MIS", engine.lastReq.Product)
	}
}

func TestProcessLiveMultiProductCNCMissingProductRejectedWithTradeRow(t *testing.T) {
	fs := &fakeStore{
		user: baseUser("u1"),
		cred: &store.BrokerCredential{
			ID:             "c1",
			BrokerType:     "zerodha",
			EncryptedCreds: "api_key:access_token",
			AccountMode:    plan.AccountLive,
			Exchange:       "NSE",
			Product:        "MIS,CNC",
		},
	}
	q := newTestQueue(t, fs, nil, brokerfactory.ModeLive, func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
		t.Fatal("broker should not be called when product is missing")
		return nil, nil
	})

	job := baseJob("u1", "c1")
	job.TradeID = "live-cnc-missing-product"
	q.process(job)

	tr := lastTrade(fs)
	if tr == nil {
		t.Fatal("expected rejected trade row")
	}
	if tr.Status != "rejected" {
		t.Fatalf("status=%q want rejected", tr.Status)
	}
	if tr.ErrorCode != "product_required" {
		t.Fatalf("error_code=%q want product_required", tr.ErrorCode)
	}
}

func strPtr(s string) *string { return &s }
