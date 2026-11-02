package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/gitlaburl"
	"github.com/just-tomdo/housenka/internal/issues"
)

// demoIssues fills the issue overview of an example that has issues.yaml:
// the issues worked on recently, as if read from GitLab and partly sorted
// by the user, plus a few new ones and one from a repository the config
// does not know.
func demoIssues(icfg *issues.Config, g *gen, entries []entry.Entry, last civil.Date) issues.File {
	type work struct{ first, last civil.Date }
	worked := map[string]*work{}
	for _, e := range entries {
		if e.IssueURL == "" {
			continue
		}
		w, ok := worked[e.IssueURL]
		if !ok {
			w = &work{first: e.Date}
			worked[e.IssueURL] = w
		}
		w.last = e.Date
	}
	status := func(i int) string { return icfg.Statuses[min(i, len(icfg.Statuses)-1)].Code }
	done := icfg.Statuses[len(icfg.Statuses)-1].Code
	r := rand.New(rand.NewPCG(g.sc.Seed, 0x6973737565730000))
	// A fixed zone, like the entries: the file must not depend on the machine.
	loc, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		loc = time.UTC
	}
	at := func(d civil.Date, hour int) time.Time {
		return time.Date(d.Year, d.Month, d.Day, hour, r.IntN(60), 0, 0, loc)
	}
	synced := at(last, 8)

	f := issues.File{Version: 1, Me: "demo", SyncedAt: synced}
	add := func(url, title string, assigned civil.Date) *issues.Issue {
		ref, err := gitlaburl.Parse(url)
		if err != nil {
			return nil
		}
		f.Issues = append(f.Issues, issues.Issue{
			Key: ref.Key(), Host: ref.Host, ProjectPath: ref.ProjectPath, IID: ref.IID, Title: title, URL: url,
			AssignedAt: at(assigned, 9), FirstSeen: at(assigned, 10),
		})
		return &f.Issues[len(f.Issues)-1]
	}

	open := map[string]bool{}
	for _, o := range g.open {
		open[o.url] = true
	}
	// The two open issues worked on last are in progress, the others wait in the first status.
	var recent []string
	for _, o := range g.open {
		if worked[o.url] != nil {
			recent = append(recent, o.url)
		}
	}
	slices.SortStableFunc(recent, func(a, b string) int { return worked[b].last.Compare(worked[a].last) })
	recent = recent[:min(2, len(recent))]

	rank := map[string]int{}
	for _, o := range g.all {
		w := worked[o.url]
		if w == nil || w.last.Before(last.AddDays(-30)) {
			continue
		}
		is := add(o.url, o.title, w.first.AddDays(-1-r.IntN(3)))
		if is == nil {
			continue
		}
		switch {
		case !open[o.url] && w.last.Before(last.AddDays(-14)):
			is.Status, is.Hidden, is.Closed = done, true, true
		case !open[o.url]:
			is.Status, is.Closed = done, true
		case slices.Contains(recent, o.url):
			is.Status = status(1)
		default:
			is.Status = status(0)
		}
		rank[is.Status] += 10
		is.Rank = rank[is.Status]
		is.TouchedAt = at(w.last, 16)
	}

	// Waiting for someone else, who now has it.
	if len(icfg.Statuses) > 3 {
		for i := range f.Issues {
			if is := &f.Issues[i]; is.Status == status(0) {
				is.Status, is.Unassigned = status(2), true
				is.Notes = "Čeká na podklady od klienta – přeřazeno na kolegu z podpory."
				break
			}
		}
	}
	// A forgotten one: sorted long ago and untouched since.
	o := g.newIssueIn(1 % len(g.sc.Issues))
	if is := add(o.url, o.title, last.AddDays(-24)); is != nil {
		rank[status(0)] += 10
		is.Status, is.Rank, is.TouchedAt = status(0), rank[status(0)], at(last.AddDays(-23), 11)
		is.Notes = "Domluvit s klientem rozsah, pak odhad."
	}

	// New issues: the next titles of the scenario, not worked on yet.
	for _, gi := range []int{0, len(g.sc.Issues) - 1} {
		o := g.newIssueIn(gi)
		add(o.url, o.title, last.AddDays(-gi%2))
	}
	unknown := fmt.Sprintf("https://%s/tools/wiki/-/issues/%d", g.host, 17+r.IntN(30))
	if is := add(unknown, "Doplnit návod k nasazení do wiki", last.AddDays(-2)); is != nil {
		is.Labels = []string{"Dokumentace"}
	}
	slices.SortStableFunc(f.Issues, func(a, b issues.Issue) int { return a.AssignedAt.Compare(b.AssignedAt) })
	return f
}

// newIssueIn opens the next issue of an issue group.
func (g *gen) newIssueIn(gi int) *openIssue {
	grp := g.sc.Issues[gi]
	title := grp.Titles[g.used[gi]%len(grp.Titles)]
	g.used[gi]++
	iid := g.iids[gi]
	g.iids[gi] += 1 + g.rnd.IntN(7)
	return &openIssue{
		group: gi,
		url:   fmt.Sprintf("https://%s/%s/-/%s/%d", g.host, grp.Repo, cmp.Or(grp.Kind, "issues"), iid),
		title: title,
	}
}

// writeIssues writes dir/issues/issues.json when dir has issues.yaml.
func writeIssues(dir string, g *gen, entries []entry.Entry, last civil.Date) error {
	icfg, err := issues.LoadConfig(filepath.Join(dir, "issues.yaml"))
	if !issues.Enabled(err) {
		return nil
	}
	if err != nil {
		return err
	}
	f := demoIssues(icfg, g, entries, last)
	if err := os.MkdirAll(filepath.Join(dir, "issues"), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "issues", "issues.json"), append(data, '\n'), 0o644)
}
