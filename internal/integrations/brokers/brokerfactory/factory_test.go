package brokerfactory

import (
	"context"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/zerodha/publisher"
	"github.com/SPSingh09/zettabridge/internal/store"
)

type mockPubStore struct{}

func (m *mockPubStore) InsertPublisherOrder(_ context.Context, _ *store.PublisherOrder) error {
	return nil
}
func (m *mockPubStore) InsertPublisherOrderEvent(_ context.Context, _ *store.PublisherOrderEvent) error {
	return nil
}

var _ publisher.PublisherStore = (*mockPubStore)(nil)

func TestNewMockModeReturnsSimulated(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "zerodha", AccountMode: "live"}
	parsed := brokercreds.Parsed{BrokerType: "zerodha", APIKey: "k", AccessToken: "t"}

	b, err := New(ModeMock, nil, cred, parsed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil broker")
	}
	eq, err := b.GetAccountEquity(context.Background())
	if err != nil || eq <= 0 {
		t.Fatalf("GetAccountEquity: eq=%v err=%v", eq, err)
	}
}

func TestNewUnsupportedModeRejected(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "zerodha"}
	parsed := brokercreds.Parsed{BrokerType: "zerodha", APIKey: "k", AccessToken: "t"}

	_, err := New("invalid", nil, cred, parsed)
	if err == nil {
		t.Fatal("expected error for unsupported BROKER_MODE")
	}
}

func TestNewLiveModeUnsupportedFails(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "unknown"}
	parsed := brokercreds.Parsed{BrokerType: "unknown"}

	_, err := New(ModeLive, nil, cred, parsed)
	if err == nil {
		t.Fatal("expected error for unsupported live broker")
	}
}

func TestNewFactory_ZerodhaAPIOAuth_LiveMode(t *testing.T) {
	cred := &store.BrokerCredential{
		BrokerType:    "zerodha",
		ExecutionMode: "user_api_oauth",
		AccountMode:   "live",
	}
	parsed := brokercreds.Parsed{BrokerType: "zerodha", APIKey: "k", APISecret: "s", AccessToken: "t"}

	f := NewFactory(livebrokers.PublisherDeps{Enabled: true, JWTSecret: "sec", Store: &mockPubStore{}})
	b, err := f(ModeLive, nil, cred, parsed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil broker")
	}
}

func TestNewFactory_ZerodhaEmptyMode_LiveMode(t *testing.T) {
	cred := &store.BrokerCredential{
		BrokerType:    "zerodha",
		ExecutionMode: "",
		AccountMode:   "live",
	}
	parsed := brokercreds.Parsed{BrokerType: "zerodha", APIKey: "k", APISecret: "s", AccessToken: "t"}

	f := NewFactory(livebrokers.PublisherDeps{Enabled: true, JWTSecret: "sec", Store: &mockPubStore{}})
	b, err := f(ModeLive, nil, cred, parsed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil broker")
	}
}

func TestNewFactory_ZerodhaPublisher_Enabled(t *testing.T) {
	cred := &store.BrokerCredential{
		BrokerType:    "zerodha",
		ExecutionMode: "publisher",
		AccountMode:   "live",
		Exchange:      "NSE",
		Product:       "MIS",
	}
	parsed := brokercreds.Parsed{BrokerType: "zerodha", APIKey: "myapikey"}

	f := NewFactory(livebrokers.PublisherDeps{
		Enabled:     true,
		JWTSecret:   "secret",
		CallbackURL: "https://api.example.com/v1/publisher/callback",
		Store:       &mockPubStore{},
	})
	b, err := f(ModeLive, nil, cred, parsed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil publisher executor")
	}
	if _, ok := b.(domain.Broker); !ok {
		t.Fatal("publisher executor must implement domain.Broker")
	}
}

func TestNewFactory_ZerodhaPublisher_Disabled(t *testing.T) {
	cred := &store.BrokerCredential{
		BrokerType:    "zerodha",
		ExecutionMode: "publisher",
	}
	parsed := brokercreds.Parsed{BrokerType: "zerodha", APIKey: "myapikey"}

	f := NewFactory(livebrokers.PublisherDeps{Enabled: false})
	_, err := f(ModeLive, nil, cred, parsed)
	if err == nil {
		t.Fatal("expected error when publisher is disabled")
	}
}

func TestNewFactory_MockMode(t *testing.T) {
	cred := &store.BrokerCredential{
		BrokerType:    "zerodha",
		ExecutionMode: "publisher",
	}
	parsed := brokercreds.Parsed{BrokerType: "zerodha", APIKey: "myapikey"}

	f := NewFactory(livebrokers.PublisherDeps{Enabled: true, Store: &mockPubStore{}})
	b, err := f(ModeMock, nil, cred, parsed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil simulated broker")
	}
}

func TestNewFactory_UnsupportedModeRejected(t *testing.T) {
	cred := &store.BrokerCredential{
		BrokerType:    "zerodha",
		ExecutionMode: "publisher",
	}
	parsed := brokercreds.Parsed{BrokerType: "zerodha", APIKey: "myapikey"}

	f := NewFactory(livebrokers.PublisherDeps{Enabled: true, Store: &mockPubStore{}})
	_, err := f("invalid", nil, cred, parsed)
	if err == nil {
		t.Fatal("expected error when mode is unsupported")
	}
}

func TestNewFactory_ZerodhaInvalidMode(t *testing.T) {
	cred := &store.BrokerCredential{
		BrokerType:    "zerodha",
		ExecutionMode: "invalid_mode",
	}
	parsed := brokercreds.Parsed{BrokerType: "zerodha", APIKey: "k"}

	f := NewFactory(livebrokers.PublisherDeps{Enabled: true, Store: &mockPubStore{}})
	_, err := f(ModeLive, nil, cred, parsed)
	if err == nil {
		t.Fatal("expected error for unsupported execution_mode")
	}
}

func TestNew_ZerodhaPublisher_AlwaysDisabled(t *testing.T) {
	cred := &store.BrokerCredential{
		BrokerType:    "zerodha",
		ExecutionMode: "publisher",
	}
	parsed := brokercreds.Parsed{BrokerType: "zerodha", APIKey: "myapikey"}

	_, err := New(ModeLive, nil, cred, parsed)
	if err == nil {
		t.Fatal("expected error: New() must not support publisher mode")
	}
}
