package execution

import (
	"strings"
	"time"

	"github.com/SPSingh09/zettabridge/internal/config"
)

const (
	TransportLocal = "local"
	TransportHTTP  = "http"
)

// TransportConfig selects in-process vs HTTP adapter execution.
type TransportConfig struct {
	Mode         string
	ServiceToken string
	PaperURL     string
	BrokerURLs   map[string]string // broker_type → adapter base URL
	HTTPTimeout  time.Duration
}

// TransportFromConfig builds transport settings from application config.
func TransportFromConfig(cfg *config.Config) TransportConfig {
	if cfg == nil {
		return TransportConfig{Mode: TransportLocal}
	}
	urls := map[string]string{
		"zerodha":   strings.TrimRight(cfg.ZerodhaAdapterURL, "/"),
		"angel":     strings.TrimRight(cfg.AngelAdapterURL, "/"),
		"dhan":      strings.TrimRight(cfg.DhanAdapterURL, "/"),
		"mt5_cloud": strings.TrimRight(cfg.MT5AdapterURL, "/"),
	}
	return TransportConfig{
		Mode:         strings.ToLower(strings.TrimSpace(cfg.ExecutionAdapterMode)),
		ServiceToken: cfg.ZBServiceToken,
		PaperURL:     cfg.EffectivePaperAdapterURL(),
		BrokerURLs:   urls,
		HTTPTimeout:  cfg.BrokerHTTPTimeout(),
	}
}

func (t TransportConfig) IsHTTP() bool {
	return strings.EqualFold(t.Mode, TransportHTTP)
}
