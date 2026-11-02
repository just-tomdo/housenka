package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/just-tomdo/housenka/internal/gitlab"
	"github.com/just-tomdo/housenka/internal/issues"
)

// tuiGitLab has one issue assigned to the user.
type tuiGitLab struct{}

func (tuiGitLab) Host() string { return "gitlab.example.com" }
func (tuiGitLab) CurrentUser(context.Context) (gitlab.User, error) {
	return gitlab.User{Username: "jnovak"}, nil
}
func (tuiGitLab) AssignedIssues(context.Context) ([]gitlab.Issue, error) {
	return []gitlab.Issue{{IID: 7, Title: "Z GitLabu", State: "opened", WebURL: "https://gitlab.example.com/clients/shop-web/-/issues/7"}}, nil
}
func (tuiGitLab) Issue(context.Context, string, int) (gitlab.Issue, error) {
	return gitlab.Issue{}, gitlab.ErrNotFound
}
func (tuiGitLab) AssignedAt(context.Context, string, int, string) (time.Time, error) {
	return time.Time{}, nil
}

// runTick delivers a tick and runs what it starts (except the next tick).
func runTick(m *Model) {
	cmd := m.issuesTicked()
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return // only the next tick: nothing was due
	}
	for _, c := range batch {
		if c == nil {
			continue
		}
		if out := c(); out != nil {
			if _, tick := out.(issuesTickMsg); !tick {
				m.Update(out)
			}
		}
	}
}

func dueStore(t *testing.T, st *issues.Store) {
	t.Helper()
	if err := st.Update(func(f *issues.File, now time.Time) error {
		f.SyncedAt = now.Add(-time.Hour)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPeriodicSyncInTUI(t *testing.T) {
	old := issuesTickEvery
	issuesTickEvery = time.Millisecond
	t.Cleanup(func() { issuesTickEvery = old })
	m, _, _ := newTestModel(t)

	// Without a token the periodic sync stays quiet on the entries.
	t.Setenv(gitlab.TokenEnv, "")
	st := enableIssues(t)
	dueStore(t, st)
	runTick(m)
	if m.toast != "" {
		t.Errorf("a failing background sync toasted on the entries: %q", m.toast)
	}

	// With a token it brings the new issue and says so.
	t.Setenv(gitlab.TokenEnv, "secret")
	oldDial := issues.Dial
	issues.Dial = func(host, token string) issues.Source { return tuiGitLab{} }
	t.Cleanup(func() { issues.Dial = oldDial })
	m.iss.lastTry = time.Time{}
	runTick(m)
	if !strings.Contains(m.toast, "1 nové issue") {
		t.Errorf("toast %q", m.toast)
	}
	m.toast = "" // the toast expired: the status line counts the new issue
	if !strings.Contains(screen(m), "1 nové issue (T)") {
		t.Errorf("status line:\n%s", screen(m))
	}
	// Not due again right after.
	runTick(m)
	if m.toast != "" {
		t.Errorf("synced again at once: %q", m.toast)
	}
}
