package queue

import (
	"context"

	httpadapter "github.com/SPSingh09/zettabridge/internal/execution/adapter/http"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/execution"
	"github.com/SPSingh09/zettabridge/internal/execution/adapter/local"
	"github.com/SPSingh09/zettabridge/internal/store"
)

type orchestratorStore interface {
	GetBrokerCred(ctx context.Context, id string) (*store.BrokerCredential, error)
	GetUserByID(ctx context.Context, id string) (*store.User, error)
}

func buildOrchestrator(
	transport execution.TransportConfig,
	enabledAdapters []string,
	liveDeps local.LiveDeps,
	paperEngine domain.ExecutionDestination,
	store orchestratorStore,
) *execution.Orchestrator {
	router := execution.NewRouter(enabledAdapters)
	if liveDeps.Router == nil {
		liveDeps.Router = router
	}

	var live execution.LiveRunner = local.NewLive(liveDeps)
	var paper execution.PaperRunner = local.NewPaper(paperEngine)

	if transport.IsHTTP() {
		clients := map[string]*httpadapter.Client{}
		for broker, url := range transport.BrokerURLs {
			if url == "" {
				continue
			}
			clients[broker] = httpadapter.NewClient(url, transport.ServiceToken, transport.HTTPTimeout)
		}
		live = httpadapter.NewLive(httpadapter.LiveDeps{
			Store:   store,
			Router:  router,
			Clients: clients,
		})
		if transport.PaperURL != "" {
			paper = httpadapter.NewPaper(httpadapter.PaperDeps{
				Store:  store,
				Client: httpadapter.NewClient(transport.PaperURL, transport.ServiceToken, transport.HTTPTimeout),
			})
		}
	}

	return execution.NewOrchestrator(execution.OrchestratorDeps{
		Live:   live,
		Paper:  paper,
		Router: router,
	})
}
