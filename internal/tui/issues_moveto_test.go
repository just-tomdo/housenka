package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/issues"
)

// With move_to: start, L puts the issue at the start of the next status.
func TestIssueMoveToStartInTUI(t *testing.T) {
	m, _, _ := newTestModel(t)
	st := enableIssues(t, gitlabIssue("a/x", 1, "jedna"), gitlabIssue("a/x", 2, "dva"))
	path := filepath.Join(os.Getenv(config.HomeEnv), "issues.yaml")
	data, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(data), "move_to: end", "move_to: start", 1)), 0o644)
	if err := st.SetStatus("a/x#1", "doing", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.SetStatus("a/x#2", "todo", ""); err != nil {
		t.Fatal(err)
	}
	press(m, "T", "2")
	m.iss.sel = "a/x#2"
	press(m, "L")
	f, _ := st.Load()
	slices.SortStableFunc(f.Issues, func(a, b issues.Issue) int { return a.Rank - b.Rank })
	if f.Issues[0].Key != "a/x#2" || f.Issues[0].Status != "doing" {
		t.Errorf("order %+v", f.Issues)
	}
	// Choosing the current status in the detail leaves the card where it is.
	press(m, "<enter>", "2")
	g, _ := st.Load()
	for i := range g.Issues {
		if g.Issues[i].Key == "a/x#2" && g.Issues[i].Rank != f.Issues[0].Rank {
			t.Errorf("the current status moved the card: rank %d", g.Issues[i].Rank)
		}
	}
}
