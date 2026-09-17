package tradepush_test

import (
	"context"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/transport/websocket"
)

type stubLookup struct {
	webhook *store.Webhook
	members []*store.OrgMemberProfile
}

func (s *stubLookup) GetWebhookByID(_ context.Context, id string) (*store.Webhook, error) {
	if s.webhook == nil || s.webhook.ID != id {
		return nil, nil
	}
	return s.webhook, nil
}

func (s *stubLookup) ListOrgMembers(_ context.Context, _ string) ([]*store.OrgMemberProfile, error) {
	return s.members, nil
}

func TestTradeRecipientsPersonalWebhook(t *testing.T) {
	hub := tradepush.NewHub(&stubLookup{
		webhook: &store.Webhook{ID: "wh1", UserID: "user-a"},
	})
	recipients := hub.TradeRecipients(context.Background(), &store.Webhook{UserID: "user-a"})
	if len(recipients) != 1 {
		t.Fatalf("expected 1 recipient, got %d", len(recipients))
	}
	if _, ok := recipients["user-a"]; !ok {
		t.Fatal("expected user-a in recipients")
	}
}

func TestTradeRecipientsOrgWebhook(t *testing.T) {
	orgID := "org1"
	hub := tradepush.NewHub(&stubLookup{
		members: []*store.OrgMemberProfile{
			{OrgMember: store.OrgMember{UserID: "member-b", Status: domain.StatusActive}},
			{OrgMember: store.OrgMember{UserID: "suspended", Status: domain.StatusSuspended}},
		},
	})
	wh := &store.Webhook{UserID: "owner", OrgID: &orgID}
	recipients := hub.TradeRecipients(context.Background(), wh)
	if len(recipients) != 2 {
		t.Fatalf("expected owner + active member, got %v", recipients)
	}
	if _, ok := recipients["owner"]; !ok {
		t.Fatal("missing owner")
	}
	if _, ok := recipients["member-b"]; !ok {
		t.Fatal("missing member-b")
	}
	if _, ok := recipients["suspended"]; ok {
		t.Fatal("suspended member should not receive pushes")
	}
}
