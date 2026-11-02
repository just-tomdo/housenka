package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/gitlab"
	"github.com/just-tomdo/housenka/internal/issues"
)

func init() {
	b, err := os.ReadFile("../../issues.example.yaml")
	if err != nil {
		panic(err)
	}
	issues.ExampleConfig = b
}

// fakeGitLab answers like a GitLab with one issue assigned to the user;
// the token "bad" is refused.
type fakeGitLab struct{ token string }

func (g fakeGitLab) CurrentUser(context.Context) (gitlab.User, error) {
	if g.token == "bad" {
		return gitlab.User{}, gitlab.ErrUnauthorized
	}
	return gitlab.User{Username: "jnovak", Name: "Jan Novák"}, nil
}

func (g fakeGitLab) AssignedIssues(context.Context) ([]gitlab.Issue, error) {
	return []gitlab.Issue{{IID: 5, Title: "Košík", State: "opened", WebURL: "https://gitlab.example.com/clients/shop-web/-/issues/5"}}, nil
}

func (g fakeGitLab) Issue(context.Context, string, int) (gitlab.Issue, error) {
	return gitlab.Issue{}, gitlab.ErrNotFound
}

func (g fakeGitLab) AssignedAt(context.Context, string, int, string) (time.Time, error) {
	return time.Time{}, nil
}

func fakeDial(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	old := issues.Dial
	issues.Dial = func(host, token string) issues.Source {
		calls = append(calls, host+" "+token)
		return fakeGitLab{token}
	}
	t.Cleanup(func() { issues.Dial = old })
	t.Setenv(gitlab.TokenEnv, "")
	return &calls
}

func TestIssuesCommands(t *testing.T) {
	home := setHome(t)
	calls := fakeDial(t)
	if code, _, errOut := run_(t, "config", "init"); code != 0 {
		t.Fatal(errOut)
	}

	if _, out, _ := run_(t, "issues"); !strings.Contains(out, "vypnutý") {
		t.Errorf("disabled: %q", out)
	}
	if code, _, errOut := run_(t, "issues", "sync"); code != 1 || !strings.Contains(errOut, "issues init") {
		t.Errorf("sync while disabled: %d %q", code, errOut)
	}
	if _, out, _ := run_(t, "config", "path"); !strings.Contains(out, "issues.yaml (vypnuto)") {
		t.Errorf("config path: %q", out)
	}

	code, out, errOut := run_(t, "issues", "init")
	if code != 0 || !strings.Contains(out, "issues token") {
		t.Fatalf("init: %d %q %q", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(home, "issues.yaml")); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run_(t, "issues", "init"); code != 1 || !strings.Contains(errOut, "--force") {
		t.Errorf("second init: %d %q", code, errOut)
	}
	if code, out, _ := run_(t, "config", "check"); code != 0 || !strings.Contains(out, "Přehled issues") || !strings.Contains(out, "4 stavy") {
		t.Errorf("config check: %d %q", code, out)
	}

	if code, _, errOut := run_(t, "issues", "sync"); code != 1 || !strings.Contains(errOut, "chybí token") {
		t.Errorf("sync without token: %d %q", code, errOut)
	}

	setStdin(t, "bad\n")
	if code, _, errOut := run_(t, "issues", "token"); code != 1 || !strings.Contains(errOut, "odmítl token") {
		t.Errorf("bad token: %d %q", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(home, "gitlab-token")); !os.IsNotExist(err) {
		t.Error("a refused token was saved")
	}
	setStdin(t, "glpat-good\n")
	if code, out, errOut := run_(t, "issues", "token"); code != 0 || !strings.Contains(out, "@jnovak") {
		t.Errorf("good token: %d %q %q", code, out, errOut)
	}

	code, out, errOut = run_(t, "issues", "sync")
	if code != 0 || !strings.Contains(out, "1 nové issue") || !strings.Contains(out, "Čeká na zařazení: 1") {
		t.Errorf("sync: %d %q %q", code, out, errOut)
	}
	if last := (*calls)[len(*calls)-1]; last != "gitlab.example.com glpat-good" {
		t.Errorf("dialed %q", last)
	}
	if _, out, _ := run_(t, "issues"); !strings.Contains(out, "Nová: 1") || !strings.Contains(out, "Na řadě: 0") || !strings.Contains(out, "Načteno z GitLabu") {
		t.Errorf("status: %q", out)
	}

	os.WriteFile(filepath.Join(home, "issues.yaml"), []byte("statuses: []\n"), 0o644)
	if code, _, errOut := run_(t, "config", "check"); code != 1 || !strings.Contains(errOut, "aspoň jeden stav") {
		t.Errorf("broken issues.yaml: %d %q", code, errOut)
	}
	if code, _, errOut := run_(t, "issues", "bogus"); code != 2 || !strings.Contains(errOut, issuesUsage) {
		t.Errorf("usage: %d %q", code, errOut)
	}
}

func TestIssuesEdit(t *testing.T) {
	home := setHome(t)
	path := filepath.Join(home, "issues.yaml")
	fakeEditor(t, func(s string) string { return strings.Replace(s, "stale_days: 14", "stale_days: 0", 1) },
		func(s string) string { return strings.Replace(s, "stale_days: 0", "stale_days: 7", 1) })
	setStdin(t, "\n")
	code, out, errOut := run_(t, "issues", "edit")
	if code != 0 || !strings.Contains(out, "neexistoval") || !strings.Contains(errOut, "stale_days") || !strings.Contains(out, "je v pořádku") {
		t.Errorf("edit: %d %q %q", code, out, errOut)
	}
	cfg, err := issues.LoadConfig(path)
	if err != nil || cfg.StaleDays != 7 {
		t.Errorf("edited file: %+v %v", cfg, err)
	}
}
