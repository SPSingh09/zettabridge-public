package plan

import (
	"errors"
	"testing"
)

func TestMaxWebhooksPro(t *testing.T) {
	if got := MaxWebhooks(PlanPro); got != 9 {
		t.Fatalf("max webhooks = %d want 9", got)
	}
}

func TestMaxBrokersPro(t *testing.T) {
	if got := MaxBrokers(PlanPro); got != 1 {
		t.Fatalf("max brokers = %d want 1", got)
	}
}

func TestCanAddBrokerAtCap(t *testing.T) {
	if err := CanAddBroker(1, 1); !errors.Is(err, ErrBrokerLimitReached) {
		t.Fatalf("expected ErrBrokerLimitReached, got %v", err)
	}
}

func TestCanAddWebhookAtCap(t *testing.T) {
	if err := CanAddWebhook(5, 5); !errors.Is(err, ErrWebhookLimitReached) {
		t.Fatalf("expected ErrWebhookLimitReached, got %v", err)
	}
}

func TestMaxWebhooksFree(t *testing.T) {
	if got := MaxWebhooks(PlanFree); got != 1 {
		t.Fatalf("max webhooks free = %d want 1", got)
	}
}

func TestMaxPaperWebhooksPaper(t *testing.T) {
	if got := MaxPaperWebhooks(PlanPaper); got != 5 {
		t.Fatalf("max paper webhooks = %d want 5", got)
	}
}
