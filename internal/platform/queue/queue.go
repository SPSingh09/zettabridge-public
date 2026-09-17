package queue

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/SPSingh09/zettabridge/internal/algo"
	"github.com/SPSingh09/zettabridge/internal/compliance"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/execution"
	"github.com/SPSingh09/zettabridge/internal/execution/adapter/local"
	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokerfactory"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
	"github.com/SPSingh09/zettabridge/internal/integrations/telegram"
	"github.com/SPSingh09/zettabridge/internal/marketdata"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/platform/mailer"
	"github.com/SPSingh09/zettabridge/internal/platform/metrics"
	"github.com/SPSingh09/zettabridge/internal/platform/security/idempotency"
	"github.com/SPSingh09/zettabridge/internal/store"
)

const tradeTimeout = 10 * time.Second

// TradeNotifier receives persisted trade rows (e.g. WebSocket fan-out).
type TradeNotifier interface {
	OnTradeInserted(ctx context.Context, trade *store.Trade)
}

type TradeStore interface {
	GetUserByID(ctx context.Context, id string) (*store.User, error)
	GetOrgByID(ctx context.Context, id string) (*store.Organization, error)
	GetBrokerCred(ctx context.Context, id string) (*store.BrokerCredential, error)
	GetPaperAccount(ctx context.Context, id string) (*store.PaperAccount, error)
	GetInstrument(ctx context.Context, marketProfileCode, exchange, symbol string) (*store.Instrument, error)
	GetMarketProfileByCode(ctx context.Context, code string) (*store.MarketProfile, error)
	GetPlatformSetting(ctx context.Context, key string) (value string, ok bool, err error)
	InsertTrade(ctx context.Context, t *store.Trade) error
	PersistTradeAudit(ctx context.Context, t *store.Trade) error
	FinalizeTrade(ctx context.Context, t *store.Trade) error
	HasActiveTradeWithSignalKey(ctx context.Context, webhookID, signalKey string, windowSec int) (bool, error)
	IncrementUserLiveOrdersUsed(ctx context.Context, userID string) error
	InsertNotification(ctx context.Context, userID, typ, title, body, tradeID string)
}

type RateLimiter interface {
	IncrRateLimit(ctx context.Context, userID string) (int64, error)
	IncrBrokerCredRateLimit(ctx context.Context, brokerCredID string) (int64, error)
}

type brokerFactory = local.BrokerFactory

// Job is pushed onto the queue by the webhook HTTP handler.
type Job struct {
	Webhook   *store.Webhook
	Signal    *store.SignalPayload
	SignalKey string
	TradeID   string
}

// Queue is a buffered channel backed by a fixed goroutine worker pool.
// The HTTP handler returns 202 immediately; workers process asynchronously.
type Queue struct {
	jobs        chan Job
	workerCount int
	pg          TradeStore
	redis       RateLimiter
	credKey     []byte
	brokerMode  string
	infra       *livebrokers.Infra
	newBroker  brokerFactory
	algoPolicy algo.Policy
	notifier    TradeNotifier
	tg          *telegram.Sender
	mailer      mailer.Sender
	billingUpgradeURL string
	paperEngine domain.ExecutionDestination
	orchestrator *execution.Orchestrator
	mdRegistry   *marketdata.Registry
	wg          sync.WaitGroup
}

// ExecutionRouter exposes enabled adapter gating for HTTP handlers.
func (q *Queue) ExecutionRouter() *execution.Router {
	if q == nil || q.orchestrator == nil {
		return nil
	}
	return q.orchestrator.Router()
}

// SetPaperEngine wires the Paper Trading Engine (internal/paperengine.Engine)
// used for webhooks whose destination is a paper account rather than a live
// broker credential.
func (q *Queue) SetPaperEngine(e domain.ExecutionDestination) {
	q.paperEngine = e
	if q.orchestrator != nil {
		q.orchestrator.SetPaperEngine(e)
	}
}

