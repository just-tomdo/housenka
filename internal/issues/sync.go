package issues

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/gitlab"
	"github.com/just-tomdo/housenka/internal/gitlaburl"
	"github.com/just-tomdo/housenka/internal/text"
)

// Source is the GitLab a sync reads (gitlab.Client, or a fake in tests).
type Source interface {
	Host() string // "gitlab.example.com"
	CurrentUser(ctx context.Context) (gitlab.User, error)
	AssignedIssues(ctx context.Context) ([]gitlab.Issue, error)
	Issue(ctx context.Context, projectPath string, iid int) (gitlab.Issue, error)
	AssignedAt(ctx context.Context, projectPath string, iid int, username string) (time.Time, error)
}

// Dial returns the GitLab at host; tests replace it with a fake.
var Dial = func(host, token string) Source { return gitlab.New(host, token) }

// Connect returns the GitLab of the configuration with the saved token.
func Connect(cfg *Config, main *config.Config) (Source, error) {
	host := cfg.GitLabHost(main)
	if host == "" {
		return nil, errors.New("není nastavený GitLab – doplň host do issues.yaml")
	}
	home, err := config.HomeDir()
	if err != nil {
		return nil, err
	}
	token, err := gitlab.Token(home)
	if err != nil {
		return nil, err
	}
	return Dial(host, token), nil
}

// SyncTimeout bounds one sync.
const SyncTimeout = 2 * time.Minute

// ErrSyncRunning means another sync (in this or another process) is running.
var ErrSyncRunning = errors.New("načítání z GitLabu už běží")

// SyncDue reports whether the overview should read GitLab again: the last
// sync and the last attempt (lastTry, kept by the caller, so that a failing
// sync is not retried at every occasion) are sync_minutes old.
func SyncDue(f File, cfg *Config, lastTry, now time.Time) bool {
	last := f.SyncedAt
	if lastTry.After(last) {
		last = lastTry
	}
	return now.Sub(last) >= time.Duration(cfg.SyncMinutes)*time.Minute
}

// SyncResult counts what changed.
type SyncResult struct {
	New        int // issues seen for the first time
	Returned   int // issues assigned again or reopened
	Closed     int // issues closed since the last sync
	Unassigned int // issues no longer assigned to the user
}

// String describes the result in Czech: "2 nová issue, 1 zavřené".
func (r SyncResult) String() string {
	var parts []string
	if r.New > 0 {
		parts = append(parts, text.Plural(r.New, "nové issue", "nová issue", "nových issue"))
	}
	if r.Returned > 0 {
		parts = append(parts, text.Plural(r.Returned, "vrácené", "vrácená", "vrácených"))
	}
	if r.Closed > 0 {
		parts = append(parts, text.Plural(r.Closed, "zavřené", "zavřená", "zavřených"))
	}
	if r.Unassigned > 0 {
		parts = append(parts, text.Plural(r.Unassigned, "už není na tobě", "už nejsou na tobě", "už není na tobě"))
	}
	if len(parts) == 0 {
		return "žádné změny"
	}
	return strings.Join(parts, ", ")
}

// fetched is what GitLab says about one issue.
type fetched struct {
	gl         gitlab.Issue
	ref        gitlaburl.Ref
	assigned   bool   // assigned to the user
	gone       bool   // deleted, no longer visible, or unreadable (403)
	movedTo    string // the issue now lives under this key (project renamed or moved)
	assignedAt time.Time
}

// Sync reads the issues assigned to the token's user and merges them into
// the store: only the fields from GitLab change, the user's own never do.
// One sync runs at a time across processes (ErrSyncRunning otherwise).
// Network requests run without the data lock; a failure is recorded in the
// file, a cancelled sync is not.
func Sync(ctx context.Context, src Source, st *Store) (SyncResult, error) {
	unlock, ok, err := st.trySyncLock()
	if err != nil {
		return SyncResult{}, err
	}
	if !ok {
		return SyncResult{}, ErrSyncRunning
	}
	defer unlock()
	res, err := pull(ctx, src, st)
	switch {
	case err == nil:
		return res, nil
	case errors.Is(ctx.Err(), context.Canceled):
		return res, err // stopped (e.g. the server shuts down), not a GitLab problem
	}
	return res, RecordSyncError(st, err)
}

// RecordSyncError keeps err as the last sync's failure in the file (the
// UIs show it) and returns it.
func RecordSyncError(st *Store, err error) error {
	msg := err.Error()
	if uerr := st.Update(func(f *File, _ time.Time) error {
		f.SyncError = msg
		return nil
	}); uerr != nil {
		return errors.Join(err, uerr)
	}
	return err
}

