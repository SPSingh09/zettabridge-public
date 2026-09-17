package metrics

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	IngestQueued       = "queued"
	IngestDeduplicated = "deduplicated"
	IngestRejected     = "rejected"
	IngestRateLimited  = "rate_limited"
	IngestQueueFull    = "queue_full"
	labelNone          = "none"
	labelUnknownBroker = "unknown"
)

var (
	IngestTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "zettabridge_ingest_total",
		Help: "Webhook signal ingest outcomes by status (queued, deduplicated, rejected, rate_limited, queue_full).",
	}, []string{"status"})

	TradesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "zettabridge_trades_total",
		Help: "Trades persisted by worker status, error_code, and broker_type.",
	}, []string{"status", "error_code", "broker_type"})

	DedupHitsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "zettabridge_dedup_hits_total",
		Help: "Duplicate signals suppressed at ingest or worker dedup layer.",
	})

	BrokerHTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "zettabridge_broker_http_duration_seconds",
		Help:    "Outbound broker HTTP request latency.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20},
	}, []string{"broker", "op"})

	BillingCheckoutTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "zettabridge_billing_checkout_total",
		Help: "Stripe Checkout sessions created successfully.",
	})

	BillingWebhookTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "zettabridge_billing_webhook_total",
		Help: "Stripe billing webhook requests by outcome status.",
	}, []string{"status"})
)

const (
	BillingWebhookOK                 = "ok"
	BillingWebhookError              = "error"
	BillingWebhookSignatureInvalid   = "signature_invalid"
	BillingWebhookDisabled           = "disabled"
)

func init() {
	prometheus.MustRegister(IngestTotal, TradesTotal, DedupHitsTotal, BrokerHTTPDuration, BillingCheckoutTotal, BillingWebhookTotal)
	// CounterVec families are omitted from scrapes until a label set exists; seed
	// known low-cardinality combinations so /metrics documents them at startup.
	for _, status := range []string{
		IngestQueued, IngestDeduplicated, IngestRejected, IngestRateLimited, IngestQueueFull,
	} {
		IngestTotal.WithLabelValues(status)
	}
	TradesTotal.WithLabelValues("unknown", labelNone, labelUnknownBroker)
	for _, st := range []string{"submitted", "rejected", "filled", "pending_confirmation"} {
		TradesTotal.WithLabelValues(st, labelNone, labelUnknownBroker)
	}
	BrokerHTTPDuration.WithLabelValues(labelUnknownBroker, "place_order")
	for _, status := range []string{BillingWebhookOK, BillingWebhookError, BillingWebhookSignatureInvalid, BillingWebhookDisabled} {
		BillingWebhookTotal.WithLabelValues(status)
	}
}

// RecordIngest increments ingest counter for the given status label.
func RecordIngest(status string) {
	IngestTotal.WithLabelValues(status).Inc()
}

// RecordTrade increments trade counter with normalized low-cardinality labels.
func RecordTrade(status, errorCode, brokerType string) {
	TradesTotal.WithLabelValues(
		normalizeStatus(status),
		normalizeErrorCode(errorCode),
		normalizeBroker(brokerType),
	).Inc()
}

// RecordDedupHit increments dedup suppression counter.
func RecordDedupHit() {
	DedupHitsTotal.Inc()
}

// ObserveBrokerHTTP records broker outbound HTTP latency.
func ObserveBrokerHTTP(broker, op string, d time.Duration) {
	BrokerHTTPDuration.WithLabelValues(normalizeBroker(broker), normalizeOp(op)).Observe(d.Seconds())
}

// RecordBillingCheckout increments successful Stripe Checkout session creations.
func RecordBillingCheckout() {
	BillingCheckoutTotal.Inc()
}

// RecordBillingWebhook increments Stripe webhook requests by outcome status.
func RecordBillingWebhook(status string) {
	s := strings.TrimSpace(strings.ToLower(status))
	if s == "" {
		s = BillingWebhookError
	}
	BillingWebhookTotal.WithLabelValues(s).Inc()
}

func normalizeStatus(status string) string {
	s := strings.TrimSpace(strings.ToLower(status))
	if s == "" {
		return "unknown"
	}
	return s
}

func normalizeErrorCode(code string) string {
	c := strings.TrimSpace(strings.ToLower(code))
	if c == "" {
		return labelNone
	}
	return c
}

func normalizeBroker(broker string) string {
	b := strings.TrimSpace(strings.ToLower(broker))
	if b == "" {
		return labelUnknownBroker
	}
	return b
}

func normalizeOp(op string) string {
	o := strings.TrimSpace(strings.ToLower(op))
	if o == "" {
		return "unknown"
	}
	return o
}
