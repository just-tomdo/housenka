package configedit

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
)

const testYAML = `# Test config
gitlab_hosts: [gitlab.example.com]

bm:
  base_url: https://bm.example.com/editor
  fields: { project: p, issue: i, hours: h, activity: a, date: d, description: t }

# Activities comment
activities:
  DEV:     { name: Vývoj,   productive: true }  # main work
  ANALYZA: { name: Analýza, productive: true }
  REZIE:   { name: Režie,   productive: false }

projects:
  # Shop development
  SHOPDEV:
    name: Shop rozvoj  # the big one
    activities: [DEV, ANALYZA]
    default_activity: ANALYZA
  SHOPSUP:
    name: Shop podpora
    activities: [REZIE]
  INTERNI: { name: Interní, issue: optional }

repos:
  clients/shop-web: [SHOPDEV, SHOPSUP]  # client shop
  clients/shop-api: [SHOPDEV]
`

func parse(t *testing.T) *Doc {
	t.Helper()
	d, err := Parse([]byte(testYAML))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func render(t *testing.T, d *Doc) (string, *config.Config) {
	t.Helper()
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Parse(data)
	if err != nil {
		t.Fatalf("invalid result: %v\n%s", err, data)
	}
	return string(data), cfg
}

func mustContain(t *testing.T, s string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(s, w) {
			t.Errorf("missing %q in:\n%s", w, s)
		}
	}
}

func TestRoundTripKeepsLayout(t *testing.T) {
	out, _ := render(t, parse(t))
	if out != testYAML {
		t.Errorf("round trip changed the file:\n%s", out)
	}
}

func TestSetActivity(t *testing.T) {
	d := parse(t)
	if err := d.SetActivity("", "TEST", Activity{Name: "Testování", Productive: true}); err != nil {
		t.Fatal(err)
	}
	if err := d.SetActivity("ANALYZA", "ANALYSIS", Activity{Name: "Analýza a návrh", Productive: true}); err != nil {
		t.Fatal(err)
	}
	out, cfg := render(t, d)
	mustContain(t, out, "# Activities comment", "# main work",
		"  DEV:      { name: Vývoj,           productive: true }",
		"  ANALYSIS: { name: Analýza a návrh, productive: true }",
		"  TEST:     { name: Testování,       productive: true }",
		"activities: [DEV, ANALYSIS]", "default_activity: ANALYSIS")
	if _, ok := cfg.Activities["ANALYZA"]; ok {
		t.Error("old code kept")
	}

	for _, tc := range []struct{ old, code, want string }{
		{"", "DEV", "už existuje"},
		{"DEV", "REZIE", "už existuje"},
		{"NONE", "X", "neexistuje"},
		{"", "s mezerou", "smí obsahovat"},
		{"", "", "chybí kód"},
	} {
		if err := parse(t).SetActivity(tc.old, tc.code, Activity{Name: "x"}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("SetActivity(%q, %q) = %v, want %q", tc.old, tc.code, err, tc.want)
		}
	}
}

func TestDeleteActivity(t *testing.T) {
	d := parse(t)
	if err := d.DeleteActivity("REZIE"); err == nil || !strings.Contains(err.Error(), "jediná povolená u projektu SHOPSUP") {
		t.Errorf("deleting the only activity: %v", err)
	}
	if err := d.DeleteActivity("ANALYZA"); err != nil {
		t.Fatal(err)
	}
	out, cfg := render(t, d)
	if !slices.Equal(cfg.Projects["SHOPDEV"].Activities, []string{"DEV"}) || cfg.Projects["SHOPDEV"].DefaultActivity != "" {
		t.Errorf("SHOPDEV = %+v", cfg.Projects["SHOPDEV"])
	}
	mustContain(t, out, "# Activities comment", "  DEV:   { name: Vývoj,")
}

