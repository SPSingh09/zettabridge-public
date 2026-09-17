package plan

import (
	"errors"

	"github.com/SPSingh09/zettabridge/internal/store"
)

const (
	PlanFree    = "free"
	PlanPaper   = "paper"
	PlanPro     = "pro"
	PlanProPlus = "pro_plus"

	AccountLive = "live"

	// PerUserOrdersPerSecCap is the enforced per-user order rate for paid live plans.
	PerUserOrdersPerSecCap = 10
)

// ValidPlans lists the only supported subscription plans.
var ValidPlans = []string{PlanFree, PlanPaper, PlanPro, PlanProPlus}

// Limits defines per-plan resource caps.
type Limits struct {
	MaxPaperAccounts       int
	MaxPaperWebhooks       int
	MaxLiveWebhooks        int
	MaxBrokers             int // live broker credentials
	MaxPaperTradesPerMonth int // 0 = unlimited
	LiveAllowed            bool
	OrdersPerSec           int
	AuditLogs              bool
	AdvancedWebhookGuards  bool
	Notifications          bool
	MultiProductCredentials bool // Indian broker: MIS + CNC + NRML on one credential (Pro Plus)
}

var (
	ErrLiveNotAllowed                  = errors.New("live trading is not included in your plan")
	ErrWebhookLimitReached             = errors.New("webhook limit reached for your plan")
	ErrBrokerLimitReached              = errors.New("broker limit reached for your plan")
	ErrAuditLogsNotAllowed             = errors.New("audit logs require a paid plan")
	ErrAdvancedWebhookGuardsNotAllowed = errors.New("advanced webhook guards require a paid plan")
	ErrNotificationsNotAllowed         = errors.New("notifications require a paid plan")
	ErrInvalidAccountMode              = errors.New("account_mode must be live")
	ErrPaperAccountLimitReached        = errors.New("paper account limit reached")
	ErrPaperTradeQuotaExceeded         = errors.New("monthly paper trade limit reached")
)

// IsValidPlan reports whether name is a supported plan.
func IsValidPlan(name string) bool {
	switch name {
	case PlanFree, PlanPaper, PlanPro, PlanProPlus:
		return true
	default:
		return false
	}
}

// LimitsFor returns the resource caps for a plan name.
// Unknown plans fall back to free-tier limits.
func LimitsFor(plan string) Limits {
	switch plan {
	case PlanFree:
		return Limits{
			MaxPaperAccounts:       1,
			MaxPaperWebhooks:       1,
			MaxLiveWebhooks:        0,
			MaxBrokers:             0,
			MaxPaperTradesPerMonth: 10,
			LiveAllowed:            false,
			OrdersPerSec:           1,
			AuditLogs:              false,
			AdvancedWebhookGuards:  false,
			Notifications:          false,
		}
	case PlanPaper:
		return Limits{
			MaxPaperAccounts:       3,
			MaxPaperWebhooks:       5,
			MaxLiveWebhooks:        0,
			MaxBrokers:             0,
			MaxPaperTradesPerMonth: 0,
			LiveAllowed:            false,
			OrdersPerSec:           5,
			AuditLogs:              true,
			AdvancedWebhookGuards:  true,
			Notifications:          true,
		}
	case PlanPro:
		return Limits{
			MaxPaperAccounts:       5,
			MaxPaperWebhooks:       8,
			MaxLiveWebhooks:        1,
			MaxBrokers:             1,
			MaxPaperTradesPerMonth: 0,
			LiveAllowed:            true,
			OrdersPerSec:           10,
			AuditLogs:              true,
			AdvancedWebhookGuards:  true,
			Notifications:          true,
		}
	case PlanProPlus:
		return Limits{
			MaxPaperAccounts:        10,
			MaxPaperWebhooks:        15,
			MaxLiveWebhooks:         5,
			MaxBrokers:              3,
			MaxPaperTradesPerMonth:  0,
			LiveAllowed:             true,
			OrdersPerSec:            10,
			AuditLogs:               true,
			AdvancedWebhookGuards:   true,
			Notifications:           true,
			MultiProductCredentials: true,
		}
	default:
		return LimitsFor(PlanFree)
	}
}

func IsValidAccountMode(mode string) bool {
	return mode == AccountLive
}

// NormalizeAccountMode applies plan rules to the requested account mode.
func NormalizeAccountMode(planName, requested string) (string, error) {
	mode := requested
	if mode == "" {
		mode = AccountLive
	}
	if !IsValidAccountMode(mode) {
		return "", ErrInvalidAccountMode
	}
	if !LimitsFor(planName).LiveAllowed {
		return "", ErrLiveNotAllowed
	}
	return mode, nil
}

// CanUseCredential checks whether a plan may use the given broker credential.
func CanUseCredential(planName string, cred *store.BrokerCredential) error {
	if cred == nil {
		return errors.New("broker credential not found")
	}
	mode := cred.AccountMode
	if mode == "" {
		mode = AccountLive
	}
	if mode == AccountLive && !LimitsFor(planName).LiveAllowed {
		return ErrLiveNotAllowed
	}
	return nil
}

// CanViewAuditLogs checks whether a plan may access trade history.
func CanViewAuditLogs(planName string) error {
	if !LimitsFor(planName).AuditLogs {
		return ErrAuditLogsNotAllowed
	}
	return nil
}

// CanReceiveNotifications checks whether a plan may receive in-app notifications
// and Telegram trade alerts.
func CanReceiveNotifications(planName string) error {
	if !LimitsFor(planName).Notifications {
		return ErrNotificationsNotAllowed
	}
	return nil
}

// PaperTradesUnlimited reports whether the plan has no monthly paper trade cap.
func PaperTradesUnlimited(planName string) bool {
	return LimitsFor(planName).MaxPaperTradesPerMonth == 0
}

// CanPlacePaperTrade checks the monthly paper trade quota (0 used count skips check for unlimited plans).
func CanPlacePaperTrade(planName string, usedThisMonth int) error {
	limits := LimitsFor(planName)
	if limits.MaxPaperTradesPerMonth == 0 {
		return nil
	}
	if usedThisMonth >= limits.MaxPaperTradesPerMonth {
		return ErrPaperTradeQuotaExceeded
	}
	return nil
}

// OrdersPerSec returns the marketed plan order rate.
func OrdersPerSec(planName string) int {
	return LimitsFor(planName).OrdersPerSec
}

// EnforceOrdersPerSec returns the worker per-user order rate cap.
func EnforceOrdersPerSec(userPlan string) int {
	switch userPlan {
	case PlanFree:
		return 1
	case PlanPaper:
		return 5
	case PlanPro, PlanProPlus:
		return PerUserOrdersPerSecCap
	default:
		return 1
	}
}

// OrderRateLimits returns plan marketing, enforced per-user cap, and per-broker-credential cap.
func OrderRateLimits(userPlan string) (display *int, enforced int, brokerCredCap int) {
	brokerCredCap = BrokerCredOrdersPerSecCap
	enforced = EnforceOrdersPerSec(userPlan)
	v := LimitsFor(userPlan).OrdersPerSec
	return &v, enforced, brokerCredCap
}

// CanConfigureAdvancedWebhookGuards checks paid access for phase-2 guard settings on live webhooks.
func CanConfigureAdvancedWebhookGuards(planName string) error {
	if LimitsFor(planName).AdvancedWebhookGuards {
		return nil
	}
	return ErrAdvancedWebhookGuardsNotAllowed
}
