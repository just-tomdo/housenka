// Command gen fills an example Housenka home directory with generated entries.
//
// The directory must contain config.yaml and scenario.yaml (which issues,
// routine meetings and occasional work to simulate). Existing entries/*.json
// files are replaced:
//
//	go run ./examples/gen examples/demo
//	HOUSENKA_HOME=examples/demo housenka
package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/hours"
)

// Scenario describes what kind of work the generated entries simulate.
type Scenario struct {
	Seed            uint64              `yaml:"seed"`
	Months          int                 `yaml:"months"`     // whole months before the current one (default 6)
	IssueHost       string              `yaml:"issue_host"` // default: first of gitlab_hosts
	ActivityWeights map[string]int      `yaml:"activity_weights"`
	ActivityNotes   map[string][]string `yaml:"activity_notes"` // optional " – note" suffixes per activity
	VacationBlocks  []int               `yaml:"vacation_blocks"`
	Issues          []IssueGroup        `yaml:"issues"`
	Routine         []Routine           `yaml:"routine"`
	Occasional      []Occasional        `yaml:"occasional"`
}

// IssueGroup is a pool of issue titles in one repository.
type IssueGroup struct {
	Repo     string   `yaml:"repo"`
	Project  string   `yaml:"project"`
	Weight   int      `yaml:"weight"`
	FirstIID int      `yaml:"first_iid"`
	Kind     string   `yaml:"kind"` // issues (default) | work_items
	Titles   []string `yaml:"titles"`
}

// Routine is work without an issue that repeats on a weekday.
type Routine struct {
	Project     string  `yaml:"project"`
	Activity    string  `yaml:"activity"`
	Issue       string  `yaml:"issue"` // optional fixed link
	Description string  `yaml:"description"`
	Weekday     string  `yaml:"weekday"`     // monday … friday
	EveryWeeks  int     `yaml:"every_weeks"` // default 1
	Hours       float64 `yaml:"hours"`
}

// Occasional is work that happens on a random day with the given chance.
type Occasional struct {
	Project      string    `yaml:"project"`
	Activity     string    `yaml:"activity"`
	Issue        string    `yaml:"issue"` // optional fixed link
	Descriptions []string  `yaml:"descriptions"`
	Chance       float64   `yaml:"chance"`
	Hours        []float64 `yaml:"hours"` // one is picked at random
}

func main() {
	to := flag.String("to", "", "last generated day, RRRR-MM-DD (default today)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "použití: go run ./examples/gen [-to RRRR-MM-DD] <adresář s config.yaml a scenario.yaml>")
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	last := civil.Today()
	if *to != "" {
		d, err := civil.Parse(*to)
		if err != nil {
			fail(err)
		}
		last = d
	}
	n, err := generateDir(flag.Arg(0), last)
	if err != nil {
		fail(err)
	}
	fmt.Printf("vygenerováno %d záznamů do %s\n", n, filepath.Join(flag.Arg(0), "entries"))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "chyba:", err)
	os.Exit(1)
}

// generateDir reads config.yaml and scenario.yaml from dir and rewrites dir/entries.
func generateDir(dir string, last civil.Date) (int, error) {
	cfg, err := config.Load(filepath.Join(dir, "config.yaml"))
	if err != nil {
		return 0, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "scenario.yaml"))
	if err != nil {
		return 0, err
	}
	var sc Scenario
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&sc); err != nil {
		return 0, fmt.Errorf("scenario.yaml: %v", err)
	}
	entries, err := Generate(cfg, sc, last)
	if err != nil {
		return 0, err
	}
	return len(entries), write(filepath.Join(dir, "entries"), entries)
}

