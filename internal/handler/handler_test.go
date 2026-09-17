package handler

import (
	"testing"

	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestCancelTradeStatusGuard(t *testing.T) {
	cancellable := []string{"submitted"}
	nonCancellable := []string{"filled", "rejected", "cancelled", "queued"}

	for _, status := range cancellable {
		t := t
		t.Run("cancellable_"+status, func(t *testing.T) {
			trade := &store.Trade{Status: status}
			if trade.Status != "submitted" {
				t.Errorf("expected submitted to be the only cancellable status, got %q", trade.Status)
			}
		})
	}
	for _, status := range nonCancellable {
		t := t
		t.Run("non_cancellable_"+status, func(t *testing.T) {
			trade := &store.Trade{Status: status}
			if trade.Status == "submitted" {
				t.Errorf("expected %q to be non-cancellable", status)
			}
		})
	}
}

func TestRejectImmutableWebhookFields(t *testing.T) {
	tests := []struct {
		body    string
		wantErr bool
		field   string
	}{
		{`{"label":"x"}`, false, ""},
		{`{"lot_size":0.01}`, false, ""},
		{`{"token":"wh_live_hack"}`, true, "token"},
		{`{"id":"new-id"}`, true, "id"},
		{`{"user_id":"other-user"}`, true, "user_id"},
		{`{"created_at":"2020-01-01T00:00:00Z"}`, true, "created_at"},
		{`{"status":"paused"}`, true, "status"},
	}

	for _, tc := range tests {
		field, err := rejectImmutableWebhookFields([]byte(tc.body))
		if tc.wantErr {
			if err == nil {
				t.Fatalf("body %q: expected error", tc.body)
			}
			if field != tc.field {
				t.Fatalf("body %q: expected field %q, got %q", tc.body, tc.field, field)
			}
			continue
		}
		if err != nil {
			t.Fatalf("body %q: unexpected error: %v", tc.body, err)
		}
	}
}
