package bm

import (
	"net/url"
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
)

// sampleURL mirrors a link produced by the BM UI itself (same parameter
// order and encoding); host, field names and data are anonymized.
const sampleURL = "https://bm.example.com/vykazy/editor?project=SHOPDEV&issue=https%3A%2F%2Fgitlab.example.com%2Fclients%2Fshop-web%2F-%2Fwork_items%2F393%23note_123456&hours=5%2C00&activity=DEV&date=9.+10.+2026&description=%C3%BAprava+ko%C5%A1%C3%ADku%2C+testov%C3%A1n%C3%AD+a+validace+formul%C3%A1%C5%99%C5%AF%2C+%C5%99e%C5%A1en%C3%AD+ovl%C3%A1d%C3%A1n%C3%AD+a+vypl%C5%88ov%C3%A1n%C3%AD"

var cfg = config.BM{
	BaseURL: "https://bm.example.com/vykazy/editor",
	Fields: config.BMFields{Project: "project", Issue: "issue", Hours: "hours",
		Activity: "activity", Date: "date", Description: "description"},
}

func sampleEntry() entry.Entry {
	return entry.Entry{
		Date:        civil.Date{Year: 2026, Month: time.October, Day: 9},
		IssueURL:    "https://gitlab.example.com/clients/shop-web/-/work_items/393#note_123456",
		ProjectCode: "SHOPDEV",
		Activity:    "DEV",
		Quarters:    20,
		Description: "úprava košíku, testování a validace formulářů, řešení ovládání a vyplňování",
	}
}

func TestBuildURLMatchesSample(t *testing.T) {
	got, err := BuildURL(cfg, sampleEntry())
	if err != nil {
		t.Fatal(err)
	}
	if got != sampleURL {
		t.Errorf("BuildURL mismatch\n got: %s\nwant: %s", got, sampleURL)
	}
}

func TestBuildURLWithoutIssue(t *testing.T) {
	e := sampleEntry()
	e.IssueURL = ""
	e.Description = "  porada  "
	got, err := BuildURL(cfg, e)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Has("issue") {
		t.Error("issue parameter must be omitted")
	}
	if q.Get("description") != "porada" {
		t.Errorf("description = %q", q.Get("description"))
	}
}

func TestBuildURLKeepsExistingQuery(t *testing.T) {
	got, err := BuildURL(config.BM{BaseURL: "https://bm.example/editor?lang=cs", Fields: cfg.Fields}, sampleEntry())
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(got)
	if u.Query().Get("lang") != "cs" || u.Query().Get("project") != "SHOPDEV" {
		t.Errorf("query = %s", u.RawQuery)
	}
}
