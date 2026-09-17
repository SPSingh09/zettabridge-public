package algo

import (
	"testing"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokerfactory"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestResolveForCredentialUsesCredThenFallback(t *testing.T) {
	p := Policy{Required: true, FallbackZerodha: "FALLBACK123"}
	cred := &store.BrokerCredential{
		BrokerType:  "zerodha",
		AccountMode: plan.AccountLive,
		AlgoID:      "CRED123",
	}
	id, err := ResolveForCredential(cred, p)
	if err != nil || id != "CRED123" {
		t.Fatalf("cred id: got %q err=%v", id, err)
	}

	cred.AlgoID = ""
	id, err = ResolveForCredential(cred, p)
	if err != nil || id != "FALLBACK123" {
		t.Fatalf("fallback: got %q err=%v", id, err)
	}
}

func TestResolveForCredentialRequiredMissing(t *testing.T) {
	p := Policy{Required: true}
	cred := &store.BrokerCredential{
		BrokerType:  "angel",
		AccountMode: plan.AccountLive,
	}
	_, err := ResolveForCredential(cred, p)
	if brokererr.CodeOf(err) != brokererr.CodeAlgoIDRequired {
		t.Fatalf("got %v", err)
	}
}

func TestResolveForCredentialSkipsMT5(t *testing.T) {
	p := Policy{Required: true}
	cred := &store.BrokerCredential{BrokerType: "mt5_cloud", AccountMode: plan.AccountLive}
	id, err := ResolveForCredential(cred, p)
	if err != nil || id != "" {
		t.Fatalf("cred=%+v id=%q err=%v", cred, id, err)
	}
}

func TestResolveForExecutionNeedsLiveRoute(t *testing.T) {
	p := Policy{Required: true, FallbackZerodha: "TAG1"}
	cred := &store.BrokerCredential{BrokerType: "zerodha", AccountMode: plan.AccountLive}

	emptyRoute := brokerfactory.ExecutionRoute{Mode: ""}
	id, err := ResolveForExecution(cred, emptyRoute, p)
	if err != nil || id != "" {
		t.Fatalf("non-live route: id=%q err=%v", id, err)
	}

	liveRoute := brokerfactory.ExecutionRoute{Mode: brokerfactory.ModeLive}
	id, err = ResolveForExecution(cred, liveRoute, p)
	if err != nil || id != "TAG1" {
		t.Fatalf("live route: id=%q err=%v", id, err)
	}
}

func TestResolveForExecutionSkipsPublisher(t *testing.T) {
	p := Policy{Required: true}
	cred := &store.BrokerCredential{
		BrokerType:    "zerodha",
		AccountMode:   plan.AccountLive,
		ExecutionMode: string(domain.ExecutionModePublisher),
	}
	liveRoute := brokerfactory.ExecutionRoute{Mode: brokerfactory.ModeLive}
	id, err := ResolveForExecution(cred, liveRoute, p)
	if err != nil || id != "" {
		t.Fatalf("publisher live route: id=%q err=%v", id, err)
	}
}

func TestResolveForCredentialSkipsPublisher(t *testing.T) {
	p := Policy{Required: true}
	cred := &store.BrokerCredential{
		BrokerType:    "zerodha",
		AccountMode:   plan.AccountLive,
		ExecutionMode: string(domain.ExecutionModePublisher),
	}
	id, err := ResolveForCredential(cred, p)
	if err != nil || id != "" {
		t.Fatalf("publisher credential: id=%q err=%v", id, err)
	}
}

func TestValidateRejectsInvalid(t *testing.T) {
	if Validate("ok123") != nil {
		t.Fatal("expected ok")
	}
	if Validate("toolongtagvalue123456") == nil {
		t.Fatal("expected length error")
	}
	if Validate("bad-tag") == nil {
		t.Fatal("expected charset error")
	}
}

func TestDhanLiveAlgoIDResolvesOK(t *testing.T) {
	p := Policy{Required: true, FallbackDhan: "FALLBACKDHAN1"}
	cred := &store.BrokerCredential{
		BrokerType:  "dhan",
		AccountMode: plan.AccountLive,
		AlgoID:      "SEBIID1234",
	}
	id, err := ResolveForCredential(cred, p)
	if err != nil || id != "SEBIID1234" {
		t.Fatalf("cred id: got %q err=%v", id, err)
	}

	cred.AlgoID = ""
	id, err = ResolveForCredential(cred, p)
	if err != nil || id != "FALLBACKDHAN1" {
		t.Fatalf("fallback: got %q err=%v", id, err)
	}
}

func TestDhanLiveAlgoIDRequired(t *testing.T) {
	p := Policy{Required: true}
	cred := &store.BrokerCredential{
		BrokerType:  "dhan",
		AccountMode: plan.AccountLive,
	}
	_, err := ResolveForCredential(cred, p)
	if brokererr.CodeOf(err) != brokererr.CodeAlgoIDRequired {
		t.Fatalf("expected algo_id_required, got %v", err)
	}
}

func TestValidateDhanAllows30Chars(t *testing.T) {
	id30 := "ABCDEFGHIJ1234567890ABCDEFGHIJ" // 30 chars
	if err := validateForBroker(id30, "dhan"); err != nil {
		t.Fatalf("30-char Dhan ID should be valid: %v", err)
	}
	id31 := id30 + "X"
	if validateForBroker(id31, "dhan") == nil {
		t.Fatal("31-char Dhan ID should be rejected")
	}
}

func TestValidateZerodhaRejects21Chars(t *testing.T) {
	id21 := "ABCDEFGHIJ12345678901" // 21 chars
	if validateForBroker(id21, "zerodha") == nil {
		t.Fatal("21-char Zerodha ID should be rejected")
	}
	id20 := id21[:20]
	if err := validateForBroker(id20, "zerodha"); err != nil {
		t.Fatalf("20-char Zerodha ID should be valid: %v", err)
	}
}
