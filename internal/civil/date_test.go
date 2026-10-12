package civil

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseAndFormat(t *testing.T) {
	d, err := Parse("2026-10-09")
	if err != nil {
		t.Fatal(err)
	}
	if d != (Date{2026, time.October, 9}) {
		t.Fatalf("Parse = %v", d)
	}
	if got := d.Czech(); got != "9. 10. 2026" {
		t.Errorf("Czech() = %q", got)
	}
	if got := d.String(); got != "2026-10-09" {
		t.Errorf("String() = %q", got)
	}
	if _, err := Parse("2026-02-30"); err == nil {
		t.Error("Parse(2026-02-30) expected error")
	}
}

func TestArithmetic(t *testing.T) {
	d := Date{2026, time.December, 31}
	if got := d.AddDays(1); got != (Date{2027, time.January, 1}) {
		t.Errorf("AddDays(1) = %v", got)
	}
	// Day arithmetic must not be affected by DST changes.
	march := Date{2026, time.March, 28}
	if got := march.AddDays(2); got != (Date{2026, time.March, 30}) {
		t.Errorf("AddDays across DST = %v", got)
	}
	if !d.After(march) || !march.Before(d) || d.Compare(d) != 0 {
		t.Error("comparison broken")
	}
	if got := (Date{2026, time.October, 12}).Weekday(); got != time.Monday {
		t.Errorf("Weekday = %v", got)
	}
}

func TestValid(t *testing.T) {
	if (Date{}).Valid() {
		t.Error("zero date must be invalid")
	}
	if (Date{2026, time.February, 30}).Valid() {
		t.Error("Feb 30 must be invalid")
	}
	if !(Date{2028, time.February, 29}).Valid() {
		t.Error("leap day must be valid")
	}
}

func TestJSON(t *testing.T) {
	type wrap struct {
		D Date `json:"d"`
	}
	b, err := json.Marshal(wrap{Date{2026, time.October, 9}})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"d":"2026-10-09"}` {
		t.Fatalf("Marshal = %s", b)
	}
	var w wrap
	if err := json.Unmarshal(b, &w); err != nil {
		t.Fatal(err)
	}
	if w.D != (Date{2026, time.October, 9}) {
		t.Errorf("Unmarshal = %v", w.D)
	}
}
