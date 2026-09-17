package brokerfactory

import (
	"errors"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestResolveExecutionProUsesLive(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "zerodha", AccountMode: plan.AccountLive}
	r, err := ResolveExecution(ModeLive, plan.PlanPro, cred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Mode != ModeLive {
		t.Fatalf("unexpected route: %+v", r)
	}
}

func TestResolveExecutionFreeRejected(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "zerodha", AccountMode: plan.AccountLive}
	_, err := ResolveExecution(ModeLive, plan.PlanFree, cred)
	if !errors.Is(err, plan.ErrLiveNotAllowed) {
		t.Fatalf("expected ErrLiveNotAllowed, got %v", err)
	}
}

func TestResolveExecutionMockServerModeUsesSimulated(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "zerodha", AccountMode: plan.AccountLive}
	r, err := ResolveExecution(ModeMock, plan.PlanPro, cred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Mode != ModeMock {
		t.Fatalf("unexpected route: %+v", r)
	}
}

func TestResolveExecutionUnsupportedServerModeRejected(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "zerodha", AccountMode: plan.AccountLive}
	_, err := ResolveExecution("invalid", plan.PlanPro, cred)
	if !errors.Is(err, plan.ErrLiveNotAllowed) {
		t.Fatalf("expected ErrLiveNotAllowed, got %v", err)
	}
}

func TestCredentialForExecutionUnchanged(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "mt5_cloud", AccountMode: plan.AccountLive}
	r := ExecutionRoute{Mode: ModeLive}
	out := r.CredentialForExecution(cred)
	if out.AccountMode != plan.AccountLive {
		t.Fatalf("account mode = %q want live", out.AccountMode)
	}
}
