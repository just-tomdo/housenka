package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/issues"
	"github.com/just-tomdo/housenka/internal/rules"
	"github.com/just-tomdo/housenka/internal/store"
)

// TestDemoIssues checks the issue overview generated for the demo: every
// status has cards and the cases worth showing are there.
func TestDemoIssues(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"config.yaml", "scenario.yaml", "issues.yaml"} {
		data, err := os.ReadFile(filepath.Join("..", "demo", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	last := civil.Date{Year: 2026, Month: time.October, Day: 15}
	if _, err := generateDir(dir, last); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	icfg, err := issues.LoadConfig(filepath.Join(dir, "issues.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := issues.OpenStore(filepath.Join(dir, "issues"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(filepath.Join(dir, "entries"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := s.All()
	if err != nil {
		t.Fatal(err)
	}
	cards := issues.Cards(f, icfg, rules.New(cfg), issues.WorkByIssue(entries), last)
	b := issues.Arrange(cards, icfg)
	if len(b.New) < 2 || len(b.Hidden) == 0 {
		t.Errorf("new %d, hidden %d", len(b.New), len(b.Hidden))
	}
	var stale, unknown, unassigned, worked bool
	for _, c := range cards {
		stale = stale || c.Stale
		unknown = unknown || !c.Repo.Known()
		unassigned = unassigned || c.Unassigned
		worked = worked || c.Work.Total > 0
	}
	if !stale || !unknown || !unassigned || !worked {
		t.Errorf("stale %v, unknown repo %v, unassigned %v, worked %v", stale, unknown, unassigned, worked)
	}
	for _, col := range b.Columns {
		if len(col.Cards) == 0 {
			t.Errorf("status %s is empty", col.Status.Code)
		}
	}
}
