package brokerfactory

import (
	"strings"

	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

const ModeMock = config.BrokerModeMock

// ExecutionRoute selects adapter mode for outbound broker orders.
type ExecutionRoute struct {
	Mode string // live | mock
}

func serverAllowsExecution(serverMode string) bool {
	switch strings.ToLower(strings.TrimSpace(serverMode)) {
	case ModeLive, ModeMock:
		return true
	default:
		return false
	}
}

// ResolveExecution picks broker adapter mode for an outbound order.
// Returns an error when server BROKER_MODE is unsupported or the user plan does not allow live trading.
func ResolveExecution(serverMode string, userPlan string, cred *store.BrokerCredential) (ExecutionRoute, error) {
	if !serverAllowsExecution(serverMode) {
		return ExecutionRoute{}, plan.ErrLiveNotAllowed
	}
	if cred == nil {
		return ExecutionRoute{}, plan.ErrLiveNotAllowed
	}
	if !plan.LimitsFor(userPlan).LiveAllowed {
		return ExecutionRoute{}, plan.ErrLiveNotAllowed
	}
	mode := ModeLive
	if strings.EqualFold(strings.TrimSpace(serverMode), ModeMock) {
		mode = ModeMock
	}
	return ExecutionRoute{Mode: mode}, nil
}

// CredentialForExecution returns cred unchanged (account_mode is always live).
func (r ExecutionRoute) CredentialForExecution(cred *store.BrokerCredential) *store.BrokerCredential {
	return cred
}
