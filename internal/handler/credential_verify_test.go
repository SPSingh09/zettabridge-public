package handler

import (
	"strings"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
)

func TestCredentialVerifyHintAuthFailed(t *testing.T) {
	err := brokererr.New(brokererr.CodeAuthFailed, "authentication failed")
	hint := credentialVerifyHint("zerodha", err)
	if hint == "" {
		t.Fatal("expected zerodha hint")
	}
	hint = credentialVerifyHint("dhan", err)
	if hint == "" {
		t.Fatal("expected dhan hint")
	}
}

func TestCredentialVerifyFailureIncludesErrorCode(t *testing.T) {
	err := brokererr.New(brokererr.CodeAuthFailed, "authentication failed")
	out := credentialVerifyFailure("zerodha", "authentication failed", err)
	if out["error_code"] != "auth_failed" {
		t.Fatalf("error_code=%v", out["error_code"])
	}
	if out["hint"] == "" {
		t.Fatal("expected hint")
	}
}

func TestCredentialVerifyHintFormat(t *testing.T) {
	out := credentialVerifyFailure("angel", "invalid format (expected api_key:client_code:jwt)", nil)
	if out["hint"] != "Use raw_creds format api_key:client_code:jwt" {
		t.Fatalf("hint=%v", out["hint"])
	}
}

func TestCredentialVerifyHintUnreachable(t *testing.T) {
	err := brokererr.New(brokererr.CodeBrokerUnreachable, "broker temporarily unavailable")
	hint := credentialVerifyHint("mt5_cloud", err)
	if hint == "" {
		t.Fatal("expected unreachable hint")
	}
}

func TestCredentialVerifyFailureLiveAuthIncludesHint(t *testing.T) {
	err := brokererr.New(brokererr.CodeAuthFailed, brokererr.PublicMessage(brokererr.CodeAuthFailed))
	out := credentialVerifyFailure("zerodha", brokererr.PublicFrom(err), err)
	if out["valid"] != false {
		t.Fatalf("valid=%v", out["valid"])
	}
	if out["error_code"] != "auth_failed" {
		t.Fatalf("error_code=%v", out["error_code"])
	}
	hint, ok := out["hint"].(string)
	if !ok || hint == "" {
		t.Fatalf("expected zerodha auth hint, got %#v", out["hint"])
	}
	if !strings.Contains(hint, "raw_creds") {
		t.Fatalf("hint should mention raw_creds: %q", hint)
	}
}
