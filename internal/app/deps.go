package app

import (
	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
	"github.com/SPSingh09/zettabridge/internal/platform/queue"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/transport/websocket"
)

// Deps holds the fully-initialized runtime dependencies.
// Exported so integration tests or future tooling can reference or inject them.
type Deps struct {
	Cfg    *config.Config
	PG     *store.PGStore
	Redis  *store.RedisStore
	Queue  *queue.Queue
	Trades *tradepush.Hub
	Infra  *livebrokers.Infra
}
