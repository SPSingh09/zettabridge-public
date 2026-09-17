package httpadapter

import (
	"context"
	"fmt"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/execution"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

type credStore interface {
	GetBrokerCred(ctx context.Context, id string) (*store.BrokerCredential, error)
	GetUserByID(ctx context.Context, id string) (*store.User, error)
}

// LiveAdapter routes live execution to a remote adapter HTTP service.
type LiveAdapter struct {
	store   credStore
	router  *execution.Router
	clients map[string]*Client
}

type LiveDeps struct {
	Store   credStore
	Router  *execution.Router
	Clients map[string]*Client
}

func NewLive(deps LiveDeps) *LiveAdapter {
	return &LiveAdapter{
		store:   deps.Store,
		router:  deps.Router,
		clients: deps.Clients,
	}
}

func (l *LiveAdapter) Execute(ctx context.Context, req domain.ExecutionRequest) (*domain.ExecutionResult, error) {
	cred, err := l.store.GetBrokerCred(ctx, req.AccountID)
	if err != nil || cred == nil {
		return nil, &execution.ExecutionError{
			Err: brokererr.New(brokererr.CodeInvalidCredentials, "broker credentials not found"),
		}
	}
	brokerType := cred.BrokerType

	if l.router != nil {
		if err := l.router.RequireBroker(brokerType); err != nil {
			return nil, &execution.ExecutionError{
				Err:        brokererr.New(brokererr.CodeInternal, err.Error()),
				BrokerType: brokerType,
			}
		}
	}

	client := l.clients[brokerType]
	if client == nil {
		return nil, &execution.ExecutionError{
			Err:        fmt.Errorf("%w: %s", execution.ErrBrokerAdapterDisabled, brokerType),
			BrokerType: brokerType,
		}
	}

	user, err := l.store.GetUserByID(ctx, req.UserID)
	if err != nil || user == nil {
		return nil, &execution.ExecutionError{
			Err:        brokererr.New(brokererr.CodeInternal, "user not found"),
			BrokerType: brokerType,
		}
	}
	if !plan.LimitsFor(plan.EffectivePlan(user.Plan)).LiveAllowed {
		return nil, &execution.ExecutionError{
			Err:        plan.ErrLiveNotAllowed,
			BrokerType: brokerType,
		}
	}

	cmd := execution.BuildLiveCommand(req, user, cred, req.SignalID)
	out, err := client.PlaceOrder(ctx, cmd)
	if err != nil {
		return nil, &execution.ExecutionError{
			Err:        err,
			BrokerType: brokerType,
		}
	}
	return execution.OutcomeToResult(out)
}
