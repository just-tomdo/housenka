package text

import (
	"slices"
	"testing"
)

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

func TestArgsRoundTrip(t *testing.T) {
	for _, args := range [][]string{
		{"wslview"},
		{"/mnt/c/Program Files/Mozilla Firefox/firefox.exe", "--new-tab"},
		{"cmd.exe", "/c", "start", ""},
		{`say "hi"`, `back\slash`, "it's"},
	} {
		line := JoinArgs(args)
		got, err := SplitArgs(line)
		if err != nil || !slices.Equal(got, args) {
			t.Errorf("SplitArgs(JoinArgs(%q)) = %q (%q), %v", args, got, line, err)
		}
	}
	if got, _ := SplitArgs(`  firefox   'a b'  "" `); !slices.Equal(got, []string{"firefox", "a b", ""}) {
		t.Errorf("typed = %q", got)
	}
	if _, err := SplitArgs(`"open`); err == nil {
		t.Error("unterminated quote accepted")
	}
}
