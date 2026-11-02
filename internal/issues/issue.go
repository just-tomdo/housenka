// Package issues is the optional overview of GitLab issues assigned to the
// user: they arrive from GitLab as new, the user sorts them onto a board of
// their own statuses, keeps notes, hides dead ones and reports work on them.
// The extension is enabled by issues.yaml; its data is issues/issues.json.
package issues

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
)

// Issue is one GitLab issue with the user's own triage.
type Issue struct {
	Key string `json:"key"` // gitlaburl.Ref.Key(), e.g. "clients/shop-web#636"

	// From GitLab: a sync overwrites these.
	Host        string      `json:"host"`
	ProjectPath string      `json:"project"`
	IID         int         `json:"iid"`
	Title       string      `json:"title"`
	URL         string      `json:"url"`
	Closed      bool        `json:"closed,omitempty"`
	Unassigned  bool        `json:"unassigned,omitempty"` // no longer assigned to the user
	Labels      []string    `json:"labels,omitempty"`
	Due         *civil.Date `json:"due,omitempty"`
	AssignedAt  time.Time   `json:"assigned_at"` // when it was assigned to the user, or FirstSeen
	FirstSeen   time.Time   `json:"first_seen"`

	// The user's own: a sync never changes these.
	Status    string    `json:"status,omitempty"` // "" = new
	Rank      int       `json:"rank,omitempty"`   // order within the status
	Notes     string    `json:"notes,omitempty"`
	NotesAt   time.Time `json:"notes_at,omitzero"` // version of Notes, against concurrent edits
	Hidden    bool      `json:"hidden,omitempty"`
	TouchedAt time.Time `json:"touched_at,omitzero"` // last change by the user
}

// Repo returns the last segment of the project path ("shop-web").
func (i Issue) Repo() string {
	return i.ProjectPath[strings.LastIndex(i.ProjectPath, "/")+1:]
}

// Label returns the short name of the issue: "shop-web#636".
func (i Issue) Label() string {
	return i.Repo() + "#" + strconv.Itoa(i.IID)
}

// File is the content of issues.json.
type File struct {
	Version   int       `json:"version"`
	Me        string    `json:"me,omitempty"` // GitLab username the token belongs to
	SyncedAt  time.Time `json:"synced_at,omitzero"`
	SyncError string    `json:"sync_error,omitempty"`
	Issues    []Issue   `json:"issues"`
}

// New returns the issues waiting for the user to sort them: not hidden and
// without a status that the configuration knows.
func (f *File) New(cfg *Config) []Issue {
	var out []Issue
	for _, i := range f.Issues {
		if !i.Hidden && cfg.StatusIndex(i.Status) < 0 {
			out = append(out, i)
		}
	}
	return out
}

// ErrNotFound means there is no issue with the key.
var ErrNotFound = errors.New("issue v přehledu není")

// ErrConflict means the notes were changed elsewhere since the caller read them.
var ErrConflict = errors.New("poznámky se mezitím změnily v jiném okně")

func (f *File) find(key string) (*Issue, error) {
	i := slices.IndexFunc(f.Issues, func(i Issue) bool { return i.Key == key })
	if i < 0 {
		return nil, ErrNotFound
	}
	return &f.Issues[i], nil
}

// column returns the visible issues of a status in board order.
func (f *File) column(status string) []*Issue {
	var col []*Issue
	for i := range f.Issues {
		if is := &f.Issues[i]; is.Status == status && !is.Hidden {
			col = append(col, is)
		}
	}
	slices.SortStableFunc(col, func(a, b *Issue) int { return byRank(*a, *b) })
	return col
}

func byRank(a, b Issue) int {
	return cmp.Or(cmp.Compare(a.Rank, b.Rank), a.AssignedAt.Compare(b.AssignedAt), strings.Compare(a.Key, b.Key))
}

func renumber(col []*Issue) {
	for n, is := range col {
		is.Rank = (n + 1) * 10
	}
}

// SetStatus puts an issue into a status (also out of the new ones), before
// the issue with key before, or at the end when before is empty or not in
// the status. A hidden issue stays hidden.
func (f *File) SetStatus(key, status, before string, now time.Time) error {
	is, err := f.find(key)
	if err != nil {
		return err
	}
	is.Status = status
	is.TouchedAt = now
	if is.Hidden {
		return nil
	}
	col := slices.DeleteFunc(f.column(status), func(o *Issue) bool { return o.Key == key })
	at := slices.IndexFunc(col, func(o *Issue) bool { return o.Key == before })
	if at < 0 {
		at = len(col)
	}
	renumber(slices.Insert(col, at, is))
	return nil
}

// MoveNextTo puts an issue right before (or after) the issue neighbour of
// the same status. The UIs pass the neighbour the user sees, so issues
// hidden by a filter or shown in another group do not count.
func (f *File) MoveNextTo(key, neighbour string, after bool, now time.Time) error {
	is, err := f.find(key)
	if err != nil {
		return err
	}
	if key == neighbour {
		return nil
	}
	col := slices.DeleteFunc(f.column(is.Status), func(o *Issue) bool { return o.Key == key })
	at := slices.IndexFunc(col, func(o *Issue) bool { return o.Key == neighbour })
	if at < 0 || is.Hidden {
		return fmt.Errorf("%s už není ve stejném stavu – načti přehled znovu", neighbour)
	}
	if after {
		at++
	}
	renumber(slices.Insert(col, at, is))
	is.TouchedAt = now
	return nil
}

// SetHidden hides or shows an issue. A shown issue goes to the end of its status.
func (f *File) SetHidden(key string, hidden bool, now time.Time) error {
	is, err := f.find(key)
	if err != nil {
		return err
	}
	if is.Hidden == hidden {
		return nil
	}
	is.Hidden = hidden
	is.TouchedAt = now
	if !hidden {
		is.Rank = 0
		for _, o := range f.column(is.Status) {
			if o != is {
				is.Rank = max(is.Rank, o.Rank+10)
			}
		}
	}
	return nil
}

// SetNotes replaces the notes. base is NotesAt as the caller read it; when
// the stored notes have another version, SetNotes refuses with ErrConflict.
func (f *File) SetNotes(key, notes string, base, now time.Time) error {
	is, err := f.find(key)
	if err != nil {
		return err
	}
	notes = strings.TrimRight(strings.ReplaceAll(notes, "\r\n", "\n"), " \n\t")
	if !is.NotesAt.Equal(base) {
		if is.Notes == notes {
			return nil
		}
		return ErrConflict
	}
	if is.Notes == notes {
		return nil
	}
	is.Notes = notes
	is.NotesAt = now
	is.TouchedAt = now
	return nil
}

// RenameStatus moves the issues of status from to status to.
func (f *File) RenameStatus(from, to string) {
	for i := range f.Issues {
		if f.Issues[i].Status == from {
			f.Issues[i].Status = to
		}
	}
}

// DropStatus returns the issues of a removed status among the new ones.
func (f *File) DropStatus(code string) {
	f.RenameStatus(code, "")
}