func pull(ctx context.Context, src Source, st *Store) (SyncResult, error) {
	before, err := st.Load()
	if err != nil {
		return SyncResult{}, err
	}
	me, err := src.CurrentUser(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	list, err := src.AssignedIssues(ctx)
	if err != nil {
		return SyncResult{}, err
	}

	got := map[string]*fetched{}
	for _, gl := range list {
		ref, err := gitlaburl.Parse(gl.WebURL)
		if err != nil {
			continue // not a link housenka understands (e.g. an incident type)
		}
		got[ref.Key()] = &fetched{gl: gl, ref: ref, assigned: true}
	}

	// Issues assigned at the last sync but missing now: closed, assigned to
	// someone else, or moved to another project. Issues of another GitLab
	// (the host changed in issues.yaml) are left as they are.
	for _, is := range before.Issues {
		if _, ok := got[is.Key]; ok || is.Closed || is.Unassigned || !strings.EqualFold(is.Host, src.Host()) {
			continue
		}
		gl, err := src.Issue(ctx, is.ProjectPath, is.IID)
		switch {
		case errors.Is(err, gitlab.ErrNotFound) || errors.Is(err, gitlab.ErrForbidden):
			got[is.Key] = &fetched{gone: true}
			continue
		case err != nil:
			return SyncResult{}, err
		}
		g := &fetched{gl: gl, assigned: gl.AssignedTo(me.Username)}
		if ref, err := gitlaburl.Parse(gl.WebURL); err == nil {
			g.ref = ref
			if ref.Key() != is.Key {
				g.movedTo = ref.Key()
			}
		}
		got[is.Key] = g
	}

	// When were new and returning issues assigned to the user? One request
	// per such issue; a failure only loses the time.
	for key, g := range got {
		if !g.assigned {
			continue
		}
		i := slices.IndexFunc(before.Issues, func(is Issue) bool { return is.Key == key })
		if i < 0 || before.Issues[i].Unassigned {
			g.assignedAt, _ = src.AssignedAt(ctx, g.ref.ProjectPath, g.ref.IID, me.Username)
		}
	}

	var res SyncResult
	err = st.Update(func(f *File, now time.Time) error {
		res = SyncResult{}
		index := func(key string) int { return slices.IndexFunc(f.Issues, func(is Issue) bool { return is.Key == key }) }
		added := map[string]bool{}
		keys := slices.Sorted(maps.Keys(got))
		for _, key := range keys {
			g := got[key]
			if g.movedTo != "" {
				continue // below: the new key may come from the list
			}
			i := index(key)
			if i < 0 {
				if !g.assigned {
					continue
				}
				f.Issues = append(f.Issues, Issue{Key: key, FirstSeen: now, AssignedAt: now})
				i = len(f.Issues) - 1
				added[key] = true
				res.New++
			}
			if merge(&f.Issues[i], g, &res) {
				f.Issues[i].AssignedAt = now
			}
			if !g.assignedAt.IsZero() {
				f.Issues[i].AssignedAt = g.assignedAt
			}
		}
		// A moved issue keeps the user's status, order, notes and dates under its new key.
		for _, old := range keys {
			g, oi := got[old], index(old)
			if g.movedTo == "" || oi < 0 {
				continue
			}
			ni := index(g.movedTo)
			switch {
			case ni < 0:
				f.Issues[oi].Key = g.movedTo
				merge(&f.Issues[oi], g, &res)
			case added[g.movedTo]: // just arrived from the list: it is not new
				f.Issues[ni].keepOwn(f.Issues[oi])
				f.Issues = slices.Delete(f.Issues, oi, oi+1)
				res.New--
			default: // both keys known already (e.g. an older sync made a duplicate)
				f.Issues[ni].adopt(f.Issues[oi])
				f.Issues = slices.Delete(f.Issues, oi, oi+1)
			}
		}
		f.Me = me.Username
		f.SyncedAt = now
		f.SyncError = ""
		return nil
	})
	return res, err
}

// keepOwn copies the user's own fields (and when the issue was assigned
// and first seen) from the record of the same issue under its old key.
func (is *Issue) keepOwn(old Issue) {
	is.Status, is.Rank, is.Notes, is.NotesAt = old.Status, old.Rank, old.Notes, old.NotesAt
	is.Hidden, is.TouchedAt, is.AssignedAt, is.FirstSeen = old.Hidden, old.TouchedAt, old.AssignedAt, old.FirstSeen
}

// adopt merges the record of the same issue under its old key into is,
// which the user may have changed too: is keeps its own status and notes,
// the old notes are added below when they differ.
func (is *Issue) adopt(old Issue) {
	if is.Status == "" && is.Notes == "" && !is.Hidden {
		is.keepOwn(old)
		return
	}
	if old.Notes != "" && old.Notes != is.Notes {
		is.Notes = strings.TrimSpace(is.Notes + "\n\n" + old.Notes)
		is.NotesAt = old.NotesAt
		if is.NotesAt.Before(is.TouchedAt) {
			is.NotesAt = is.TouchedAt
		}
	}
	if old.AssignedAt.Before(is.AssignedAt) {
		is.AssignedAt, is.FirstSeen = old.AssignedAt, old.FirstSeen
	}
}

// merge copies GitLab's view of an issue into is and counts the changes.
// It reports whether the issue came back to the user (assigned again or
// reopened); a hidden one then shows up again.
func merge(is *Issue, g *fetched, res *SyncResult) (back bool) {
	wasOut := is.Closed || is.Unassigned
	if g.gone {
		if !is.Closed {
			res.Closed++
		}
		is.Closed = true
		return false
	}
	closed := g.gl.State == "closed"
	switch {
	case closed && !is.Closed:
		res.Closed++
	case !closed && !g.assigned && !is.Unassigned:
		res.Unassigned++
	}
	is.Closed = closed
	is.Unassigned = !g.assigned
	if ref, err := gitlaburl.Parse(g.gl.WebURL); err == nil {
		is.Host, is.ProjectPath, is.IID = ref.Host, ref.ProjectPath, ref.IID
	}
	is.Title = g.gl.Title
	is.URL = g.gl.WebURL
	is.Labels = g.gl.Labels
	is.Due = nil
	if d, err := civil.Parse(g.gl.DueDate); err == nil {
		is.Due = &d
	}
	if wasOut && !is.Closed && !is.Unassigned {
		is.Hidden = false
		res.Returned++
		return true
	}
	return false
}
