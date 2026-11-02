package issues

import (
	"slices"
	"testing"

	"github.com/just-tomdo/housenka/internal/civil"
)

func TestMutedIssues(t *testing.T) {
	cfg, r := testConfigs(t)
	f := File{Issues: []Issue{issue("clients/shop-web", 1), issue("clients/shop-web", 2), issue("clients/shop-web", 3)}}
	for _, is := range f.Issues {
		must(t, f.SetStatus(is.Key, "todo", "", t0))
	}
	must(t, f.SetMuted("clients/shop-web#1", true, t0))
	if !f.Issues[0].Muted || !f.Issues[0].TouchedAt.Equal(t0) {
		t.Fatalf("%+v", f.Issues[0])
	}

	today := civil.Of(t0).AddDays(30) // all three untouched for a month
	cards := Cards(f, cfg, r, nil, today)
	for _, s := range Sorts {
		got := slices.Clone(cards)
		SortCards(got, s)
		if got[len(got)-1].Key != "clients/shop-web#1" {
			t.Errorf("sort %q: muted issue not last: %v", s, cardKeys(got))
		}
	}
	if cards[0].Stale || !cards[1].Stale {
		t.Errorf("a muted issue must not be stale: %+v", cards[:2])
	}
	if got := cardKeys(Filter(cards, "is:muted")); !slices.Equal(got, []string{"clients/shop-web#1"}) {
		t.Errorf("is:muted %v", got)
	}
	if got := cards[0].Marks(); len(got) == 0 || got[0].Text != "odložené" {
		t.Errorf("marks %+v", got)
	}

	shown := slices.Clone(cards)
	SortCards(shown, SortManual) // #2, #3, then muted #1
	if _, _, ok := Shift(shown, "clients/shop-web#3", 1, false); ok {
		t.Error("J moved a card among the muted ones")
	}
	if _, _, ok := Shift(shown, "clients/shop-web#1", -1, false); ok {
		t.Error("K moved a muted card among the others")
	}
	must(t, f.SetMuted("clients/shop-web#1", false, t0))
	if f.Issues[0].Muted {
		t.Error("not back")
	}
}
