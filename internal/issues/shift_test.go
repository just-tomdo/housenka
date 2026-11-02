package issues

import (
	"slices"
	"testing"
)

// J/K move a card among the cards the user sees: with a filter the
// invisible ones do not count, grouped by repository a card stays in its group.
func TestShiftAmongShownCards(t *testing.T) {
	f := File{Issues: []Issue{issue("a/x", 1), issue("a/x", 3), issue("b/y", 2), issue("a/x", 4)}}
	for _, is := range f.Issues {
		must(t, f.SetStatus(is.Key, "todo", "", t0))
	}
	card := func(key string) Card {
		for _, is := range f.Issues {
			if is.Key == key {
				return Card{Issue: is}
			}
		}
		t.Fatalf("no %s", key)
		return Card{}
	}

	// Filtered to a/x: #4 goes above #3, skipping the hidden b/y#2.
	shown := []Card{card("a/x#1"), card("a/x#3"), card("a/x#4")}
	n, after, ok := Shift(shown, "a/x#4", -1, false)
	if !ok || n != "a/x#3" || after {
		t.Fatalf("Shift = %q %v %v", n, after, ok)
	}
	must(t, f.MoveNextTo("a/x#4", n, after, t0))
	if got := keys(column(&f, "todo")); !slices.Equal(got, []string{"a/x#1", "a/x#4", "a/x#3", "b/y#2"}) {
		t.Errorf("order %v", got)
	}

	// Grouped by repository: b/y#2 cannot move into the a/x group.
	grouped := []Card{card("a/x#1"), card("a/x#4"), card("a/x#3"), card("b/y#2")}
	if _, _, ok := Shift(grouped, "b/y#2", -1, true); ok {
		t.Error("moved out of its repository group")
	}
	if _, _, ok := Shift(grouped, "a/x#1", -1, true); ok {
		t.Error("moved past the top")
	}
	if n, after, ok := Shift(grouped, "a/x#4", 1, true); !ok || n != "a/x#3" || !after {
		t.Errorf("Shift down = %q %v %v", n, after, ok)
	}
}
