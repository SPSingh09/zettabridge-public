package app

import (
	"context"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberlogger "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"golang.org/x/crypto/bcrypt"

	zb "github.com/SPSingh09/zettabridge"
	"github.com/SPSingh09/zettabridge/internal/algo"
	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/execution"
	"github.com/SPSingh09/zettabridge/internal/handler"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokerfactory"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
	fyersrefresh "github.com/SPSingh09/zettabridge/internal/integrations/brokers/fyers/refresh"
	zerodharefresh "github.com/SPSingh09/zettabridge/internal/integrations/brokers/zerodha/refresh"
	zerodhafillpoll "github.com/SPSingh09/zettabridge/internal/integrations/brokers/zerodha/fillpoll"
	"github.com/SPSingh09/zettabridge/internal/integrations/telegram"
	"github.com/SPSingh09/zettabridge/internal/marketdata"
	"github.com/SPSingh09/zettabridge/internal/marketdata/fyers"
	"github.com/SPSingh09/zettabridge/internal/platform/mailer"
	credenc "github.com/SPSingh09/zettabridge/internal/platform/crypto"
	_ "github.com/SPSingh09/zettabridge/internal/platform/metrics" // register Prometheus collectors
	"github.com/SPSingh09/zettabridge/internal/platform/migrate"
	"github.com/SPSingh09/zettabridge/internal/modules/publisher"
	"github.com/SPSingh09/zettabridge/internal/paperengine"
	"github.com/SPSingh09/zettabridge/internal/paperengine/missquareoff"
	"github.com/SPSingh09/zettabridge/internal/platform/queue"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/transport/websocket"
)

// App is the fully-wired server. Call Run() to block until shutdown.
type App struct {
	fiber    *fiber.App
	deps     Deps
	bgCancel context.CancelFunc
}

