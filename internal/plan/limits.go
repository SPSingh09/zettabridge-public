package plan

import "fmt"

const (
	// BrokerCredOrdersPerSecCap is the max orders/sec per broker credential (shared by all webhooks on that cred).
	BrokerCredOrdersPerSecCap = 10
)

// MaxWebhooks returns combined paper + live webhook caps (legacy display helper).
func MaxWebhooks(effectivePlan string) int {
	l := LimitsFor(effectivePlan)
	return l.MaxPaperWebhooks + l.MaxLiveWebhooks
}

// MaxPaperWebhooks returns the paper webhook cap for the effective plan.
func MaxPaperWebhooks(effectivePlan string) int {
	return LimitsFor(effectivePlan).MaxPaperWebhooks
}

// MaxLiveWebhooks returns the live webhook cap for the effective plan.
func MaxLiveWebhooks(effectivePlan string) int {
	return LimitsFor(effectivePlan).MaxLiveWebhooks
}

// MaxBrokers returns the live broker credential cap for the effective plan.
func MaxBrokers(effectivePlan string) int {
	return LimitsFor(effectivePlan).MaxBrokers
}

// MaxPaperAccounts returns the paper account cap for the effective plan.
func MaxPaperAccounts(effectivePlan string) int {
	return LimitsFor(effectivePlan).MaxPaperAccounts
}

// CanAddWebhook checks whether another webhook may be created under the computed cap.
func CanAddWebhook(maxAllowed, currentCount int) error {
	if currentCount >= maxAllowed {
		return fmt.Errorf("%w (max %d)", ErrWebhookLimitReached, maxAllowed)
	}
	return nil
}

// CanAddBroker checks whether another broker credential may be created under the computed cap.
func CanAddBroker(maxAllowed, currentCount int) error {
	if currentCount >= maxAllowed {
		return fmt.Errorf("%w (max %d)", ErrBrokerLimitReached, maxAllowed)
	}
	return nil
}

// CanAddPaperAccount checks whether another paper account may be created under the plan cap.
func CanAddPaperAccount(planName string, currentCount int) error {
	max := MaxPaperAccounts(planName)
	if currentCount >= max {
		return fmt.Errorf("%w (max %d)", ErrPaperAccountLimitReached, max)
	}
	return nil
}
