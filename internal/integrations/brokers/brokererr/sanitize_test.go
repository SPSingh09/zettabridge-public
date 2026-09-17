package brokererr

import (
	"fmt"
	"strings"
	"testing"
)

func TestSanitizeStripsInternalURLs(t *testing.T) {
	raw := `adapter request failed: Post "http://localhost:8091/v1/orders": dial tcp 127.0.0.1:8091: connect: connection refused`
	got := Sanitize(raw)
	if got == raw {
		t.Fatalf("expected sanitized message, got %q", got)
	}
	for _, forbidden := range []string{"localhost", "127.0.0.1", "8091", "http://", "/v1/orders"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized message still contains %q: %q", forbidden, got)
		}
	}
}

func TestTradeFieldsAdapterConnectionError(t *testing.T) {
	err := fmt.Errorf(`adapter request failed: Post "http://localhost:8091/v1/orders": dial tcp 127.0.0.1:8091: connect: connection refused`)
	code, msg := TradeFields(err)
	if code != string(CodeBrokerUnreachable) {
		t.Fatalf("code=%q", code)
	}
	if msg != PublicMessage(CodeBrokerUnreachable) {
		t.Fatalf("msg=%q", msg)
	}
}

func TestPublicFromTypedErrorWithUnsafeMessage(t *testing.T) {
	err := New(CodeBrokerUnreachable, `Post "http://internal:8091/v1/orders" failed`)
	got := PublicFrom(err)
	if strings.Contains(got, "http://") || strings.Contains(got, "8091") {
		t.Fatalf("got %q", got)
	}
}
