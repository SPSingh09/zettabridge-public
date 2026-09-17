package algo

import (
	"strings"
	"unicode"

	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokerfactory"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

const (
	maxTagLenDefault = 20 // Zerodha, Angel (Kite/SmartAPI limit)
	maxTagLenDhan    = 30 // Dhan correlationId limit
)

// Policy controls SEBI algo ID resolution and enforcement.
type Policy struct {
	Required        bool
	FallbackZerodha string
	FallbackAngel   string
	FallbackDhan    string
}

// FromConfig builds enforcement policy from application config.
func FromConfig(cfg *config.Config) Policy {
	if cfg == nil {
		return Policy{}
	}
	return Policy{
		Required:        cfg.SebiAlgoIDRequired,
		FallbackZerodha: cfg.BrokerZerodhaAlgoID,
		FallbackAngel:   cfg.BrokerAngelAlgoID,
		FallbackDhan:    cfg.BrokerDhanAlgoID,
	}
}

// IsIndianBroker reports whether broker_type is an NSE/BSE equity adapter.
func IsIndianBroker(brokerType string) bool {
	switch strings.ToLower(strings.TrimSpace(brokerType)) {
	case "zerodha", "angel", "dhan":
		return true
	default:
		return false
	}
}

// IsPublisherCredential reports whether cred uses Zerodha Kite Publisher handoff
// (browser basket) rather than direct OAuth API placement.
func IsPublisherCredential(cred *store.BrokerCredential) bool {
	if cred == nil {
		return false
	}
	return domain.ExecutionMode(cred.ExecutionMode) == domain.ExecutionModePublisher
}

// NeedsLiveTag reports whether an outbound order hits a live Indian broker API.
func NeedsLiveTag(route brokerfactory.ExecutionRoute, brokerType string) bool {
	if route.Mode != brokerfactory.ModeLive {
		return false
	}
	return IsIndianBroker(brokerType)
}

// ResolveForCredential validates algo ID when saving a live Indian credential.
func ResolveForCredential(cred *store.BrokerCredential, p Policy) (string, error) {
	if cred == nil || !IsIndianBroker(cred.BrokerType) {
		return "", nil
	}
	if IsPublisherCredential(cred) {
		return "", nil
	}
	if !strings.EqualFold(strings.TrimSpace(cred.AccountMode), plan.AccountLive) {
		return "", nil
	}
	return resolve(cred, p, p.Required)
}

// ResolveForExecution returns the algo ID to attach to a live Indian order.
func ResolveForExecution(cred *store.BrokerCredential, route brokerfactory.ExecutionRoute, p Policy) (string, error) {
	if cred == nil || !NeedsLiveTag(route, cred.BrokerType) {
		return "", nil
	}
	if IsPublisherCredential(cred) {
		return "", nil
	}
	required := p.Required
	return resolve(cred, p, required)
}

func resolve(cred *store.BrokerCredential, p Policy, required bool) (string, error) {
	id := strings.TrimSpace(cred.AlgoID)
	if id == "" {
		id = strings.TrimSpace(fallbackFor(cred.BrokerType, p))
	}
	if id == "" {
		if required {
			return "", brokererr.New(brokererr.CodeAlgoIDRequired, brokererr.PublicMessage(brokererr.CodeAlgoIDRequired))
		}
		return "", nil
	}
	if err := validateForBroker(id, cred.BrokerType); err != nil {
		return "", err
	}
	return id, nil
}

func brokerMaxTagLen(brokerType string) int {
	if strings.EqualFold(strings.TrimSpace(brokerType), "dhan") {
		return maxTagLenDhan
	}
	return maxTagLenDefault
}

func validateForBroker(id, brokerType string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	if len(id) > brokerMaxTagLen(brokerType) {
		return brokererr.New(brokererr.CodeInvalidCredentials, "algo_id exceeds maximum length for this broker")
	}
	for _, r := range id {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return brokererr.New(brokererr.CodeInvalidCredentials, "algo_id must be alphanumeric")
		}
	}
	return nil
}

func fallbackFor(brokerType string, p Policy) string {
	switch strings.ToLower(strings.TrimSpace(brokerType)) {
	case "zerodha":
		return p.FallbackZerodha
	case "angel":
		return p.FallbackAngel
	case "dhan":
		return p.FallbackDhan
	default:
		return ""
	}
}

// Validate checks broker tag length (20-char conservative limit) and charset.
// Use validateForBroker internally when the broker type is known.
func Validate(id string) error {
	return validateForBroker(id, "")
}
