package livebrokers

import (
	"fmt"

	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/zerodha/publisher"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// PublisherDeps carries Kite Publisher configuration for the factory.
// Set Enabled=false (or leave zero) to keep the existing API OAuth path.
type PublisherDeps struct {
	Enabled     bool
	JWTSecret   string
	CallbackURL string
	Store       publisher.PublisherStore
}

// New parses decrypted credentials and returns a live broker adapter.
func New(cred *store.BrokerCredential) (domain.Broker, error) {
	parsed, err := brokercreds.Parse(cred.BrokerType, cred.EncryptedCreds)
	if err != nil {
		return nil, fmt.Errorf("live broker creds: %w", err)
	}
	return NewWithParsed(cred, parsed, nil, nil)
}

// NewWithParsed returns a live adapter using pre-validated credentials from the worker.
// pub may be nil — in that case publisher mode credentials return an error.
func NewWithParsed(cred *store.BrokerCredential, parsed brokercreds.Parsed, infra *Infra, pub *PublisherDeps) (domain.Broker, error) {
	if infra == nil {
		infra = DefaultInfra()
	}
	base := adapterBase{cred: cred, infra: infra}

	switch cred.BrokerType {
	case "mt5_cloud":
		return &MT5Broker{adapterBase: base, parsed: parsed}, nil

	case "zerodha":
		switch domain.ExecutionMode(cred.ExecutionMode) {
		case domain.ExecutionModePublisher:
			if pub == nil || !pub.Enabled {
				return nil, fmt.Errorf("Zerodha Publisher mode is not enabled on this server")
			}
			if pub.Store == nil {
				return nil, fmt.Errorf("Zerodha Publisher mode requires a publisher store")
			}
			return publisher.NewExecutor(cred, parsed, pub.Store, pub.JWTSecret, pub.CallbackURL), nil

		case domain.ExecutionModeUserAPIOAuth, domain.ExecutionModeDirectAPI, "":
			// empty string: backfill safety — treat same as user_api_oauth
			return &ZerodhaBroker{adapterBase: base, parsed: parsed}, nil

		default:
			return nil, fmt.Errorf("unsupported execution_mode %q for zerodha", cred.ExecutionMode)
		}

	case "angel":
		return &AngelBroker{adapterBase: base, parsed: parsed}, nil
	case "dhan":
		return &DhanBroker{adapterBase: base, parsed: parsed}, nil
	default:
		return nil, fmt.Errorf("unsupported broker_type %q", cred.BrokerType)
	}
}

