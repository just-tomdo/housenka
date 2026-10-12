package rules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/gitlaburl"
	"github.com/just-tomdo/housenka/internal/hours"
)

// Field identifies an entry field a finding relates to.
type Field string

const (
	FieldDate        Field = "date"
	FieldIssue       Field = "issue"
	FieldProject     Field = "project"
	FieldActivity    Field = "activity"
	FieldHours       Field = "hours"
	FieldDescription Field = "description"
)

// Finding is a single validation error or warning.
type Finding struct {
	Field   Field
	Message string
	Warning bool // warnings do not make the entry invalid
}

// Result is the outcome of validating an entry.
type Result struct {
	Findings []Finding
	Issue    *gitlaburl.Ref // parsed issue link, if valid
}

// Valid reports whether the entry has no errors (warnings are allowed).
func (r Result) Valid() bool {
	return !slices.ContainsFunc(r.Findings, func(f Finding) bool { return !f.Warning })
}

// Errors returns only the findings that are errors.
func (r Result) Errors() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if !f.Warning {
			out = append(out, f)
		}
	}
	return out
}

// Validate checks an entry against the configuration. today is used to warn
// about entries dated in the future.
func (r *Rules) Validate(e entry.Entry, today civil.Date) Result {
	var res Result
	fail := func(f Field, format string, args ...any) {
		res.Findings = append(res.Findings, Finding{Field: f, Message: fmt.Sprintf(format, args...)})
	}
	warn := func(f Field, format string, args ...any) {
		res.Findings = append(res.Findings, Finding{Field: f, Message: fmt.Sprintf(format, args...), Warning: true})
	}

	switch {
	case !e.Date.Valid():
		fail(FieldDate, "neplatné datum")
	case e.Date.After(today):
		warn(FieldDate, "datum %s je v budoucnosti", e.Date.Czech())
	}

	project, projectKnown := r.cfg.Projects[e.ProjectCode]
	switch {
	case e.ProjectCode == "":
		fail(FieldProject, "kód projektu není vybraný")
	case !projectKnown:
		fail(FieldProject, "kód projektu %s není v configu", e.ProjectCode)
	}

	if strings.TrimSpace(e.IssueURL) == "" {
		if projectKnown && !project.IssueOptional() {
			fail(FieldIssue, "projekt %s vyžaduje odkaz na issue", e.ProjectCode)
		}
	} else if ref, err := r.ParseIssue(e.IssueURL); err != nil {
		fail(FieldIssue, "%v", err)
	} else {
		res.Issue = &ref
		allowed, err := r.ProjectsForRepo(ref.ProjectPath)
		switch {
		case err != nil:
			fail(FieldIssue, "%v", err)
		case projectKnown && !slices.Contains(allowed, e.ProjectCode):
			fail(FieldProject, "kód %s nepatří k repozitáři %s (povolené: %s)", e.ProjectCode, ref.ProjectPath, strings.Join(allowed, ", "))
		}
	}

	_, activityKnown := r.cfg.Activities[e.Activity]
	switch {
	case e.Activity == "":
		fail(FieldActivity, "činnost není vybraná")
	case !activityKnown:
		fail(FieldActivity, "činnost %s není v configu", e.Activity)
	case projectKnown && !slices.Contains(r.AllowedActivities(e.ProjectCode), e.Activity):
		fail(FieldActivity, "činnost %s není u projektu %s povolená (povolené: %s)",
			e.Activity, e.ProjectCode, strings.Join(r.AllowedActivities(e.ProjectCode), ", "))
	}

	maxQ := int(r.cfg.Time.MaxEntry)
	switch {
	case e.Quarters <= 0:
		fail(FieldHours, "čas musí být větší než 0")
	case e.Quarters > maxQ:
		fail(FieldHours, "čas %s h přesahuje maximum %s h na záznam", hours.Format(e.Quarters), hours.Format(maxQ))
	}

	if strings.TrimSpace(e.Description) == "" {
		fail(FieldDescription, "popis práce není vyplněný")
	}

	return res
}
