package tui

import "testing"

// h/l move into an empty column and stay there (only showing the board
// picks a column with cards).
func TestIssuesMoveIntoEmptyColumn(t *testing.T) {
	m, _, _ := newTestModel(t)
	st := enableIssues(t, gitlabIssue("clients/shop-web", 1, "jedna"))
	if err := st.SetStatus("clients/shop-web#1", "doing", ""); err != nil {
		t.Fatal(err)
	}
	press(m, "T", "2")
	if m.iss.col != 1 {
		t.Fatalf("board opened in column %d", m.iss.col)
	}
	press(m, "l")
	if m.iss.col != 2 || m.iss.sel != "" {
		t.Errorf("l: col %d sel %q", m.iss.col, m.iss.sel)
	}
	press(m, "h", "h")
	if m.iss.col != 0 {
		t.Errorf("h h: col %d", m.iss.col)
	}
}
