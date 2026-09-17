package brokerfactory

import (
	"fmt"
	"strings"

	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/simulated"
	"github.com/SPSingh09/zettabridge/internal/store"
)

const (
	ModeLive = "live"
)

// New returns a broker adapter for live or simulated (mock) execution using pre-parsed credentials.
// Publisher mode is disabled — use NewFactory when publisher support is needed.
func New(mode string, infra *livebrokers.Infra, cred *store.BrokerCredential, parsed brokercreds.Parsed) (domain.Broker, error) {
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case ModeMock:
		return simulated.NewFromCredential(cred, parsed)
	case ModeLive:
		b, err := livebrokers.NewWithParsed(cred, parsed, infra, nil)
		if err != nil {
			return nil, fmt.Errorf("live broker adapter: %w", err)
		}
		return b, nil
	default:
		return nil, fmt.Errorf("broker execution requires BROKER_MODE=live or mock")
	}
}

// NewFactory returns a broker factory function with publisher support baked in via closure.
func NewFactory(pub livebrokers.PublisherDeps) func(string, *livebrokers.Infra, *store.BrokerCredential, brokercreds.Parsed) (domain.Broker, error) {
	return func(mode string, infra *livebrokers.Infra, cred *store.BrokerCredential, parsed brokercreds.Parsed) (domain.Broker, error) {
		m := strings.ToLower(strings.TrimSpace(mode))
		switch m {
		case ModeMock:
			return simulated.NewFromCredential(cred, parsed)
		case ModeLive:
			b, err := livebrokers.NewWithParsed(cred, parsed, infra, &pub)
			if err != nil {
				return nil, fmt.Errorf("live broker adapter: %w", err)
			}
			return b, nil
		default:
			return nil, fmt.Errorf("broker execution requires BROKER_MODE=live or mock")
		}
	}
}