// Generate simulates entries from the first day of the month sc.Months
// before last up to last (inclusive).
func Generate(cfg *config.Config, sc Scenario, last civil.Date) ([]entry.Entry, error) {
	if len(sc.Issues) == 0 {
		return nil, errors.New("scenario.yaml: chybí issues")
	}
	host := cmp.Or(sc.IssueHost, firstOr(cfg.GitLabHosts, "gitlab.example.com"))
	months := cmp.Or(sc.Months, 6)
	first := civil.Of(time.Date(last.Year, last.Month-time.Month(months), 1, 0, 0, 0, 0, time.UTC))

	g := &gen{
		cfg:  cfg,
		sc:   sc,
		host: host,
		rnd:  rand.New(rand.NewPCG(sc.Seed, 0x686f7573656e6b61)),
		iids: map[int]int{},
		used: map[int]int{},
	}
	for i, grp := range sc.Issues {
		g.iids[i] = cmp.Or(grp.FirstIID, 100)
	}

	workdays := workdaysBetween(first, last)
	vacation := g.vacation(workdays, last)
	loc, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		loc = time.Local
	}

	var out []entry.Entry
	for _, d := range workdays {
		if vacation[d] {
			continue
		}
		day := g.day(d, d == last, first)
		for i, e := range day {
			created := time.Date(d.Year, d.Month, d.Day, 15, 30+7*i, g.rnd.IntN(60), 0, loc)
			e.ID = fmt.Sprintf("%012x", g.rnd.Uint64()>>16)
			e.Date = d
			e.Sent = d.Before(last.AddDays(-3))
			e.CreatedAt, e.UpdatedAt = created, created
			out = append(out, e)
		}
	}
	return out, nil
}

type openIssue struct {
	group  int
	url    string
	title  string
	budget int // remaining quarters
}

type gen struct {
	cfg  *config.Config
	sc   Scenario
	host string
	rnd  *rand.Rand
	open []*openIssue
	iids map[int]int // next IID per issue group
	used map[int]int // titles used per issue group
}

// day returns the entries of one working day (without date and metadata).
func (g *gen) day(d civil.Date, today bool, first civil.Date) []entry.Entry {
	target := pick(g.rnd, []int{32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 30, 34, 34, 36})
	if today {
		target = 14 + 2*g.rnd.IntN(4)
	}
	var out []entry.Entry
	add := func(e entry.Entry) {
		if e.Quarters > target {
			e.Quarters = target
		}
		if e.Quarters > 0 {
			target -= e.Quarters
			out = append(out, e)
		}
	}

	week := int(d.Time().Sub(first.Time()).Hours()) / (24 * 7)
	for _, r := range g.sc.Routine {
		if strings.EqualFold(d.Weekday().String(), r.Weekday) && week%cmp.Or(r.EveryWeeks, 1) == 0 {
			add(entry.Entry{ProjectCode: r.Project, Activity: r.Activity, IssueURL: r.Issue, Quarters: quarters(r.Hours), Description: r.Description})
		}
	}
	for _, o := range g.sc.Occasional {
		if !today && g.rnd.Float64() < o.Chance {
			add(entry.Entry{ProjectCode: o.Project, Activity: o.Activity, IssueURL: o.Issue,
				Quarters: quarters(pick(g.rnd, o.Hours)), Description: pick(g.rnd, o.Descriptions)})
		}
	}

	n := pick(g.rnd, []int{1, 2, 2, 2, 3, 3})
	for i := range n {
		if target < 2 {
			break
		}
		q := target
		if i < n-1 {
			q = 2 * (1 + g.rnd.IntN(max(1, target/2-1)))
			q = min(q, target-2)
		}
		iss := g.issue(i)
		act := g.activity(iss)
		add(entry.Entry{ProjectCode: g.sc.Issues[iss.group].Project, Activity: act, IssueURL: iss.url,
			Quarters: q, Description: g.describe(iss.title, act)})
		iss.budget -= q
	}
	g.closeDone()
	return out
}

// issue returns the i-th issue worked on today, opening new ones as needed.
func (g *gen) issue(i int) *openIssue {
	for len(g.open) < 4 {
		g.open = append(g.open, g.newIssue())
	}
	if i == 0 {
		// The oldest open issue comes first so that issues get finished;
		// the others are picked at random.
		g.rnd.Shuffle(len(g.open)-1, func(a, b int) { g.open[a+1], g.open[b+1] = g.open[b+1], g.open[a+1] })
	}
	return g.open[i]
}

