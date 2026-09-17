package store

import (
	"testing"
	"time"
)

func TestPublisherOrderIsExpired(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name   string
		order  *PublisherOrder
		expiry bool
	}{
		{
			name:   "nil order",
			order:  nil,
			expiry: true,
		},
		{
			name:   "status expired",
			order:  &PublisherOrder{Status: "expired", ExpiresAt: now.Add(time.Hour)},
			expiry: true,
		},
		{
			name:   "past deadline",
			order:  &PublisherOrder{Status: "created", ExpiresAt: now.Add(-time.Minute)},
			expiry: true,
		},
		{
			name:   "still valid",
			order:  &PublisherOrder{Status: "created", ExpiresAt: now.Add(10 * time.Minute)},
			expiry: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := PublisherOrderIsExpired(tc.order, now); got != tc.expiry {
				t.Fatalf("PublisherOrderIsExpired() = %v, want %v", got, tc.expiry)
			}
		})
	}
}

func TestPublisherOrderIsTerminal(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"user_returned", "user_cancelled", "expired"} {
		if !PublisherOrderIsTerminal(status) {
			t.Fatalf("expected terminal status %q", status)
		}
	}
	if PublisherOrderIsTerminal("created") {
		t.Fatal("created should not be terminal")
	}
}
