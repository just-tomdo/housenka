package civil

import (
	"strings"
	"testing"
	"time"
)

func TestLayoutFormat(t *testing.T) {
	d := Date{2026, time.October, 9}
	tests := map[string]string{
		"d. m. yyyy":   "9. 10. 2026",
		"dd. mm. yyyy": "09. 10. 2026",
		"dd.mm.yyyy":   "09.10.2026",
		"yyyy-mm-dd":   "2026-10-09",
		"DD/MM/YY":     "09/10/26",
		"mm/dd/yyyy":   "10/09/2026",
	}
	for pattern, want := range tests {
		if got := MustLayout(pattern).Format(d); got != want {
			t.Errorf("%q.Format = %q, want %q", pattern, got, want)
		}
	}
	if got := (Layout{}).Format(d); got != d.Czech() {
		t.Errorf("zero Layout formats %q, want %q", got, d.Czech())
	}
}

func TestLayoutFormatRange(t *testing.T) {
	day := func(y int, m time.Month, d int) Date { return Date{y, m, d} }
	tests := []struct {
		pattern  string
		from, to Date
		want     string
	}{
		{"d. m. yyyy", day(2026, 10, 12), day(2026, 10, 18), "12.–18. 10. 2026"},
		{"d. m. yyyy", day(2026, 9, 28), day(2026, 10, 4), "28. 9.–4. 10. 2026"},
		{"d. m. yyyy", day(2026, 12, 28), day(2027, 1, 3), "28. 12. 2026–3. 1. 2027"},
		{"dd.mm.yyyy", day(2026, 9, 28), day(2026, 10, 4), "28.09.–04.10.2026"},
		{"yyyy-mm-dd", day(2026, 10, 12), day(2026, 10, 18), "2026-10-12 – 2026-10-18"},
	}
	for _, tt := range tests {
		if got := MustLayout(tt.pattern).FormatRange(tt.from, tt.to); got != tt.want {
			t.Errorf("%q.FormatRange = %q, want %q", tt.pattern, got, tt.want)
		}
	}
}

func TestLayoutParse(t *testing.T) {
	want := Date{2026, time.October, 9}
	tests := map[string][]string{
		"d. m. yyyy": {"9. 10. 2026", "09.10.2026", "9.10.26"},
		"dd.mm.yyyy": {"09.10.2026", "9.10.2026"},
		"yyyy-mm-dd": {"2026-10-09", "2026-10-9"},
		"mm/dd/yyyy": {"10/09/2026", "10/9/26"},
	}
	for pattern, inputs := range tests {
		for _, in := range inputs {
			if got, ok := MustLayout(pattern).Parse(in); !ok || got != want {
				t.Errorf("%q.Parse(%q) = %v, %v", pattern, in, got, ok)
			}
		}
	}
	for _, in := range []string{"", "9. 10.", "31. 2. 2026", "9. 10. 2026x", "2026-10-09"} {
		if _, ok := MustLayout("d. m. yyyy").Parse(in); ok {
			t.Errorf("Parse(%q) should fail", in)
		}
	}
}

func TestParseLayoutErrors(t *testing.T) {
	for _, p := range []string{"", "dd.mm", "ddd.mm.yyyy", "dd.mm.yyy", "dd.mm.yyyy hh", "dd.dd.mm.yyyy"} {
		if _, err := ParseLayout(p); err == nil {
			t.Errorf("ParseLayout(%q) should fail", p)
		}
	}
}

func TestParseHumanInLayout(t *testing.T) {
	today := Date{2026, time.October, 12}
	us := MustLayout("mm/dd/yyyy")
	if got, err := ParseHuman("10/09/2026", today, us); err != nil || got != (Date{2026, time.October, 9}) {
		t.Errorf("layout input: %v, %v", got, err)
	}
	if got, err := ParseHuman("včera", today, us); err != nil || got != today.AddDays(-1) {
		t.Errorf("relative input: %v, %v", got, err)
	}
	if _, err := ParseHuman("xx", today, us); err == nil || !strings.Contains(err.Error(), "10/12/2026") {
		t.Errorf("error should show an example in the layout: %v", err)
	}
}
