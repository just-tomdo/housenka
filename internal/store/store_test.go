package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/entry"
)

func day(y int, m time.Month, d int) civil.Date { return civil.Date{Year: y, Month: m, Day: d} }

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "entries")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	s.now = func() time.Time { clock = clock.Add(time.Minute); return clock }
	return s, dir
}

func ids(entries []entry.Entry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Description)
	}
	return out
}

func TestSaveAndReload(t *testing.T) {
	s, dir := newStore(t)
	saved, err := s.Save(entry.Entry{Date: day(2026, 10, 12), ProjectCode: "BETASUP", Quarters: 11, Description: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" || saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatalf("ID/timestamps not set: %+v", saved)
	}
	if _, err := s.Save(entry.Entry{Date: day(2026, 10, 11), Description: "b"}); err != nil {
		t.Fatal(err)
	}

	// A fresh store reads from disk.
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.Range(day(2026, 10, 1), day(2026, 10, 31))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Description != "b" || got[1].Description != "a" {
		t.Fatalf("Range = %v", ids(got))
	}
	if got[1].Quarters != 11 || got[1].ProjectCode != "BETASUP" || got[1].ID != saved.ID {
		t.Errorf("round trip lost data: %+v", got[1])
	}
	if _, err := os.Stat(filepath.Join(dir, "2026.json")); err != nil {
		t.Errorf("year file missing: %v", err)
	}
}

func TestUpdateKeepsCreatedAt(t *testing.T) {
	s, _ := newStore(t)
	e, _ := s.Save(entry.Entry{Date: day(2026, 10, 12), Description: "old"})
	e.Description = "new"
	updated, err := s.Save(e)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.CreatedAt.Equal(e.CreatedAt) || !updated.UpdatedAt.After(e.UpdatedAt) {
		t.Errorf("timestamps wrong: %+v", updated)
	}
	got, _ := s.Range(day(2026, 10, 12), day(2026, 10, 12))
	if len(got) != 1 || got[0].Description != "new" {
		t.Errorf("Range = %v", ids(got))
	}
}

func TestMoveBetweenYears(t *testing.T) {
	s, dir := newStore(t)
	e, _ := s.Save(entry.Entry{Date: day(2026, 12, 31), Description: "x"})
	e.Date = day(2027, 1, 2)
	if _, err := s.Save(e); err != nil {
		t.Fatal(err)
	}

	s2, _ := Open(dir)
	old, _ := s2.Range(day(2026, 1, 1), day(2026, 12, 31))
	moved, _ := s2.Range(day(2027, 1, 1), day(2027, 12, 31))
	if len(old) != 0 || len(moved) != 1 || moved[0].ID != e.ID {
		t.Errorf("old=%v moved=%v", ids(old), ids(moved))
	}
}

func TestRangeAcrossYears(t *testing.T) {
	s, _ := newStore(t)
	for _, d := range []civil.Date{day(2026, 12, 30), day(2027, 1, 3), day(2027, 1, 5)} {
		if _, err := s.Save(entry.Entry{Date: d, Description: d.String()}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Range(day(2026, 12, 28), day(2027, 1, 3))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("Range = %v", ids(got))
	}
}

func TestDelete(t *testing.T) {
	s, dir := newStore(t)
	e, _ := s.Save(entry.Entry{Date: day(2026, 10, 12), Description: "x"})
	if err := s.Delete(e.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(e.ID); err == nil {
		t.Error("second delete should fail")
	}
	s2, _ := Open(dir)
	if got, _ := s2.Range(day(2026, 1, 1), day(2026, 12, 31)); len(got) != 0 {
		t.Errorf("Range after delete = %v", ids(got))
	}
}

func TestSaveUnknownID(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Save(entry.Entry{ID: "nope", Date: day(2026, 10, 12)}); err == nil {
		t.Error("expected error for unknown ID")
	}
	if _, err := s.Save(entry.Entry{}); err == nil {
		t.Error("expected error for invalid date")
	}
}

func TestCorruptFile(t *testing.T) {
	s, dir := newStore(t)
	if err := os.WriteFile(filepath.Join(dir, "2026.json"), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Range(day(2026, 1, 1), day(2026, 1, 1)); err == nil {
		t.Error("expected error for corrupt file")
	}
}

func TestNoTempFilesLeft(t *testing.T) {
	s, dir := newStore(t)
	for range 3 {
		if _, err := s.Save(entry.Entry{Date: day(2026, 10, 12)}); err != nil {
			t.Fatal(err)
		}
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 || files[0].Name() != "2026.json" {
		var names []string
		for _, f := range files {
			names = append(names, f.Name())
		}
		t.Errorf("unexpected files: %v", names)
	}
}