func (g *gen) newIssue() *openIssue {
	weights := make([]int, len(g.sc.Issues))
	for i, grp := range g.sc.Issues {
		weights[i] = cmp.Or(grp.Weight, 1)
	}
	gi := weighted(g.rnd, weights)
	grp := g.sc.Issues[gi]
	title := grp.Titles[g.used[gi]%len(grp.Titles)]
	g.used[gi]++
	iid := g.iids[gi]
	g.iids[gi] += 1 + g.rnd.IntN(7)
	return &openIssue{
		group:  gi,
		url:    fmt.Sprintf("https://%s/%s/-/%s/%d", g.host, grp.Repo, cmp.Or(grp.Kind, "issues"), iid),
		title:  title,
		budget: 4 * (2 + g.rnd.IntN(30)),
	}
}

func (g *gen) closeDone() {
	g.open = slices.DeleteFunc(g.open, func(o *openIssue) bool { return o.budget <= 0 })
}

func (g *gen) activity(iss *openIssue) string {
	project := g.cfg.Projects[g.sc.Issues[iss.group].Project]
	var codes []string
	var weights []int
	for _, code := range slices.Sorted(maps.Keys(g.sc.ActivityWeights)) {
		if len(project.Activities) == 0 || slices.Contains(project.Activities, code) {
			codes = append(codes, code)
			weights = append(weights, g.sc.ActivityWeights[code])
		}
	}
	if len(codes) == 0 {
		return cmp.Or(project.DefaultActivity, firstOr(project.Activities, ""))
	}
	return codes[weighted(g.rnd, weights)]
}

func (g *gen) describe(title, activity string) string {
	notes := g.sc.ActivityNotes[activity]
	if len(notes) == 0 || g.rnd.IntN(10) < 3 {
		return title
	}
	return title + " – " + pick(g.rnd, notes)
}

// vacation picks blocks of consecutive working days off, away from the last two weeks.
func (g *gen) vacation(workdays []civil.Date, last civil.Date) map[civil.Date]bool {
	off := map[civil.Date]bool{}
	var candidates []int
	for i, d := range workdays {
		if d.Before(last.AddDays(-14)) {
			candidates = append(candidates, i)
		}
	}
	for _, n := range g.sc.VacationBlocks {
		if len(candidates) <= n {
			break
		}
		start := candidates[g.rnd.IntN(len(candidates)-n)]
		for i := start; i < start+n && i < len(workdays); i++ {
			off[workdays[i]] = true
		}
	}
	return off
}

func workdaysBetween(first, last civil.Date) []civil.Date {
	var out []civil.Date
	for d := first; !d.After(last); d = d.AddDays(1) {
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday && !holiday(d) {
			out = append(out, d)
		}
	}
	return out
}

// holiday reports Czech public holidays.
func holiday(d civil.Date) bool {
	fixed := []string{"01-01", "05-01", "05-08", "07-05", "07-06", "09-28", "10-28", "11-17", "12-24", "12-25", "12-26"}
	if slices.Contains(fixed, fmt.Sprintf("%02d-%02d", d.Month, d.Day)) {
		return true
	}
	easter := easterSunday(d.Year)
	return d == easter.AddDays(-2) || d == easter.AddDays(1)
}

// easterSunday uses the anonymous Gregorian algorithm.
func easterSunday(y int) civil.Date {
	a, b, c := y%19, y/100, y%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return civil.Date{Year: y, Month: time.Month(month), Day: day}
}

// write replaces dir/<year>.json files with entries in the store's format.
func write(dir string, entries []entry.Entry) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	for _, f := range old {
		if err := os.Remove(f); err != nil {
			return err
		}
	}
	byYear := map[int][]entry.Entry{}
	for _, e := range entries {
		byYear[e.Date.Year] = append(byYear[e.Date.Year], e)
	}
	for y, es := range byYear {
		data, err := json.MarshalIndent(struct {
			Version int           `json:"version"`
			Entries []entry.Entry `json:"entries"`
		}{1, es}, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(y)+".json"), append(data, '\n'), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func quarters(h float64) int { return int(math.Round(h * hours.PerHour)) }

func pick[T any](r *rand.Rand, xs []T) T { return xs[r.IntN(len(xs))] }

func weighted(r *rand.Rand, weights []int) int {
	total := 0
	for _, w := range weights {
		total += w
	}
	n := r.IntN(max(total, 1))
	for i, w := range weights {
		if n < w {
			return i
		}
		n -= w
	}
	return len(weights) - 1
}

func firstOr(xs []string, def string) string {
	if len(xs) == 0 {
		return def
	}
	return xs[0]
}
