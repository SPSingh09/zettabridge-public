package compliance

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"errors"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestUserMayTradeActive(t *testing.T) {
	if err := UserMayTrade(&store.User{Status: domain.StatusActive}); err != nil {
		t.Fatalf("active user: %v", err)
	}
	if err := UserMayTrade(&store.User{}); err != nil {
		t.Fatalf("empty status treated as active: %v", err)
	}
}

func TestUserMayTradeSuspended(t *testing.T) {
	err := UserMayTrade(&store.User{Status: domain.StatusSuspended})
	if !errors.Is(err, ErrAccountSuspended) {
		t.Fatalf("want ErrAccountSuspended, got %v", err)
	}
}

func TestOrgMayTradeSuspended(t *testing.T) {
	err := WebhookMayTrade(
		&store.User{Status: domain.StatusActive},
		&store.Organization{Status: domain.StatusSuspended},
	)
	if !errors.Is(err, ErrOrgSuspended) {
		t.Fatalf("want ErrOrgSuspended, got %v", err)
	}
}

func TestWebhookMayTradePersonal(t *testing.T) {
	if err := WebhookMayTrade(&store.User{Status: domain.StatusActive}, nil); err != nil {
		t.Fatalf("personal webhook: %v", err)
	}
}
