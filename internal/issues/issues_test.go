package issues

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/rules"
)

const mainYAML = `
gitlab_hosts: [gitlab.example.com]
bm: { base_url: https://bm.example.com/vykazy/editor, fields: { project: project, issue: issue, hours: hours, activity: activity, date: date, description: description } }
activities:
  DEV: { name: Vývoj, productive: true }
projects:
  SHOPDEV: { name: Shop, color: teal }
  BETADEV: { name: Beta }
  BETASUP: { name: Beta podpora }
repos:
  clients/shop-web: [SHOPDEV]
  clients/beta-app: [BETADEV, BETASUP]
`

const issuesYAML = `
host: gitlab.example.com
statuses:
  - {code: todo, name: Na řadě, color: blue}
  - {code: doing, name: Rozpracováno}
  - {code: done, name: Hotovo, color: green, watch: false}
`

func testConfigs(t *testing.T) (*Config, *rules.Rules) {
	t.Helper()
	main, err := config.Parse([]byte(mainYAML))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseConfig([]byte(issuesYAML))
	if err != nil {
		t.Fatal(err)
	}
	return cfg, rules.New(main)
}

var t0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.Local)

func issue(path string, iid int) Issue {
	return Issue{
		Key: strings.ToLower(path) + "#" + itoa(iid), Host: "gitlab.example.com", ProjectPath: path, IID: iid,
		Title: "Issue " + itoa(iid), URL: "https://gitlab.example.com/" + path + "/-/issues/" + itoa(iid),
		AssignedAt: t0, FirstSeen: t0,
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func keys(issues []Issue) []string {
	var out []string
	for _, is := range issues {
		out = append(out, is.Key)
	}
	return out
}

func cardKeys(cards []Card) []string {
	var out []string
	for _, c := range cards {
		out = append(out, c.Key)
	}
	return out
}

func TestParseConfig(t *testing.T) {
	cfg, err := ParseConfig([]byte(issuesYAML))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SyncMinutes != 15 || cfg.StaleDays != 14 {
		t.Errorf("defaults not applied: %+v", cfg)
	}
	if cfg.StatusIndex("doing") != 1 || cfg.StatusIndex("") != -1 || cfg.StatusIndex("gone") != -1 {
		t.Error("StatusIndex")
	}
	if cfg.Statuses[0].Label() != "Na řadě" || !cfg.Statuses[0].Watched() || cfg.Statuses[2].Watched() {
		t.Error("status label or watch")
	}

	example, err := os.ReadFile("../../issues.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseConfig(example); err != nil {
		t.Errorf("issues.example.yaml: %v", err)
	}
}

func TestParseConfigErrors(t *testing.T) {
	tests := []struct{ yaml, want string }{
		{"statuses: []", "aspoň jeden stav"},
		{"statuses: [{code: a}, {code: a}]", "dvakrát"},
		{"statuses: [{code: Bad Code}]", "smí obsahovat"},
		{"statuses: [{name: X}]", "chybí code"},
		{"statuses: [{code: a, color: brown}]", "neznámá barva"},
		{"sync_minutes: 0\nstatuses: [{code: a}]", "sync_minutes"},
		{"host: https://gitlab.example.com\nstatuses: [{code: a}]", "jen doménu"},
		{"hots: x\nstatuses: [{code: a}]", "neznámý klíč"},
	}
	for _, tt := range tests {
		_, err := ParseConfig([]byte(tt.yaml))
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%q: error %v, want %q", tt.yaml, err, tt.want)
		}
	}
}

func TestLoadConfigMissing(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "issues.yaml"))
	if Enabled(err) {
		t.Errorf("missing file must mean disabled, got %v", err)
	}
	path := filepath.Join(t.TempDir(), "issues.yaml")
	os.WriteFile(path, []byte("statuses: []"), 0o600)
	if _, err := LoadConfig(path); !Enabled(err) || err == nil {
		t.Errorf("broken file must be an error of an enabled extension, got %v", err)
	}
}