func TestSetProject(t *testing.T) {
	d := parse(t)
	removed, err := d.SetProject("", "ACME", Project{Name: "Acme", Color: "#aa5500", Activities: []string{"DEV"}, Repos: []string{"clients/shop-api"}})
	if err != nil || removed != nil {
		t.Fatal(removed, err)
	}
	out, cfg := render(t, d)
	mustContain(t, out, "  ACME:\n    name: Acme\n    color: \"#aa5500\"\n    activities: [DEV]\n", "clients/shop-api: [SHOPDEV, ACME]")
	if cfg.Projects["ACME"].IssueOptional() {
		t.Error("issue optional")
	}

	// Rename and move between repos; the comments of the project stay.
	removed, err = d.SetProject("SHOPDEV", "SHOP", Project{Name: "Shop rozvoj", Activities: []string{"DEV", "ANALYZA"},
		DefaultActivity: "DEV", IssueOptional: true, Repos: []string{"clients/shop-web"}})
	if err != nil {
		t.Fatal(err)
	}
	out, cfg = render(t, d)
	mustContain(t, out, "# Shop development\n  SHOP:\n    name: Shop rozvoj  # the big one", "issue: optional",
		"clients/shop-web: [SHOP, SHOPSUP]     # client shop", "clients/shop-api: [ACME]")
	if len(removed) != 0 || cfg.Projects["SHOP"].DefaultActivity != "DEV" {
		t.Errorf("removed = %v, %+v", removed, cfg.Projects["SHOP"])
	}

	// Taking the last project from a rule removes the rule.
	removed, err = d.SetProject("ACME", "ACME", Project{Name: "Acme"})
	if err != nil || !slices.Equal(removed, []string{"clients/shop-api"}) {
		t.Errorf("removed = %v, %v", removed, err)
	}
	out, _ = render(t, d)
	if strings.Contains(out, "shop-api") || strings.Contains(out, "color:") {
		t.Errorf("after clearing:\n%s", out)
	}

	if _, err := parse(t).SetProject("", "SHOPSUP", Project{Name: "x"}); err == nil {
		t.Error("duplicate project accepted")
	}
	if _, err := parse(t).SetProject("", "X", Project{Name: "x", Repos: []string{"nope/*"}}); err == nil {
		t.Error("unknown repo accepted")
	}
}

func TestDeleteProject(t *testing.T) {
	d := parse(t)
	removed, err := d.DeleteProject("SHOPDEV")
	if err != nil || !slices.Equal(removed, []string{"clients/shop-api"}) {
		t.Fatalf("removed = %v, %v", removed, err)
	}
	out, cfg := render(t, d)
	if _, ok := cfg.Projects["SHOPDEV"]; ok || !slices.Equal(cfg.Repos["clients/shop-web"], []string{"SHOPSUP"}) {
		t.Errorf("after delete: %+v", cfg.Repos)
	}
	// The section comment of the removed first project moves to the next one.
	mustContain(t, out, "projects:\n  # Shop development\n  SHOPSUP:")
}

func TestRepos(t *testing.T) {
	d := parse(t)
	if err := d.SetRepo("", "interni/*", []string{"INTERNI"}); err != nil {
		t.Fatal(err)
	}
	if err := d.SetRepo("clients/shop-api", "clients/shop-backend", []string{"SHOPDEV", "SHOPSUP"}); err != nil {
		t.Fatal(err)
	}
	out, cfg := render(t, d)
	mustContain(t, out, "  clients/shop-backend: [SHOPDEV, SHOPSUP]", "  interni/*: [INTERNI]")
	if _, ok := cfg.Repos["clients/shop-api"]; ok {
		t.Error("old pattern kept")
	}
	if err := d.SetRepo("", "Clients/Shop-Web", []string{"SHOPDEV"}); err == nil || !strings.Contains(err.Error(), "už existuje") {
		t.Errorf("case-insensitive duplicate: %v", err)
	}
	if err := d.SetRepo("", "x/y", nil); err == nil {
		t.Error("rule without projects accepted")
	}
	if err := d.DeleteRepo("interni/*"); err != nil {
		t.Fatal(err)
	}
	if _, cfg := render(t, d); len(cfg.Repos) != 2 {
		t.Errorf("repos = %v", cfg.Repos)
	}
}

func TestGeneralSettings(t *testing.T) {
	d := parse(t)
	d.AddGitLabHost("GitLab.Other.example.org")
	d.AddGitLabHost("gitlab.example.com")
	d.SetBM("https://bm.example.com/new", config.BMFields{Project: "a1", Issue: "a2", Hours: "a3", Activity: "a4", Date: "a5", Description: "a6"}, []string{"wslview"})
	d.SetTime(config.Time{DailyGoal: 30, DailyWarning: 34, MaxEntry: 48})
	d.SetUI(config.UI{Theme: "gruvbox", Caterpillar: true, DateFormat: "dd.mm.yyyy"})
	d.SetWeb(config.Web{Port: 9000, Hostnames: []string{"housenka.local"}, OpenBrowser: false, Theme: "auto"})
	out, cfg := render(t, d)
	if !slices.Equal(cfg.GitLabHosts, []string{"gitlab.example.com", "gitlab.other.example.org"}) {
		t.Errorf("hosts = %v", cfg.GitLabHosts)
	}
	if cfg.BM.Fields.Description != "a6" || cfg.BM.BaseURL != "https://bm.example.com/new" || cfg.BM.OpenCommand[0] != "wslview" {
		t.Errorf("bm = %+v", cfg.BM)
	}
	if cfg.Time.DailyGoal != 30 || cfg.Time.MaxEntry != 48 || cfg.UI.DateFormat != "dd.mm.yyyy" || cfg.Web.Port != 9000 {
		t.Errorf("cfg = %+v %+v %+v", cfg.Time, cfg.UI, cfg.Web)
	}
	mustContain(t, out, "daily_goal_hours: 7.5", "daily_warning_hours: 8.5", "max_entry_hours: 12")
}

func TestSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte(testYAML), 0o644)
	d, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	d.SetActivity("", "TEST", Activity{Name: "Test", Productive: true})
	if err := d.Save(path); err != nil {
		t.Fatal(err)
	}
	if bak, _ := os.ReadFile(path + ".bak"); string(bak) != testYAML {
		t.Error("backup missing")
	}

	// The document is stale now: saving it again conflicts.
	if err := d.Save(path); !errors.Is(err, ErrConflict) {
		t.Errorf("stale save: %v", err)
	}

	// An invalid result is not written.
	d, _ = Load(path)
	before, _ := os.ReadFile(path)
	d.SetGitLabHosts(nil)
	if err := d.Save(path); err == nil || !strings.Contains(err.Error(), "gitlab_hosts") {
		t.Errorf("invalid save: %v", err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Error("invalid config written")
	}
}

func TestBMFromLink(t *testing.T) {
	base, f, err := BMFromLink("https://bm.example.com/editor?mode=new&prj=SHOP&iss=https%3A%2F%2Fgit&hrs=2%2C5&act=DEV&dt=1.+10.+2026&txt=x")
	if err != nil {
		t.Fatal(err)
	}
	if base != "https://bm.example.com/editor?mode=new" || f != (config.BMFields{Project: "prj", Issue: "iss", Hours: "hrs", Activity: "act", Date: "dt", Description: "txt"}) {
		t.Errorf("base = %q, fields = %+v", base, f)
	}
	if _, _, err := BMFromLink("https://bm.example.com/editor?prj=SHOP&hrs=2"); err == nil || !strings.Contains(err.Error(), "jen 2 parametrů") {
		t.Errorf("short link: %v", err)
	}
	if _, _, err := BMFromLink("bm.example.com"); err == nil {
		t.Error("link without scheme accepted")
	}
}

func TestSetupAndUsage(t *testing.T) {
	example, err := config.Parse(readExample(t))
	if err != nil {
		t.Fatal(err)
	}
	if SetupDone(Setup(example, example)) {
		t.Error("example counts as set up")
	}
	_, cfg := render(t, parse(t))
	steps := Setup(cfg, example)
	if !steps[1].Done || steps[0].Done { // same GitLab host, different BM address
		t.Errorf("steps = %+v", steps)
	}

	u := CountUsage([]entry.Entry{{ProjectCode: "SHOPDEV", Activity: "DEV"}, {ProjectCode: "SHOPDEV", Activity: "REZIE"}})
	if u.Projects["SHOPDEV"] != 2 || u.Activities["DEV"] != 1 {
		t.Errorf("usage = %+v", u)
	}
	if got := ReposOf(cfg, "SHOPDEV"); !slices.Equal(got, []string{"clients/shop-api", "clients/shop-web"}) {
		t.Errorf("ReposOf = %v", got)
	}
	if got := OnlyRepoOf(cfg, "SHOPDEV"); !slices.Equal(got, []string{"clients/shop-api"}) {
		t.Errorf("OnlyRepoOf = %v", got)
	}
	if got := ProjectsOf(cfg, "REZIE"); !slices.Equal(got, []string{"SHOPSUP"}) {
		t.Errorf("ProjectsOf = %v", got)
	}
}

func TestSaveKeepsSymlinkAndMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "config.yaml")
	os.MkdirAll(filepath.Dir(target), 0o755)
	os.WriteFile(target, []byte(testYAML), 0o600)
	link := filepath.Join(dir, "config.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not available:", err)
	}
	d, _ := Load(link)
	d.SetActivity("", "TEST", Activity{Name: "Test", Productive: true})
	if err := d.Save(link); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink replaced")
	}
	for _, p := range []string{target, link + ".bak"} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("%s: mode %v, %v", p, fi.Mode().Perm(), err)
		}
	}
}