func (q *Queue) SetBrokerFactory(f brokerFactory) {
	q.newBroker = f
	if q.orchestrator != nil {
		q.orchestrator.SetBrokerFactory(f)
	}
}

func (q *Queue) SetTradeNotifier(n TradeNotifier) {
	q.notifier = n
}

func (q *Queue) SetTelegramSender(s *telegram.Sender) {
	q.tg = s
}

func (q *Queue) SetMailer(m mailer.Sender) {
	q.mailer = m
}

func (q *Queue) SetBillingUpgradeURL(url string) {
	q.billingUpgradeURL = url
}

// SetMarketDataRegistry wires the LTP provider registry used to price paper
// MARKET orders when the signal omits an explicit price.
func (q *Queue) SetMarketDataRegistry(r *marketdata.Registry) {
	q.mdRegistry = r
}

// MarketDataRegistry returns the registry set via SetMarketDataRegistry, so
// other handlers built alongside the queue (e.g. paperaccounts.Handler's
// Diagnostics section) can report the active market data provider without
// each needing their own copy of app.go's registry construction.
func (q *Queue) MarketDataRegistry() *marketdata.Registry {
	return q.mdRegistry
}

func New(workerCount, bufferSize int, pg *store.PGStore, redis *store.RedisStore, credKey []byte, brokerMode string, infra *livebrokers.Infra, algoPolicy algo.Policy, enabledAdapters []string, transport execution.TransportConfig) *Queue {
	q := &Queue{
		jobs:        make(chan Job, bufferSize),
		workerCount: workerCount,
		pg:          pg,
		redis:       redis,
		credKey:     credKey,
		brokerMode:  brokerMode,
		infra:       infra,
		newBroker:   brokerfactory.New,
		algoPolicy:  algoPolicy,
	}
	q.initOrchestrator(enabledAdapters, transport)
	return q
}

func (q *Queue) initOrchestrator(enabledAdapters []string, transport execution.TransportConfig) {
	var linker local.PublisherLinker
	if l, ok := q.pg.(local.PublisherLinker); ok {
		linker = l
	}
	q.orchestrator = buildOrchestrator(
		transport,
		enabledAdapters,
		local.LiveDeps{
			Store:      q.pg,
			Redis:      q.redis,
			CredKey:    q.credKey,
			BrokerMode: q.brokerMode,
			Infra:      q.infra,
			NewBroker:  q.newBroker,
			AlgoPolicy: q.algoPolicy,
			Linker:     linker,
		},
		q.paperEngine,
		q.pg,
	)
}

// NewForTest wires a queue for integration tests with injectable stores and the real broker factory.
func NewForTest(store TradeStore, limiter RateLimiter, credKey []byte, brokerMode string, infra *livebrokers.Infra) *Queue {
	q := &Queue{
		jobs:       make(chan Job, 1),
		pg:         store,
		redis:      limiter,
		credKey:    credKey,
		brokerMode: brokerMode,
		infra:      infra,
		newBroker:  brokerfactory.New,
	}
	q.initOrchestrator(execution.DefaultEnabledAdapters(), execution.TransportConfig{Mode: execution.TransportLocal})
	return q
}

// ProcessJobSync runs one job synchronously (integration tests).
func (q *Queue) ProcessJobSync(job Job) {
	q.process(job)
}

func (q *Queue) Start() {
	for i := 0; i < q.workerCount; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}
	log.Printf("queue: started %d workers (broker_mode=%s)", q.workerCount, q.brokerMode)
}

func (q *Queue) Stop() {
	close(q.jobs)
	q.wg.Wait()
	log.Println("queue: all workers stopped")
}

// Enqueue pushes a job. Returns false if the buffer is full (back-pressure).
func (q *Queue) Enqueue(j Job) bool {
	select {
	case q.jobs <- j:
		return true
	default:
		return false
	}
}

func (q *Queue) worker(id int) {
	defer q.wg.Done()
	log.Printf("queue: worker %d ready", id)

	for job := range q.jobs {
		q.process(job)
	}
	log.Printf("queue: worker %d exiting", id)
}

