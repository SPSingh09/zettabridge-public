package plan

import "testing"

func TestEffectivePlan(t *testing.T) {
	if got := EffectivePlan(PlanFree); got != PlanFree {
		t.Fatalf("free = %q", got)
	}
	if got := EffectivePlan(PlanPro); got != PlanPro {
		t.Fatalf("pro = %q", got)
	}
}

func TestOrderRateLimitsFree(t *testing.T) {
	display, enforced, credCap := OrderRateLimits(PlanFree)
	if display == nil || *display != 1 {
		t.Fatalf("expected display=1 for free, got %v", display)
	}
	if enforced != 1 {
		t.Fatalf("enforced = %d want 1", enforced)
	}
	if credCap != BrokerCredOrdersPerSecCap {
		t.Fatalf("credCap = %d", credCap)
	}
}
