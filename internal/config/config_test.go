package config

import (
	"os"
	"testing"
)

func TestAESKeyBytes(t *testing.T) {
	c := &Config{AESKey: "a1b2c3d4e5f60718293a4b5c6d7e8f90112233445566778899aabbccddeeff00"}
	key, err := c.AESKeyBytes()
	if err != nil || len(key) != 32 {
		t.Fatalf("AESKeyBytes: err=%v len=%d", err, len(key))
	}
}

func TestValidateProductionRejectsWeakKey(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	c := &Config{AESKey: "0000000000000000000000000000000000000000000000000000000000000000", BrokerMode: BrokerModeMock}
	if err := c.Validate(); err == nil {
		t.Fatal("expected weak key rejection in production")
	}
}

func TestValidateDevAllowsWeakKey(t *testing.T) {
	os.Unsetenv("APP_ENV")
	c := &Config{AESKey: "0000000000000000000000000000000000000000000000000000000000000000", BrokerMode: BrokerModeMock}
	if err := c.Validate(); err != nil {
		t.Fatalf("dev should allow weak key: %v", err)
	}
}

func TestValidateRejectsInvalidBrokerMode(t *testing.T) {
	c := &Config{
		AESKey:     "a1b2c3d4e5f60718293a4b5c6d7e8f90112233445566778899aabbccddeeff00",
		BrokerMode: "paper",
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected invalid BROKER_MODE rejection")
	}
}

func TestValidateAcceptsBrokerModeCaseInsensitive(t *testing.T) {
	c := &Config{
		AESKey:     "a1b2c3d4e5f60718293a4b5c6d7e8f90112233445566778899aabbccddeeff00",
		BrokerMode: " LIVE ",
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected LIVE accepted: %v", err)
	}
	if c.BrokerMode != BrokerModeLive {
		t.Fatalf("BrokerMode normalized to %q", c.BrokerMode)
	}
}

func TestValidateBillingEnabledRequiresStripeConfig(t *testing.T) {
	c := &Config{
		AESKey:         "a1b2c3d4e5f60718293a4b5c6d7e8f90112233445566778899aabbccddeeff00",
		BrokerMode:     BrokerModeMock,
		BillingEnabled: true,
		AppPublicURL:   "https://api.example.com",
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected billing config validation error")
	}
}

func TestValidateBillingEnabledWithoutPriceIDs(t *testing.T) {
	c := &Config{
		AESKey:              "a1b2c3d4e5f60718293a4b5c6d7e8f90112233445566778899aabbccddeeff00",
		BrokerMode:          BrokerModeMock,
		BillingEnabled:      true,
		StripeSecretKey:     "sk_test",
		StripeWebhookSecret: "whsec_test",
		AppPublicURL:        "https://api.example.com",
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("billing should not require stripe price IDs at startup: %v", err)
	}
}

func TestValidateBillingDisabledSkipsStripe(t *testing.T) {
	c := &Config{
		AESKey:         "a1b2c3d4e5f60718293a4b5c6d7e8f90112233445566778899aabbccddeeff00",
		BrokerMode:     BrokerModeMock,
		BillingEnabled: false,
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateExecutionAdaptersRequiresZerodhaURL(t *testing.T) {
	c := &Config{
		AESKey:               "a1b2c3d4e5f60718293a4b5c6d7e8f90112233445566778899aabbccddeeff00",
		BrokerMode:           BrokerModeMock,
		ExecutionAdapterMode: "http",
		EnabledAdapters:      []string{"paper", "zerodha"},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected missing ZERODHA_ADAPTER_URL error")
	}
	c.ZerodhaAdapterURL = "http://zerodha-adapter:8092"
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestZerodhaOAuthCallbackURLUsesAdapterInHTTPMode(t *testing.T) {
	c := &Config{
		AppPublicURL:         "http://localhost:3000",
		ExecutionAdapterMode: "http",
		ZerodhaAdapterURL:      "http://localhost:8092",
	}
	if got := c.ZerodhaOAuthCallbackURL(); got != "http://localhost:8092/v1/credentials/zerodha/callback" {
		t.Fatalf("got %q", got)
	}
}

func TestEffectiveDashboardURLPrefersZerodhaFrontendOnAdapter(t *testing.T) {
	c := &Config{
		ZerodhaFrontendURL: "https://staging.zettabridge.net",
		FrontendURL:        "http://localhost:3000",
		AppPublicURL:       "https://api.staging.zettabridge.net",
	}
	if got := c.EffectiveDashboardURL(); got != "https://staging.zettabridge.net" {
		t.Fatalf("got %q", got)
	}
}
