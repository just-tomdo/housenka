package issues

import (
	"context"
	"errors"
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
	assigned   bool // assigned to the user
	gone       bool // deleted, moved away or no longer visible
	assignedAt time.Time
}

// Sync reads the issues assigned to the token's user and merges them into
// the store: only the fields from GitLab change, the user's own never do.
// Network requests run without the lock; an error is recorded in the file.
func Sync(ctx context.Context, src Source, st *Store) (SyncResult, error) {
	res, err := pull(ctx, src, st)
	if err != nil {
		return res, RecordSyncError(st, err)
	}
	return res, nil
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

	// Issues assigned at the last sync but missing now: closed, or assigned
	// to someone else.
	for _, is := range before.Issues {
		if _, ok := got[is.Key]; ok || is.Closed || is.Unassigned {
			continue
		}
		gl, err := src.Issue(ctx, is.ProjectPath, is.IID)
		switch {
		case errors.Is(err, gitlab.ErrNotFound):
			got[is.Key] = &fetched{gone: true}
		case err != nil:
			return SyncResult{}, err
		default:
			got[is.Key] = &fetched{gl: gl, assigned: gl.AssignedTo(me.Username)}
		}
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
		for key, g := range got {
			i := slices.IndexFunc(f.Issues, func(is Issue) bool { return is.Key == key })
			if i < 0 {
				if !g.assigned {
					continue
				}
				f.Issues = append(f.Issues, Issue{Key: key, FirstSeen: now, AssignedAt: now})
				i = len(f.Issues) - 1
				res.New++
			}
			if merge(&f.Issues[i], g, &res) {
				f.Issues[i].AssignedAt = now
			}
			if !g.assignedAt.IsZero() {
				f.Issues[i].AssignedAt = g.assignedAt
			}
		}
		f.Me = me.Username
		f.SyncedAt = now
		f.SyncError = ""
		return nil
	})
	return res, err
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
