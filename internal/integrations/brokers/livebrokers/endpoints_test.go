package livebrokers

import (
	"testing"
)

func TestURLsBaseURLAccountMode(t *testing.T) {
	u := URLs{
		MT5Live:     "https://live-mt.example",
		MT5Demo:     "https://demo-mt.example",
		ZerodhaLive: "https://live-kite.example",
	}

	if got := u.BaseURL("mt5_cloud", "live"); got != "https://live-mt.example" {
		t.Fatalf("live mt5: %q", got)
	}
	if got := u.BaseURL("mt5_cloud", "demo"); got != "https://demo-mt.example" {
		t.Fatalf("demo mt5: %q", got)
	}
	if got := u.BaseURL("zerodha", "demo"); got != "https://live-kite.example" {
		t.Fatalf("zerodha demo falls back to live base: %q", got)
	}
	if got := u.BaseURL("angel", "live"); got != defaultAngelBaseURL {
		t.Fatalf("angel default: %q", got)
	}
}
