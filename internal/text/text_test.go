package text

import "testing"

func TestPlural(t *testing.T) {
	for n, want := range map[int]string{0: "0 dní", 1: "1 den", 2: "2 dny", 4: "4 dny", 5: "5 dní", 12: "12 dní"} {
		if got := Plural(n, "den", "dny", "dní"); got != want {
			t.Errorf("Plural(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestOneLine(t *testing.T) {
	if got := OneLine(" a\n  b\tc \n"); got != "a b c" {
		t.Errorf("OneLine = %q", got)
	}
}