// New wires up all dependencies and returns a ready-to-run App.
// Returns (nil, nil) when MIGRATE_ONLY=true — the caller should exit cleanly.
func New(cfg *config.Config) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	credKey, err := cfg.AESKeyBytes()
	if err != nil {
		return nil, err
	}
	if credenc.IsWeakKey(credKey) {
		log.Printf("warning: AES_KEY is weak; set a random 32-byte hex key (openssl rand -hex 32) before production")
	}

	log.Printf("startup: broker_mode=%s workers=%d queue_buffer=%d execution_adapter_mode=%s", cfg.BrokerMode, cfg.WorkerCount, cfg.QueueBuffer, cfg.ExecutionAdapterMode)
	if os.Getenv("APP_ENV") == "production" && cfg.BrokerMode == config.BrokerModeMock {
		log.Printf("warning: APP_ENV=production but BROKER_MODE=mock — orders will not hit live brokers")
	}
	if cfg.BrokerMode == config.BrokerModeLive {
		log.Printf("startup: angel egress local_ip=%s public_ip=%s mac=%s",
			cfg.BrokerAngelClientLocalIP, cfg.BrokerAngelClientPublicIP, cfg.BrokerAngelMACAddress)
		log.Printf("startup: sebi_algo_id_required=%v", cfg.SebiAlgoIDRequired)
	}

	// ── Stores ────────────────────────────────────────────────────────────────
	redisStore := store.NewRedis(cfg.RedisURL)
	pgStore := store.NewPostgres(cfg.DatabaseURL)

	migrationsFS, err := fs.Sub(zb.MigrationsFS, "migrations")
	if err != nil {
		pgStore.Close()
		return nil, err
	}
	if os.Getenv("RESET_SCHEMA") == "true" {
		if err := migrate.WipeSchema(pgStore.DB()); err != nil {
			pgStore.Close()
			return nil, err
		}
	}
	if err := migrate.RunAll(pgStore.DB(), migrationsFS); err != nil {
		pgStore.Close()
		return nil, err
	}
	if os.Getenv("MIGRATE_ONLY") == "true" {
		log.Printf("MIGRATE_ONLY=true — exiting after migrations")
		pgStore.Close()
		return nil, nil
	}

	// ── Bootstrap admin ───────────────────────────────────────────────────────
	if err := bootstrapAdmin(cfg, pgStore); err != nil {
		pgStore.Close()
		return nil, err
	}

	// ── Shared live-broker HTTP infrastructure ────────────────────────────────
	infra := livebrokers.NewInfra(cfg, redisStore)

	mdFallback := cfg.MarketDataProvider
	if mdFallback != marketdata.ProviderFyers {
		mdFallback = marketdata.ProviderSignal
	}
	fyersProvider := fyers.New(cfg.FyersAppID)
	mdRegistry := marketdata.NewRegistry(mdFallback, map[string]marketdata.Provider{
		marketdata.ProviderSignal: marketdata.SignalPriceProvider{},
		marketdata.ProviderFyers:  fyersProvider,
	})

	// ── Worker queue ──────────────────────────────────────────────────────────
	q := queue.New(cfg.WorkerCount, cfg.QueueBuffer, pgStore, redisStore, credKey, cfg.BrokerMode, infra, algo.FromConfig(cfg), cfg.EnabledAdapters, execution.TransportFromConfig(cfg))
	q.SetMarketDataRegistry(mdRegistry)
	if cfg.ZerodhaPublisherEnabled {
		pubCallbackURL := cfg.EffectiveZerodhaPublisherCallbackURL()
		if pubCallbackURL == "" {
			pubCallbackURL = cfg.AppPublicURL + "/v1/publisher/callback"
		}
		q.SetBrokerFactory(brokerfactory.NewFactory(livebrokers.PublisherDeps{
			Enabled:     true,
			JWTSecret:   cfg.JWTSecret,
			CallbackURL: pubCallbackURL,
			Store:       pgStore,
		}))
	}
	tradeHub := tradepush.NewHub(pgStore)
	q.SetTradeNotifier(tradeHub)
	q.SetTelegramSender(telegram.New(cfg.TelegramBotToken))
	q.SetMailer(mailer.New(cfg))
	q.SetBillingUpgradeURL(cfg.FrontendURL + "/billing")
	// Paper engine: use in-process engine whenever execution is local, or when
	// HTTP mode is configured without a paper sidecar URL (cohosted paper).
	transport := execution.TransportFromConfig(cfg)
	var localPaperEngine *paperengine.Engine
	if !transport.IsHTTP() || cfg.EffectivePaperAdapterURL() == "" {
		localPaperEngine = paperengine.New(pgStore)
		q.SetPaperEngine(localPaperEngine)
	} else {
		log.Printf("startup: paper execution routed to sidecar at %s", cfg.EffectivePaperAdapterURL())
	}
	q.Start()

	deps := Deps{
		Cfg:    cfg,
		PG:     pgStore,
		Redis:  redisStore,
		Queue:  q,
		Trades: tradeHub,
		Infra:  infra,
	}

	// ── Fiber app ─────────────────────────────────────────────────────────────
	fiberApp := fiber.New(fiber.Config{
		AppName:               "ZettaBridge v1",
		ReadTimeout:           5 * time.Second,
		WriteTimeout:          10 * time.Second,
		IdleTimeout:           30 * time.Second,
		DisableStartupMessage: false,
		ErrorHandler:          handler.ErrorHandler,
	})
	fiberApp.Use(recover.New())
	fiberApp.Use(fiberlogger.New(fiberlogger.Config{
		Format: "[${time}] ${status} ${latency} ${method} ${path}\n",
	}))
	fiberApp.Use(cors.New(cors.Config{
		AllowOrigins: cfg.CORSOrigins,
		AllowHeaders: "Origin, Content-Type, Authorization",
		AllowMethods: "GET, POST, PUT, PATCH, DELETE",
	}))

	h := handler.New(pgStore, redisStore, q, cfg, infra, tradeHub)
	h.Register(fiberApp)

	// ── Background services ───────────────────────────────────────────────────
	bgCtx, bgCancel := context.WithCancel(context.Background())
	if cfg.ZerodhaAPIKey != "" {
		reconnectURL := cfg.ZerodhaFrontendURL
		if reconnectURL == "" {
			reconnectURL = cfg.AppPublicURL
		}
		reconnectURL += "/dashboard"
		zerodharefresh.New(pgStore, mailer.New(cfg), telegram.New(cfg.TelegramBotToken), reconnectURL).Start(bgCtx)
		log.Printf("startup: zerodha refresh notifier started (fires at 05:50 IST daily)")
	}

	snapshotInterval := time.Duration(cfg.MarketDataSnapshotIntervalSec) * time.Second
	snapshotJob := marketdata.New(pgStore, mdRegistry, snapshotInterval)
	snapshotJob.SetMarkNotifier(tradeHub)
	snapshotJob.Start(bgCtx)
	log.Printf("startup: paper trading snapshot job started (default_provider=%s interval=%s, admin-configurable at runtime)", mdFallback, snapshotInterval)

	if localPaperEngine != nil {
		missquareoff.New(pgStore, localPaperEngine, mdRegistry).Start(bgCtx)
		log.Printf("startup: paper MIS auto square-off job started (15:15–15:30 IST weekdays)")
	}

	if cfg.FyersAppID != "" && cfg.FyersSecretID != "" {
		fyersReconnectURL := cfg.FrontendURL + "/admin/settings"
		fyersRefreshInterval := time.Duration(cfg.FyersRefreshInterval) * time.Minute
		fyersrefresh.New(pgStore, fyersProvider, cfg.FyersAppID, cfg.FyersSecretID, credKey,
			mailer.New(cfg), telegram.New(cfg.TelegramBotToken), fyersRefreshInterval, fyersReconnectURL).Start(bgCtx)
		log.Printf("startup: fyers refresh job started (interval=%s)", fyersRefreshInterval)
	}

	if cfg.ZerodhaFillPollIntervalSec > 0 {
		fillInterval := time.Duration(cfg.ZerodhaFillPollIntervalSec) * time.Second
		zerodhafillpoll.New(pgStore, infra, credKey, tradeHub, fillInterval).Start(bgCtx)
		log.Printf("startup: zerodha fill poll job started (interval=%s)", fillInterval)
	}

	if cfg.ZerodhaPublisherEnabled {
		publisher.NewExpiryJob(pgStore, tradeHub).Start(bgCtx)
		log.Printf("startup: publisher handoff expiry job started (interval=1m)")
	}

	return &App{
		fiber:    fiberApp,
		deps:     deps,
		bgCancel: bgCancel,
	}, nil
}

