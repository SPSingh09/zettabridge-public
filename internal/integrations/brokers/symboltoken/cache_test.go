package symboltoken

import (
	"testing"
	"time"
)

func TestCacheKey(t *testing.T) {
	got := CacheKey("angel", "nse", "reliance")
	if got != "symtoken:angel:NSE:RELIANCE" {
		t.Fatalf("got %q", got)
	}
	got = CacheKey("dhan", "NSE", "TCS")
	if got != "symtoken:dhan:NSE:TCS" {
		t.Fatalf("got %q", got)
	}
}

func TestDefaultTTL(t *testing.T) {
	if DefaultTTL != 24*time.Hour {
		t.Fatalf("got %v", DefaultTTL)
	}
}
