package issues

import (
	"slices"
	"strings"
	"testing"
)

func TestPlace(t *testing.T) {
	f := File{Issues: []Issue{issue("a/x", 1), issue("a/x", 2), issue("a/x", 3)}}
	must(t, f.SetStatus("a/x#1", "todo", "", t0))
	must(t, f.SetStatus("a/x#2", "todo", "", t0))
	must(t, f.Place("a/x#3", "todo", true, t0))
	if got := keys(column(&f, "todo")); !slices.Equal(got, []string{"a/x#3", "a/x#1", "a/x#2"}) {
		t.Errorf("start: %v", got)
	}
	must(t, f.Place("a/x#3", "doing", false, t0))
	must(t, f.Place("a/x#1", "doing", false, t0))
	if got := keys(column(&f, "doing")); !slices.Equal(got, []string{"a/x#3", "a/x#1"}) {
		t.Errorf("end: %v", got)
	}
	must(t, f.Place("a/x#2", "doing", true, t0)) // into a column with cards, at the start
	if got := keys(column(&f, "doing")); got[0] != "a/x#2" {
		t.Errorf("start of a filled column: %v", got)
	}
}

func TestMoveToConfig(t *testing.T) {
	for in, start := range map[string]bool{"": false, "move_to: end\n": false, "move_to: start\n": true} {
		cfg, err := ParseConfig([]byte(in + "statuses: [{code: a}]"))
		if err != nil || cfg.MoveToStart() != start {
			t.Errorf("%q: %v %v", in, cfg, err)
		}
	}
	if _, err := ParseConfig([]byte("move_to: top\nstatuses: [{code: a}]")); err == nil || !strings.Contains(err.Error(), "move_to") {
		t.Errorf("move_to: top accepted: %v", err)
	}
}