// Run blocks until an OS interrupt/SIGTERM, then shuts down gracefully.
func (a *App) Run() {
	defer a.deps.PG.Close()
	defer a.deps.Queue.Stop()
	defer a.bgCancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Server listening on :%s", a.deps.Cfg.Port)
		if err := a.fiber.Listen(":" + a.deps.Cfg.Port); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down gracefully…")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.fiber.ShutdownWithContext(ctx); err != nil {
		log.Fatalf("Forced shutdown: %v", err)
	}
	log.Println("Bye.")
}

func bootstrapAdmin(cfg *config.Config, pg *store.PGStore) error {
	if cfg.BootstrapAdminEmail == "" {
		return nil
	}
	if cfg.BootstrapAdminPassword != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(cfg.BootstrapAdminPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if err := pg.CreateOrPromoteBootstrapAdmin(context.Background(), cfg.BootstrapAdminEmail, string(hash)); err != nil {
			log.Printf("bootstrap admin warning: %v", err)
		} else {
			log.Printf("bootstrap admin: created or promoted %s", cfg.BootstrapAdminEmail)
		}
	} else {
		if err := pg.PromoteBootstrapAdmin(context.Background(), cfg.BootstrapAdminEmail); err != nil {
			log.Printf("bootstrap admin warning: %v", err)
		} else {
			log.Printf("bootstrap admin: promoted %s (if account exists)", cfg.BootstrapAdminEmail)
		}
	}
	if err := pg.VerifyUserEmailByAddress(context.Background(), cfg.BootstrapAdminEmail); err != nil {
		log.Printf("bootstrap admin email verify warning: %v", err)
	}
	return nil
}
