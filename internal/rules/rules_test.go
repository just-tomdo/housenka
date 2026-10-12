package rules

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
)

const testYAML = `
gitlab_hosts: [gitlab.example.com]
bm: { base_url: https://bm.example.com/vykazy/editor, fields: { project: project, issue: issue, hours: hours, activity: activity, date: date, description: description } }
activities:
  DEV:  { name: Vývoj, productive: true }
  ANALYZA: { name: Analýza, productive: true }
  REZIE:   { name: Režie, productive: false }
projects:
  BETADEV: { name: Beta rozvoj, activities: [DEV, ANALYZA], default_activity: ANALYZA }
  BETASUP:   { name: Beta podpora, activities: [REZIE] }
  ACME:  { name: Acme }
  INT01:  { name: Interní }
  INTSHOP: { name: Interní shop }
  INTERNI: { name: Interní, issue: optional }
repos:
  clients/beta-app: [BETADEV, BETASUP]
  clients/acme-web: [ACME]
  interni/*: [INT01]
  interni/shop/*: [INTSHOP]
  interni/shop/special: [ACME]
`

func newRules(t *testing.T) *Rules {
	t.Helper()
	cfg, err := config.Parse([]byte(testYAML))
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg)
}

func TestProjectsForRepo(t *testing.T) {
	r := newRules(t)
	tests := []struct {
		path    string
		want    []string
		wantErr bool
	}{
		{"clients/beta-app", []string{"BETADEV", "BETASUP"}, false},
		{"CLIENTS/BETA-APP", []string{"BETADEV", "BETASUP"}, false},
		{"clients/acme-web", []string{"ACME"}, false},
		{"interni/cms", []string{"INT01"}, false},
		{"interni/backend/api", []string{"INT01"}, false},
		{"interni/shop/core", []string{"INTSHOP"}, false}, // longer wildcard wins
		{"interni/shop/special", []string{"ACME"}, false}, // exact wins over wildcard
		{"internix/cms", nil, true},                       // prefix must end at a segment
		{"clients/beta-app.old", nil, true},
		{"clients/unknown", nil, true},
	}
	for _, tt := range tests {
		got, err := r.ProjectsForRepo(tt.path)
		if (err != nil) != tt.wantErr {
			t.Errorf("ProjectsForRepo(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			continue
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("ProjectsForRepo(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestAllowedProjects(t *testing.T) {
	r := newRules(t)

	codes, ref, err := r.AllowedProjects("https://gitlab.example.com/clients/beta-app/-/work_items/12#note_1")
	if err != nil {
		t.Fatal(err)
	}
	if ref == nil || ref.Short() != "clients / beta-app / #12" {
		t.Errorf("ref = %+v", ref)
	}
	if !slices.Equal(codes, []string{"BETADEV", "BETASUP"}) {
		t.Errorf("codes = %v", codes)
	}

	codes, ref, err = r.AllowedProjects("  ")
	if err != nil || ref != nil || !slices.Equal(codes, []string{"INTERNI"}) {
		t.Errorf("empty issue: codes=%v ref=%v err=%v", codes, ref, err)
	}

	if _, _, err := r.AllowedProjects("https://github.com/clients/beta-app/-/issues/1"); err == nil || !strings.Contains(err.Error(), "github.com") {
		t.Errorf("foreign host error = %v", err)
	}

	_, ref, err = r.AllowedProjects("https://gitlab.example.com/clients/neznamy/-/issues/1")
	if err == nil || ref == nil {
		t.Errorf("unmapped repo: ref=%v err=%v", ref, err)
	}
}

func TestActivities(t *testing.T) {
	r := newRules(t)
	if got := r.AllowedActivities("BETADEV"); !slices.Equal(got, []string{"DEV", "ANALYZA"}) {
		t.Errorf("BETADEV activities = %v", got)
	}
	if got := r.AllowedActivities("ACME"); !slices.Equal(got, []string{"ANALYZA", "DEV", "REZIE"}) {
		t.Errorf("ACME activities = %v", got)
	}
	if got := r.DefaultActivity("BETADEV"); got != "ANALYZA" {
		t.Errorf("default BETADEV = %q", got)
	}
	if got := r.DefaultActivity("BETASUP"); got != "REZIE" {
		t.Errorf("default single = %q", got)
	}
	if got := r.DefaultActivity("ACME"); got != "" {
		t.Errorf("default none = %q", got)
	}
}

func TestValidate(t *testing.T) {
	r := newRules(t)
	today := civil.Date{Year: 2026, Month: time.October, Day: 12}
	valid := entry.Entry{
		Date:        today,
		IssueURL:    "https://gitlab.example.com/clients/beta-app/-/work_items/12",
		ProjectCode: "BETADEV",
		Activity:    "DEV",
		Quarters:    11,
		Description: "úprava formuláře",
	}

	tests := []struct {
		name      string
		mutate    func(*entry.Entry)
		wantField Field // "" = expect valid
		wantWarn  bool
	}{
		{"valid", func(*entry.Entry) {}, "", false},
		{"project not for repo", func(e *entry.Entry) { e.ProjectCode = "ACME" }, FieldProject, false},
		{"unknown project", func(e *entry.Entry) { e.ProjectCode = "XXX" }, FieldProject, false},
		{"activity not allowed", func(e *entry.Entry) { e.Activity = "REZIE" }, FieldActivity, false},
		{"unknown activity", func(e *entry.Entry) { e.Activity = "NOPE" }, FieldActivity, false},
		{"zero hours", func(e *entry.Entry) { e.Quarters = 0 }, FieldHours, false},
		{"too many hours", func(e *entry.Entry) { e.Quarters = 49 }, FieldHours, false},
		{"empty description", func(e *entry.Entry) { e.Description = "  " }, FieldDescription, false},
		{"missing issue", func(e *entry.Entry) { e.IssueURL = "" }, FieldIssue, false},
		{"bad issue url", func(e *entry.Entry) { e.IssueURL = "https://gitlab.example.com/clients/beta-app" }, FieldIssue, false},
		{"unmapped repo", func(e *entry.Entry) { e.IssueURL = "https://gitlab.example.com/clients/x/-/issues/1" }, FieldIssue, false},
		{"invalid date", func(e *entry.Entry) { e.Date = civil.Date{} }, FieldDate, false},
		{"future date warns", func(e *entry.Entry) { e.Date = today.AddDays(1) }, FieldDate, true},
		{"optional issue project without issue", func(e *entry.Entry) {
			e.IssueURL, e.ProjectCode, e.Activity = "", "INTERNI", "REZIE"
		}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := valid
			tt.mutate(&e)
			res := r.Validate(e, today)
			if tt.wantField == "" {
				if len(res.Findings) != 0 {
					t.Fatalf("expected no findings, got %+v", res.Findings)
				}
				return
			}
			if res.Valid() != tt.wantWarn {
				t.Errorf("Valid() = %v, want %v (findings %+v)", res.Valid(), tt.wantWarn, res.Findings)
			}
			if !slices.ContainsFunc(res.Findings, func(f Finding) bool { return f.Field == tt.wantField }) {
				t.Errorf("no finding for field %s: %+v", tt.wantField, res.Findings)
			}
		})
	}
}

func TestValidateReturnsParsedIssue(t *testing.T) {
	r := newRules(t)
	res := r.Validate(entry.Entry{IssueURL: "https://gitlab.example.com/clients/acme-web/-/work_items/636"}, civil.Today())
	if res.Issue == nil || res.Issue.IID != 636 || res.Issue.Repo != "acme-web" {
		t.Errorf("Issue = %+v", res.Issue)
	}
}
