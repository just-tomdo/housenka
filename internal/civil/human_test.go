package civil

import (
	"testing"
	"time"
)

func TestParseHuman(t *testing.T) {
	today := Date{2026, time.October, 12}
	tests := []struct {
		in   string
		want Date
	}{
		{"12. 10. 2026", Date{2026, time.October, 12}},
		{"9.10.2026", Date{2026, time.October, 9}},
		{"9. 10.", Date{2026, time.October, 9}},
		{"9.10", Date{2026, time.October, 9}},
		{"3.", Date{2026, time.October, 3}},
		{"31.12.25", Date{2025, time.December, 31}},
		{"2026-01-05", Date{2026, time.January, 5}},
		{"dnes", today},
		{" Včera ", Date{2026, time.October, 11}},
		{"-3", Date{2026, time.October, 9}},
		{"+1", Date{2026, time.October, 13}},
	}
	for _, tt := range tests {
		got, err := ParseHuman(tt.in, today)
		if err != nil {
			t.Errorf("ParseHuman(%q) error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseHuman(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
	for _, in := range []string{"", "abc", "32.10.", "1.13.2026", "1.2.3.4", "-x", "2026-13-01", "1.-2."} {
		if _, err := ParseHuman(in, today); err == nil {
			t.Errorf("ParseHuman(%q) expected error", in)
		}
	}
}
