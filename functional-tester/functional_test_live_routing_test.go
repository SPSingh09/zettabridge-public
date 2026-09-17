//go:build functional

package functionaltester

import (
	"os"
	"testing"
)

// TestFreeUserBlockedFromLiveWhenBrokerLive verifies free users cannot create live
// broker credentials when the server runs with BROKER_MODE=live (Phase 0: no demo/mock routing).
//
// Run:
//
//	make functional-test-live-routing
func TestFreeUserBlockedFromLiveWhenBrokerLive(t *testing.T) {
	if os.Getenv("FUNCTIONAL_TEST_LIVE_ROUTING") != "1" {
		t.Skip("set FUNCTIONAL_TEST_LIVE_ROUTING=1 with BROKER_MODE=live server (make functional-test-live-routing)")
	}

	s := newState()
	s.registerOwner(t)
	s.loginOwner(t)
	s.assertMe(t, s.ownerToken, s.ownerEmail, "free", "user", false)
	s.assertLiveCredentialBlockedOnFree(t)
}
