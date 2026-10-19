package stats

import (
	"cmp"
	"slices"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
)

// IssueSummary is all work reported on one issue.
type IssueSummary struct {
	Label       string // "shop-web#393"
	URL         string // issue link without fragment
	Total       int
	Unsent      int // quarters not yet sent to BM
	Days        int // distinct days with work
	First, Last civil.Date
	Activities  []IssueShare  // sorted by hours, descending
	Projects    []IssueShare  // sorted by hours, descending
	Entries     []entry.Entry // chronological
}

// IssueShare is the work of one activity or project code on the issue.
type IssueShare struct {
	Code       string
	Name       string
	Total      int
	Productive bool // activities only
	Known      bool // the code is in the config
}

// BuildIssueSummary collects the entries of the issue that issueURL points
// to (entries differing only in the #note fragment belong to it too).
func BuildIssueSummary(issueURL string, entries []entry.Entry, cfg *config.Config) IssueSummary {
	key, label, link := issueKey(issueURL)
	s := IssueSummary{Label: label, URL: link}
	if key == "" {
		return s
	}
	activities := map[string]*IssueShare{}
	projects := map[string]*IssueShare{}
	days := map[civil.Date]bool{}
	for _, e := range entries {
		if k, _, _ := issueKey(e.IssueURL); k != key {
			continue
		}
		s.Entries = append(s.Entries, e)
		s.Total += e.Quarters
		if !e.Sent {
			s.Unsent += e.Quarters
		}
		days[e.Date] = true

		a, ok := activities[e.Activity]
		if !ok {
			def, known := cfg.Activities[e.Activity]
			a = &IssueShare{Code: e.Activity, Name: def.Name, Productive: def.IsProductive(), Known: known}
			activities[e.Activity] = a
		}
		a.Total += e.Quarters

		p, ok := projects[e.ProjectCode]
		if !ok {
			def, known := cfg.Projects[e.ProjectCode]
			p = &IssueShare{Code: e.ProjectCode, Name: def.Name, Known: known}
			projects[e.ProjectCode] = p
		}
		p.Total += e.Quarters
	}
	slices.SortStableFunc(s.Entries, func(a, b entry.Entry) int {
		return cmp.Or(a.Date.Compare(b.Date), a.CreatedAt.Compare(b.CreatedAt))
	})
	if n := len(s.Entries); n > 0 {
		s.First, s.Last = s.Entries[0].Date, s.Entries[n-1].Date
	}
	s.Days = len(days)
	s.Activities = sortedShares(activities)
	s.Projects = sortedShares(projects)
	return s
}

func sortedShares(m map[string]*IssueShare) []IssueShare {
	out := make([]IssueShare, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	slices.SortFunc(out, func(a, b IssueShare) int { return cmp.Or(b.Total-a.Total, cmp.Compare(a.Code, b.Code)) })
	return out
}
