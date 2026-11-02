package web

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/configedit"
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

// ipost sends an Issues settings change with the hash of issues.yaml.
func (f *fixture) ipost(target string, form url.Values) string {
	f.t.Helper()
	data, _ := os.ReadFile(filepath.Join(os.Getenv(config.HomeEnv), "issues.yaml"))
	form.Set("ihash", configedit.Hash(data))
	body, _ := f.post(target, form)
	return body
}

func (f *fixture) icfg() *issues.Config {
	f.t.Helper()
	cfg, err := issues.LoadConfig(filepath.Join(os.Getenv(config.HomeEnv), "issues.yaml"))
	if err != nil {
		f.t.Fatal(err)
	}
	return cfg
}

func statusCodes(cfg *issues.Config) string {
	var out []string
	for _, s := range cfg.Statuses {
		out = append(out, s.Code)
	}
	return strings.Join(out, ",")
}

func TestIssueSettingsOnOff(t *testing.T) {
	f := newFixture(t)
	contains(t, f.get("/settings/issues"), "Přehled issues je vypnutý", "/settings/issues/enable")

	f.post("/settings/issues/enable", url.Values{})
	contains(t, f.get("/settings/issues"), "Na řadě", "Token zatím chybí", "issues.yaml jako text")
	contains(t, f.get("/"), `href="/issues"`)

	f.ipost("/settings/issues/status", url.Values{"code": {"review"}, "name": {"Review"}, "color": {"teal"}, "watch": {"on"}})
	f.post("/settings/issues/disable", url.Values{})
	lacks(t, f.get("/"), `href="/issues"`)
	f.post("/settings/issues/enable", url.Values{})
	if got := statusCodes(f.icfg()); !strings.HasSuffix(got, ",review") {
		t.Errorf("enabling again must bring the old file back, got %s", got)
	}
}

func TestIssueSettingsStatuses(t *testing.T) {
	f := newFixture(t)
	st := f.enableIssues(testIssue("clients/shop-web", 12, "Košík"), testIssue("clients/shop-web", 13, "Filtr"))
	st.SetStatus("clients/shop-web#12", "todo", "")
	st.SetStatus("clients/shop-web#13", "doing", "")

	f.ipost("/settings/issues/general", url.Values{"host": {"GitLab.Example.com"}, "sync_minutes": {"5"}, "stale_days": {"10"}})
	if c := f.icfg(); c.Host != "gitlab.example.com" || c.SyncMinutes != 5 || c.StaleDays != 10 {
		t.Errorf("general %+v", c)
	}
	body := f.ipost("/settings/issues/general", url.Values{"host": {""}, "sync_minutes": {"x"}, "stale_days": {"10"}})
	contains(t, body, "musí být celé číslo")

	contains(t, f.get("/settings/issues/status-form?code=todo"), `value="todo"`, `name="ihash"`)
	f.ipost("/settings/issues/status", url.Values{"old": {"todo"}, "code": {"next"}, "name": {"Další"}, "color": {"custom"}, "custom": {"#112233"}})
	f.ipost("/settings/issues/status/move", url.Values{"code": {"done"}, "delta": {"-5"}})
	if got := statusCodes(f.icfg()); got != "done,next,doing" {
		t.Errorf("statuses %s", got)
	}
	fl, _ := st.Load()
	if fl.Issues[0].Status != "next" {
		t.Errorf("renaming a status must move its issues, got %q", fl.Issues[0].Status)
	}
	f.ipost("/settings/issues/status/delete", url.Values{"code": {"doing"}})
	fl, _ = st.Load()
	if fl.Issues[1].Status != "" {
		t.Errorf("deleting a status must return its issues among the new ones, got %q", fl.Issues[1].Status)
	}
	contains(t, f.ipost("/settings/issues/status", url.Values{"code": {"next"}}), "už existuje")

	data, _ := os.ReadFile(filepath.Join(os.Getenv(config.HomeEnv), "issues.yaml"))
	contains(t, string(data), "statuses:", `color: "#112233"`)
}

func TestIssueSettingsToken(t *testing.T) {
	f := newFixture(t)
	f.enableIssues()
	t.Setenv(gitlab.TokenEnv, "")
	old := issues.Dial
	issues.Dial = func(host, token string) issues.Source {
		if token == "bad" {
			return badSource{}
		}
		return fakeSource{}
	}
	t.Cleanup(func() { issues.Dial = old })
	home := os.Getenv(config.HomeEnv)

	contains(t, f.ipost("/settings/issues/token", url.Values{"token": {"bad"}}), "odmítl token")
	if _, err := gitlab.Token(home); err == nil {
		t.Error("a refused token was saved")
	}
	body := f.ipost("/settings/issues/token", url.Values{"token": {"glpat-secret"}})
	contains(t, body, "@jnovak", "Token je uložený")
	lacks(t, body, "glpat-secret")
	if tok, _ := gitlab.Token(home); tok != "glpat-secret" {
		t.Errorf("token %q", tok)
	}
	lacks(t, f.get("/settings/issues"), "glpat-secret")
	f.ipost("/settings/issues/token", url.Values{"remove": {"1"}})
	if _, err := gitlab.Token(home); err == nil {
		t.Error("token not removed")
	}
}

type badSource struct{ fakeSource }

func (badSource) CurrentUser(context.Context) (gitlab.User, error) {
	return gitlab.User{}, gitlab.ErrUnauthorized
}

func TestIssueSettingsYAML(t *testing.T) {
	f := newFixture(t)
	f.enableIssues()
	contains(t, f.ipost("/settings/issues/yaml", url.Values{"yaml": {"statuses: []\n"}, "action": {"save"}}), "aspoň jeden stav")
	f.ipost("/settings/issues/yaml", url.Values{"yaml": {"statuses: [{code: a, name: Áčko}]\n"}, "action": {"save"}})
	if got := statusCodes(f.icfg()); got != "a" {
		t.Errorf("statuses %s", got)
	}
	// A broken file leaves only the text editor.
	os.WriteFile(filepath.Join(os.Getenv(config.HomeEnv), "issues.yaml"), []byte("statuses: []\n"), 0o644)
	page := f.get("/settings/issues")
	contains(t, page, "issues.yaml obsahuje chyby", "issues.yaml jako text")
	lacks(t, page, "Token GitLabu")
}

func TestRepoFormFromIssue(t *testing.T) {
	f := newFixture(t)
	contains(t, f.get("/settings/repos/form?repo="+url.QueryEscape("https://gitlab.example.com/other/tool")), `value="https://gitlab.example.com/other/tool"`, "other/tool")
	contains(t, f.get("/settings/repos?add="+url.QueryEscape("https://gitlab.example.com/other/tool")), `hx-trigger="load"`)
}
