package plan

import "github.com/SPSingh09/zettabridge/internal/store"

// EffectivePlan returns the plan used for limits, live access, and API display.
func EffectivePlan(billingPlan string) string {
	return billingPlan
}

// EffectivePlanForUser applies EffectivePlan to a user row.
func EffectivePlanForUser(user *store.User) string {
	if user == nil {
		return PlanFree
	}
	return EffectivePlan(user.Plan)
}
