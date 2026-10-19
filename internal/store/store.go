// Package store persists entries as JSON, one file per year
// (<dir>/2026.json). Loaded years are cached in memory and re-read when the
// file changes, so several processes (the TUI and the web interface) can
// share the directory. Changes are written under an advisory file lock.
package store

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
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

// Store reads and writes entries. It is safe for concurrent use, also by
// several processes sharing the directory.
type Store struct {
	dir   string
	now   func() time.Time
	mu    sync.Mutex
	years map[int]cachedYear
}

// cachedYear is a loaded year file with the file state it was read from.
type cachedYear struct {
	entries []entry.Entry
	stamp   fileStamp
}

// fileStamp identifies a version of a file; the zero value means "missing".
type fileStamp struct {
	modTime time.Time
	size    int64
}

// Open returns a store rooted at dir, creating the directory if needed.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir, now: time.Now, years: map[int]cachedYear{}}, nil
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

// All returns the entries of every year, sorted like Range.
func (s *Store) All() ([]entry.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	years, err := s.yearFiles()
	if err != nil {
		return nil, err
	}
	var out []entry.Entry
	for _, year := range years {
		entries, err := s.load(year)
		if err != nil {
			return nil, err
		}
		out = append(out, entries...)
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
	unlock, err := s.lock()
	if err != nil {
		return e, err
	}
	defer unlock()

	now := s.now()
	e.UpdatedAt = now
	oldYear := 0
	if e.ID == "" {
		e.ID = entry.NewID()
		e.CreatedAt = now
	} else if y, i, ok, err := s.find(e.ID); err != nil {
		return e, err
	} else if ok {
		oldYear = y
		e.CreatedAt = s.years[y].entries[i].CreatedAt
	} else {
		return e, fmt.Errorf("záznam %s neexistuje", e.ID)
	}

	newYear := e.Date.Year
	entries, err := s.load(newYear)
	if err != nil {
		return e, err
	}
	if oldYear == newYear {
		i := slices.IndexFunc(entries, func(o entry.Entry) bool { return o.ID == e.ID })
		entries = slices.Clone(entries)
		entries[i] = e
		return e, s.write(newYear, entries)
	}

	// Write the new year first: a crash in between leaves a duplicate, never a loss.
	if err := s.write(newYear, append(slices.Clone(entries), e)); err != nil {
		return e, err
	}
	if oldYear != 0 {
		return e, s.write(oldYear, without(s.years[oldYear].entries, e.ID))
	}
	return e, nil
}

// Delete removes the entry with the given ID.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	y, _, ok, err := s.find(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("záznam %s neexistuje", id)
	}
	return s.write(y, without(s.years[y].entries, id))
}

// lock takes the directory's advisory lock; call the returned function to release it.
func (s *Store) lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("nelze zamknout data: %w", err)
	}
	return func() {
		unlockFile(f)
		f.Close()
	}, nil
}

func (s *Store) path(year int) string {
	return filepath.Join(s.dir, strconv.Itoa(year)+".json")
}

// yearFiles returns the years that have a file in the directory.
func (s *Store) yearFiles() ([]int, error) {
	files, err := filepath.Glob(filepath.Join(s.dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var years []int
	for _, f := range files {
		if year, err := strconv.Atoi(strings.TrimSuffix(filepath.Base(f), ".json")); err == nil {
			years = append(years, year)
		}
	}
	return years, nil
}

// stamp returns the current state of a year file.
func (s *Store) stamp(year int) (fileStamp, error) {
	fi, err := os.Stat(s.path(year))
	if errors.Is(err, os.ErrNotExist) {
		return fileStamp{}, nil
	}
	if err != nil {
		return fileStamp{}, err
	}
	return fileStamp{fi.ModTime(), fi.Size()}, nil
}

// load returns the entries of a year, from the cache unless the file changed
// since it was read (e.g. by another process).
func (s *Store) load(year int) ([]entry.Entry, error) {
	st, err := s.stamp(year)
	if err != nil {
		return nil, err
	}
	if c, ok := s.years[year]; ok && c.stamp == st {
		return c.entries, nil
	}
	if st == (fileStamp{}) {
		s.years[year] = cachedYear{}
		return nil, nil
	}
	data, err := os.ReadFile(s.path(year))
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
	s.years[year] = cachedYear{f.Entries, st}
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
	st, err := s.stamp(year)
	if err != nil {
		return err
	}
	s.years[year] = cachedYear{entries, st}
	return nil
}

// find locates an entry by ID in up-to-date year files: first among the
// years loaded before, then in all the others.
func (s *Store) find(id string) (year, index int, ok bool, err error) {
	search := func(years []int) bool {
		for _, y := range years {
			entries, e := s.load(y)
			if e != nil {
				err = e
				return true
			}
			if i := slices.IndexFunc(entries, func(e entry.Entry) bool { return e.ID == id }); i >= 0 {
				year, index, ok = y, i, true
				return true
			}
		}
		return false
	}
	loaded := slices.Collect(maps.Keys(s.years))
	if search(loaded) {
		return
	}
	all, e := s.yearFiles()
	if e != nil {
		return 0, 0, false, e
	}
	search(slices.DeleteFunc(all, func(y int) bool { return slices.Contains(loaded, y) }))
	return
}

func without(entries []entry.Entry, id string) []entry.Entry {
	return slices.DeleteFunc(slices.Clone(entries), func(e entry.Entry) bool { return e.ID == id })
}

func sortEntries(entries []entry.Entry) {
	slices.SortStableFunc(entries, func(a, b entry.Entry) int {
		return cmp.Or(a.Date.Compare(b.Date), a.CreatedAt.Compare(b.CreatedAt))
	})
}
