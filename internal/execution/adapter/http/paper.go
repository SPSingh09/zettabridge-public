package httpadapter

import (
	"context"
	"fmt"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/execution"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/store"
)

type paperUserStore interface {
	GetUserByID(ctx context.Context, id string) (*store.User, error)
}

type PaperAdapter struct {
	store  paperUserStore
	client *Client
}

type PaperDeps struct {
	Store  paperUserStore
	Client *Client
}

func NewPaper(deps PaperDeps) *PaperAdapter {
	return &PaperAdapter{store: deps.Store, client: deps.Client}
}

func (p *PaperAdapter) Execute(ctx context.Context, req domain.ExecutionRequest) (*domain.ExecutionResult, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("paper adapter URL not configured")
	}
	user, err := p.store.GetUserByID(ctx, req.UserID)
	if err != nil || user == nil {
		return nil, brokererr.New(brokererr.CodeInternal, "user not found")
	}
	cmd := execution.BuildPaperCommand(req, user, req.SignalID)
	out, err := p.client.PlaceOrder(ctx, cmd)
	if err != nil {
		if _, ok := err.(*brokererr.Error); ok {
			return nil, err
		}
		return nil, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	return execution.OutcomeToResult(out)
}
