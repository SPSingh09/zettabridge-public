package livebrokers

import (
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/symboltoken"
	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers/httpclient"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// AngelHeaders are SmartAPI client metadata sent on every request.
// Use server egress IPs (not per-user) — Angel validates API key + JWT, not end-user IP.
type AngelHeaders struct {
	ClientLocalIP  string
	ClientPublicIP string
	MACAddress     string
}

// Infra is shared live-broker infrastructure (HTTP client + API base URLs).
type Infra struct {
	HTTP         *httpclient.Client
	URLs         URLs
	Angel        AngelHeaders
	SymbolTokens *symboltoken.Resolver
}

// NewInfra builds shared infrastructure from application config.
func NewInfra(cfg *config.Config, redis *store.RedisStore) *Infra {
	if cfg == nil {
		cfg = config.Load()
	}
	var cache symboltoken.TokenCache
	if redis != nil {
		cache = redis
	}
	return &Infra{
		HTTP: httpclient.New(cfg.BrokerHTTPTimeout(), cfg.HTTPGetRetries()),
		URLs: URLs{
			MT5Live:     cfg.BrokerMT5BaseURL,
			MT5Demo:     cfg.BrokerMT5DemoBaseURL,
			ZerodhaLive: cfg.BrokerZerodhaBaseURL,
			ZerodhaDemo: cfg.BrokerZerodhaDemoBaseURL,
			AngelLive:   cfg.BrokerAngelBaseURL,
			AngelDemo:   cfg.BrokerAngelDemoBaseURL,
			DhanLive:    cfg.BrokerDhanBaseURL,
			DhanDemo:    cfg.BrokerDhanDemoBaseURL,
		},
		Angel: AngelHeaders{
			ClientLocalIP:  cfg.BrokerAngelClientLocalIP,
			ClientPublicIP: cfg.BrokerAngelClientPublicIP,
			MACAddress:     cfg.BrokerAngelMACAddress,
		},
		SymbolTokens: symboltoken.NewResolver(cache),
	}
}

// DefaultInfra returns infrastructure with package defaults (tests).
func DefaultInfra() *Infra {
	return NewInfra(nil, nil)
}

type adapterBase struct {
	cred  *store.BrokerCredential
	infra *Infra
}

func (a *adapterBase) baseURL() string {
	if a.cred == nil || a.infra == nil {
		return ""
	}
	return a.infra.URLs.BaseURL(a.cred.BrokerType, a.cred.AccountMode)
}