func TestWarnings(t *testing.T) {
	main, _ := config.Parse([]byte(mainYAML))
	cfg, _ := ParseConfig([]byte(issuesYAML))
	if w := cfg.Warnings(main); len(w) != 0 {
		t.Errorf("unexpected %v", w)
	}
	cfg.Host = "gitlab.other.example.com"
	if w := cfg.Warnings(main); len(w) != 1 || !strings.Contains(w[0], "není v gitlab_hosts") {
		t.Errorf("got %v", w)
	}
	cfg.Host = ""
	if got := cfg.GitLabHost(main); got != "gitlab.example.com" {
		t.Errorf("GitLabHost fallback = %q", got)
	}
}

func TestSetStatusAndMove(t *testing.T) {
	cfg, _ := testConfigs(t)
	f := File{Issues: []Issue{issue("clients/shop-web", 1), issue("clients/shop-web", 2), issue("clients/shop-web", 3)}}
	now := t0.Add(time.Hour)
	k1, k2, k3 := f.Issues[0].Key, f.Issues[1].Key, f.Issues[2].Key

	if got := keys(f.New(cfg)); len(got) != 3 {
		t.Fatalf("all new at first, got %v", got)
	}
	must(t, f.SetStatus(k1, "todo", "", now))
	must(t, f.SetStatus(k2, "todo", "", now))
	must(t, f.SetStatus(k3, "todo", k1, now)) // before the first
	if got := keys(column(&f, "todo")); !slices.Equal(got, []string{k3, k1, k2}) {
		t.Errorf("order %v", got)
	}
	if got := f.New(cfg); len(got) != 0 {
		t.Errorf("none new, got %v", keys(got))
	}
	if !f.Issues[0].TouchedAt.Equal(now) {
		t.Error("TouchedAt not set")
	}

	must(t, f.Move(k3, 1, now))
	if got := keys(column(&f, "todo")); !slices.Equal(got, []string{k1, k3, k2}) {
		t.Errorf("after move down %v", got)
	}
	must(t, f.Move(k3, 5, now)) // clamps to the end
	must(t, f.Move(k1, -1, now))
	if got := keys(column(&f, "todo")); !slices.Equal(got, []string{k1, k2, k3}) {
		t.Errorf("after clamped moves %v", got)
	}
	must(t, f.SetStatus(k2, "todo", "", now)) // to the end of its own status
	if got := keys(column(&f, "todo")); !slices.Equal(got, []string{k1, k3, k2}) {
		t.Errorf("after moving to the end %v", got)
	}

	must(t, f.SetStatus(k1, "doing", "", now))
	if got := keys(column(&f, "doing")); !slices.Equal(got, []string{k1}) {
		t.Errorf("doing %v", got)
	}
	if err := f.SetStatus("nope#1", "todo", "", now); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing issue: %v", err)
	}
}

