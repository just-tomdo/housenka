package issues

import (
	"context"
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/gitlab"
)

// A sync that runs out of time is a failure the UIs show (only a cancel is not).
func TestSyncTimeoutIsRecorded(t *testing.T) {
	st := newSyncStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	if _, err := Sync(ctx, &fakeSource{err: context.DeadlineExceeded}, st); err == nil {
		t.Fatal("no error")
	}
	if f, _ := st.Load(); f.SyncError == "" {
		t.Error("a timeout was not recorded")
	}
}

// Two records of one moved issue (both keys known) become one, keeping
// the user's status, the notes of both and the earlier assignment.
func TestSyncMergesKnownDuplicate(t *testing.T) {
	st := newSyncStore(t)
	early := t0.AddDate(0, 0, -10)
	must(t, st.Update(func(f *File, now time.Time) error {
		old := issue("clients/old-shop", 5)
		old.Status, old.Notes, old.AssignedAt = "doing", "stará poznámka", early
		dup := issue("clients/new-shop", 5)
		dup.Notes = "nová poznámka"
		dup.Status = "todo"
		f.Issues = append(f.Issues, old, dup)
		return nil
	}))
	moved := gl("clients/new-shop", 5, "opened", "jnovak")
	src := &fakeSource{assigned: []gitlab.Issue{moved}, all: map[string]gitlab.Issue{"clients/old-shop#5": moved}}
	res, err := Sync(context.Background(), src, st)
	must(t, err)
	f, _ := st.Load()
	if len(f.Issues) != 1 || res.New != 0 {
		t.Fatalf("res %+v issues %+v", res, f.Issues)
	}
	is := f.Issues[0]
	if is.Key != "clients/new-shop#5" || is.Status != "todo" || is.Notes != "nová poznámka\n\nstará poznámka" || !is.AssignedAt.Equal(early) {
		t.Errorf("%+v", is)
	}
	// The next sync does not look the old key up again.
	src.lookups = nil
	_, err = Sync(context.Background(), src, st)
	must(t, err)
	if len(src.lookups) != 0 {
		t.Errorf("looked up %v", src.lookups)
	}
}
