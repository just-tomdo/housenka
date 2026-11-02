package issues

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/gitlab"
)

// fakeSource is a GitLab in memory.
type fakeSource struct {
	assigned []gitlab.Issue
	all      map[string]gitlab.Issue // "path#iid" -> issue, for single lookups
	notes    map[string]time.Time    // "path#iid" -> assigned at
	err      error
	lookups  []string
}

func (s *fakeSource) CurrentUser(context.Context) (gitlab.User, error) {
	return gitlab.User{Username: "jnovak"}, s.err
}

func (s *fakeSource) AssignedIssues(context.Context) ([]gitlab.Issue, error) {
	return s.assigned, s.err
}

func (s *fakeSource) Issue(_ context.Context, path string, iid int) (gitlab.Issue, error) {
	key := fmt.Sprintf("%s#%d", path, iid)
	s.lookups = append(s.lookups, key)
	is, ok := s.all[key]
	if !ok {
		return is, gitlab.ErrNotFound
	}
	return is, nil
}

func (s *fakeSource) AssignedAt(_ context.Context, path string, iid int, _ string) (time.Time, error) {
	return s.notes[fmt.Sprintf("%s#%d", path, iid)], nil
}

func gl(path string, iid int, state string, assignees ...string) gitlab.Issue {
	is := gitlab.Issue{
		IID: iid, Title: fmt.Sprintf("Issue %d", iid), State: state,
		WebURL: fmt.Sprintf("https://gitlab.example.com/%s/-/issues/%d", path, iid),
	}
	for _, a := range assignees {
		is.Assignees = append(is.Assignees, gitlab.User{Username: a})
	}
	return is
}

func TestSync(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	must(t, err)
	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	st.now = func() time.Time { return now }
	assignedAt := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)

	first := gl("clients/shop-web", 1, "opened", "jnovak")
	first.Labels = []string{"Bug"}
	first.DueDate = "2026-11-20"
	src := &fakeSource{
		assigned: []gitlab.Issue{first, gl("clients/shop-web", 2, "opened", "jnovak"), gl("Other/Tool", 3, "opened", "jnovak")},
		notes:    map[string]time.Time{"clients/shop-web#1": assignedAt},
	}
	res, err := Sync(context.Background(), src, st)
	must(t, err)
	if res != (SyncResult{New: 3}) || res.String() != "3 nová issue" {
		t.Errorf("first sync %+v %q", res, res.String())
	}
	f, _ := st.Load()
	if f.Me != "jnovak" || !f.SyncedAt.Equal(now) || len(f.Issues) != 3 {
		t.Fatalf("file %+v", f)
	}
	one := f.Issues[indexOf(f, "clients/shop-web#1")]
	if !one.AssignedAt.Equal(assignedAt) || !one.FirstSeen.Equal(now) || one.Due == nil || one.Due.String() != "2026-11-20" || one.Labels[0] != "Bug" {
		t.Errorf("issue 1 %+v", one)
	}
	if two := f.Issues[indexOf(f, "clients/shop-web#2")]; !two.AssignedAt.Equal(now) {
		t.Errorf("without a note the issue is assigned when first seen: %+v", two)
	}
	if indexOf(f, "other/tool#3") < 0 {
		t.Error("key must be lower-case")
	}

	// The user sorts and annotates; a sync must keep that.
	must(t, st.SetStatus("clients/shop-web#1", "todo", ""))
	_, err = st.SetNotes("clients/shop-web#1", "moje poznámka", time.Time{})
	must(t, err)
	must(t, st.SetHidden("clients/shop-web#2", true))

	// Issue 1 renamed, 2 closed, 3 assigned to someone else.
	renamed := first
	renamed.Title = "Přejmenované"
	src.assigned = []gitlab.Issue{renamed}
	src.all = map[string]gitlab.Issue{
		"clients/shop-web#2": gl("clients/shop-web", 2, "closed", "jnovak"),
		"Other/Tool#3":       gl("Other/Tool", 3, "opened", "someone"),
	}
	now = now.Add(time.Hour)
	res, err = Sync(context.Background(), src, st)
	must(t, err)
	if res != (SyncResult{Closed: 1, Unassigned: 1}) {
		t.Errorf("second sync %+v", res)
	}
	f, _ = st.Load()
	one = f.Issues[indexOf(f, "clients/shop-web#1")]
	if one.Title != "Přejmenované" || one.Status != "todo" || one.Notes != "moje poznámka" || !one.AssignedAt.Equal(assignedAt) {
		t.Errorf("user fields lost or GitLab fields not updated: %+v", one)
	}
	if two := f.Issues[indexOf(f, "clients/shop-web#2")]; !two.Closed || !two.Hidden {
		t.Errorf("issue 2 %+v", two)
	}
	if three := f.Issues[indexOf(f, "other/tool#3")]; !three.Unassigned || three.Closed {
		t.Errorf("issue 3 %+v", three)
	}

	// Closed and unassigned issues are not looked up again.
	src.lookups = nil
	now = now.Add(time.Hour)
	_, err = Sync(context.Background(), src, st)
	must(t, err)
	if len(src.lookups) != 0 {
		t.Errorf("looked up %v", src.lookups)
	}

	// Issue 2 is reopened and issue 3 assigned back: both return, the hidden one shows.
	src.assigned = []gitlab.Issue{renamed, gl("clients/shop-web", 2, "opened", "jnovak"), gl("Other/Tool", 3, "opened", "jnovak")}
	now = now.Add(time.Hour)
	res, err = Sync(context.Background(), src, st)
	must(t, err)
	if res != (SyncResult{Returned: 2}) {
		t.Errorf("third sync %+v", res)
	}
	f, _ = st.Load()
	if two := f.Issues[indexOf(f, "clients/shop-web#2")]; two.Closed || two.Hidden {
		t.Errorf("issue 2 did not return: %+v", two)
	}
	if three := f.Issues[indexOf(f, "other/tool#3")]; three.Unassigned || !three.AssignedAt.Equal(now) {
		t.Errorf("issue 3 did not return: %+v", three)
	}

	// A deleted issue counts as closed.
	src.assigned = []gitlab.Issue{renamed, gl("Other/Tool", 3, "opened", "jnovak")}
	src.all = nil
	res, err = Sync(context.Background(), src, st)
	must(t, err)
	if res != (SyncResult{Closed: 1}) {
		t.Errorf("deleted issue %+v", res)
	}
}

func TestSyncErrorIsRecorded(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	must(t, err)
	src := &fakeSource{err: gitlab.ErrUnauthorized}
	if _, err := Sync(context.Background(), src, st); !errors.Is(err, gitlab.ErrUnauthorized) {
		t.Fatalf("got %v", err)
	}
	f, _ := st.Load()
	if f.SyncError != gitlab.ErrUnauthorized.Error() {
		t.Errorf("sync error %q", f.SyncError)
	}
	src.err = nil
	_, err = Sync(context.Background(), src, st)
	must(t, err)
	if f, _ = st.Load(); f.SyncError != "" {
		t.Error("a successful sync must clear the error")
	}
}

func indexOf(f File, key string) int {
	for i, is := range f.Issues {
		if is.Key == key {
			return i
		}
	}
	return -1
}