func column(f *File, status string) []Issue {
	var out []Issue
	for _, is := range f.column(status) {
		out = append(out, *is)
	}
	return out
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestHiddenAndLostStatus(t *testing.T) {
	cfg, _ := testConfigs(t)
	f := File{Issues: []Issue{issue("clients/shop-web", 1), issue("clients/shop-web", 2)}}
	k1, k2 := f.Issues[0].Key, f.Issues[1].Key
	must(t, f.SetStatus(k1, "todo", "", t0))
	must(t, f.SetStatus(k2, "todo", "", t0))
	must(t, f.SetHidden(k1, true, t0))
	if got := keys(column(&f, "todo")); !slices.Equal(got, []string{k2}) {
		t.Errorf("hidden still in column: %v", got)
	}
	must(t, f.SetHidden(k1, false, t0))
	if got := keys(column(&f, "todo")); !slices.Equal(got, []string{k2, k1}) {
		t.Errorf("shown issue must go last: %v", got)
	}

	f.RenameStatus("todo", "next")
	if got := keys(f.New(cfg)); len(got) != 2 {
		t.Errorf("unknown status must count as new: %v", got)
	}
	f.DropStatus("next")
	if f.Issues[0].Status != "" {
		t.Error("DropStatus")
	}
}

func TestSetNotesConflict(t *testing.T) {
	f := File{Issues: []Issue{issue("clients/shop-web", 1)}}
	k := f.Issues[0].Key
	must(t, f.SetNotes(k, "první\r\n\n", time.Time{}, t0))
	if f.Issues[0].Notes != "první" || !f.Issues[0].NotesAt.Equal(t0) {
		t.Fatalf("notes %+v", f.Issues[0])
	}
	if err := f.SetNotes(k, "jiná", time.Time{}, t0.Add(time.Minute)); !errors.Is(err, ErrConflict) {
		t.Errorf("stale base must conflict, got %v", err)
	}
	must(t, f.SetNotes(k, "první", time.Time{}, t0.Add(time.Minute))) // same text: no conflict
	must(t, f.SetNotes(k, "druhá", t0, t0.Add(time.Minute)))
	if f.Issues[0].Notes != "druhá" {
		t.Error("notes not replaced")
	}
}

func TestStoreSharedBetweenProcesses(t *testing.T) {
	dir := t.TempDir()
	a, err := OpenStore(dir)
	must(t, err)
	b, err := OpenStore(dir)
	must(t, err)

	must(t, a.Update(func(f *File, now time.Time) error {
		f.Issues = append(f.Issues, issue("clients/shop-web", 1), issue("clients/shop-web", 2))
		return nil
	}))
	f, err := b.Load()
	must(t, err)
	if len(f.Issues) != 2 {
		t.Fatalf("b sees %d issues", len(f.Issues))
	}
	k := f.Issues[0].Key
	must(t, b.SetStatus(k, "todo", ""))
	f, err = a.Load()
	must(t, err)
	if f.Issues[0].Status != "todo" {
		t.Error("a does not see b's change")
	}

	stored, err := a.SetNotes(k, "a", time.Time{})
	must(t, err)
	if _, err := b.SetNotes(k, "b", time.Time{}); !errors.Is(err, ErrConflict) {
		t.Errorf("b overwrote a's notes: %v", err)
	}
	if _, err := b.SetNotes(k, "b", stored.NotesAt); err != nil {
		t.Error(err)
	}

	// Concurrent changes of different issues are all kept.
	var wg sync.WaitGroup
	for i, s := range []*Store{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				if err := s.SetStatus(f.Issues[i].Key, "doing", ""); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	f, _ = a.Load()
	if f.Issues[0].Status != "doing" || f.Issues[1].Status != "doing" {
		t.Errorf("lost a concurrent change: %+v", f.Issues)
	}
	info, err := os.Stat(filepath.Join(dir, "issues.json"))
	must(t, err)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("issues.json mode %v", info.Mode().Perm())
	}
}

func TestStoreRejectsNewerFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "issues.json"), []byte(`{"version": 99, "issues": []}`), 0o600)
	s, _ := OpenStore(dir)
	if _, err := s.Load(); err == nil || !strings.Contains(err.Error(), "novější formát") {
		t.Errorf("got %v", err)
	}
}

func TestRepo(t *testing.T) {
	_, r := testConfigs(t)
	tests := []struct {
		issue     Issue
		known     bool
		projects  []string
		colorKey  string
		colorSpec string
		hostKnown bool
	}{
		{issue("clients/shop-web", 1), true, []string{"SHOPDEV"}, "SHOPDEV", "teal", true},
		{issue("clients/beta-app", 1), true, []string{"BETADEV", "BETASUP"}, "clients/beta-app", "", true},
		{issue("Clients/Unknown", 1), false, nil, "clients/unknown", "", true},
		{func() Issue { i := issue("clients/shop-web", 1); i.Host = "gitlab.other.example.com"; return i }(), false, []string{"SHOPDEV"}, "SHOPDEV", "teal", false},
	}
	for _, tt := range tests {
		got := Repo(tt.issue, r)
		if got.Known() != tt.known || !slices.Equal(got.Projects, tt.projects) || got.ColorKey != tt.colorKey || got.ColorSpec != tt.colorSpec || got.HostKnown != tt.hostKnown {
			t.Errorf("%s@%s: %+v", tt.issue.ProjectPath, tt.issue.Host, got)
		}
	}
	if got := Repo(issue("clients/shop-web", 1), nil); got.Known() || got.ColorKey != "clients/shop-web" {
		t.Errorf("without rules: %+v", got)
	}
}

