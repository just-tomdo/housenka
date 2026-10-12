package stats

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/gitlaburl"
	"github.com/just-tomdo/housenka/internal/hours"
	"github.com/just-tomdo/housenka/internal/period"
)

// Status is a work summary for a team status report, grouped by project and issue.
type Status struct {
	Period   period.Period
	Total    int
	Projects []StatusProject // sorted by hours, descending
}

// StatusProject groups work on one project code.
type StatusProject struct {
	Code   string
	Name   string
	Total  int
	Issues []StatusIssue // sorted by hours, descending; "no issue" last
}

// StatusIssue groups work on one issue (entries differing only in #note are merged).
type StatusIssue struct {
	Label        string // "shop-web#393", or "" for work without an issue
	URL          string // issue link without fragment
	Total        int
	Descriptions []string // unique, in chronological order
}

// BuildStatus aggregates entries of period p for a status report.
func BuildStatus(p period.Period, entries []entry.Entry, cfg *config.Config) Status {
	st := Status{Period: p}
	projects := map[string]*StatusProject{}
	issues := map[string]map[string]*StatusIssue{}

	for _, e := range entries {
		if !p.Contains(e.Date) {
			continue
		}
		st.Total += e.Quarters

		sp, ok := projects[e.ProjectCode]
		if !ok {
			sp = &StatusProject{Code: e.ProjectCode, Name: cfg.Projects[e.ProjectCode].Name}
			projects[e.ProjectCode] = sp
			issues[e.ProjectCode] = map[string]*StatusIssue{}
		}
		sp.Total += e.Quarters

		key, label, link := issueKey(e.IssueURL)
		si, ok := issues[e.ProjectCode][key]
		if !ok {
			si = &StatusIssue{Label: label, URL: link}
			issues[e.ProjectCode][key] = si
		}
		si.Total += e.Quarters
		if d := oneLine(e.Description); d != "" && !slices.Contains(si.Descriptions, d) {
			si.Descriptions = append(si.Descriptions, d)
		}
	}

	for code, sp := range projects {
		for _, si := range issues[code] {
			sp.Issues = append(sp.Issues, *si)
		}
		slices.SortFunc(sp.Issues, func(a, b StatusIssue) int {
			if (a.Label == "") != (b.Label == "") {
				if a.Label == "" {
					return 1
				}
				return -1
			}
			return cmp.Or(b.Total-a.Total, cmp.Compare(a.Label, b.Label))
		})
		st.Projects = append(st.Projects, *sp)
	}
	slices.SortFunc(st.Projects, func(a, b StatusProject) int {
		return cmp.Or(b.Total-a.Total, cmp.Compare(a.Code, b.Code))
	})
	return st
}

// oneLine joins the lines of a multi-line description with "; ".
func oneLine(s string) string {
	var parts []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			parts = append(parts, l)
		}
	}
	return strings.Join(parts, "; ")
}

// issueKey identifies an issue regardless of #note fragments.
func issueKey(raw string) (key, label, link string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", ""
	}
	ref, err := gitlaburl.Parse(raw)
	if err != nil {
		return raw, raw, raw
	}
	sep := "#"
	if ref.Kind == gitlaburl.MergeRequest {
		sep = "!"
	}
	link, _, _ = strings.Cut(raw, "#")
	return strings.ToLower(ref.ProjectPath) + sep + fmt.Sprint(ref.IID), fmt.Sprintf("%s%s%d", ref.Repo, sep, ref.IID), link
}

// Text renders the status as plain text.
func (st Status) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s – celkem %s h\n", capitalize(st.Period.Label()), hours.Format(st.Total))
	for _, p := range st.Projects {
		fmt.Fprintf(&b, "\n%s (%s h)\n", projectTitle(p), hours.Format(p.Total))
		for _, i := range p.Issues {
			fmt.Fprintf(&b, "  • %s (%s h): %s\n", issueTitle(i.Label), hours.Format(i.Total), strings.Join(i.Descriptions, "; "))
		}
	}
	return b.String()
}

// Markdown renders the status as Markdown with links to issues.
func (st Status) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s – celkem %s h\n", capitalize(st.Period.Label()), hours.Format(st.Total))
	for _, p := range st.Projects {
		fmt.Fprintf(&b, "\n**%s** (%s h)\n", projectTitle(p), hours.Format(p.Total))
		for _, i := range p.Issues {
			title := issueTitle(i.Label)
			if i.URL != "" {
				title = fmt.Sprintf("[%s](%s)", i.Label, i.URL)
			}
			fmt.Fprintf(&b, "- %s – %s h – %s\n", title, hours.Format(i.Total), strings.Join(i.Descriptions, "; "))
		}
	}
	return b.String()
}

func projectTitle(p StatusProject) string {
	if p.Name == "" {
		return p.Code
	}
	return p.Code + " – " + p.Name
}

func issueTitle(label string) string {
	if label == "" {
		return "bez issue"
	}
	return label
}

func capitalize(s string) string {
	for i := range s {
		if i > 0 {
			return strings.ToUpper(s[:i]) + s[i:]
		}
	}
	return strings.ToUpper(s)
}
