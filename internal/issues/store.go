package issues

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/just-tomdo/housenka/internal/atomicfile"
)

const fileVersion = 1

// Store keeps issues.json. Like the entry store it re-reads the file when
// another process changed it and writes under an advisory lock, so the TUI,
// the web interface and a sync can share it.
type Store struct {
	dir   string
	now   func() time.Time
	mu    sync.Mutex
	file  File
	stamp atomicfile.Stamp
	read  bool
}

// OpenStore returns the store in dir, creating the directory if needed.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir, now: time.Now}, nil
}

func (s *Store) path() string { return filepath.Join(s.dir, "issues.json") }

// Load returns the current content. The caller may keep and change it: it
// is a copy.
func (s *Store) Load() (File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return File{}, err
	}
	return s.copy(), nil
}

// Update runs change on the current content under the lock and writes the
// result, unless change returns an error. now is the time of the change.
func (s *Store) Update(change func(f *File, now time.Time) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := atomicfile.Lock(filepath.Join(s.dir, ".lock"))
	if err != nil {
		return fmt.Errorf("nelze zamknout přehled issues: %w", err)
	}
	defer unlock()
	if err := s.load(); err != nil {
		return err
	}
	f := s.copy()
	if err := change(&f, s.now()); err != nil {
		return err
	}
	return s.write(f)
}

// SetStatus, MoveNextTo, SetHidden and SetNotes are File's changes, stored.

func (s *Store) SetStatus(key, status, before string) error {
	return s.Update(func(f *File, now time.Time) error { return f.SetStatus(key, status, before, now) })
}

func (s *Store) Place(key, status string, start bool) error {
	return s.Update(func(f *File, now time.Time) error { return f.Place(key, status, start, now) })
}

func (s *Store) MoveNextTo(key, neighbour string, after bool) error {
	return s.Update(func(f *File, now time.Time) error { return f.MoveNextTo(key, neighbour, after, now) })
}

func (s *Store) SetMuted(key string, muted bool) error {
	return s.Update(func(f *File, now time.Time) error { return f.SetMuted(key, muted, now) })
}

func (s *Store) SetHidden(key string, hidden bool) error {
	return s.Update(func(f *File, now time.Time) error { return f.SetHidden(key, hidden, now) })
}

// SetNotes stores the notes and returns the issue as stored.
func (s *Store) SetNotes(key, notes string, base time.Time) (Issue, error) {
	var out Issue
	err := s.Update(func(f *File, now time.Time) error {
		if err := f.SetNotes(key, notes, base, now); err != nil {
			return err
		}
		is, _ := f.find(key)
		out = *is
		return nil
	})
	return out, err
}

func (s *Store) copy() File {
	f := s.file
	f.Issues = slices.Clone(f.Issues)
	for i := range f.Issues {
		is := &f.Issues[i]
		is.Labels = slices.Clone(is.Labels)
		if is.Due != nil {
			d := *is.Due
			is.Due = &d
		}
	}
	return f
}

// trySyncLock takes the lock that lets one sync run at a time (also across
// processes); ok is false while another sync holds it.
func (s *Store) trySyncLock() (unlock func(), ok bool, err error) {
	return atomicfile.TryLock(filepath.Join(s.dir, ".sync.lock"))
}

func (s *Store) load() error {
	st, err := atomicfile.StatStamp(s.path())
	if err != nil {
		return err
	}
	if s.read && st == s.stamp {
		return nil
	}
	f := File{Version: fileVersion}
	if st != (atomicfile.Stamp{}) {
		data, err := os.ReadFile(s.path())
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &f); err != nil {
			return fmt.Errorf("soubor %s je poškozený: %v", s.path(), err)
		}
		if f.Version > fileVersion {
			return fmt.Errorf("soubor %s má novější formát (%d) – aktualizuj housenku", s.path(), f.Version)
		}
	}
	s.file, s.stamp, s.read = f, st, true
	return nil
}

func (s *Store) write(f File) error {
	f.Version = fileVersion
	if f.Issues == nil {
		f.Issues = []Issue{}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	// Notes are private like entries.
	if err := atomicfile.Write(s.path(), append(data, '\n'), 0o600); err != nil {
		return err
	}
	st, err := atomicfile.StatStamp(s.path())
	if err != nil {
		return err
	}
	s.file, s.stamp, s.read = f, st, true
	return nil
}
