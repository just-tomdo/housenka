package stats

import (
	"testing"

	"github.com/just-tomdo/housenka/internal/entry"
)

func TestBuildIssueSummary(t *testing.T) {
	cfg := testConfig(t)
	other := "https://gitlab.example.com/clients/shop-web/-/work_items/400"
	entries := []entry.Entry{
		{Date: day(14), IssueURL: issue + "#note_1", ProjectCode: "SHOPDEV", Activity: "REZIE", Quarters: 2, Sent: true},
		{Date: day(12), IssueURL: issue, ProjectCode: "SHOPDEV", Activity: "DEV", Quarters: 8},
		{Date: day(12), IssueURL: issue, ProjectCode: "INTERNI", Activity: "DEV", Quarters: 4},
		{Date: day(13), IssueURL: other, ProjectCode: "SHOPDEV", Activity: "DEV", Quarters: 16},
		{Date: day(13), ProjectCode: "INTERNI", Activity: "REZIE", Quarters: 4},
	}
	s := BuildIssueSummary(issue+"#note_99", entries, cfg)

	if s.Label != "shop-web#393" || s.URL != issue {
		t.Errorf("label %q, url %q", s.Label, s.URL)
	}
	if len(s.Entries) != 3 || s.Total != 14 || s.Unsent != 12 || s.Days != 2 {
		t.Errorf("entries %d, total %d, unsent %d, days %d", len(s.Entries), s.Total, s.Unsent, s.Days)
	}
	if s.First != day(12) || s.Last != day(14) || s.Entries[0].Date != day(12) {
		t.Errorf("first %v, last %v", s.First, s.Last)
	}
	if len(s.Activities) != 2 || s.Activities[0].Code != "DEV" || s.Activities[0].Total != 12 ||
		!s.Activities[0].Productive || s.Activities[1].Productive || s.Activities[0].Name != "Vývoj" {
		t.Errorf("activities %+v", s.Activities)
	}
	if len(s.Projects) != 2 || s.Projects[0].Code != "SHOPDEV" || s.Projects[0].Total != 10 {
		t.Errorf("projects %+v", s.Projects)
	}

	if s := BuildIssueSummary("", entries, cfg); len(s.Entries) != 0 {
		t.Error("an empty link must not match entries without an issue")
	}
}
