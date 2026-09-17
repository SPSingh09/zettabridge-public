package handler

import (
	"testing"

	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestWebhookAccessibleSoloOnly(t *testing.T) {
	scope := store.ResourceScope{UserID: "u1", InOrg: false}
	wh := &store.Webhook{UserID: "u1"}
	if !webhookAccessible(scope, wh) {
		t.Fatal("expected owner to access webhook")
	}
	other := &store.Webhook{UserID: "u2"}
	if webhookAccessible(scope, other) {
		t.Fatal("expected other user's webhook to be inaccessible")
	}
}

func TestCredentialAccessibleSoloOnly(t *testing.T) {
	scope := store.ResourceScope{UserID: "u1", InOrg: false}
	cred := &store.BrokerCredential{UserID: "u1"}
	if !credentialAccessible(scope, cred) {
		t.Fatal("expected owner to access credential")
	}
	other := &store.BrokerCredential{UserID: "u2"}
	if credentialAccessible(scope, other) {
		t.Fatal("expected other user's credential to be inaccessible")
	}
}
