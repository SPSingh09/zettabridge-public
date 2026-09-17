package runtime

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"

	"github.com/SPSingh09/zettabridge/internal/adapters/paper"
	zerodhaoauth "github.com/SPSingh09/zettabridge/internal/adapters/zerodha/oauth"
	"github.com/SPSingh09/zettabridge/internal/algo"
	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/execution"
	"github.com/SPSingh09/zettabridge/internal/execution/adapter/local"
	"github.com/SPSingh09/zettabridge/internal/execution/adapter/server"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokerfactory"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
	"github.com/SPSingh09/zettabridge/internal/modules/publisher"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// Bootstrap holds shared adapter runtime dependencies.
type Bootstrap struct {
	Cfg   *config.Config
	PG    *store.PGStore
	Redis *store.RedisStore
	Infra *livebrokers.Infra
}

// LoadBootstrap connects stores for adapter sidecars (shared DB with Core).
func LoadBootstrap() (*Bootstrap, error) {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	pg := store.NewPostgres(cfg.DatabaseURL)
	redis := store.NewRedis(cfg.RedisURL)
	return &Bootstrap{
		Cfg:   cfg,
		PG:    pg,
		Redis: redis,
		Infra: livebrokers.NewInfra(cfg, redis),
	}, nil
}

// RunPaper starts the paper execution adapter HTTP server.
func RunPaper(b *Bootstrap) error {
	port := os.Getenv("ADAPTER_PORT")
	if port == "" {
		port = "8091"
	}
	engine := paper.New(b.PG)
	exec := &server.PaperExecutor{Paper: local.NewPaper(engine)}
	idempotency := &server.RedisIdempotency{Redis: b.Redis}
	app := server.NewApp(server.Config{
		Name:         execution.AdapterPaper,
		Port:         port,
		Executor:     exec,
		ServiceToken: b.Cfg.ZBServiceToken,
		Idempotency:  idempotency,
	})
	return app.Listen(":" + port)
}

// RunZerodha starts the Zerodha execution adapter with OAuth + publisher callbacks.
func RunZerodha(b *Bootstrap) error {
	port := os.Getenv("ADAPTER_PORT")
	if port == "" {
		port = "8092"
	}
	credKey, err := b.Cfg.AESKeyBytes()
	if err != nil {
		return err
	}

	router := execution.NewRouter([]string{execution.AdapterZerodha})
	live := local.NewLive(local.LiveDeps{
		Store:      b.PG,
		Redis:      b.Redis,
		CredKey:    credKey,
		BrokerMode: b.Cfg.BrokerMode,
		Infra:      b.Infra,
		NewBroker:  brokerfactory.New,
		AlgoPolicy: algo.FromConfig(b.Cfg),
		Router:     router,
		Linker:     b.PG,
	})
	if b.Cfg.ZerodhaPublisherEnabled {
		pubCallbackURL := b.Cfg.EffectiveZerodhaPublisherCallbackURL()
		if pubCallbackURL == "" {
			pubCallbackURL = b.Cfg.AdapterPublicURL() + "/v1/publisher/callback"
		}
		live.SetBrokerFactory(brokerfactory.NewFactory(livebrokers.PublisherDeps{
			Enabled:     true,
			JWTSecret:   b.Cfg.JWTSecret,
			CallbackURL: pubCallbackURL,
			Store:       b.PG,
		}))
	}

	exec := &server.LiveExecutor{Live: live}
	oauthDeps := zerodhaoauth.Deps{Cfg: b.Cfg, Store: b.PG}
	base := &shared.Handler{PG: b.PG, Redis: b.Redis, Cfg: b.Cfg}
	pubH := &publisher.Handler{Handler: base}

	app := server.NewApp(server.Config{
		Name:         execution.AdapterZerodha,
		Port:         port,
		Executor:     exec,
		ServiceToken: b.Cfg.ZBServiceToken,
		Idempotency:  &server.RedisIdempotency{Redis: b.Redis},
		MountPublic: func(app *fiber.App) {
			app.Get("/v1/oauth/callback", func(c *fiber.Ctx) error {
				return zerodhaoauth.HandleCallback(c, oauthDeps)
			})
			app.Get("/v1/credentials/zerodha/callback", func(c *fiber.Ctx) error {
				return zerodhaoauth.HandleCallback(c, oauthDeps)
			})
			app.Get("/v1/publisher/callback", pubH.Callback)
		},
	})

	log.Printf("zerodha adapter listening on :%s", port)
	return app.Listen(":" + port)
}
