package brokererr

import (
	"errors"
	"testing"
)

func TestCodeOfTypedError(t *testing.T) {
	err := Wrap(CodeAuthFailed, PublicMessage(CodeAuthFailed), errors.New("http 401"))
	if CodeOf(err) != CodeAuthFailed {
		t.Fatalf("got %q", CodeOf(err))
	}
}

func TestPublicFromTypedError(t *testing.T) {
	err := New(CodeInsufficientMargin, PublicMessage(CodeInsufficientMargin))
	if PublicFrom(err) != "insufficient margin" {
		t.Fatalf("got %q", PublicFrom(err))
	}
}

func TestPublicFromGenericError(t *testing.T) {
	if PublicFrom(errors.New("boom")) != PublicMessage(CodeInternal) {
		t.Fatal("expected internal public message")
	}
}

func TestTradeFields(t *testing.T) {
	code, msg := TradeFields(New(CodeNoPosition, PublicMessage(CodeNoPosition)))
	if code != string(CodeNoPosition) || msg != "no open position to close" {
		t.Fatalf("got %q %q", code, msg)
	}
}
