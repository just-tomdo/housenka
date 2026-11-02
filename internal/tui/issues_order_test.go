package tui

import (
	"slices"
	"testing"

	"github.com/just-tomdo/housenka/internal/issues"
)

func todoOrder(t *testing.T, st *issues.Store) []string {
	t.Helper()
	f, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	slices.SortStableFunc(f.Issues, func(a, b issues.Issue) int { return a.Rank - b.Rank })
	var out []string
	for _, is := range f.Issues {
		if is.Status == "todo" {
			out = append(out, is.Key)
		}
	}
	return out
}

// J/K move among the cards shown: grouped by repository a card stays in its group.
func TestIssueOrderGroupedByRepo(t *testing.T) {
	m, _, _ := newTestModel(t)
	st := enableIssues(t, gitlabIssue("a/x", 1, "jedna"), gitlabIssue("a/x", 3, "tři"), gitlabIssue("b/y", 2, "dva"))
	for _, k := range []string{"a/x#1", "b/y#2", "a/x#3"} {
		if err := st.SetStatus(k, "todo", ""); err != nil {
			t.Fatal(err)
		}
	}
	press(m, "T", "2", "p") // the board, grouped: a/x#1, a/x#3 | b/y#2
	m.iss.sel = "b/y#2"
	press(m, "K")
	if got := todoOrder(t, st); !slices.Equal(got, []string{"a/x#1", "b/y#2", "a/x#3"}) {
		t.Errorf("K left the repository group: %v", got)
	}
	m.iss.sel = "a/x#3"
	press(m, "K")
	if got := todoOrder(t, st); !slices.Equal(got, []string{"a/x#3", "a/x#1", "b/y#2"}) {
		t.Errorf("K within the group: %v", got)
	}
	press(m, "O", "J") // another order: refused
	if got := todoOrder(t, st); !slices.Equal(got, []string{"a/x#3", "a/x#1", "b/y#2"}) || m.toast == "" {
		t.Errorf("J in another order: %v (toast %q)", got, m.toast)
	}
}
