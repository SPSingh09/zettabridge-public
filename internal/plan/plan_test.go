package plan

import (
	"errors"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestLimitsForFree(t *testing.T) {
	limits := LimitsFor(PlanFree)
	if limits.MaxPaperWebhooks != 1 || limits.MaxLiveWebhooks != 0 || limits.MaxBrokers != 0 ||
		limits.LiveAllowed || limits.OrdersPerSec != 1 || limits.MaxPaperTradesPerMonth != 10 {
		t.Fatalf("unexpected free limits: %+v", limits)
	}
}

func TestLimitsForPaper(t *testing.T) {
	limits := LimitsFor(PlanPaper)
	if limits.MaxPaperAccounts != 3 || limits.MaxPaperWebhooks != 5 || limits.MaxLiveWebhooks != 0 ||
		limits.LiveAllowed || limits.MaxPaperTradesPerMonth != 0 {
		t.Fatalf("unexpected paper limits: %+v", limits)
	}
}

func TestLimitsForPro(t *testing.T) {
	limits := LimitsFor(PlanPro)
	if limits.MaxPaperAccounts != 5 || limits.MaxPaperWebhooks != 8 || limits.MaxLiveWebhooks != 1 ||
		limits.MaxBrokers != 1 || !limits.LiveAllowed || limits.OrdersPerSec != 10 {
		t.Fatalf("unexpected pro limits: %+v", limits)
	}
}

func TestLimitsForProPlus(t *testing.T) {
	limits := LimitsFor(PlanProPlus)
	if limits.MaxPaperAccounts != 10 || limits.MaxPaperWebhooks != 15 || limits.MaxLiveWebhooks != 5 ||
		limits.MaxBrokers != 3 || !limits.LiveAllowed {
		t.Fatalf("unexpected pro_plus limits: %+v", limits)
	}
}

func TestCanUseCredentialBlocksLiveOnFree(t *testing.T) {
	cred := &store.BrokerCredential{AccountMode: AccountLive}
	err := CanUseCredential(PlanFree, cred)
	if !errors.Is(err, ErrLiveNotAllowed) {
		t.Fatalf("expected ErrLiveNotAllowed, got %v", err)
	}
}

func TestNormalizeAccountModeRejectsLiveOnFree(t *testing.T) {
	_, err := NormalizeAccountMode(PlanFree, AccountLive)
	if !errors.Is(err, ErrLiveNotAllowed) {
		t.Fatalf("expected ErrLiveNotAllowed, got %v", err)
	}
}

func TestNormalizeAccountModeDefaultsLive(t *testing.T) {
	mode, err := NormalizeAccountMode(PlanPro, "")
	if err != nil || mode != AccountLive {
		t.Fatalf("expected live default for pro, got %q err=%v", mode, err)
	}
}

func TestCanAddWebhookEnforcesCap(t *testing.T) {
	err := CanAddWebhook(5, 5)
	if !errors.Is(err, ErrWebhookLimitReached) {
		t.Fatalf("expected ErrWebhookLimitReached, got %v", err)
	}
}

func TestCanConfigureAdvancedWebhookGuards(t *testing.T) {
	if err := CanConfigureAdvancedWebhookGuards(PlanFree); !errors.Is(err, ErrAdvancedWebhookGuardsNotAllowed) {
		t.Fatalf("expected ErrAdvancedWebhookGuardsNotAllowed, got %v", err)
	}
	if err := CanConfigureAdvancedWebhookGuards(PlanPaper); err != nil {
		t.Fatalf("expected nil for paper, got %v", err)
	}
}

func TestCanPlacePaperTrade(t *testing.T) {
	if err := CanPlacePaperTrade(PlanFree, 9); err != nil {
		t.Fatalf("expected under quota, got %v", err)
	}
	if err := CanPlacePaperTrade(PlanFree, 10); !errors.Is(err, ErrPaperTradeQuotaExceeded) {
		t.Fatalf("expected quota exceeded, got %v", err)
	}
	if err := CanPlacePaperTrade(PlanPaper, 1000); err != nil {
		t.Fatalf("paper plan should be unlimited, got %v", err)
	}
}

func TestEnforceOrdersPerSec(t *testing.T) {
	tests := []struct {
		plan string
		want int
	}{
		{PlanFree, 1},
		{PlanPaper, 5},
		{PlanPro, PerUserOrdersPerSecCap},
		{PlanProPlus, PerUserOrdersPerSecCap},
	}
	for _, tc := range tests {
		if got := EnforceOrdersPerSec(tc.plan); got != tc.want {
			t.Fatalf("EnforceOrdersPerSec(%q) = %d want %d", tc.plan, got, tc.want)
		}
	}
}

func TestIsValidPlan(t *testing.T) {
	for _, p := range ValidPlans {
		if !IsValidPlan(p) {
			t.Fatalf("expected valid plan %q", p)
		}
	}
	if IsValidPlan("unknown") {
		t.Fatal("expected unknown plan to be invalid")
	}
}

func TestUnknownPlanFallsBackToFree(t *testing.T) {
	limits := LimitsFor("unknown")
	if limits.MaxPaperWebhooks != 1 || limits.OrdersPerSec != 1 {
		t.Fatalf("unknown plan should fall back to free limits, got %+v", limits)
	}
}

func TestCanAddPaperAccount(t *testing.T) {
	if err := CanAddPaperAccount(PlanFree, 0); err != nil {
		t.Fatalf("expected allow first account, got %v", err)
	}
	if err := CanAddPaperAccount(PlanFree, 1); !errors.Is(err, ErrPaperAccountLimitReached) {
		t.Fatalf("expected limit, got %v", err)
	}
}
