package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordIngestQueued(t *testing.T) {
	before := testutil.ToFloat64(IngestTotal.WithLabelValues(IngestQueued))
	RecordIngest(IngestQueued)
	after := testutil.ToFloat64(IngestTotal.WithLabelValues(IngestQueued))
	if after != before+1 {
		t.Fatalf("ingest queued: want %f got %f", before+1, after)
	}
}

func TestRecordTradeNormalizesLabels(t *testing.T) {
	before := testutil.ToFloat64(TradesTotal.WithLabelValues("submitted", labelNone, "zerodha"))
	RecordTrade("submitted", "", "zerodha")
	after := testutil.ToFloat64(TradesTotal.WithLabelValues("submitted", labelNone, "zerodha"))
	if after != before+1 {
		t.Fatalf("trade counter: want %f got %f", before+1, after)
	}
}

func TestRecordDedupHit(t *testing.T) {
	before := testutil.ToFloat64(DedupHitsTotal)
	RecordDedupHit()
	after := testutil.ToFloat64(DedupHitsTotal)
	if after != before+1 {
		t.Fatalf("dedup hits: want %f got %f", before+1, after)
	}
}

func TestObserveBrokerHTTP(t *testing.T) {
	ObserveBrokerHTTP("mt5_cloud", "place_order", 250*time.Millisecond)
	if testutil.CollectAndCount(BrokerHTTPDuration) == 0 {
		t.Fatal("expected broker HTTP histogram sample")
	}
}

func TestRecordIngestRateLimitedAndQueueFull(t *testing.T) {
	beforeRL := testutil.ToFloat64(IngestTotal.WithLabelValues(IngestRateLimited))
	RecordIngest(IngestRateLimited)
	afterRL := testutil.ToFloat64(IngestTotal.WithLabelValues(IngestRateLimited))
	if afterRL != beforeRL+1 {
		t.Fatalf("ingest rate_limited: want %f got %f", beforeRL+1, afterRL)
	}

	beforeQF := testutil.ToFloat64(IngestTotal.WithLabelValues(IngestQueueFull))
	RecordIngest(IngestQueueFull)
	afterQF := testutil.ToFloat64(IngestTotal.WithLabelValues(IngestQueueFull))
	if afterQF != beforeQF+1 {
		t.Fatalf("ingest queue_full: want %f got %f", beforeQF+1, afterQF)
	}
}

func TestRecordBillingCheckout(t *testing.T) {
	before := testutil.ToFloat64(BillingCheckoutTotal)
	RecordBillingCheckout()
	after := testutil.ToFloat64(BillingCheckoutTotal)
	if after != before+1 {
		t.Fatalf("billing checkout: want %f got %f", before+1, after)
	}
}

func TestRecordBillingWebhook(t *testing.T) {
	before := testutil.ToFloat64(BillingWebhookTotal.WithLabelValues(BillingWebhookOK))
	RecordBillingWebhook(BillingWebhookOK)
	after := testutil.ToFloat64(BillingWebhookTotal.WithLabelValues(BillingWebhookOK))
	if after != before+1 {
		t.Fatalf("billing webhook ok: want %f got %f", before+1, after)
	}
}
