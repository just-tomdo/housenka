package tui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/issues"
)

// enableIssues writes the example issues.yaml and the given issues into the home.
func enableIssues(t *testing.T, list ...issues.Issue) *issues.Store {
	t.Helper()
	home := os.Getenv(config.HomeEnv)
	example, err := os.ReadFile("../../issues.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "issues.yaml"), example, 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := issues.OpenStore(filepath.Join(home, "issues"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Update(func(f *issues.File, now time.Time) error {
		f.Issues = append(f.Issues, list...)
		f.SyncedAt = now // no sync when the view opens
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return st
}

func gitlabIssue(path string, iid int, title string) issues.Issue {
	url := "https://gitlab.example.com/" + path + "/-/issues/" + itoa(iid)
	at := time.Date(2026, 10, 9, 9, 0, 0, 0, time.Local)
	return issues.Issue{Key: strings.ToLower(path) + "#" + itoa(iid), Host: "gitlab.example.com", ProjectPath: path, IID: iid,
		Title: title, URL: url, AssignedAt: at, FirstSeen: at}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestIssuesOffInTUI(t *testing.T) {
	m, _, _ := newTestModel(t)
	press(m, "T")
	if m.issuesOpen || !strings.Contains(m.toast, "issues init") {
		t.Errorf("open=%v toast=%q", m.issuesOpen, m.toast)
	}
}

func TestIssuesBoardInTUI(t *testing.T) {
	m, _, o := newTestModel(t)
	st := enableIssues(t,
		gitlabIssue("clients/shop-web", 393, "Košík nepřepočítá dopravu"),
		gitlabIssue("other/tool", 7, "Cizí repozitář"),
	)
	if !strings.Contains(screen(m), "2 nová issue (T)") {
		t.Errorf("entries status line lacks the count:\n%s", screen(m))
	}

	press(m, "T")
	dump(t, "20-issues-new", m)
	out := screen(m)
	for _, want := range []string{"Issues", "1 Nová 2", "Košík nepřepočítá dopravu", "repozitář není v housence", "přiřazeno před 3 dny", "L zařadí"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}

	press(m, "L") // sorts the selected (first assigned) issue into the first status
	f, _ := st.Load()
	if f.Issues[0].Status != "todo" {
		t.Fatalf("status %q", f.Issues[0].Status)
	}
	press(m, "2")
	if m.iss.tab != "board" || m.iss.sel != "clients/shop-web#393" {
		t.Fatalf("tab %q sel %q", m.iss.tab, m.iss.sel)
	}
	press(m, "L") // and on to the next status
	f, _ = st.Load()
	if f.Issues[0].Status != "doing" || m.iss.col != 1 {
		t.Errorf("status %q col %d", f.Issues[0].Status, m.iss.col)
	}
	dump(t, "21-issues-board", m)
	if out := screen(m); !strings.Contains(out, "Rozpracováno") || !strings.Contains(out, "Na řadě") {
		t.Errorf("board columns missing:\n%s", out)
	}

	// Detail: status by number, notes.
	press(m, "<enter>")
	if m.iss.detail == nil {
		t.Fatal("detail not opened")
	}
	dump(t, "22-issue-detail", m)
	press(m, "n", "zavolat klientovi", "<ctrl+s>")
	f, _ = st.Load()
	if f.Issues[0].Notes != "zavolat klientovi" {
		t.Errorf("notes %q", f.Issues[0].Notes)
	}
	press(m, "3")
	f, _ = st.Load()
	if f.Issues[0].Status != "waiting" || m.iss.detail != nil {
		t.Errorf("status %q detail %v", f.Issues[0].Status, m.iss.detail != nil)
	}

	// Report: the form has the issue; saving returns to the board with the hours.
	press(m, "v")
	if m.mode != modeForm || !strings.Contains(m.form.inputs[fieldIssue].Value(), "/clients/shop-web/-/issues/393") || m.form.focus != fieldProject {
		t.Fatalf("mode %v focus %v", m.mode, m.form.focus)
	}
	press(m, "l", "<tab>", "<tab>", "1,5", "<tab>", "oprava", "<ctrl+s>")
	if m.mode != modeConfirmOpen {
		t.Fatalf("not saved: %v", m.form.errs)
	}
	press(m, "n")
	if !m.issuesOpen || !strings.Contains(screen(m), "1,50 h") {
		t.Errorf("back on the board with the hours:\n%s", screen(m))
	}

	// Open in GitLab, hide.
	press(m, "o")
	if len(o.opened) == 0 {
		m.Update(m.openIssueURL()())
	}
	press(m, "x")
	f, _ = st.Load()
	if !f.Issues[0].Hidden {
		t.Error("x did not hide")
	}
	press(m, "T")
	if m.issuesOpen {
		t.Error("T did not return to the entries")
	}
}