func TestCards(t *testing.T) {
	cfg, r := testConfigs(t)
	today := civil.Of(t0).AddDays(20)
	fresh := issue("clients/shop-web", 1)
	fresh.AssignedAt = t0.AddDate(0, 0, 18)
	worked := issue("clients/shop-web", 2)
	touched := issue("clients/shop-web", 3)
	touched.TouchedAt = t0.AddDate(0, 0, 10)
	done := issue("clients/shop-web", 4)
	done.Status = "done"
	closed := issue("clients/shop-web", 5)
	closed.Closed = true
	lost := issue("clients/shop-web", 6)
	lost.Status = "gone"

	entries := []entry.Entry{
		{Date: civil.Of(t0).AddDays(15), IssueURL: worked.URL + "#note_1", Quarters: 4},
		{Date: civil.Of(t0).AddDays(2), IssueURL: strings.Replace(worked.URL, "issues", "work_items", 1), Quarters: 2},
		{Date: civil.Of(t0), IssueURL: "", Quarters: 8},
	}
	work := WorkByIssue(entries)
	cards := Cards(File{Issues: []Issue{fresh, worked, touched, done, closed, lost}}, cfg, r, work, today)

	want := []struct {
		idle  int
		stale bool
	}{{2, false}, {5, false}, {10, false}, {20, false}, {20, false}, {20, true}}
	for i, w := range want {
		if cards[i].Idle != w.idle || cards[i].Stale != w.stale {
			t.Errorf("%s: idle %d stale %v, want %d %v", cards[i].Key, cards[i].Idle, cards[i].Stale, w.idle, w.stale)
		}
	}
	if cards[1].Work.Total != 6 {
		t.Errorf("work of issue 2: %+v", cards[1].Work)
	}
	if !cards[5].LostStatus || cards[5].Column != -1 || cards[3].Column != 2 {
		t.Error("columns")
	}

	b := Arrange(cards, cfg)
	if got := cardKeys(b.New); !slices.Equal(got, []string{fresh.Key, worked.Key, touched.Key, closed.Key, lost.Key}) {
		t.Errorf("new %v", got)
	}
	if len(b.Columns) != 3 || len(b.Columns[2].Cards) != 1 || b.Columns[0].Status.Code != "todo" {
		t.Errorf("columns %+v", b.Columns)
	}
}

func TestSortAndFilter(t *testing.T) {
	cfg, r := testConfigs(t)
	a := issue("clients/shop-web", 1)
	a.AssignedAt = t0.AddDate(0, 0, 3)
	a.Rank = 20
	a.Labels = []string{"Bug", "Frontend"}
	b := issue("clients/beta-app", 2)
	b.Rank = 10
	b.Notes = "zeptat se na API"
	due := civil.Of(t0).AddDays(4)
	b.Due = &due
	c := issue("other/tool", 3)
	c.AssignedAt = t0.AddDate(0, 0, 1)
	c.Rank = 30
	c.Closed = true
	cards := Cards(File{Issues: []Issue{a, b, c}}, cfg, r, nil, civil.Of(t0).AddDays(5))

	sorts := map[Sort][]string{
		SortManual:   {b.Key, a.Key, c.Key},
		SortAssigned: {b.Key, c.Key, a.Key},
		SortIdle:     {b.Key, c.Key, a.Key},
		SortDue:      {b.Key, a.Key, c.Key},
		SortRepo:     {b.Key, a.Key, c.Key},
	}
	for s, want := range sorts {
		got := slices.Clone(cards)
		SortCards(got, s)
		if !slices.Equal(cardKeys(got), want) {
			t.Errorf("sort %q: %v, want %v", s, cardKeys(got), want)
		}
	}
	if SortDue.Next() != SortRepo || SortRepo.Next() != SortManual || ParseSort("bogus") != SortManual {
		t.Error("sort cycling")
	}

	filters := map[string][]string{
		"":                {a.Key, b.Key, c.Key},
		"r:beta":          {b.Key},
		"p:shop":          {a.Key},
		"l:front":         {a.Key},
		"is:unknown":      {c.Key},
		"-is:unknown":     {a.Key, b.Key},
		"is:closed":       {c.Key},
		"is:notes":        {b.Key},
		"is:due":          {b.Key},
		"API":             {b.Key},
		"issue r:clients": {a.Key, b.Key},
		"is:bogus":        nil,
	}
	for q, want := range filters {
		if got := cardKeys(Filter(cards, q)); !slices.Equal(got, want) {
			t.Errorf("filter %q: %v, want %v", q, got, want)
		}
	}

	lanes := Lanes(cards)
	if len(lanes) != 3 || lanes[0].ProjectPath != "clients/beta-app" || lanes[2].ProjectPath != "other/tool" {
		t.Errorf("lanes %+v", lanes)
	}
}
