package missquareoff

import (
	"testing"
	"time"
)

func TestInWindow(t *testing.T) {
	t.Parallel()
	loc := ist

	cases := []struct {
		name string
		at   time.Time
		want bool
	}{
		{
			name: "weekday before window",
			at:   time.Date(2026, 7, 8, 15, 14, 0, 0, loc), // Wed
			want: false,
		},
		{
			name: "weekday window start",
			at:   time.Date(2026, 7, 8, 15, 15, 0, 0, loc),
			want: true,
		},
		{
			name: "weekday mid window",
			at:   time.Date(2026, 7, 8, 15, 22, 0, 0, loc),
			want: true,
		},
		{
			name: "weekday window end",
			at:   time.Date(2026, 7, 8, 15, 30, 0, 0, loc),
			want: true,
		},
		{
			name: "weekday after window",
			at:   time.Date(2026, 7, 8, 15, 31, 0, 0, loc),
			want: false,
		},
		{
			name: "saturday in window time",
			at:   time.Date(2026, 7, 11, 15, 20, 0, 0, loc),
			want: false,
		},
		{
			name: "sunday in window time",
			at:   time.Date(2026, 7, 12, 15, 20, 0, 0, loc),
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := InWindow(tc.at); got != tc.want {
				t.Fatalf("InWindow(%v) = %v, want %v", tc.at, got, tc.want)
			}
		})
	}
}
