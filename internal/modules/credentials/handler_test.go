package credentials

import "testing"

func TestNormalizeExecutionMode(t *testing.T) {
	tests := []struct {
		broker   string
		mode     string
		wantMode string
		wantErr  bool
	}{
		// zerodha — all valid modes
		{"zerodha", "publisher", "publisher", false},
		{"zerodha", "user_api_oauth", "user_api_oauth", false},
		{"zerodha", "", "user_api_oauth", false}, // empty defaults to user_api_oauth

		// zerodha — invalid modes
		{"zerodha", "direct_api", "", true},
		{"zerodha", "bogus_mode", "", true},

		// non-zerodha — always forced to direct_api
		{"angel", "", "direct_api", false},
		{"angel", "direct_api", "direct_api", false},
		{"dhan", "", "direct_api", false},
		{"dhan", "direct_api", "direct_api", false},
		{"mt5_cloud", "", "direct_api", false},
		{"mt5_cloud", "direct_api", "direct_api", false},

		// non-zerodha — publisher not allowed
		{"angel", "publisher", "", true},
		{"dhan", "publisher", "", true},
		{"mt5_cloud", "publisher", "", true},

		// non-zerodha — user_api_oauth not allowed
		{"angel", "user_api_oauth", "", true},
		{"dhan", "user_api_oauth", "", true},
	}

	for _, tc := range tests {
		got, err := NormalizeExecutionMode(tc.broker, tc.mode)
		if tc.wantErr {
			if err == nil {
				t.Errorf("NormalizeExecutionMode(%q, %q): expected error, got %q", tc.broker, tc.mode, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeExecutionMode(%q, %q): unexpected error: %v", tc.broker, tc.mode, err)
			continue
		}
		if got != tc.wantMode {
			t.Errorf("NormalizeExecutionMode(%q, %q) = %q, want %q", tc.broker, tc.mode, got, tc.wantMode)
		}
	}
}
