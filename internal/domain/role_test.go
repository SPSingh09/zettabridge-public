package domain

import "testing"

func TestIsValidPlan(t *testing.T) {
	if !IsValidPlan(PlanPro) || IsValidPlan("invalid") {
		t.Fatalf("PlanPro should be valid, invalid should not")
	}
}
