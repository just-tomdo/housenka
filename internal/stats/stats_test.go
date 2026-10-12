package stats

import (
	"strings"
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/period"
)

const testYAML = `
gitlab_hosts: [gitlab.example.com]
bm: { base_url: https://bm.example.com/vykazy/editor, fields: { project: project, issue: issue, hours: hours, activity: activity, date: date, description: description } }
activities:
  DEV: { name: Vývoj, productive: true }
  REZIE:  { name: Režie, productive: false }
projects:
  SHOPDEV: { name: E-shop rozvoj }
  INTERNI:  { name: Interní, issue: optional }
repos:
  clients/shop-web: [SHOPDEV]
`

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(testYAML))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func day(d int) civil.Date { return civil.Date{Year: 2026, Month: time.October, Day: d} }

const issue = "https://gitlab.example.com/clients/shop-web/-/work_items/393"

func testEntries() []entry.Entry {
	return []entry.Entry{
		{Date: day(12), ProjectCode: "SHOPDEV", Activity: "DEV", Quarters: 20, IssueURL: issue, Description: "JSON-LD"},
		{Date: day(12), ProjectCode: "INTERNI", Activity: "REZIE", Quarters: 4, Description: "porada"},
		{Date: day(13), ProjectCode: "SHOPDEV", Activity: "DEV", Quarters: 8, IssueURL: issue + "#note_1", Description: "testy"},
		{Date: day(13), ProjectCode: "SHOPDEV", Activity: "DEV", Quarters: 4, IssueURL: issue, Description: "JSON-LD"},
		{Date: day(14), ProjectCode: "INTERNI", Activity: "SMAZANA", Quarters: 2, Description: "?"},
		{Date: day(20), ProjectCode: "INTERNI", Activity: "REZIE", Quarters: 4, Description: "next week"},
	}
}

func TestTotals(t *testing.T) {
	tot := Compute(testEntries()[:2], testConfig(t))
	if tot.Total != 24 || tot.Productive != 20 || tot.Unproductive != 4 {
		t.Errorf("Totals = %+v", tot)
	}
	if got := tot.ProductivePercent(); got != 83 {
		t.Errorf("ProductivePercent = %d", got)
	}
	if (Totals{}).ProductivePercent() != 0 || (Totals{}).ProductiveShare() != 0 {
		t.Error("empty totals must give 0")
	}
}

func TestSummarizeWeek(t *testing.T) {
	s := Summarize(period.Of(period.Week, day(14)), testEntries(), testConfig(t))
	if s.Total != 38 || s.Productive != 32 || s.Unproductive != 4 || s.Unknown != 2 {
		t.Errorf("week totals = %+v", s.Totals)
	}
	if len(s.Days) != 7 || s.WorkedDays != 3 {
		t.Errorf("days = %d, worked = %d", len(s.Days), s.WorkedDays)
	}
	if s.Days[0].Total != 24 || s.Days[1].Total != 12 || s.Days[3].Total != 0 {
		t.Errorf("per-day totals wrong: %+v", s.Days)
	}
	if got := s.AvgHoursPerWorkedDay(); got != 38.0/4/3 {
		t.Errorf("avg = %v", got)
	}
}

func TestBuildStatus(t *testing.T) {
	st := BuildStatus(period.Of(period.Week, day(14)), testEntries(), testConfig(t))
	if st.Total != 38 || len(st.Projects) != 2 {
		t.Fatalf("status = %+v", st)
	}
	port := st.Projects[0]
	if port.Code != "SHOPDEV" || port.Total != 32 || len(port.Issues) != 1 {
		t.Fatalf("SHOPDEV = %+v", port)
	}
	iss := port.Issues[0]
	if iss.Label != "shop-web#393" || iss.URL != issue || iss.Total != 32 {
		t.Errorf("issue = %+v", iss)
	}
	if strings.Join(iss.Descriptions, "|") != "JSON-LD|testy" {
		t.Errorf("descriptions = %v", iss.Descriptions)
	}

	text := st.Text()
	for _, want := range []string{
		"Týden 42 · 12.–18. 10. 2026 – celkem 9,50 h",
		"SHOPDEV – E-shop rozvoj (8,00 h)",
		"  • shop-web#393 (8,00 h): JSON-LD; testy",
		"  • bez issue (1,50 h): porada; ?",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("Text() missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "next week") {
		t.Error("entry outside the period leaked into status")
	}

	md := st.Markdown()
	if !strings.Contains(md, "- [shop-web#393]("+issue+") – 8,00 h – JSON-LD; testy") {
		t.Errorf("Markdown() =\n%s", md)
	}
}