func (q *Queue) process(job Job) {
	ctx, cancel := context.WithTimeout(context.Background(), tradeTimeout)
	defer cancel()

	wh := job.Webhook
	sig := job.Signal
	signalKey := job.SignalKey
	if signalKey == "" {
		signalKey = idempotency.SignalKey(wh.ID, wh, sig)
	}
	tradeID := job.TradeID
	if tradeID == "" {
		tradeID = newTradeID()
	}

	if err := guard.ValidateAction(wh, sig.Action); err != nil {
		q.rejectTrade(ctx, wh, sig, signalKey, tradeID, err)
		return
	}

	params := guard.ResolveParams(wh, sig)
	if strings.TrimSpace(params.Symbol) == "" {
		q.rejectTrade(ctx, wh, sig, signalKey, tradeID, guard.ErrSymbolRequired)
		return
	}
	if err := guard.ValidateSymbol(wh, params.Symbol); err != nil {
		q.rejectTrade(ctx, wh, sig, signalKey, tradeID, err)
		return
	}
	lot, err := guard.ResolveQuantity(wh, params.Lot)
	if err != nil {
		q.rejectTrade(ctx, wh, sig, signalKey, tradeID, err)
		return
	}
	params.Lot = lot

	if err := guard.ValidateOrderType(wh, sig.OrderType); err != nil {
		q.rejectTrade(ctx, wh, sig, signalKey, tradeID, err)
		return
	}

	if err := q.resolveWebhookProduct(ctx, wh, &params); err != nil {
		q.rejectTradeWithCode(ctx, wh, sig, signalKey, tradeID, tradeRejectCodeForProductErr(err), err.Error(), brokerTypeForWebhook(ctx, q.pg, wh))
		return
	}

	trade := &store.Trade{
		ID:           tradeID,
		UserID:       wh.UserID,
		WebhookID:    wh.ID,
		WebhookLabel: wh.Label,
		Signal:       sig.Action,
		Symbol:       params.Symbol,
		LotSize:      lot,
		SignalKey:    signalKey,
		Comment:      sig.Comment,
		Status:       "queued",
		CreatedAt:    nowUTC(),
	}
	applyTradeSignalMeta(trade, wh, sig, params, params.Product)

	if idempotency.Enabled(wh) {
		dup, err := q.pg.HasActiveTradeWithSignalKey(ctx, wh.ID, signalKey, wh.DedupWindowSec)
		if err != nil {
			log.Printf("worker: dedup lookup error webhook=%s: %v", wh.ID, err)
		} else if dup {
			trade.Status = "rejected"
			trade.Error = "duplicate signal suppressed within dedup window"
			trade.ErrorCode = "duplicate_suppressed"
			log.Printf("worker: dedup skip webhook=%s signal_key=%s", wh.ID, signalKey)
			metrics.RecordDedupHit()
			q.persistTrade(ctx, trade, brokerTypeForWebhook(ctx, q.pg, wh))
			return
		}
	}

	// Persist immediately so every accepted ingest (202) has a matching trades
	// row even if broker execution fails later in this worker. This insert
	// must succeed before we place any real broker/paper order below —
	// otherwise a later FinalizeTrade UPDATE has no row to match (a Postgres
	// UPDATE on zero rows is not an error), so the trade silently vanishes
	// from the Trades tab and Summary counts while the broker order still
	// went through, with no audit row to show for it.
	if err := q.pg.InsertTrade(ctx, trade); err != nil {
		log.Printf("worker: ALERT queued trade insert failed webhook=%s id=%s — aborting before broker execution to avoid an unrecorded order: %v", wh.ID, trade.ID, err)
		trade.Status = "rejected"
		trade.ErrorCode = string(brokererr.CodeInternal)
		trade.Error = fmt.Sprintf("trade audit row could not be saved: %v", err)
		q.persistTrade(ctx, trade, brokerTypeForWebhook(ctx, q.pg, wh))
		return
	}
	if q.notifier != nil {
		q.notifier.OnTradeInserted(ctx, trade)
	}

	user, err := q.pg.GetUserByID(ctx, wh.UserID)
	brokerType := brokerTypeForWebhook(ctx, q.pg, wh)
	if err != nil || user == nil {
		q.rejectExistingTrade(ctx, trade, string(brokererr.CodeInternal), "user not found", brokerType)
		return
	}

	if err := compliance.WebhookMayTrade(user, nil); err != nil {
		q.rejectExistingTrade(ctx, trade, string(tradeCodeForCompliance(err)), err.Error(), brokerType)
		return
	}

	userPlan := user.Plan
	userLimit := int64(plan.EnforceOrdersPerSec(userPlan))
	if userLimit > 0 {
		count, err := q.redis.IncrRateLimit(ctx, wh.UserID)
		if err != nil {
			log.Printf("worker: rate-limit redis error: %v", err)
		} else if count > userLimit {
			q.rejectExistingTrade(ctx, trade, string(brokererr.CodeRateLimited),
				fmt.Sprintf("rate limit exceeded (%d req/s)", userLimit), brokerType)
			return
		}
	}

	if wh.PaperAccountID != nil {
		q.processPaperOrder(ctx, wh, sig, params, trade)
		return
	}
	if wh.BrokerCredID == nil {
		q.rejectExistingTrade(ctx, trade, string(brokererr.CodeInternal), "webhook has no execution destination", brokerType)
		return
	}

	req := domain.ExecutionRequest{
		AccountID:    *wh.BrokerCredID,
		WebhookID:    wh.ID,
		SignalID:     trade.ID,
		UserID:       user.ID,
		OrgID:        wh.OrgID,
		Symbol:       params.Symbol,
		Side:         sig.Action,
		OrderType:    guard.WebhookOrderType(wh),
		Product:      params.Product,
		Quantity:     params.Lot,
		Price:        params.Price,
		Comment:      sig.Comment,
		SLPoints:     params.SLPts,
		TPPoints:     params.TPPts,
		FixedLotSize: wh.LotSize,
		MaxRiskPct:   wh.MaxRiskPct,
		MaxLotSize:   wh.MaxLotSize,
	}

	result, err := q.orchestrator.ExecuteLive(ctx, req)
	if err != nil {
		brokerType := ""
		innerErr := err
		var ee *local.ExecutionError
		if errors.As(err, &ee) {
			brokerType = ee.BrokerType
			innerErr = ee.Err
		}
		var remote *execution.ExecutionError
		if errors.As(err, &remote) {
			brokerType = remote.BrokerType
			innerErr = remote.Err
		}
		code, message := brokererr.TradeFields(innerErr)
		trade.Status = "rejected"
		trade.Error = message
		trade.ErrorCode = code
		log.Printf("worker: order failed webhook=%s code=%s err=%v", wh.ID, code, err)
		q.finalizeTrade(ctx, trade, brokerType)
		return
	}

	trade.LotSize = result.Quantity
	if result.OrderType == domain.OrderBracket {
		trade.OrderType = "BRACKET"
	}
	trade.Status = result.Status
	trade.BrokerOrder = result.OrderID
	trade.FillPrice = result.FillPrice
	trade.AlgoID = result.AlgoID
	if result.SLPrice > 0 {
		trade.SLPrice = &result.SLPrice
	}
	if result.TPPrice > 0 {
		trade.TPPrice = &result.TPPrice
	}
	log.Printf("worker: %s webhook=%s order=%s symbol=%s action=%s",
		trade.Status, wh.ID, result.OrderID, params.Symbol, sig.Action)
	q.finalizeTrade(ctx, trade, result.BrokerType)
}

// resolveLotSize is a thin wrapper kept on *Queue for existing direct-call tests.
func (q *Queue) resolveLotSize(
	ctx context.Context,
	b domain.Broker,
	brokerType string,
	signalLot, fixedLot, maxRiskPct float64,
	slPts int,
) (float64, error) {
	return local.ResolveLot(ctx, b, brokerType, signalLot, fixedLot, maxRiskPct, slPts)
}
