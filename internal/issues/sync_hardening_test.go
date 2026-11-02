package issues

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/gitlab"
)

// forbiddingSource answers 403 for one issue.
type forbiddingSource struct {
	*fakeSource
	forbidden string // "path#iid"
}

func (s forbiddingSource) Issue(ctx context.Context, path string, iid int) (gitlab.Issue, error) {
	if path+"#"+itoa(iid) == s.forbidden {
		return gitlab.Issue{}, gitlab.ErrForbidden
	}
	return s.fakeSource.Issue(ctx, path, iid)
}

func newSyncStore(t *testing.T) *Store {
	t.Helper()
	st, err := OpenStore(t.TempDir())
	must(t, err)
	return st
}

func TestSyncSurvivesForbiddenIssue(t *testing.T) {
	st := newSyncStore(t)
	src := &fakeSource{assigned: []gitlab.Issue{gl("clients/shop-web", 1, "opened", "jnovak")}}
	_, err := Sync(context.Background(), src, st)
	must(t, err)

	// The user lost access to #1; #2 is newly assigned.
	src.assigned = []gitlab.Issue{gl("clients/shop-web", 2, "opened", "jnovak")}
	res, err := Sync(context.Background(), forbiddingSource{src, "clients/shop-web#1"}, st)
	if err != nil {
		t.Fatalf("one unreadable issue stopped the sync: %v", err)
	}
	f, _ := st.Load()
	if res.New != 1 || indexOf(f, "clients/shop-web#2") < 0 || !f.Issues[indexOf(f, "clients/shop-web#1")].Closed {
		t.Errorf("res %+v issues %+v", res, f.Issues)
	}
}

func TestSyncMovedProject(t *testing.T) {
	for _, inList := range []bool{true, false} {
		st := newSyncStore(t)
		src := &fakeSource{assigned: []gitlab.Issue{gl("clients/old-shop", 5, "opened", "jnovak")}}
		_, err := Sync(context.Background(), src, st)
		must(t, err)
		must(t, st.SetStatus("clients/old-shop#5", "doing", ""))
		_, err = st.SetNotes("clients/old-shop#5", "poznámka", time.Time{})
		must(t, err)
		before, _ := st.Load()

		// The project was renamed; GitLab finds the issue under the old path too.
		moved := gl("clients/new-shop", 5, "opened", "jnovak")
		src.assigned = nil
		if inList {
			src.assigned = []gitlab.Issue{moved}
		}
		src.all = map[string]gitlab.Issue{"clients/old-shop#5": moved}
		res, err := Sync(context.Background(), src, st)
		must(t, err)
		f, _ := st.Load()
		if len(f.Issues) != 1 || res.New != 0 {
			t.Fatalf("inList=%v: res %+v, issues %+v", inList, res, f.Issues)
		}
		is := f.Issues[0]
		if is.Key != "clients/new-shop#5" || is.ProjectPath != "clients/new-shop" || is.Status != "doing" || is.Notes != "poznámka" ||
			!is.AssignedAt.Equal(before.Issues[0].AssignedAt) || is.Closed {
			t.Errorf("inList=%v: %+v", inList, is)
		}
	}
}

func TestSyncLeavesOtherHost(t *testing.T) {
	st := newSyncStore(t)
	must(t, st.Update(func(f *File, now time.Time) error {
		is := issue("clients/shop-web", 9)
		is.Host = "gitlab.other.example.com"
		f.Issues = append(f.Issues, is)
		return nil
	}))
	src := &fakeSource{}
	_, err := Sync(context.Background(), src, st)
	must(t, err)
	f, _ := st.Load()
	if len(src.lookups) != 0 || f.Issues[0].Closed {
		t.Errorf("an issue of another GitLab was looked up (%v) or closed", src.lookups)
	}
}

func TestSyncCancelledIsNotRecorded(t *testing.T) {
	st := newSyncStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Sync(ctx, &fakeSource{err: context.Canceled}, st)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if f, _ := st.Load(); f.SyncError != "" {
		t.Errorf("a cancelled sync was recorded: %q", f.SyncError)
	}
}

func TestSyncRunsOnce(t *testing.T) {
	st := newSyncStore(t)
	unlock, ok, err := st.trySyncLock()
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err := Sync(context.Background(), &fakeSource{}, st); !errors.Is(err, ErrSyncRunning) {
		t.Errorf("second sync: %v", err)
	}
	if f, _ := st.Load(); f.SyncError != "" {
		t.Errorf("a refused sync was recorded: %q", f.SyncError)
	}
	unlock()
	if _, err := Sync(context.Background(), &fakeSource{}, st); err != nil {
		t.Error(err)
	}
}

func TestSyncDue(t *testing.T) {
	cfg := &Config{SyncMinutes: 15}
	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		synced, tried time.Duration // ago; 0 = never
		want          bool
	}{
		{0, 0, true},
		{20 * time.Minute, 0, true},
		{5 * time.Minute, 0, false},
		{20 * time.Minute, 5 * time.Minute, false}, // a failing sync is not retried at once
	}
	for _, tt := range tests {
		var f File
		var tried time.Time
		if tt.synced > 0 {
			f.SyncedAt = now.Add(-tt.synced)
		}
		if tt.tried > 0 {
			tried = now.Add(-tt.tried)
		}
		if got := SyncDue(f, cfg, tried, now); got != tt.want {
			t.Errorf("synced %v ago, tried %v ago: %v", tt.synced, tt.tried, got)
		}
	}
}

func TestLoadReturnsACopy(t *testing.T) {
	st := newSyncStore(t)
	must(t, st.Update(func(f *File, now time.Time) error {
		is := issue("clients/shop-web", 1)
		is.Labels = []string{"Bug"}
		f.Issues = append(f.Issues, is)
		return nil
	}))
	f, _ := st.Load()
	f.Issues[0].Labels[0] = "changed"
	if g, _ := st.Load(); g.Issues[0].Labels[0] != "Bug" {
		t.Error("changing a loaded copy changed the store")
	}
}
