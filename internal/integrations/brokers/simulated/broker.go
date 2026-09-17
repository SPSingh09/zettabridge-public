package simulated

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/store"
)

var orderSeq atomic.Uint64

// Broker is an in-process simulated broker for BROKER_MODE=mock (CI/dev only).
type Broker struct {
	cred   *store.BrokerCredential
	parsed brokercreds.Parsed
}

func NewFromCredential(cred *store.BrokerCredential, parsed brokercreds.Parsed) (domain.Broker, error) {
	if cred == nil {
		return nil, fmt.Errorf("credential required")
	}
	return &Broker{cred: cred, parsed: parsed}, nil
}

func (b *Broker) PlaceOrder(_ context.Context, req *domain.PlaceRequest) (*domain.OrderResult, error) {
	if req == nil {
		return nil, fmt.Errorf("place request required")
	}
	id := fmt.Sprintf("mock-%s-%d", b.cred.BrokerType, orderSeq.Add(1))
	return &domain.OrderResult{
		Status:  domain.StatusSubmitted,
		OrderID: id,
	}, nil
}

func (b *Broker) CancelOrder(_ context.Context, _ *domain.CancelRequest) error {
	return nil
}

func (b *Broker) GetAccountEquity(_ context.Context) (float64, error) {
	return 100_000, nil
}
