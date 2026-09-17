package execution

import "testing"

func TestRouterDayZeroGating(t *testing.T) {
	r := NewRouter([]string{AdapterPaper, AdapterZerodha})
	if !r.IsBrokerEnabled("zerodha") {
		t.Fatal("expected zerodha enabled")
	}
	if r.IsBrokerEnabled("angel") {
		t.Fatal("expected angel disabled on day 0")
	}
	if err := r.RequireBroker("angel"); err == nil {
		t.Fatal("expected error for disabled angel")
	}
}

func TestDefaultEnabledAdaptersIncludesCIBrokers(t *testing.T) {
	got := DefaultEnabledAdapters()
	if len(got) < 5 {
		t.Fatalf("expected full dev adapter set, got %v", got)
	}
}
