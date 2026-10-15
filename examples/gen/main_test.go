package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/rules"
	"github.com/just-tomdo/housenka/internal/store"
)

// TestExamples generates each example into a temp dir and checks every entry
// passes the same validation as the TUI form. The private example (git-ignored)
// is checked only when present.
func TestExamples(t *testing.T) {
	t.Run("demo", func(t *testing.T) { testExample(t, "demo") })
	t.Run("firma", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join("..", "firma", "scenario.yaml")); err != nil {
			t.Skip("private example not present")
		}
		testExample(t, "firma")
	})
}

func testExample(t *testing.T, example string) {
	dir := t.TempDir()
	for _, name := range []string{"config.yaml", "scenario.yaml"} {
		data, err := os.ReadFile(filepath.Join("..", example, name))
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
	s, err := store.Open(filepath.Join(dir, "entries"))
	if err != nil {
		t.Fatal(err)
	}
	first := civil.Date{Year: 2026, Month: time.April, Day: 1}
	entries, err := s.Range(first, last)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 200 {
		t.Fatalf("only %d entries generated", len(entries))
	}

	r := rules.New(cfg)
	perDay := map[civil.Date]int{}
	for _, e := range entries {
		if res := r.Validate(e, last); !res.Valid() {
			t.Errorf("%s %s %q: %v", e.Date, e.ProjectCode, e.Description, res.Errors())
		}
		if wd := e.Date.Weekday(); wd == time.Saturday || wd == time.Sunday {
			t.Errorf("entry on a weekend: %s", e.Date)
		}
		perDay[e.Date] += e.Quarters
	}
	for d, q := range perDay {
		if d != last && q < 30 {
			t.Errorf("%s has only %d quarters", d, q)
		}
	}
	if entries[0].Date.Month != time.April || entries[len(entries)-1].Date != last {
		t.Errorf("entries span %s – %s", entries[0].Date, entries[len(entries)-1].Date)
	}
}

func TestHoliday(t *testing.T) {
	for _, iso := range []string{"2026-04-03", "2026-04-06", "2026-05-08", "2025-04-21", "2026-12-24"} {
		if d, _ := civil.Parse(iso); !holiday(d) {
			t.Errorf("%s should be a holiday", iso)
		}
	}
	if holiday(civil.Date{Year: 2026, Month: time.April, Day: 7}) {
		t.Error("2026-04-07 is not a holiday")
	}
}
