// Package entryedit holds the entry editing logic shared by the TUI and the
// web interface: parsing form input, the allowed choices, saving rules and
// the BM link.
package entryedit

import (
	"fmt"
	"slices"
	"strings"

	"github.com/just-tomdo/housenka/internal/bm"
	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/gitlaburl"
	"github.com/just-tomdo/housenka/internal/hours"
	"github.com/just-tomdo/housenka/internal/period"
	"github.com/just-tomdo/housenka/internal/rules"
)

// Input is the raw text of an entry form.
type Input struct {
	Date        string
	Issue       string
	Project     string
	Activity    string
	Hours       string
	Description string
}

// Messages maps form fields to an error or warning text.
type Messages map[rules.Field]string

// Parse turns form input into an entry based on base (ID, Sent and timestamps
// are kept). Parse errors win over validation findings of the same field.
// The entry may be saved only when errs is empty.
func Parse(r *rules.Rules, base entry.Entry, in Input, today civil.Date) (e entry.Entry, errs, warns Messages) {
	errs, warns = Messages{}, Messages{}
	e = base
	e.IssueURL = strings.TrimSpace(in.Issue)
	e.ProjectCode = in.Project
	e.Activity = in.Activity
	e.Description = CleanDescription(in.Description)

	d, err := civil.ParseHuman(in.Date, today, r.Config().UI.DateLayout())
	if err != nil {
		errs[rules.FieldDate] = err.Error()
	}
	e.Date = d
	q, err := hours.Parse(in.Hours)
	if err != nil {
		errs[rules.FieldHours] = err.Error()
	}
	e.Quarters = q

	for _, fd := range r.Validate(e, today).Findings {
		if _, taken := errs[fd.Field]; taken {
			continue
		}
		if fd.Warning {
			warns[fd.Field] = fd.Message
		} else {
			errs[fd.Field] = fd.Message
		}
	}
	return e, errs, warns
}

// CleanDescription trims trailing spaces of every line and surrounding empty lines.
func CleanDescription(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n ")
}

// Choices are the project codes and activities offered for an issue link.
type Choices struct {
	Projects   []string
	Project    string // selected project, "" when none fits
	Activities []string
	Activity   string // selected activity, "" when none fits
	IssueRef   *gitlaburl.Ref
	IssueErr   error
}

// Options computes allowed projects for issueURL and allowed activities for
// the selected project. The current project and activity are kept when
// allowed; otherwise the only project, or the default activity, is selected.
func Options(r *rules.Rules, issueURL, project, activity string) Choices {
	codes, ref, err := r.AllowedProjects(issueURL)
	c := Choices{Projects: codes, Project: project, IssueRef: ref, IssueErr: err}
	switch {
	case slices.Contains(codes, project):
	case len(codes) == 1:
		c.Project = codes[0]
	default:
		c.Project = ""
	}
	c.Activities, c.Activity = Activities(r, c.Project, activity)
	return c
}

// Activities returns the activities allowed for project and the one to
// select: activity when allowed, otherwise the project's default.
func Activities(r *rules.Rules, project, activity string) ([]string, string) {
	if project == "" {
		return nil, ""
	}
	all := r.AllowedActivities(project)
	if !slices.Contains(all, activity) {
		activity = r.DefaultActivity(project)
	}
	return all, activity
}

// ReportedChanged reports whether any value that is sent to BM differs.
func ReportedChanged(a, b entry.Entry) bool {
	return a.Date != b.Date || a.IssueURL != b.IssueURL || a.ProjectCode != b.ProjectCode ||
		a.Activity != b.Activity || a.Quarters != b.Quarters || a.Description != b.Description
}

// PrepareSave returns the entry to store when old is replaced by e. A sent
// entry whose reported values changed is marked unsent again (BM still holds
// the old version); resent reports that case.
func PrepareSave(old, e entry.Entry) (_ entry.Entry, resent bool) {
	if e.Sent && ReportedChanged(old, e) {
		e.Sent = false
		return e, true
	}
	return e, false
}

// CopyOf returns a new, unsent entry with the reported values of e on date.
func CopyOf(e entry.Entry, date civil.Date) entry.Entry {
	return entry.Entry{Date: date, IssueURL: e.IssueURL, ProjectCode: e.ProjectCode,
		Activity: e.Activity, Quarters: e.Quarters, Description: e.Description}
}

// Matches reports whether e contains the filter query (case-insensitive).
// An empty query matches everything.
func Matches(e entry.Entry, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	text := strings.Join([]string{e.Description, e.IssueURL, e.ProjectCode, e.Activity}, " ")
	return strings.Contains(strings.ToLower(text), q)
}

// PeriodGoal is the expected work in quarters: the daily goal for a day, or
// the daily goal times the number of working days (Mon–Fri) in the period.
func PeriodGoal(cfg *config.Config, p period.Period) int {
	goal := int(cfg.Time.DailyGoal)
	if p.Kind == period.Day {
		return goal
	}
	n := 0
	for _, d := range p.Days() {
		if !d.IsWeekend() {
			n++
		}
	}
	return goal * n
}

// BMURL builds the pre-filled BM link for a validated entry. An invalid
// entry yields its first error instead.
func BMURL(cfg *config.Config, res rules.Result, e entry.Entry) (string, error) {
	if !res.Valid() {
		return "", fmt.Errorf("záznam je neplatný: %s", res.Errors()[0].Message)
	}
	return bm.BuildURL(cfg.BM, e)
}
