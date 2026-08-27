package handler

import "testing"

func TestParseLimit(t *testing.T) {
	cases := []struct {
		name  string
		input string
		def   int
		max   int
		want  int
	}{
		{"empty falls back to default", "", 20, 100, 20},
		{"valid value within range", "50", 20, 100, 50},
		{"non-numeric falls back to default", "abc", 20, 100, 20},
		{"zero falls back to default", "0", 20, 100, 20},
		{"negative falls back to default", "-5", 20, 100, 20},
		{"exceeding max is clamped", "1000", 20, 100, 100},
		{"exactly max is allowed", "100", 20, 100, 100},
		{"exactly 1 is allowed", "1", 20, 100, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseLimit(tc.input, tc.def, tc.max)
			if got != tc.want {
				t.Errorf("parseLimit(%q, %d, %d) = %d, want %d", tc.input, tc.def, tc.max, got, tc.want)
			}
		})
	}
}
