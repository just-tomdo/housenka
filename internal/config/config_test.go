package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validYAML = `
gitlab_hosts: [gitlab.example.com]

bm:
  base_url: https://bm.example.com/editor
  fields: { project: project, issue: issue, hours: hours, activity: activity, date: date, description: description }

ui:
  theme: gruvbox

time:
  daily_goal_hours: 7.5

activities:
  DEV:  { name: Vývoj, productive: true }
  ANALYZA: { name: Analýza, productive: true }
  REZIE:   { name: Režie, productive: false }

projects:
  BETADEV:
    name: Beta – rozvoj
    activities: [DEV, ANALYZA]
    default_activity: DEV
  BETASUP:
    name: Beta – podpora
  INTERNI:
    name: Interní
    issue: optional

repos:
  clients/beta-app: [BETADEV, BETASUP]
  interni/*: [INTERNI]
`

func TestParseValid(t *testing.T) {
	cfg, err := Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UI.Theme != "gruvbox" {
		t.Errorf("theme = %q", cfg.UI.Theme)
	}
	// Defaults survive a partially specified section.
	if !cfg.UI.Caterpillar {
		t.Error("ui.caterpillar default lost")
	}
	for _, c := range []string{"blue", "#7aa2f7"} {
		if _, err := Parse([]byte(strings.Replace(validYAML, "issue: optional", "issue: optional\n    color: \""+c+"\"", 1))); err != nil {
			t.Errorf("color %q rejected: %v", c, err)
		}
	}
	if cfg.UI.DateFormat != "d. m. yyyy" {
		t.Errorf("ui.date_format default = %q", cfg.UI.DateFormat)
	}
	if cfg.Time.DailyGoal != 30 {
		t.Errorf("daily goal = %d quarters, want 30", cfg.Time.DailyGoal)
	}
	if cfg.Time.MaxEntry != 48 {
		t.Errorf("max entry default = %d, want 48", cfg.Time.MaxEntry)
	}
	if !cfg.Activities["DEV"].IsProductive() || cfg.Activities["REZIE"].IsProductive() {
		t.Error("productive flags wrong")
	}
	if !cfg.Projects["INTERNI"].IssueOptional() || cfg.Projects["BETASUP"].IssueOptional() {
		t.Error("issue policy wrong")
	}
	if got := strings.Join(cfg.ProjectCodes(), ","); got != "BETADEV,BETASUP,INTERNI" {
		t.Errorf("ProjectCodes = %s", got)
	}
}

