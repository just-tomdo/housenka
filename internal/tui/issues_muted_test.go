package tui

import (
	"strings"
	"testing"
)

func TestIssueMutedInTUI(t *testing.T) {
	m, _, _ := newTestModel(t)
	st := enableIssues(t, gitlabIssue("clients/shop-web", 1, "jedna"), gitlabIssue("clients/shop-web", 2, "dva"))
	for _, k := range []string{"clients/shop-web#1", "clients/shop-web#2"} {
		if err := st.SetStatus(k, "todo", ""); err != nil {
			t.Fatal(err)
		}
	}
	press(m, "T", "2")
	if m.iss.sel != "clients/shop-web#1" {
		t.Fatalf("sel %q", m.iss.sel)
	}
	press(m, "z")
	f, _ := st.Load()
	if !f.Issues[0].Muted || !strings.Contains(m.toast, "odloženo") {
		t.Fatalf("muted %v toast %q", f.Issues[0].Muted, m.toast)
	}
	// The muted card goes after the other one, the selection follows it.
	if list := m.issueList(); list[len(list)-1].Key != "clients/shop-web#1" || m.iss.sel != "clients/shop-web#1" {
		t.Errorf("order %v sel %q", list, m.iss.sel)
	}
	if out := screen(m); !strings.Contains(out, "odložené") {
		t.Errorf("no mark on the card:\n%s", out)
	}
	press(m, "<enter>", "z") // in the detail too
	if f, _ = st.Load(); f.Issues[0].Muted {
		t.Error("z in the detail did not bring it back")
	}
}
