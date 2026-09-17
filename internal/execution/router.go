package execution

import (
	"fmt"
	"strings"
)

const (
	AdapterPaper   = "paper"
	AdapterZerodha = "zerodha"
	AdapterAngel   = "angel"
	AdapterDhan    = "dhan"
	AdapterMT5     = "mt5"
)

var brokerToAdapter = map[string]string{
	"zerodha":   AdapterZerodha,
	"angel":     AdapterAngel,
	"dhan":      AdapterDhan,
	"mt5_cloud": AdapterMT5,
}

// Router gates live execution and credential create to deployed adapters.
type Router struct {
	enabled map[string]struct{}
}

func NewRouter(enabledAdapters []string) *Router {
	set := make(map[string]struct{}, len(enabledAdapters))
	for _, name := range enabledAdapters {
		set[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	return &Router{enabled: set}
}

func (r *Router) IsEnabled(adapter string) bool {
	if r == nil {
		return true
	}
	_, ok := r.enabled[strings.ToLower(strings.TrimSpace(adapter))]
	return ok
}

func (r *Router) IsPaperEnabled() bool {
	return r.IsEnabled(AdapterPaper)
}

func (r *Router) IsBrokerEnabled(brokerType string) bool {
	adapter, ok := brokerToAdapter[brokerType]
	if !ok {
		return false
	}
	return r.IsEnabled(adapter)
}

func (r *Router) EnabledBrokers() []string {
	out := make([]string, 0, 4)
	for broker, adapter := range brokerToAdapter {
		if r.IsEnabled(adapter) {
			out = append(out, broker)
		}
	}
	return out
}

func (r *Router) EnabledAdapters() []string {
	names := []string{AdapterPaper, AdapterZerodha, AdapterAngel, AdapterDhan, AdapterMT5}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if r.IsEnabled(name) {
			out = append(out, name)
		}
	}
	return out
}

var ErrBrokerAdapterDisabled = fmt.Errorf("broker adapter is not enabled on this deployment")

// DefaultEnabledAdapters returns all in-process adapters for local dev and CI.
func DefaultEnabledAdapters() []string {
	return []string{AdapterPaper, AdapterZerodha, AdapterAngel, AdapterDhan, AdapterMT5}
}

func (r *Router) RequireBroker(brokerType string) error {
	if r.IsBrokerEnabled(brokerType) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrBrokerAdapterDisabled, brokerType)
}