func TestActivityLabel(t *testing.T) {
	cfg, err := Parse([]byte(strings.Replace(validYAML, "{ name: Analýza, productive: true }", "{ name: Analýza, short: AN, productive: true }", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cfg.ActivityLabels([]string{"ANALYZA", "DEV", "UNKNOWN"}), ","); got != "AN,DEV,UNKNOWN" {
		t.Errorf("ActivityLabels = %s", got)
	}
}

func TestWebDefaults(t *testing.T) {
	cfg, err := Parse([]byte(validYAML))
	if err != nil {
		t.Fatal(err)
	}
	w := cfg.Web
	if w.Port != DefaultWebPort || len(w.Hostnames) != 1 || w.Hostnames[0] != "housenka.localhost" || !w.OpenBrowser || w.Theme != "auto" {
		t.Errorf("defaults = %+v", w)
	}
	cfg, err = Parse([]byte(validYAML + "web:\n  port: 9000\n  hostnames: [housenka.local]\n  open_browser: false\n  theme: gruvbox-light\n"))
	if err != nil {
		t.Fatal(err)
	}
	w = cfg.Web
	if w.Port != 9000 || w.Hostnames[0] != "housenka.local" || w.OpenBrowser || w.Theme != "gruvbox-light" {
		t.Errorf("configured = %+v", w)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(string) string
		wantMsg string
	}{
		{
			name:    "typo in key",
			mutate:  func(s string) string { return strings.Replace(s, "default_activity:", "default_activty:", 1) },
			wantMsg: "neznámý klíč „default_activty“",
		},
		{
			name:    "unknown project code in repos",
			mutate:  func(s string) string { return strings.Replace(s, "[BETADEV, BETASUP]", "[BETADEV, BETAOLD]", 1) },
			wantMsg: `repos.clients/beta-app: kód projektu "BETAOLD" není v sekci projects`,
		},
		{
			name: "missing productive",
			mutate: func(s string) string {
				return strings.Replace(s, "{ name: Režie, productive: false }", "{ name: Režie }", 1)
			},
			wantMsg: "activities.REZIE.productive: chybí",
		},
		{
			name: "short label used twice",
			mutate: func(s string) string {
				s = strings.Replace(s, "{ name: Analýza, productive: true }", "{ name: Analýza, short: Návrh, productive: true }", 1)
				return strings.Replace(s, "{ name: Režie, productive: false }", "{ name: Režie, short: Návrh, productive: false }", 1)
			},
			wantMsg: `activities.REZIE.short: stejný popisek "Návrh" má i činnost ANALYZA`,
		},
		{
			name: "short label equal to another code",
			mutate: func(s string) string {
				return strings.Replace(s, "{ name: Analýza, productive: true }", "{ name: Analýza, short: DEV, productive: true }", 1)
			},
			wantMsg: `activities.ANALYZA.short: stejný popisek "DEV" má i činnost DEV`,
		},
		{
			name:    "unknown activity in project",
			mutate:  func(s string) string { return strings.Replace(s, "[DEV, ANALYZA]", "[DEV, TEST]", 1) },
			wantMsg: `projects.BETADEV.activities: činnost "TEST" není v sekci activities`,
		},
		{
			name: "default activity not allowed",
			mutate: func(s string) string {
				return strings.Replace(s, "default_activity: DEV", "default_activity: REZIE", 1)
			},
			wantMsg: "není mezi povolenými činnostmi projektu",
		},
		{
			name:    "bad issue policy",
			mutate:  func(s string) string { return strings.Replace(s, "issue: optional", "issue: maybe", 1) },
			wantMsg: `projects.INTERNI.issue: neplatná hodnota "maybe"`,
		},
		{
			name:    "bad repo wildcard",
			mutate:  func(s string) string { return strings.Replace(s, "interni/*:", "interni/*/x:", 1) },
			wantMsg: "hvězdička je povolená jen na konci",
		},
		{
			name:    "repo without group",
			mutate:  func(s string) string { return strings.Replace(s, "clients/beta-app:", "beta-app:", 1) },
			wantMsg: "přesná cesta musí obsahovat skupinu i repozitář",
		},
		{
			name:    "case-insensitive duplicate repo",
			mutate:  func(s string) string { return s + "  CLIENTS/beta-app: [BETASUP]\n" },
			wantMsg: "duplicitní",
		},
		{
			name:    "duplicate yaml key",
			mutate:  func(s string) string { return s + "  clients/beta-app: [BETASUP]\n" },
			wantMsg: "already defined",
		},
		{
			name:    "time not in quarters",
			mutate:  func(s string) string { return strings.Replace(s, "daily_goal_hours: 7.5", "daily_goal_hours: 7.3", 1) },
			wantMsg: "není po čtvrthodinách",
		},
		{
			name:    "unknown theme",
			mutate:  func(s string) string { return strings.Replace(s, "theme: gruvbox", "theme: neon", 1) },
			wantMsg: `ui.theme: neznámé téma "neon"`,
		},
		{
			name: "unquoted hex color",
			mutate: func(s string) string {
				return strings.Replace(s, "issue: optional", "issue: optional\n    color: #e06c75", 1)
			},
			wantMsg: "barvu zapiš v uvozovkách",
		},
		{
			name: "unknown project color",
			mutate: func(s string) string {
				return strings.Replace(s, "issue: optional", "issue: optional\n    color: rdeč", 1)
			},
			wantMsg: `color: neznámá barva "rdeč"`,
		},
		{
			name: "bad date format",
			mutate: func(s string) string {
				return strings.Replace(s, "theme: gruvbox", "theme: gruvbox\n  date_format: dd.mm", 1)
			},
			wantMsg: `ui.date_format: "dd.mm"`,
		},
		{
			name: "host with scheme",
			mutate: func(s string) string {
				return strings.Replace(s, "[gitlab.example.com]", "[https://gitlab.example.com]", 1)
			},
			wantMsg: "gitlab_hosts[0]",
		},
		{
			name:    "missing bm field",
			mutate:  func(s string) string { return strings.Replace(s, "hours: hours, ", "", 1) },
			wantMsg: "bm.fields.hours: chybí název pole formuláře",
		},
		{
			name:    "duplicate bm field",
			mutate:  func(s string) string { return strings.Replace(s, "date: date", "date: hours", 1) },
			wantMsg: `bm.fields.date: název "hours" už používá bm.fields.hours`,
		},
		{
			name:    "bad web port",
			mutate:  func(s string) string { return s + "web: { port: 70000 }\n" },
			wantMsg: "web.port: 70000 není platný port",
		},
		{
			name:    "web hostname with scheme",
			mutate:  func(s string) string { return s + "web: { hostnames: [\"http://housenka.local\"] }\n" },
			wantMsg: "web.hostnames[0]",
		},
		{
			name:    "bad web theme",
			mutate:  func(s string) string { return s + "web: { theme: Gruvbox Light }\n" },
			wantMsg: "web.theme",
		},
		{
			name:    "empty",
			mutate:  func(string) string { return "" },
			wantMsg: "config je prázdný",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.mutate(validYAML)))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err, tt.wantMsg)
			}
		})
	}
}

func TestValidateReportsAllProblems(t *testing.T) {
	cfg := Default()
	err := cfg.Validate()
	p, ok := err.(Problems)
	if !ok {
		t.Fatalf("expected Problems, got %T: %v", err, err)
	}
	// gitlab_hosts, activities and projects are all missing.
	if len(p) < 3 {
		t.Errorf("expected at least 3 problems, got %d: %v", len(p), p)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil || !strings.Contains(err.Error(), "housenka config init") {
		t.Errorf("expected hint about config init, got %v", err)
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(validYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
}

func TestWarnings(t *testing.T) {
	cfg, err := Parse([]byte(validYAML + "\n" + "  # end\n"))
	if err != nil {
		t.Fatal(err)
	}
	if w := cfg.Warnings(); len(w) != 0 {
		t.Errorf("unexpected warnings: %v", w)
	}
	cfg.Projects["ORPHAN"] = Project{Name: "Sirotek"}
	w := cfg.Warnings()
	if len(w) != 1 || !strings.Contains(w[0], "projects.ORPHAN") {
		t.Errorf("warnings = %v", w)
	}
}
