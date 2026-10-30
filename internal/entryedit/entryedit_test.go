package entryedit

import (
	"slices"
	"strings"
	"testing"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/period"
	"github.com/just-tomdo/housenka/internal/rules"
)

const testYAML = `
gitlab_hosts: [gitlab.example.com]
bm: { base_url: https://bm.example.com/editor, fields: { project: p, issue: i, hours: h, activity: a, date: d, description: t } }
activities:
  DEV:     { name: Vývoj, short: Kód, productive: true }
  ANALYZA: { name: Analýza, productive: true }
  REZIE:   { name: Režie, productive: false }
projects:
  SHOPDEV: { name: Shop rozvoj, activities: [DEV, ANALYZA], default_activity: ANALYZA }
  SHOPSUP: { name: Shop podpora, activities: [REZIE] }
  INTERNI: { name: Interní, issue: optional }
repos:
  clients/shop-web: [SHOPDEV, SHOPSUP]
`

const issue = "https://gitlab.example.com/clients/shop-web/-/issues/12"

var today = civil.Date{Year: 2026, Month: 10, Day: 19}

func newRules(t *testing.T) *rules.Rules {
	t.Helper()
	cfg, err := config.Parse([]byte(testYAML))
	if err != nil {
		t.Fatal(err)
	}
	return rules.New(cfg)
}

func TestParse(t *testing.T) {
	r := newRules(t)
	base := entry.Entry{ID: "x1", Sent: true}
	in := Input{Date: "dnes", Issue: "  " + issue + " ", Project: "SHOPDEV", Activity: "DEV",
		Hours: "2,75", Description: "práce  \n\n"}
	e, errs, warns := Parse(r, base, in, today)
	if len(errs) > 0 || len(warns) > 0 {
		t.Fatalf("errs = %v, warns = %v", errs, warns)
	}
	want := entry.Entry{ID: "x1", Sent: true, Date: today, IssueURL: issue, ProjectCode: "SHOPDEV",
		Activity: "DEV", Quarters: 11, Description: "práce"}
	if e != want {
		t.Errorf("entry = %+v, want %+v", e, want)
	}
}

func TestParseErrors(t *testing.T) {
	r := newRules(t)
	in := Input{Date: "zítra?", Issue: issue, Project: "SHOPDEV", Activity: "DEV", Hours: "abc"}
	_, errs, _ := Parse(r, entry.Entry{}, in, today)
	for _, f := range []rules.Field{rules.FieldDate, rules.FieldHours, rules.FieldDescription} {
		if errs[f] == "" {
			t.Errorf("missing error for %s in %v", f, errs)
		}
	}
	// The parse error wins over the validation finding of the same field.
	if strings.Contains(errs[rules.FieldHours], "0") {
		t.Errorf("hours error = %q, want the parse error", errs[rules.FieldHours])
	}
}

func TestOptions(t *testing.T) {
	r := newRules(t)
	c := Options(r, issue, "", "")
	if !slices.Equal(c.Projects, []string{"SHOPDEV", "SHOPSUP"}) || c.Project != "" || c.Activities != nil {
		t.Errorf("two projects: %+v", c)
	}
	c = Options(r, issue, "SHOPDEV", "")
	if c.Project != "SHOPDEV" || c.Activity != "ANALYZA" || !slices.Equal(c.Activities, []string{"DEV", "ANALYZA"}) {
		t.Errorf("default activity: %+v", c)
	}
	c = Options(r, issue, "SHOPDEV", "DEV")
	if c.Activity != "DEV" {
		t.Errorf("kept activity: %+v", c)
	}
	c = Options(r, "", "SHOPDEV", "DEV")
	if c.Project != "INTERNI" || c.IssueErr != nil {
		t.Errorf("no issue: %+v", c)
	}
	c = Options(r, "https://elsewhere.example.org/a/b/-/issues/1", "", "")
	if c.IssueErr == nil {
		t.Errorf("unknown host: %+v", c)
	}
}

func TestPrepareSave(t *testing.T) {
	old := entry.Entry{ID: "x", Date: today, ProjectCode: "SHOPDEV", Quarters: 4, Sent: true}
	if e, resent := PrepareSave(old, old); resent || !e.Sent {
		t.Errorf("unchanged: resent = %v, sent = %v", resent, e.Sent)
	}
	changed := old
	changed.Quarters = 8
	if e, resent := PrepareSave(old, changed); !resent || e.Sent {
		t.Errorf("changed: resent = %v, sent = %v", resent, e.Sent)
	}
	unsent := changed
	unsent.Sent = false
	if _, resent := PrepareSave(old, unsent); resent {
		t.Error("unsent entry reported as resent")
	}
}

func TestCopyOf(t *testing.T) {
	e := entry.Entry{ID: "x", Sent: true, IssueURL: issue, ProjectCode: "SHOPDEV", Activity: "DEV", Quarters: 3, Description: "d"}
	c := CopyOf(e, today)
	if c.ID != "" || c.Sent || c.Date != today || c.Quarters != 3 || c.IssueURL != issue {
		t.Errorf("copy = %+v", c)
	}
}

func TestMatches(t *testing.T) {
	cfg, err := config.Parse([]byte(testYAML))
	if err != nil {
		t.Fatal(err)
	}
	e := entry.Entry{Description: "Oprava košíku", ProjectCode: "SHOPDEV", Activity: "DEV"}
	for q, want := range map[string]bool{"": true, "  ": true, "KOŠÍK": true, "shopdev": true, "dev": true, "kód": true, "faktura": false} {
		if got := Matches(cfg, e, q); got != want {
			t.Errorf("Matches(%q) = %v", q, got)
		}
	}
}

func TestPeriodGoal(t *testing.T) {
	cfg, err := config.Parse([]byte(testYAML))
	if err != nil {
		t.Fatal(err)
	}
	daily := int(cfg.Time.DailyGoal)
	if got := PeriodGoal(cfg, period.Of(period.Day, today)); got != daily {
		t.Errorf("day = %d", got)
	}
	if got := PeriodGoal(cfg, period.Of(period.Week, today)); got != 5*daily {
		t.Errorf("week = %d", got)
	}
	// October 2026 has 22 working days.
	if got := PeriodGoal(cfg, period.Of(period.Month, today)); got != 22*daily {
		t.Errorf("month = %d", got)
	}
}

func TestBMURL(t *testing.T) {
	r := newRules(t)
	e := entry.Entry{Date: today, IssueURL: issue, ProjectCode: "SHOPDEV", Activity: "DEV", Quarters: 4, Description: "d"}
	u, err := BMURL(r.Config(), r.Validate(e, today), e)
	if err != nil || !strings.HasPrefix(u, "https://bm.example.com/editor?") {
		t.Errorf("valid: %q, %v", u, err)
	}
	e.ProjectCode = ""
	if _, err := BMURL(r.Config(), r.Validate(e, today), e); err == nil {
		t.Error("invalid entry got a link")
	}
}
