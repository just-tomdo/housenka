package period

import (
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
)

func day(y int, m time.Month, d int) civil.Date { return civil.Date{Year: y, Month: m, Day: d} }

func TestOfAndEnd(t *testing.T) {
	tests := []struct {
		kind       Kind
		d          civil.Date
		start, end civil.Date
	}{
		{Day, day(2026, 10, 14), day(2026, 10, 14), day(2026, 10, 14)},
		{Week, day(2026, 10, 14), day(2026, 10, 12), day(2026, 10, 18)}, // Wednesday
		{Week, day(2026, 10, 12), day(2026, 10, 12), day(2026, 10, 18)}, // Monday
		{Week, day(2026, 10, 18), day(2026, 10, 12), day(2026, 10, 18)}, // Sunday
		{Week, day(2027, 1, 1), day(2026, 12, 28), day(2027, 1, 3)},     // across New Year
		{Month, day(2026, 10, 14), day(2026, 10, 1), day(2026, 10, 31)},
		{Month, day(2028, 2, 10), day(2028, 2, 1), day(2028, 2, 29)}, // leap year
	}
	for _, tt := range tests {
		p := Of(tt.kind, tt.d)
		if p.Start != tt.start || p.End() != tt.end {
			t.Errorf("Of(%v, %v) = %v..%v, want %v..%v", tt.kind, tt.d, p.Start, p.End(), tt.start, tt.end)
		}
	}
}

func TestNextPrev(t *testing.T) {
	w := Of(Week, day(2026, 12, 30))
	if got := w.Next().Start; got != day(2027, 1, 4) {
		t.Errorf("week Next = %v", got)
	}
	if got := w.Prev().Start; got != day(2026, 12, 21) {
		t.Errorf("week Prev = %v", got)
	}
	m := Of(Month, day(2026, 1, 31))
	if got := m.Next().Start; got != day(2026, 2, 1) {
		t.Errorf("month Next = %v", got)
	}
	if got := m.Prev().Start; got != day(2025, 12, 1) {
		t.Errorf("month Prev = %v", got)
	}
	if got := Of(Day, day(2026, 3, 1)).Prev().Start; got != day(2026, 2, 28) {
		t.Errorf("day Prev = %v", got)
	}
}

func TestDaysAndContains(t *testing.T) {
	w := Of(Week, day(2026, 10, 14))
	days := w.Days()
	if len(days) != 7 || days[0] != day(2026, 10, 12) || days[6] != day(2026, 10, 18) {
		t.Errorf("Days = %v", days)
	}
	if !w.Contains(day(2026, 10, 18)) || w.Contains(day(2026, 10, 19)) {
		t.Error("Contains wrong")
	}
	if n := len(Of(Month, day(2026, 2, 1)).Days()); n != 28 {
		t.Errorf("February days = %d", n)
	}
}

func TestLabels(t *testing.T) {
	tests := []struct {
		p    Period
		want string
	}{
		{Of(Day, day(2026, 10, 12)), "po 12. 10. 2026"},
		{Of(Week, day(2026, 10, 14)), "týden 42 · 12.–18. 10. 2026"},
		{Of(Week, day(2026, 10, 1)), "týden 40 · 28. 9.–4. 10. 2026"},
		{Of(Week, day(2027, 1, 1)), "týden 53 · 28. 12. 2026–3. 1. 2027"},
		{Of(Month, day(2026, 10, 14)), "říjen 2026"},
	}
	for _, tt := range tests {
		if got := tt.p.Label(civil.Layout{}); got != tt.want {
			t.Errorf("Label() = %q, want %q", got, tt.want)
		}
	}
}

func TestKindCycle(t *testing.T) {
	if Day.Next() != Week || Week.Next() != Month || Month.Next() != Day {
		t.Error("Kind.Next cycle broken")
	}
}
