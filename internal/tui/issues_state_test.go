package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/gitlab"
	"github.com/just-tomdo/housenka/internal/issues"
)

// A failing sync (here: no token) is not tried again at every T.
func TestIssuesSyncNotRetriedAtEveryT(t *testing.T) {
	m, _, _ := newTestModel(t)
	t.Setenv(gitlab.TokenEnv, "")
	st := enableIssues(t)
	if err := st.Update(func(f *issues.File, now time.Time) error {
		f.SyncedAt = now.Add(-time.Hour) // due
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	press(m, "T")
	if !strings.Contains(m.toast, "chybí token") {
		t.Fatalf("first T: toast %q", m.toast)
	}
	press(m, "T")
	m.toast = ""
	press(m, "T")
	if m.toast != "" {
		t.Errorf("second T tried again: %q", m.toast)
	}
}

// Switching to the board selects a card even when the last column was empty.
func TestIssuesBoardSelectsAColumnWithCards(t *testing.T) {
	m, _, _ := newTestModel(t)
	st := enableIssues(t, gitlabIssue("clients/shop-web", 1, "jedna"))
	if err := st.SetStatus("clients/shop-web#1", "doing", ""); err != nil {
		t.Fatal(err)
	}
	press(m, "T", "1", "2")
	if m.iss.col != 1 || m.iss.sel != "clients/shop-web#1" {
		t.Errorf("col %d sel %q", m.iss.col, m.iss.sel)
	}
}

// Removing issues.yaml while the overview is open says so.
func TestIssuesTurnedOffWhileOpen(t *testing.T) {
	m, _, _ := newTestModel(t)
	enableIssues(t, gitlabIssue("clients/shop-web", 1, "jedna"))
	press(m, "T")
	if err := os.Remove(filepath.Join(os.Getenv(config.HomeEnv), "issues.yaml")); err != nil {
		t.Fatal(err)
	}
	press(m, "j")
	if out := screen(m); !strings.Contains(out, "Přehled issues je vypnutý") || strings.Contains(out, "Oprav issues.yaml") {
		t.Errorf("screen:\n%s", out)
	}
	press(m, "T")
	if m.issuesOpen {
		t.Error("T did not leave")
	}
}
