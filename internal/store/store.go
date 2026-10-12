// Package store persists entries as JSON, one file per year
// (<dir>/2026.json). Loaded years are cached in memory.
package store

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/entry"
)

const fileVersion = 1

type yearFile struct {
	Version int           `json:"version"`
	Entries []entry.Entry `json:"entries"`
}

// Store reads and writes entries. It is safe for concurrent use within one process.
type Store struct {
	dir   string
	now   func() time.Time
	mu    sync.Mutex
	years map[int][]entry.Entry
}

// Open returns a store rooted at dir, creating the directory if needed.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir, now: time.Now, years: map[int][]entry.Entry{}}, nil
}

// Range returns entries with from <= Date <= to, sorted by date and creation time.
func (s *Store) Range(from, to civil.Date) ([]entry.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []entry.Entry
	for y := from.Year; y <= to.Year; y++ {
		entries, err := s.load(y)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.Date.Before(from) && !e.Date.After(to) {
				out = append(out, e)
			}
		}
	}
	sortEntries(out)
	return out, nil
}

// Save inserts e (when e.ID is empty) or replaces the entry with the same ID.
// If the date moved to another year, the entry is moved between files.
// It returns the stored entry with ID and timestamps filled in.
func (s *Store) Save(e entry.Entry) (entry.Entry, error) {
	if !e.Date.Valid() {
		return e, errors.New("záznam nemá platné datum")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	e.UpdatedAt = now
	oldYear := 0
	if e.ID == "" {
		e.ID = entry.NewID()
		e.CreatedAt = now
	} else if y, i, ok := s.find(e.ID); ok {
		oldYear = y
		e.CreatedAt = s.years[y][i].CreatedAt
	} else {
		return e, fmt.Errorf("záznam %s neexistuje", e.ID)
	}

	newYear := e.Date.Year
	entries, err := s.load(newYear)
	if err != nil {
		return e, err
	}
	if oldYear == newYear {
		_, i, _ := s.find(e.ID)
		entries = slices.Clone(entries)
		entries[i] = e
		return e, s.write(newYear, entries)
	}

	// Write the new year first: a crash in between leaves a duplicate, never a loss.
	if err := s.write(newYear, append(slices.Clone(entries), e)); err != nil {
		return e, err
	}
	if oldYear != 0 {
		return e, s.write(oldYear, without(s.years[oldYear], e.ID))
	}
	return e, nil
}

// Delete removes the entry with the given ID. The entry's year must have been
// loaded before (e.g. via Range), which is always the case when the UI shows it.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	y, _, ok := s.find(id)
	if !ok {
		return fmt.Errorf("záznam %s neexistuje", id)
	}
	return s.write(y, without(s.years[y], id))
}

func (s *Store) path(year int) string {
	return filepath.Join(s.dir, strconv.Itoa(year)+".json")
}

// load returns the cached entries of a year, reading the file on first use.
func (s *Store) load(year int) ([]entry.Entry, error) {
	if entries, ok := s.years[year]; ok {
		return entries, nil
	}
	data, err := os.ReadFile(s.path(year))
	if errors.Is(err, os.ErrNotExist) {
		s.years[year] = nil
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f yearFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("soubor %s je poškozený: %v", s.path(year), err)
	}
	if f.Version > fileVersion {
		return nil, fmt.Errorf("soubor %s má novější formát (%d) – aktualizuj housenku", s.path(year), f.Version)
	}
	s.years[year] = f.Entries
	return f.Entries, nil
}

// write atomically replaces a year file and updates the cache.
func (s *Store) write(year int, entries []entry.Entry) error {
	sortEntries(entries)
	data, err := json.MarshalIndent(yearFile{Version: fileVersion, Entries: entries}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".tmp-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), s.path(year)); err != nil {
		return err
	}
	s.years[year] = entries
	return nil
}

// find locates an entry by ID among loaded years.
func (s *Store) find(id string) (year, index int, ok bool) {
	for y, entries := range s.years {
		if i := slices.IndexFunc(entries, func(e entry.Entry) bool { return e.ID == id }); i >= 0 {
			return y, i, true
		}
	}
	return 0, 0, false
}

func without(entries []entry.Entry, id string) []entry.Entry {
	return slices.DeleteFunc(slices.Clone(entries), func(e entry.Entry) bool { return e.ID == id })
}

func sortEntries(entries []entry.Entry) {
	slices.SortStableFunc(entries, func(a, b entry.Entry) int {
		return cmp.Or(a.Date.Compare(b.Date), a.CreatedAt.Compare(b.CreatedAt))
	})
}
